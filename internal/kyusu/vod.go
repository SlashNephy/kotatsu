package kyusu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/stream"
)

const (
	// RecordingPathPrefix は、kyusu の録画の仮想 HLS のパスの前置き。/internal/kyusu/recordings/{録画 ID}/{画質}/{ファイル} で受け付ける。
	RecordingPathPrefix = "/internal/kyusu/recordings/"
	// recordingDir は、作業ディレクトリのうち、録画の仮想 HLS を置くディレクトリ。
	recordingDir = "recordings"
	// segmentTimeout は、要求されたセグメントを書き終えるのを待つ時間の上限。エンコーダーを起動し直したときも、ここまで待つ。
	segmentTimeout = 30 * time.Second
	// seekMargin は、k 番目のセグメントを書くとき、その始まりより手前から入力を流す時間。
	// 索引の時刻とエンコーダーの時刻のずれで、始まりより後の I フレームから流してしまわないようにする。
	seekMargin = time.Second
	// aheadSegments は、動いているエンコーダーが、次に書くセグメントからいくつ先までなら、起動し直さずに待つか。
	// プレイヤーが先読みで少し先のセグメントを要求しても、起動し直さないようにする。
	aheadSegments = 5
)

var (
	errNotIndexed     = errors.New("the recording is not indexed yet; try again after the analysis")
	errSegmentTimeout = errors.New("the encoder did not write the segment in time")
	errOutOfRange     = errors.New("the segment is out of the recording")
)

// RecordingOpener は、録画を開く。*stream.RecordingSource が満たす。
type RecordingOpener interface {
	Open(ctx context.Context, recordingID uuid.UUID) (*stream.Recording, error)
}

// VODEncoder は、録画の仮想 HLS のセグメントを書く。*Transcoder が満たす。
type VODEncoder interface {
	RunVOD(ctx context.Context, src io.Reader, preset *Preset, dir string, opts *RunOptions) error
}

var (
	_ RecordingOpener = new(stream.RecordingSource)
	_ VODEncoder      = new(Transcoder)
)

// vodKey は、録画の仮想 HLS のセッションを見分けるもの。同じ録画を同じ画質で見る視聴者は、1 つのセッションを共有する。
type vodKey struct {
	recordingID uuid.UUID
	preset      string
}

// vodRun は、k 番目のセグメントから書くエンコーダーの 1 回の起動。
type vodRun struct {
	start int
	// startedAt は、起動した時刻。これより前に書かれたセグメントは、前の起動が書いたものとみなす。
	startedAt time.Time
	cancel    context.CancelFunc
	// done は、エンコーダーが止まったら閉じる。err は、閉じる前に書く。
	done chan struct{}
	err  error
}

// vodSession は、セグメントを書くディレクトリと、動いているエンコーダー。
type vodSession struct {
	key vodKey
	dir string
	// runMu は run を守る。エンコーダーを止めて起動し直す間も、ほかのセッションへのリクエストを待たせないよう、セッションごとに持つ。
	runMu sync.Mutex
	run   *vodRun
	// lastAccess は、VOD の mu で守る。
	lastAccess time.Time
}

// VOD は、録画の仮想 HLS を配信する (設計書 7.3)。
//
// プレイリストは、索引から、録画の始まりからの時間を 2 秒ずつ区切ったセグメントの一覧として先に作る。
// まだ書いていないセグメントを要求されたら、その時刻の手前の I フレームからエンコーダーを起動し直す。
// 動いているエンコーダーがすぐ後に書くセグメントなら、起動し直さずに待つ。
// 書いたセグメントは消さないので、巻き戻しても書き直さない。セグメントへの要求が 10 秒途絶えたら、エンコーダーを止めて消す。
type VOD struct {
	opener      RecordingOpener
	encoder     VODEncoder
	dir         string
	clock       clock.Clock
	logger      *slog.Logger
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[vodKey]*vodSession
	stopped  bool
}

