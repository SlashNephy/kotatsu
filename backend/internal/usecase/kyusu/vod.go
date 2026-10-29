package kyusu

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// RecordingPathPrefix は、kyusu の録画の仮想 HLS のパスの前置き。/internal/kyusu/recordings/{録画 ID}/{画質}/{音声}/{ファイル} で受け付ける。
	RecordingPathPrefix = "/internal/kyusu/recordings/"
	// TimeshiftPathPrefix は、kyusu のタイムシフトのバッファの仮想 HLS のパスの前置き。
	// /internal/kyusu/timeshift/{サービス ID}-{起点の Unix マイクロ秒}/{画質}/{音声}/{ファイル} で受け付ける (設計書 5.7)。
	TimeshiftPathPrefix = "/internal/kyusu/timeshift/"
	// recordingDir は、作業ディレクトリのうち、録画とタイムシフトのバッファの仮想 HLS を置くディレクトリ。
	recordingDir = "recordings"
	// segmentTimeout は、要求されたセグメントを書き終えるのを待つ時間の上限。エンコーダーを起動し直したときも、ここまで待つ。
	segmentTimeout = 30 * time.Second
	// seekMargin は、k 番目のセグメントを書くとき、その始まりより手前から入力を流す時間。
	// 索引の時刻とエンコーダーの時刻のずれで、始まりより後の I フレームから流してしまわないようにする。
	seekMargin = time.Second
	// aheadSegments は、動いているエンコーダーが、次に書くセグメントからいくつ先までなら、起動し直さずに待つか。
	// プレイヤーが先読みで少し先のセグメントを要求しても、起動し直さないようにする。
	aheadSegments = 5
	// maxVODRuns は、1 つのセッションで同時に動かすエンコーダーの数の上限。同じ録画を離れた位置で見る視聴者に、
	// それぞれエンコーダーを動かす (#531)。超えたら、最も長く使われていないものを止める。
	maxVODRuns = 2
	// lookaheadSegments は、エンコーダーが、視聴者の要求した最も先のセグメントから、いくつ先まで書くか (#532)。
	// 超えたら、録画を流すのを止めて待たせる。見ない部分のために CPU を使い、セグメントをためないようにする。
	lookaheadSegments = 30
	// keepBehindSegments は、視聴者の位置からいくつ前までのセグメントを残すか (#532)。それより前は消し、巻き戻したら書き直す。
	// 長い録画を見続けても、作業ディレクトリにセグメントをためないようにする。
	keepBehindSegments = 150
	// DefaultMaxVODSessions は、同時に開く録画の再生のセッションの数の上限の既定値 (#532)。
	DefaultMaxVODSessions = 4
	// passthroughBase は、映像を変換しないプリセットで、録画の始まりに付ける出力の PTS (#806)。
	// 流し始めた位置 (seekMargin より手前) の音声と映像の PTS が、負にならないようにする。
	passthroughBase = 10 * time.Second
	// ptsWrap は、PTS (33 ビット) が一周する刻みの数。
	ptsWrap = 1 << 33
)

var (
	errNotIndexed      = errors.New("the recording is not indexed yet; try again after the analysis")
	errSegmentTimeout  = errors.New("the encoder did not write the segment in time")
	errOutOfRange      = errors.New("the segment is out of the recording")
	errTooManySessions = errors.New("too many recording sessions")
	errNoPTS           = errors.New("the recording has a segment without an index; it cannot be streamed without transcoding")
)

// RecordingOpener は、録画を開く。*stream.RecordingSource が満たす。
type RecordingOpener interface {
	Open(ctx context.Context, recordingID uuid.UUID) (*stream.Recording, error)
}

// TimeshiftOpener は、タイムシフトのバッファを、起点の時刻から開く。*stream.TimeshiftSource が満たす。
type TimeshiftOpener interface {
	Open(ctx context.Context, serviceID int64, from time.Time) (*stream.Recording, error)
}

// VODEncoder は、録画の仮想 HLS のセグメントを書く。*Transcoder が満たす。
type VODEncoder interface {
	RunVOD(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string, opts *RunOptions) error
}

var (
	_ RecordingOpener = new(stream.RecordingSource)
	_ TimeshiftOpener = new(stream.TimeshiftSource)
	_ VODEncoder      = new(Transcoder)
	_ io.Writer       = new(lookaheadWriter)
)

// vodSource は、仮想 HLS で流すもの。録画か、タイムシフトのバッファ。
type vodSource struct {
	// kind は、録画 (recordingDir) か、タイムシフトのバッファ (timeshiftKind)。
	kind string
	// id は、録画 ID か、タイムシフトのバッファの {サービス ID}-{起点の Unix マイクロ秒}。同じものは同じ文字列にそろえる。
	id string
}

