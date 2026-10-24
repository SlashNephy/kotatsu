package kyusu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// LivePathPrefix は、kyusu のライブ視聴の HLS のパスの前置き。/internal/kyusu/live/{サービス ID}/{画質}/{音声}/{ファイル} で受け付ける。
	LivePathPrefix = "/internal/kyusu/live/"
	// DefaultIdleTimeout は、セグメントへのリクエストが途絶えてから、エンコーダーを止めるまでの時間 (設計書 7.3、付録 A)。
	DefaultIdleTimeout = 10 * time.Second
	// playlistTimeout は、エンコーダーを起動してから、最初のセグメントがプレイリストに載るのを待つ時間の上限。
	playlistTimeout = 20 * time.Second
	// pollInterval は、プレイリストができたかを確かめる間隔。
	pollInterval = 100 * time.Millisecond
	// reapInterval は、使われていないセッションを探す間隔。
	reapInterval = time.Second
	// livePriority は、ライブ視聴でチューナーを確保するときの優先度 (設計書 7.2)。
	livePriority = 0
	// liveDir は、作業ディレクトリのうち、ライブ視聴の HLS を置くディレクトリ。
	liveDir = "live"
)

// segmentName は、HLS のセグメントのファイルの名前。これ以外の名前のファイルは配信しない。
var segmentName = regexp.MustCompile(`^segment[0-9]{5}\.ts$`)

var (
	errNoTuner    = errors.New("tuner server is not configured")
	errNoEncoder  = errors.New("FFmpeg or tsreadex is not installed")
	errNoPlaylist = errors.New("the encoder did not write a playlist in time")
	errEnded      = errors.New("the encoder stopped")
)

// ServiceOpener は、サービスのストリームを開く。*mirakurun.Client が満たす。
type ServiceOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

// Encoder は、TS を HLS にする。*Transcoder が満たす。
type Encoder interface {
	Run(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string) error
}

var (
	_ ServiceOpener = new(mirakurun.Client)
	_ Encoder       = new(Transcoder)
)

// sessionKey は、ライブ視聴のセッションを見分けるもの。同じサービスを同じ画質と音声で見る視聴者は、1 つのセッションを共有する。
type sessionKey struct {
	serviceID int64
	preset    string
	audio     Audio
}

// session は、1 つのエンコーダーと、それが HLS を書くディレクトリ。
type session struct {
	key    sessionKey
	dir    string
	cancel context.CancelFunc
	// done は、エンコーダーが止まって、ディレクトリを消したら閉じる。err は、閉じる前に書く。
	done chan struct{}
	err  error
	// startedAt は、エンコーダーを起動した時刻。
	startedAt time.Time
	// ready は、最初のセグメントがプレイリストに載ったかどうか。lastAccess は、最後にセグメントを要求された時刻。
	// どちらも Live の mu で守る。
	ready      bool
	lastAccess time.Time
}

// Live は、ライブ視聴のセッションを、(サービス, 画質, 音声) の組ごとに作って共有し、使われなくなったら止める (設計書 7.3)。
type Live struct {
	// opener は、チューナーサーバーのクライアント。設定していなければ nil。
	opener ServiceOpener
	// encoder は、FFmpeg と tsreadex がなければ nil。
	encoder     Encoder
	dir         string
	clock       clock.Clock
	logger      *slog.Logger
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[sessionKey]*session
	// stopped は、Run が終わったかどうか。終わった後は、新しいセッションを作らない。
	stopped bool
}

// LiveOptions は Live の設定。
type LiveOptions struct {
	// Opener は、チューナーサーバーのクライアント。設定していなければ nil にし、ライブ視聴に 503 を返す。
	Opener ServiceOpener
	// Encoder は、TS を HLS にする。FFmpeg と tsreadex がなければ nil にし、ライブ視聴に 503 を返す。
	Encoder Encoder
	// WorkDir は、HLS を置く作業ディレクトリ。空なら、OS の一時ディレクトリの下にする。
	WorkDir string
	Clock   clock.Clock
	Logger  *slog.Logger
	// IdleTimeout は、セグメントへのリクエストが途絶えてから、エンコーダーを止めるまでの時間。0 なら 10 秒。
	IdleTimeout time.Duration
}