// VODOptions は VOD の設定。
type VODOptions struct {
	// Opener は、録画を開く。録画のある保存先プールを、このインスタンスにマウントしておく。
	Opener RecordingOpener
	// Encoder は、セグメントを書く。FFmpeg と tsreadex がなければ nil にし、503 を返す。
	Encoder VODEncoder
	// WorkDir は、HLS を置く作業ディレクトリ。空なら、OS の一時ディレクトリの下にする。
	WorkDir string
	Clock   clock.Clock
	Logger  *slog.Logger
	// IdleTimeout は、セグメントへのリクエストが途絶えてから、エンコーダーを止めるまでの時間。0 なら 10 秒。
	IdleTimeout time.Duration
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

	return &VOD{
		opener: opts.Opener, encoder: opts.Encoder, dir: dir, clock: opts.Clock, logger: opts.Logger, idleTimeout: idle,
		mu: sync.Mutex{}, sessions: map[vodKey]*vodSession{}, stopped: false,
	}, nil
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
	mux.HandleFunc("GET "+RecordingPathPrefix+"{id}/{preset}/{file}", v.serveHTTP)

	return token.Middleware(mux)
}

// reap は、now の時点で、セグメントを idleTimeout より長く要求されていないセッションを止めて、セグメントを消す。
func (v *VOD) reap(now time.Time) {
	v.mu.Lock()
	idle := []*vodSession{}
	for key, s := range v.sessions {
		if now.Sub(s.lastAccess) > v.idleTimeout {
			delete(v.sessions, key)
			idle = append(idle, s)
		}
	}
	v.mu.Unlock()
	for _, s := range idle {
		v.logger.Info("stopping an idle recording session", slog.String("recording_id", s.key.recordingID.String()), slog.String("preset", s.key.preset))
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
	run := s.run
	s.runMu.Unlock()
	if run != nil {
		run.cancel()
		<-run.done
	}
	if err := os.RemoveAll(s.dir); err != nil {
		v.logger.Warn("failed to remove the session directory", slog.Any("error", err))
	}
}

// session は、(録画, 画質) のセッションを返す。なければ作る。セッションを作っても、エンコーダーは起動しない。
func (v *VOD) session(key vodKey) (*vodSession, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stopped {
		return nil, errEnded
	}
	if s, ok := v.sessions[key]; ok {
		return s, nil
	}
	dir, err := os.MkdirTemp(v.dir, key.recordingID.String()+"-"+key.preset+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create the session directory: %w", err)
	}
	s := &vodSession{key: key, dir: dir, runMu: sync.Mutex{}, run: nil, lastAccess: v.clock.Now()}
	v.sessions[key] = s

	return s, nil
}

func (v *VOD) serveHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	preset, ok := PresetByName(r.PathValue("preset"))
	if !ok {
		http.Error(w, "unknown preset", http.StatusNotFound)

		return
	}
	file := r.PathValue("file")
	switch {
	case file == PlaylistName:
		v.servePlaylist(w, r, recordingID)
	case segmentName.MatchString(file):
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file, "segment"), ".ts"))
		if err != nil {
			http.NotFound(w, r)

			return
		}
		v.serveSegment(w, r, vodKey{recordingID: recordingID, preset: preset.Name}, preset, number)
	default:
		http.NotFound(w, r)
	}
}

// servePlaylist は、索引から作ったセグメントの一覧を返す。録画中なら、書き終えた区間のセグメントだけを載せ、終わりの印を付けない。
func (v *VOD) servePlaylist(w http.ResponseWriter, r *http.Request, recordingID uuid.UUID) {
	recording, err := v.open(r.Context(), recordingID)
	if err != nil {
		v.writeError(w, r, err)

		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, vodPlaylist(recording.Keyframes(), recording.InProgress))
}