// timeshiftKind は、タイムシフトのバッファの vodSource の種類。
const timeshiftKind = "timeshift"

// String は、ログとディレクトリの名前に使う文字列を返す。
func (s vodSource) String() string {
	return s.kind + "-" + s.id
}

// vodKey は、仮想 HLS のセッションを見分けるもの。同じ録画を同じ画質と音声で見る視聴者は、1 つのセッションを共有する。
type vodKey struct {
	source vodSource
	preset string
	audio  Audio
}

// vodRun は、k 番目のセグメントから書くエンコーダーの 1 回の起動。
type vodRun struct {
	start int
	// startedAt は、起動した時刻。これより前に書かれたセグメントは、前の起動が書いたものとみなす。
	startedAt time.Time
	// lastUsed は、このエンコーダーに書かせるセグメントを最後に要求された時刻。replaced は、ほかの視聴者のために止めたかどうか。
	// どちらもセッションの runMu で守る。
	lastUsed time.Time
	replaced bool
	// wanted は、このエンコーダーに書かせるセグメントのうち、視聴者が要求した最も先の番号。
	// 録画を流す goroutine が読むので、runMu でなく atomic で持つ。
	wanted atomic.Int64
	// nextMu は next を守る。next は、次に書くセグメントの番号。毎回 start から数え直さないよう、前に数えたところから進める。
	nextMu sync.Mutex
	next   int
	cancel context.CancelFunc
	// done は、エンコーダーが止まったら閉じる。err は、閉じる前に書く。
	done chan struct{}
	err  error
}

// vodSession は、セグメントを書くディレクトリと、動いているエンコーダー。
type vodSession struct {
	key vodKey
	dir string
	// runMu は runs を守る。エンコーダーを止めて起動し直す間も、ほかのセッションへのリクエストを待たせないよう、セッションごとに持つ。
	runMu sync.Mutex
	// runs は、動いているエンコーダー。離れた位置を見る視聴者ごとに、maxVODRuns まで持つ。
	runs []*vodRun
	// lastAccess は、VOD の mu で守る。
	lastAccess time.Time
}

// VOD は、録画の仮想 HLS を配信する (設計書 7.3)。
//
// プレイリストは、索引から、録画の始まりからの時間を 2 秒ずつ区切ったセグメントの一覧として先に作る。
// まだ書いていないセグメントを要求されたら、その時刻の手前の I フレームからエンコーダーを起動する。
// 動いているエンコーダーがすぐ後に書くセグメントなら、起動せずに待つ。1 つのセッションには、離れた位置を見る視聴者のために、
// エンコーダーを maxVODRuns まで動かす。後ろのエンコーダーが前のエンコーダーの書いた位置に追いついたら、後ろを止める。
// エンコーダーは、視聴者の要求した最も先のセグメントから lookaheadSegments 先まで書いたら、見進められるまで待つ。
// 書いたセグメントは、視聴者の位置から keepBehindSegments より前になるまで消さないので、少しの巻き戻しなら書き直さない。
// エンコーダーは、書かせるセグメントへの要求が 10 秒途絶えたら止める。同時に開くセッションの数には上限を設ける。
// セッションへの要求が 10 秒途絶えたら、セッションを止めて消す。
type VOD struct {
	opener      RecordingOpener
	timeshift   TimeshiftOpener
	encoder     VODEncoder
	dir         string
	clock       clock.Clock
	logger      *slog.Logger
	idleTimeout time.Duration
	maxSessions int

	mu       sync.Mutex
	sessions map[vodKey]*vodSession
	stopped  bool
}

// VODOptions は VOD の設定。
type VODOptions struct {
	// Opener は、録画を開く。録画のある保存先プールを、このインスタンスにマウントしておく。
	Opener RecordingOpener
	// Timeshift は、タイムシフトのバッファを開く。タイムシフト用のディレクトリを、このインスタンスにマウントしておく。
	Timeshift TimeshiftOpener
	// Encoder は、セグメントを書く。FFmpeg と tsreadex がなければ nil にし、503 を返す。
	Encoder VODEncoder
	// WorkDir は、HLS を置く作業ディレクトリ。空なら、OS の一時ディレクトリの下にする。
	WorkDir string
	Clock   clock.Clock
	Logger  *slog.Logger
	// IdleTimeout は、セグメントへのリクエストが途絶えてから、エンコーダーを止めるまでの時間。0 なら 10 秒。
	IdleTimeout time.Duration
	// MaxSessions は、同時に開くセッションの数の上限。0 なら DefaultMaxVODSessions。
	MaxSessions int
}