// NewLive は Live を返す。前に動いていたときの HLS が作業ディレクトリに残っていれば消す。
func NewLive(opts *LiveOptions) (*Live, error) {
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "kotatsu-kyusu")
	}
	dir := filepath.Join(workDir, liveDir)
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

	return &Live{
		opener: opts.Opener, encoder: opts.Encoder, dir: dir, clock: opts.Clock, logger: opts.Logger, idleTimeout: idle,
		mu: sync.Mutex{}, sessions: map[sessionKey]*session{}, stopped: false,
	}, nil
}

// Sessions は、今あるライブ視聴のセッションの数を返す。/metrics に出す (設計書 11.3)。
func (l *Live) Sessions() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.sessions)
}

// Run は、ctx が終わるまで、使われなくなったセッションを止める。ctx が終わったら、すべてのセッションを止める。
func (l *Live) Run(ctx context.Context) error {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.stopAll()

			return nil
		case <-ticker.C:
			l.reap(l.clock.Now())
		}
	}
}

// Handler は、ライブ視聴の HLS を配信するハンドラーを返す。token を付けたリクエストだけを受け付ける。
func (l *Live) Handler(token *Token) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+LivePathPrefix+"{service}/{preset}/{audio}/{file}", l.serveHTTP)

	return token.Middleware(mux)
}

// CanTranscode は、エンコーダーがあり、トランスコードできるかどうかを返す。
// FFmpeg か tsreadex がなければ、kyusu は起動しても、トランスコードの要求に 503 を返す。
func (l *Live) CanTranscode() bool {
	return l.encoder != nil
}

// reap は、now の時点で、セグメントを idleTimeout より長く要求されていないセッションを止める。
// 最初のセグメントができるまでの間は、プレイリストを待つ時間の上限まで止めない。
// 止めたセッションは、すぐに一覧から外すので、止まり終わる前に来た視聴者には、新しいセッションを作る。
func (l *Live) reap(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, s := range l.sessions {
		if !s.ready && now.Sub(s.startedAt) <= playlistTimeout {
			continue
		}
		if now.Sub(s.lastAccess) > l.idleTimeout {
			l.logger.Info("stopping an idle live session",
				slog.Int64("service_id", s.key.serviceID), slog.String("preset", s.key.preset), slog.String("audio", s.key.audio.String()))
			delete(l.sessions, key)
			s.cancel()
		}
	}
}

// stopAll は、新しいセッションを作らないようにし、すべてのセッションを止めて、止まるまで待つ。
func (l *Live) stopAll() {
	l.mu.Lock()
	l.stopped = true
	sessions := make([]*session, 0, len(l.sessions))
	for _, s := range l.sessions {
		s.cancel()
		sessions = append(sessions, s)
	}
	l.mu.Unlock()
	for _, s := range sessions {
		<-s.done
	}
}

// start は、(サービス, 画質, 音声) のセッションを返す。なければ、エンコーダーを起動して作る。
// エンコーダーは、リクエストより長く動くので、ctx の値だけを引き継ぎ、ctx が終わっても止めない。
func (l *Live) start(ctx context.Context, serviceID int64, preset *Preset, audio Audio) (*session, error) {
	if l.opener == nil {
		return nil, errNoTuner
	}
	if l.encoder == nil {
		return nil, errNoEncoder
	}
	key := sessionKey{serviceID: serviceID, preset: preset.Name, audio: audio}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return nil, errEnded
	}
	if s, ok := l.sessions[key]; ok {
		return s, nil
	}
	dir, err := os.MkdirTemp(l.dir, strconv.FormatInt(serviceID, 10)+"-"+preset.Name+"-a"+audio.String()+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create the session directory: %w", err)
	}
	sessionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	now := l.clock.Now()
	s := &session{key: key, dir: dir, cancel: cancel, done: make(chan struct{}), err: nil, startedAt: now, ready: false, lastAccess: now}
	l.sessions[key] = s
	l.logger.InfoContext(ctx, "starting a live session",
		slog.Int64("service_id", serviceID), slog.String("preset", preset.Name), slog.String("audio", audio.String()))
	go l.encode(sessionCtx, s, preset)

	return s, nil
}