// open は、録画を開く。索引がまだなければ errNotIndexed を返す。
func (v *VOD) open(ctx context.Context, recordingID uuid.UUID) (*stream.Recording, error) {
	recording, err := v.opener.Open(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	if !recording.Indexed() || len(recording.Keyframes()) == 0 {
		return nil, errNotIndexed
	}

	return recording, nil
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

// vodPlaylist は、I フレームの時間の並びから、録画の仮想 HLS のプレイリストを作る。keyframes は空でない。
func vodPlaylist(keyframes []time.Duration, inProgress bool) string {
	duration, count := vodTimeline(keyframes, inProgress)
	playlistType := "VOD"
	if inProgress {
		playlistType = "EVENT"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:%s\n#EXT-X-INDEPENDENT-SEGMENTS\n",
		int(VODSegmentDuration/time.Second), playlistType)
	for k := range count {
		length := VODSegmentDuration
		if !inProgress && k == count-1 {
			length = duration - time.Duration(k)*VODSegmentDuration
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\nsegment%05d.ts\n", length.Seconds(), k)
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
	file, err := os.Open(path)
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
		return path, nil
	}
	// 録画の外のセグメントのために、エンコーダーを起動し直さない。
	recording, err := v.open(ctx, s.key.recordingID)
	if err != nil {
		return "", err
	}
	if _, count := vodTimeline(recording.Keyframes(), recording.InProgress); number < 0 || number >= count {
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
			replaced := s.run != run
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

// ensureRun は、number 番目のセグメントを書くエンコーダーを返す。動いているエンコーダーが、次に書くセグメントから
// aheadSegments 先までに number を書くなら、それを返す。そうでなければ、止めてから、number 番目から起動し直す。
func (v *VOD) ensureRun(ctx context.Context, s *vodSession, preset *Preset, number int) (*vodRun, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if run := s.run; run != nil {
		select {
		case <-run.done:
		default:
			next := nextSegment(s.dir, run.start, run.startedAt)
			if number >= run.start && number <= next+aheadSegments {
				return run, nil
			}
			run.cancel()
			<-run.done
		}
	}
	run, err := v.start(ctx, s, preset, number)
	if err != nil {
		return nil, err
	}
	s.run = run

	return run, nil
}

// nextSegment は、start 番目から書いているエンコーダーが、次に書くセグメントの番号を返す。
// since より前に書かれたセグメントは、前の起動が書いたものなので、数えない。
func nextSegment(dir string, start int, since time.Time) int {
	next := start
	for {
		info, err := os.Stat(filepath.Join(dir, fmt.Sprintf(segmentPattern, next)))
		if err != nil || info.ModTime().Before(since) {
			return next
		}
		next++
	}
}

// start は、number 番目のセグメントの時刻の手前の I フレームから、録画を流してエンコーダーを起動する。
// エンコーダーはリクエストより長く動くので、ctx の値だけを引き継ぎ、ctx が終わっても止めない。
func (v *VOD) start(ctx context.Context, s *vodSession, preset *Preset, number int) (*vodRun, error) {
	recording, err := v.opener.Open(ctx, s.key.recordingID)
	if err != nil {
		return nil, err
	}
	at := time.Duration(number) * VODSegmentDuration
	pos := recording.Seek(max(at-seekMargin, 0))
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run := &vodRun{start: number, startedAt: time.Now(), cancel: cancel, done: make(chan struct{}), err: nil}
	reader, writer := io.Pipe()
	go func() {
		writer.CloseWithError(recording.Write(runCtx, writer, pos))
	}()
	go func() {
		err := v.encoder.RunVOD(runCtx, reader, preset, s.dir, &RunOptions{StartNumber: number, Trim: max(at-pos.At, 0)})
		reader.CloseWithError(err)
		if runCtx.Err() != nil {
			err = errEnded
		}
		if err != nil && !errors.Is(err, errEnded) {
			v.logger.WarnContext(runCtx, "the recording session failed", slog.String("recording_id", s.key.recordingID.String()), slog.Any("error", err))
		}
		run.err = err
		cancel()
		close(run.done)
	}()

	return run, nil
}

// writeError は、エラーに応じた状態を返す。
func (v *VOD) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case r.Context().Err() != nil:
	case errors.Is(err, stream.ErrRecordingNotFound), errors.Is(err, stream.ErrNoSegment), errors.Is(err, errOutOfRange):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, stream.ErrNotMounted), errors.Is(err, errNotIndexed), errors.Is(err, errNoEncoder),
		errors.Is(err, errEnded), errors.Is(err, errSegmentTimeout):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		v.logger.ErrorContext(r.Context(), "failed to serve the recording", slog.Any("error", err))
		http.Error(w, "failed to transcode the recording", http.StatusBadGateway)
	}
}