// NewVOD は VOD を返す。前に動いていたときの HLS が作業ディレクトリに残っていれば消す。
func NewVOD(opts *VODOptions) (*VOD, error) {
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "kotatsu-kyusu")
	}
	dir := filepath.Join(workDir, recordingDir)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("failed to clean the work directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create the work directory: %w", err)
	}
	idle := opts.IdleTimeout
	if idle == 0 {
		idle = DefaultIdleTimeout
	}
	maxSessions := opts.MaxSessions
	if maxSessions == 0 {
		maxSessions = DefaultMaxVODSessions
	}

	return &VOD{
		opener: opts.Opener, timeshift: opts.Timeshift, encoder: opts.Encoder, dir: dir, clock: opts.Clock, logger: opts.Logger, idleTimeout: idle, maxSessions: maxSessions,
		mu: sync.Mutex{}, sessions: map[vodKey]*vodSession{}, stopped: false,
	}, nil
}

// Sessions は、今ある録画の再生のセッションの数を返す。/metrics に出す (設計書 11.3)。
func (v *VOD) Sessions() int {
	v.mu.Lock()
	defer v.mu.Unlock()

	return len(v.sessions)
}

// Run は、ctx が終わるまで、使われなくなったセッションを止める。ctx が終わったら、すべてのセッションを止める。
func (v *VOD) Run(ctx context.Context) error {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			v.stopAll()

			return nil
		case <-ticker.C:
			v.reap(v.clock.Now())
		}
	}
}

// Handler は、録画の仮想 HLS を配信するハンドラーを返す。token を付けたリクエストだけを受け付ける。
func (v *VOD) Handler(token *Token) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+RecordingPathPrefix+"{id}/{preset}/{audio}/{file}", func(w http.ResponseWriter, r *http.Request) {
		v.serveHTTP(w, r, recordingDir)
	})
	mux.HandleFunc("GET "+TimeshiftPathPrefix+"{id}/{preset}/{audio}/{file}", func(w http.ResponseWriter, r *http.Request) {
		v.serveHTTP(w, r, timeshiftKind)
	})

	return token.Middleware(mux)
}

// reap は、now の時点で、セグメントを idleTimeout より長く要求されていないセッションを止めて、セグメントを消す。
func (v *VOD) reap(now time.Time) {
	v.mu.Lock()
	idle := []*vodSession{}
	active := []*vodSession{}
	for key, s := range v.sessions {
		if now.Sub(s.lastAccess) > v.idleTimeout {
			delete(v.sessions, key)
			idle = append(idle, s)
		} else {
			active = append(active, s)
		}
	}
	v.mu.Unlock()
	// 視聴者が離れたか、ほかの位置へシークしたエンコーダーは、セッションが使われていても止める。
	for _, s := range active {
		s.runMu.Lock()
		s.pruneLocked()
		for _, run := range slices.Clone(s.runs) {
			if now.Sub(run.lastUsed) > v.idleTimeout {
				s.stopLocked(run)
			}
		}
		s.removeFarBehindLocked(v.logger)
		s.runMu.Unlock()
	}
	for _, s := range idle {
		v.logger.Info("stopping an idle recording session", slog.String("source", s.key.source.String()),
			slog.String("preset", s.key.preset), slog.String("audio", s.key.audio.String()))
		v.close(s)
	}
}

// stopAll は、新しいセッションを作らないようにし、すべてのセッションを止める。
func (v *VOD) stopAll() {
	v.mu.Lock()
	v.stopped = true
	sessions := make([]*vodSession, 0, len(v.sessions))
	for key, s := range v.sessions {
		delete(v.sessions, key)
		sessions = append(sessions, s)
	}
	v.mu.Unlock()
	for _, s := range sessions {
		v.close(s)
	}
}

// close は、セッションのエンコーダーを止め、止まるのを待ってから、セグメントを消す。
func (v *VOD) close(s *vodSession) {
	s.runMu.Lock()
	runs := s.runs
	s.runs = []*vodRun{}
	s.runMu.Unlock()
	for _, run := range runs {
		run.cancel()
		<-run.done
	}
	if err := os.RemoveAll(s.dir); err != nil {
		v.logger.Warn("failed to remove the session directory", slog.Any("error", err))
	}
}