// encode は、サービスのストリームを開き、止められるか途切れるまでエンコードする。止まったら、セッションを外して、HLS を消す。
func (l *Live) encode(ctx context.Context, s *session, preset *Preset) {
	err := func() error {
		upstream, err := l.opener.OpenServiceStream(ctx, s.key.serviceID, &mirakurun.StreamOptions{Decode: true, Priority: livePriority})
		if err != nil {
			return err
		}
		defer func() {
			_ = upstream.Close()
		}()

		return l.encoder.Run(ctx, upstream, preset, s.key.audio, s.dir)
	}()
	if ctx.Err() != nil {
		// 使われなくなったか、kyusu を止めたので止めた。
		err = errEnded
	}
	if err != nil && !errors.Is(err, errEnded) {
		l.logger.WarnContext(ctx, "the live session failed", slog.Int64("service_id", s.key.serviceID), slog.String("preset", preset.Name), slog.Any("error", err))
	}
	l.mu.Lock()
	if l.sessions[s.key] == s {
		delete(l.sessions, s.key)
	}
	l.mu.Unlock()
	s.cancel()
	if removeErr := os.RemoveAll(s.dir); removeErr != nil {
		l.logger.WarnContext(ctx, "failed to remove the session directory", slog.Any("error", removeErr))
	}
	if err == nil {
		err = errEnded
	}
	s.err = err
	close(s.done)
}

// playlist は、セッションのプレイリストに最初のセグメントが載るまで待ち、プレイリストを返す。
func (l *Live) playlist(ctx context.Context, s *session) ([]byte, error) {
	deadline := time.NewTimer(playlistTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(s.dir, PlaylistName))
		if err == nil && bytes.Contains(data, []byte("#EXTINF:")) {
			l.markReady(s)

			return data, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.done:
			return nil, s.err
		case <-deadline.C:
			return nil, errNoPlaylist
		case <-ticker.C:
		}
	}
}

// markReady は、セッションの最初のセグメントができたことを記録する。セグメントを要求されるまでの猶予は、ここから数える。
func (l *Live) markReady(s *session) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !s.ready {
		s.ready = true
		s.lastAccess = l.clock.Now()
	}
}

// segment は、セッションのセグメントのパスを返し、セッションを使われているものとして残す。セッションがなければ false を返す。
func (l *Live) segment(serviceID int64, preset *Preset, audio Audio, name string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[sessionKey{serviceID: serviceID, preset: preset.Name, audio: audio}]
	if !ok {
		return "", false
	}
	s.lastAccess = l.clock.Now()

	return filepath.Join(s.dir, name), true
}

func (l *Live) serveHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("service"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

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
		l.servePlaylist(w, r, serviceID, preset, audio)
	case segmentName.MatchString(file):
		l.serveSegment(w, r, serviceID, preset, audio, file)
	default:
		http.NotFound(w, r)
	}
}

func (l *Live) servePlaylist(w http.ResponseWriter, r *http.Request, serviceID int64, preset *Preset, audio Audio) {
	s, err := l.start(r.Context(), serviceID, preset, audio)
	var data []byte
	if err == nil {
		data, err = l.playlist(r.Context(), s)
	}
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	case r.Context().Err() != nil:
	case mirakurun.IsStatus(err, http.StatusNotFound):
		http.Error(w, "service not found", http.StatusNotFound)
	case mirakurun.IsStatus(err, http.StatusServiceUnavailable):
		http.Error(w, "no tuner is available", http.StatusServiceUnavailable)
	case errors.Is(err, errNoTuner), errors.Is(err, errNoEncoder), errors.Is(err, errNoPlaylist), errors.Is(err, errEnded):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		l.logger.ErrorContext(r.Context(), "failed to start the live session", slog.Any("error", err))
		http.Error(w, "failed to start the live session", http.StatusBadGateway)
	}
}

func (l *Live) serveSegment(w http.ResponseWriter, r *http.Request, serviceID int64, preset *Preset, audio Audio, name string) {
	path, ok := l.segment(serviceID, preset, audio, name)
	if !ok {
		http.Error(w, "the live session is not running", http.StatusNotFound)

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