// session は、(録画, 画質, 音声) のセッションを返す。なければ作る。セッションを作っても、エンコーダーは起動しない。
func (v *VOD) session(key vodKey) (*vodSession, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stopped {
		return nil, errEnded
	}
	if s, ok := v.sessions[key]; ok {
		return s, nil
	}
	if len(v.sessions) >= v.maxSessions {
		return nil, errTooManySessions
	}
	dir, err := os.MkdirTemp(v.dir, key.source.String()+"-"+key.preset+"-a"+key.audio.String()+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create the session directory: %w", err)
	}
	s := &vodSession{key: key, dir: dir, runMu: sync.Mutex{}, runs: []*vodRun{}, lastAccess: v.clock.Now()}
	v.sessions[key] = s

	return s, nil
}

func (v *VOD) serveHTTP(w http.ResponseWriter, r *http.Request, kind string) {
	source, ok := parseVODSource(kind, r.PathValue("id"))
	if !ok {
		http.Error(w, "invalid ID", http.StatusBadRequest)

		return
	}
	preset, ok := PresetByName(r.PathValue("preset"))
	if !ok {
		http.Error(w, "unknown preset", http.StatusNotFound)

		return
	}
	audio, ok := ParseAudio(r.PathValue("audio"))
	if !ok {
		http.Error(w, "unknown audio", http.StatusNotFound)

		return
	}
	file := r.PathValue("file")
	switch {
	case file == PlaylistName:
		v.servePlaylist(w, r, source, preset)
	case segmentName.MatchString(file):
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file, "segment"), ".ts"))
		if err != nil {
			http.NotFound(w, r)

			return
		}
		v.serveSegment(w, r, vodKey{source: source, preset: preset.Name, audio: audio}, preset, number)
	default:
		http.NotFound(w, r)
	}
}

// servePlaylist は、索引から作ったセグメントの一覧を返す。録画中なら、書き終えた区間のセグメントだけを載せ、終わりの印を付けない。
func (v *VOD) servePlaylist(w http.ResponseWriter, r *http.Request, source vodSource, preset *Preset) {
	recording, err := v.open(r.Context(), source)
	if err != nil {
		v.writeError(w, r, err)

		return
	}
	if preset.Passthrough && slices.ContainsFunc(recording.KeyframePoints(), func(point stream.Keyframe) bool { return point.PTS < 0 }) {
		// セグメントを要求されても書けないので、プレイリストの時点で断る。
		v.writeError(w, r, errNoPTS)

		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, vodPlaylist(recording.Keyframes(), recording.InProgress, preset.Passthrough))
}

// open は、録画かタイムシフトのバッファを開く。索引がまだなければ errNotIndexed を返す。
func (v *VOD) open(ctx context.Context, source vodSource) (*stream.Recording, error) {
	recording, err := v.openSource(ctx, source)
	if err != nil {
		return nil, err
	}
	if !recording.Indexed() || len(recording.Keyframes()) == 0 {
		return nil, errNotIndexed
	}

	return recording, nil
}

// openSource は、source の種類に応じて、録画かタイムシフトのバッファを開く。タイムシフトのバッファを開けないインスタンスでは、
// stream.ErrNotMounted を返す。
func (v *VOD) openSource(ctx context.Context, source vodSource) (*stream.Recording, error) {
	if source.kind == timeshiftKind {
		if v.timeshift == nil {
			return nil, stream.ErrNotMounted
		}
		serviceID, from, _ := ParseTimeshiftID(source.id)

		return v.timeshift.Open(ctx, serviceID, from)
	}
	recordingID, err := uuid.Parse(source.id)
	if err != nil {
		return nil, fmt.Errorf("invalid recording ID: %w", err)
	}

	return v.opener.Open(ctx, recordingID)
}

// parseVODSource は、パスの {id} を、種類に応じて確かめ、同じものが同じ文字列になるようにそろえる。
func parseVODSource(kind, id string) (vodSource, bool) {
	if kind == timeshiftKind {
		serviceID, from, ok := ParseTimeshiftID(id)

		return vodSource{kind: kind, id: FormatTimeshiftID(serviceID, from)}, ok
	}
	recordingID, err := uuid.Parse(id)

	return vodSource{kind: kind, id: recordingID.String()}, err == nil
}

// FormatTimeshiftID は、タイムシフトのバッファの仮想 HLS のパスの {id} を、サービス ID と起点の時刻から作る。
// HLS のセグメントは、プレイリストからの相対の URL で引くので、起点もクエリでなくパスに入れる。
func FormatTimeshiftID(serviceID int64, from time.Time) string {
	return strconv.FormatInt(serviceID, 10) + "-" + strconv.FormatInt(from.UnixMicro(), 10)
}

// ParseTimeshiftID は、FormatTimeshiftID で作った {id} から、サービス ID と起点の時刻を返す。読めなければ false を返す。
func ParseTimeshiftID(id string) (int64, time.Time, bool) {
	service, micros, ok := strings.Cut(id, "-")
	if !ok {
		return 0, time.Time{}, false
	}
	serviceID, err := strconv.ParseInt(service, 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}

	return serviceID, time.UnixMicro(at).UTC(), true
}

// vodTimeline は、I フレームの時間の並びから、録画の長さと、プレイリストに載せるセグメントの数を返す。
// 録画の長さは、最後の I フレームの時間に、I フレームの平均の間隔を足したものとみなす。
// 録画中は、書き終えた区間のセグメントだけを数える。最後のセグメントは、まだ伸びる。keyframes は空でない。
func vodTimeline(keyframes []time.Duration, inProgress bool) (time.Duration, int) {
	interval := VODSegmentDuration
	if n := len(keyframes); n > 1 {
		interval = (keyframes[n-1] - keyframes[0]) / time.Duration(n-1)
	}
	duration := keyframes[len(keyframes)-1] + interval
	if inProgress {
		return duration, int(duration / VODSegmentDuration)
	}

	return duration, int(math.Ceil(float64(duration) / float64(VODSegmentDuration)))
}

// vodSegment は、プレイリストに載せるセグメントの、録画の始まりからの時間と長さ。
type vodSegment struct {
	start, length time.Duration
}

// vodSegments は、I フレームの時間の並びから、プレイリストに載せるセグメントを返す。keyframes は空でない。
// 映像を変換するプリセットでは、2 秒ずつに区切る。変換しないプリセット (passthrough) では、I フレームでしか切れないので、
// 前のセグメントの始まりから 2 秒以上進んだ最初の I フレームで区切る (#806)。
// 録画を終えていれば、最後のセグメントを録画の終わりまでにする。録画中は、書き終えた区間のセグメントだけを返す。
func vodSegments(keyframes []time.Duration, inProgress, passthrough bool) []vodSegment {
	duration, count := vodTimeline(keyframes, inProgress)
	if !passthrough {
		segments := make([]vodSegment, 0, count)
		for k := range count {
			start := time.Duration(k) * VODSegmentDuration
			segments = append(segments, vodSegment{start: start, length: min(VODSegmentDuration, duration-start)})
		}

		return segments
	}
	starts := passthroughStarts(keyframes)
	segments := make([]vodSegment, 0, len(starts))
	for k, start := range starts {
		if k+1 < len(starts) {
			segments = append(segments, vodSegment{start: start, length: starts[k+1] - start})
		} else if !inProgress {
			// 録画中なら、最後のセグメントは、次の始まりが索引に載るまで伸びるので載せない。
			segments = append(segments, vodSegment{start: start, length: duration - start})
		}
	}

	return segments
}

// passthroughStarts は、映像を変換しないプリセットのセグメントの始まりを、keyframes の中から選ぶ。keyframes は空でない。
// 最初の I フレームから始め、前の始まりから VODSegmentDuration 以上進んだ最初の I フレームを、次の始まりにする。
func passthroughStarts(keyframes []time.Duration) []time.Duration {
	starts := []time.Duration{keyframes[0]}
	for _, at := range keyframes[1:] {
		if at-starts[len(starts)-1] >= VODSegmentDuration {
			starts = append(starts, at)
		}
	}

	return starts
}

// vodPlaylist は、I フレームの時間の並びから、録画の仮想 HLS のプレイリストを作る。keyframes は空でない。
func vodPlaylist(keyframes []time.Duration, inProgress, passthrough bool) string {
	segments := vodSegments(keyframes, inProgress, passthrough)
	playlistType := "VOD"
	if inProgress {
		playlistType = "EVENT"
	}
	// EXT-X-TARGETDURATION は、最も長いセグメントの長さを秒に切り上げたもの。
	target := VODSegmentDuration
	for _, segment := range segments {
		target = max(target, segment.length)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:%s\n#EXT-X-INDEPENDENT-SEGMENTS\n",
		int(math.Ceil(target.Seconds())), playlistType)
	for k, segment := range segments {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\nsegment%05d.ts\n", segment.length.Seconds(), k)
	}
	if !inProgress {
		b.WriteString("#EXT-X-ENDLIST\n")
	}

	return b.String()
}

// serveSegment は、number 番目のセグメントを返す。まだ書いていなければ、書き終えるまで待つ。
func (v *VOD) serveSegment(w http.ResponseWriter, r *http.Request, key vodKey, preset *Preset, number int) {
	if v.encoder == nil {
		v.writeError(w, r, errNoEncoder)

		return
	}
	s, err := v.session(key)
	if err != nil {
		v.writeError(w, r, err)

		return
	}
	path, err := v.awaitSegment(r.Context(), s, preset, number)
	if err != nil {
		v.writeError(w, r, err)

		return
	}
	file, err := sharedfile.Open(path)
	if err != nil {
		http.NotFound(w, r)

		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, file)
}

// awaitSegment は、number 番目のセグメントのパスを返す。まだ書いていなければ、動いているエンコーダーが
// すぐ後に書くセグメントなら待ち、そうでなければ、その時刻からエンコーダーを起動し直して待つ。
// 待っている間に、ほかの視聴者のシークでエンコーダーが起動し直されたら、改めて書かせる。
func (v *VOD) awaitSegment(ctx context.Context, s *vodSession, preset *Preset, number int) (string, error) {
	path := filepath.Join(s.dir, fmt.Sprintf(segmentPattern, number))
	v.mu.Lock()
	s.lastAccess = v.clock.Now()
	v.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		// エンコーダーが先に書いたセグメントを見ている間は、そのエンコーダーを使っているとみなす。
		s.runMu.Lock()
		if run := s.coveringLocked(number, true); run != nil {
			run.lastUsed = v.clock.Now()
			run.want(number)
		}
		s.runMu.Unlock()

		return path, nil
	}
	// 録画の外のセグメントのために、エンコーダーを起動し直さない。
	recording, err := v.open(ctx, s.key.source)
	if err != nil {
		return "", err
	}
	if segments := vodSegments(recording.Keyframes(), recording.InProgress, preset.Passthrough); number < 0 || number >= len(segments) {
		return "", errOutOfRange
	}

	deadline := time.NewTimer(segmentTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		run, err := v.ensureRun(ctx, s, preset, number)
		if err != nil {
			return "", err
		}
		path, err := v.waitFor(ctx, s, run, path, deadline.C, ticker.C)
		if !errors.Is(err, errReplaced) {
			return path, err
		}
	}
}

// errReplaced は、待っていたエンコーダーが、ほかの視聴者のシークで起動し直されたことを表す。
var errReplaced = errors.New("the encoder was replaced")

// waitFor は、run が path を書き終えるのを待つ。
func (v *VOD) waitFor(ctx context.Context, s *vodSession, run *vodRun, path string, deadline, tick <-chan time.Time) (string, error) {
	for {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-run.done:
			// 止まる前に書き終えていれば、そのセグメントを返す。
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
			s.runMu.Lock()
			replaced := run.replaced
			s.runMu.Unlock()
			if replaced {
				return "", errReplaced
			}
			if run.err != nil {
				return "", run.err
			}

			return "", errEnded
		case <-deadline:
			return "", errSegmentTimeout
		case <-tick:
		}
	}
}

// ensureRun は、number 番目のセグメントを書くエンコーダーを返す。動いているエンコーダーのどれかが、次に書くセグメントから
// aheadSegments 先までに number を書くなら、それを返す。そうでなければ、number 番目から起動する。
// エンコーダーが maxVODRuns だけ動いていたら、最も長く使われていないものを止めてから起動する。
func (v *VOD) ensureRun(ctx context.Context, s *vodSession, preset *Preset, number int) (*vodRun, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.pruneLocked()
	now := v.clock.Now()
	// 要求を受けてからここまでに、書き終えたかもしれない。
	path := filepath.Join(s.dir, fmt.Sprintf(segmentPattern, number))
	_, err := os.Stat(path)
	run := s.coveringLocked(number, err == nil)
	if run == nil && err != nil {
		// 確かめてから、エンコーダーが次に書くセグメントを数えるまでに、number より先まで書いたかもしれない。
		// セグメントを消すのは runMu を持つときだけなので、今あれば、エンコーダーが書いたものである。
		if _, err := os.Stat(path); err == nil {
			run = s.coveringLocked(number, true)
		}
	}
	if run != nil {
		run.lastUsed = now
		run.want(number)

		return run, nil
	}
	if len(s.runs) >= maxVODRuns {
		s.stopLocked(slices.MinFunc(s.runs, func(a, b *vodRun) int { return a.lastUsed.Compare(b.lastUsed) }))
	}
	run, err = v.start(ctx, s, preset, number)
	if err != nil {
		return nil, err
	}
	run.lastUsed = now
	s.runs = append(s.runs, run)

	return run, nil
}

// coveringLocked は、次に書くセグメントから aheadSegments 先までに number を書く (written なら、書いた) エンコーダーのうち、
// 最も後ろから書き始めたもの、つまり number に最も近いものを返す。なければ nil を返す。runMu を持って呼ぶ。
// まだない number は、エンコーダーが書いた後に消したものかもしれないので、次に書くセグメントより前なら、書くとはみなさない。
func (s *vodSession) coveringLocked(number int, written bool) *vodRun {
	var covering *vodRun
	for _, run := range s.runs {
		next := run.nextSegment(s.dir)
		from := next
		if written {
			from = run.start
		}
		if number >= from && number <= next+aheadSegments && (covering == nil || run.start > covering.start) {
			covering = run
		}
	}

	return covering
}

// pruneLocked は、止まったエンコーダーを runs から除き、前のエンコーダーが書いた位置に追いついた後ろのエンコーダーを止める。
// 後ろのエンコーダーは、そこから先も前のエンコーダーと同じセグメントを書くだけだからである。runMu を持って呼ぶ。
func (s *vodSession) pruneLocked() {
	s.runs = slices.DeleteFunc(s.runs, func(run *vodRun) bool {
		select {
		case <-run.done:
			return true
		default:
			return false
		}
	})
	for _, behind := range slices.Clone(s.runs) {
		next := behind.nextSegment(s.dir)
		if slices.ContainsFunc(s.runs, func(ahead *vodRun) bool { return ahead.start > behind.start && ahead.start <= next }) {
			s.stopLocked(behind)
		}
	}
}

// removeFarBehindLocked は、動いているエンコーダーに書かせている視聴者のうち、最も後ろの位置から、
// keepBehindSegments より前のセグメントを消す。runMu を持って呼ぶ。
func (s *vodSession) removeFarBehindLocked(logger *slog.Logger) {
	if len(s.runs) == 0 {
		return
	}
	behind := slices.MinFunc(s.runs, func(a, b *vodRun) int { return cmp.Compare(a.wanted.Load(), b.wanted.Load()) }).wanted.Load()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		logger.Warn("failed to list the session directory", slog.Any("error", err))

		return
	}
	for _, entry := range entries {
		if !segmentName.MatchString(entry.Name()) {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "segment"), ".ts"))
		if err != nil || int64(number) >= behind-keepBehindSegments {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("failed to remove a segment", slog.String("segment", entry.Name()), slog.Any("error", err))
		}
	}
}

// stopLocked は、ほかの視聴者のために run を止め、止まるのを待って、runs から除く。待っていた視聴者は、改めて書かせる。runMu を持って呼ぶ。
func (s *vodSession) stopLocked(run *vodRun) {
	run.replaced = true
	run.cancel()
	<-run.done
	s.runs = slices.DeleteFunc(s.runs, func(r *vodRun) bool { return r == run })
}

// nextSegment は、dir に書いているエンコーダーが、次に書くセグメントの番号を返す。
// startedAt より前に書かれたセグメントは、前の起動が書いたものなので、数えない。
func (run *vodRun) nextSegment(dir string) int {
	run.nextMu.Lock()
	defer run.nextMu.Unlock()
	for {
		info, err := os.Stat(filepath.Join(dir, fmt.Sprintf(segmentPattern, run.next)))
		if err != nil || info.ModTime().Before(run.startedAt) {
			return run.next
		}
		run.next++
	}
}

// want は、視聴者が、このエンコーダーに書かせる number 番目のセグメントを要求したことを記録する。
func (run *vodRun) want(number int) {
	for {
		wanted := run.wanted.Load()
		if int64(number) <= wanted || run.wanted.CompareAndSwap(wanted, int64(number)) {
			return
		}
	}
}

// lookaheadWriter は、エンコーダーが視聴者の要求より lookaheadSegments 先まで書いたら、追いつかれるまで書き込みを待たせる。
// 待たせている間に、エンコーダーが止められたら (stopped が閉じたら) 抜ける。
type lookaheadWriter struct {
	w       io.Writer
	run     *vodRun
	dir     string
	stopped <-chan struct{}
}

func (l *lookaheadWriter) Write(p []byte) (int, error) {
	for int64(l.run.nextSegment(l.dir)) > l.run.wanted.Load()+lookaheadSegments {
		select {
		case <-l.stopped:
			return 0, errEnded
		case <-time.After(pollInterval):
		}
	}

	return l.w.Write(p)
}

// fileClock は、dir のファイルシステムが今書いたファイルに付ける更新時刻を返す。
// 更新時刻はカーネルの粗い時計で付くので、time.Now() と比べると、直後に書いたファイルが前に書いたように見えることがある。
// 測れなければ time.Now() を返す。
func fileClock(dir string) time.Time {
	file, err := os.CreateTemp(dir, ".clock-")
	if err != nil {
		return time.Now()
	}
	defer os.Remove(file.Name())
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return time.Now()
	}

	return info.ModTime()
}

// start は、number 番目のセグメントの時刻の手前の I フレームから、録画を流してエンコーダーを起動する。
// エンコーダーはリクエストより長く動くので、ctx の値だけを引き継ぎ、ctx が終わっても止めない。
func (v *VOD) start(ctx context.Context, s *vodSession, preset *Preset, number int) (*vodRun, error) {
	recording, err := v.openSource(ctx, s.key.source)
	if err != nil {
		return nil, err
	}
	at := time.Duration(number) * VODSegmentDuration
	var passthrough *RunOptions
	if preset.Passthrough {
		passthrough, at, err = passthroughOptions(recording.KeyframePoints(), number)
		if err != nil {
			return nil, err
		}
	}
	pos := recording.Seek(max(at-seekMargin, 0))
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run := &vodRun{
		start: number, startedAt: fileClock(s.dir), lastUsed: time.Time{}, replaced: false, wanted: atomic.Int64{},
		nextMu: sync.Mutex{}, next: number, cancel: cancel, done: make(chan struct{}), err: nil,
	}
	run.wanted.Store(int64(number))
	reader, writer := io.Pipe()
	go func() {
		writer.CloseWithError(recording.Write(runCtx, &lookaheadWriter{w: writer, run: run, dir: s.dir, stopped: runCtx.Done()}, pos))
	}()
	go func() {
		opts := passthrough
		if opts == nil {
			opts = &RunOptions{StartNumber: number, Trim: max(at-pos.At, 0), Cuts: nil, Offset: 0}
		}
		err := v.encoder.RunVOD(runCtx, reader, preset, s.key.audio, s.dir, opts)
		reader.CloseWithError(err)
		if runCtx.Err() != nil {
			err = errEnded
		}
		if err != nil && !errors.Is(err, errEnded) {
			v.logger.WarnContext(runCtx, "the recording session failed", slog.String("source", s.key.source.String()), slog.Any("error", err))
		}
		run.err = err
		cancel()
		close(run.done)
	}()

	return run, nil
}

// passthroughOptions は、映像を変換しないプリセットで、number 番目のセグメントから書くときの設定と、その始まりの時間を返す (#806)。
// セグメントを、索引の I フレームの PTS で区切らせる。出力の PTS は、number 番目のセグメントの始まりが、passthroughBase に
// その始まりの時間を足したものになるようにずらす。索引のないセグメントがあれば、PTS が分からないので errNoPTS を返す。
func passthroughOptions(points []stream.Keyframe, number int) (*RunOptions, time.Duration, error) {
	keyframes := make([]time.Duration, 0, len(points))
	for _, point := range points {
		if point.PTS < 0 {
			return nil, 0, errNoPTS
		}
		keyframes = append(keyframes, point.At)
	}
	if len(keyframes) == 0 {
		return nil, 0, errNotIndexed
	}
	starts := passthroughStarts(keyframes)
	if number < 0 || number >= len(starts) {
		return nil, 0, errOutOfRange
	}
	cuts := make([]int64, 0, len(starts)-number)
	j := number
	for _, point := range points {
		if j < len(starts) && point.At == starts[j] {
			cuts = append(cuts, point.PTS)
			j++
		}
	}
	at := starts[number]
	// 刻みは、マイクロ秒から求める。ナノ秒に PTSClock を掛けると、長い時間で int64 を超えるため。
	offset := (passthroughBase+at).Microseconds()*ts.PTSClock/int64(time.Second/time.Microsecond) - cuts[0]
	// FFmpeg はマイクロ秒で受け取るので、9 の倍数 (100 マイクロ秒) に切り捨てて、刻みの数を保つ。
	offset -= ((offset % 9) + 9) % 9
	for i := range cuts {
		cuts[i] = ((cuts[i]+offset)%ptsWrap + ptsWrap) % ptsWrap
	}

	return &RunOptions{StartNumber: number, Trim: 0, Cuts: cuts, Offset: offset}, at, nil
}

// writeError は、エラーに応じた状態を返す。
func (v *VOD) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case r.Context().Err() != nil:
	case errors.Is(err, stream.ErrRecordingNotFound), errors.Is(err, stream.ErrTimeshiftNotFound), errors.Is(err, stream.ErrNoSegment),
		errors.Is(err, errOutOfRange):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, stream.ErrNotMounted), errors.Is(err, errNotIndexed), errors.Is(err, errNoPTS), errors.Is(err, errNoEncoder),
		errors.Is(err, errEnded), errors.Is(err, errSegmentTimeout), errors.Is(err, errTooManySessions):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		v.logger.ErrorContext(r.Context(), "failed to serve the recording", slog.Any("error", err))
		http.Error(w, "failed to transcode the recording", http.StatusBadGateway)
	}
}
