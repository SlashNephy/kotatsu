package kyusu_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// keyMaterial は、テストのアプリの秘密キー。
	keyMaterial = "kotatsu-kyusu-test-application-key-0123456789"
	// livePath は、「こたつテスト1」を 360p で見るときのパス。
	livePath = kyusu.LivePathPrefix + "3273601024/360p/1/"
	// fakePlaylist は、fakeEncoder が書くプレイリスト。
	fakePlaylist = "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nsegment00000.ts\n"
)

// fakeEncoder は、プレイリストとセグメントを 1 つずつ書き、止められるまで待つ。起動した回数と、動いている数を数える。
// warmup があれば、閉じられるまで、プレイリストを書かずに待つ。stopDelay は、止められてから戻るまでの時間。
type fakeEncoder struct {
	runs atomic.Int32
	// audios は、起動されたときの音声を、起動した順に持つ。
	mu        sync.Mutex
	audios    []kyusu.Audio
	running   atomic.Int32
	warmup    chan struct{}
	stopDelay time.Duration
}

func (e *fakeEncoder) Run(ctx context.Context, _ io.Reader, _ *kyusu.Preset, audio kyusu.Audio, dir string) error {
	e.runs.Add(1)
	e.mu.Lock()
	e.audios = append(e.audios, audio)
	e.mu.Unlock()
	e.running.Add(1)
	defer e.running.Add(-1)
	if e.warmup != nil {
		select {
		case <-e.warmup:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := writeFileAtomically(filepath.Join(dir, "segment00000.ts"), []byte{0x47}); err != nil {
		return err
	}
	if err := writeFileAtomically(filepath.Join(dir, kyusu.PlaylistName), []byte(fakePlaylist)); err != nil {
		return err
	}
	<-ctx.Done()
	time.Sleep(e.stopDelay)

	return ctx.Err()
}

var _ kyusu.Encoder = new(fakeEncoder)

// writeFileAtomically は、一時ファイルに書いてから名前を変える。FFmpeg の hls_flags の temp_file と同じく、
// 書きかけのファイルを読ませない。os.WriteFile で直接書くと、作ったばかりの空のファイルを返してしまうことがある。
func writeFileAtomically(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// fakeOpener は、空のストリームを開くか、決まったエラーを返す。
type fakeOpener struct {
	err error
}

func (o *fakeOpener) OpenServiceStream(context.Context, int64, *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if o.err != nil {
		return nil, o.err
	}

	return io.NopCloser(strings.NewReader("")), nil
}

var _ kyusu.ServiceOpener = new(fakeOpener)

type liveFixture struct {
	live    *kyusu.Live
	clock   *clocktest.Fake
	server  *httptest.Server
	token   *kyusu.Token
	encoder *fakeEncoder
	workDir string
}

func newLiveFixture(t *testing.T, opener kyusu.ServiceOpener, encoder kyusu.Encoder) *liveFixture {
	t.Helper()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	workDir := t.TempDir()
	live, err := kyusu.NewLive(&kyusu.LiveOptions{
		Opener: opener, Encoder: encoder, WorkDir: workDir, Clock: clk, Logger: slog.New(slog.DiscardHandler), IdleTimeout: 0,
	})
	require.NoError(t, err)
	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	server := httptest.NewServer(live.Handler(token))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- live.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	fake, _ := encoder.(*fakeEncoder)

	return &liveFixture{live: live, clock: clk, server: server, token: token, encoder: fake, workDir: workDir}
}

// get は、トークンを付けて path を取得し、状態と本文を返す。
func (f *liveFixture) get(t *testing.T, path string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+path, nil)
	require.NoError(t, err)
	f.token.Authorize(req)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

// sessionDirs は、作業ディレクトリに残っているセッションのディレクトリの数を返す。
func (f *liveFixture) sessionDirs(t *testing.T) int {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(f.workDir, "live"))
	require.NoError(t, err)

	return len(entries)
}

// TestLiveSharesSession は、同じサービスを同じ画質で見る視聴者が、1 つのエンコーダーを共有することを確かめる (設計書 7.3)。
func TestLiveSharesSession(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t, &fakeOpener{err: nil}, new(fakeEncoder))
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			status, body := f.get(t, livePath+kyusu.PlaylistName)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, fakePlaylist, body)
		})
	}
	wg.Wait()
	status, body := f.get(t, livePath+"segment00000.ts")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "\x47", body)

	assert.Equal(t, int32(1), f.encoder.runs.Load(), "viewers share one encoder")
	// 別の画質と、別の音声は、別のエンコーダーにする。
	status, _ = f.get(t, kyusu.LivePathPrefix+"3273601024/720p/1/"+kyusu.PlaylistName)
	assert.Equal(t, http.StatusOK, status)
	status, _ = f.get(t, kyusu.LivePathPrefix+"3273601024/360p/2/"+kyusu.PlaylistName)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int32(3), f.encoder.runs.Load())
	f.encoder.mu.Lock()
	defer f.encoder.mu.Unlock()
	assert.Equal(t, []kyusu.Audio{kyusu.AudioFirst, kyusu.AudioFirst, kyusu.AudioSecond}, f.encoder.audios)
}

// TestLiveStopsIdleSession は、セグメントへのリクエストが 10 秒途絶えたら、エンコーダーを止め、HLS を消すことを確かめる。
// プレイリストへのリクエストだけでは、止めずにおく理由にならない。
func TestLiveStopsIdleSession(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t, &fakeOpener{err: nil}, new(fakeEncoder))
	status, _ := f.get(t, livePath+kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status)
	start := f.clock.Now()

	// 9 秒後にセグメントを要求したので、始めてから 15 秒の時点では、まだ止めない。
	f.clock.Advance(9 * time.Second)
	status, _ = f.get(t, livePath+"segment00000.ts")
	require.Equal(t, http.StatusOK, status)
	f.live.Reap(start.Add(15 * time.Second))
	assert.Never(t, func() bool { return f.encoder.running.Load() == 0 }, 200*time.Millisecond, 10*time.Millisecond)

	// プレイリストを要求しても、最後のセグメントの要求から 10 秒を過ぎたら止める。
	f.clock.Advance(5 * time.Second)
	status, _ = f.get(t, livePath+kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status)
	f.live.Reap(start.Add(20 * time.Second))
	assert.Eventually(t, func() bool { return f.encoder.running.Load() == 0 && f.sessionDirs(t) == 0 }, 5*time.Second, 10*time.Millisecond)
	status, _ = f.get(t, livePath+"segment00000.ts")
	assert.Equal(t, http.StatusNotFound, status, "segments of a stopped session are gone")
}

// TestLiveWaitsForSlowStart は、最初のセグメントができるまでに 10 秒以上かかっても、セッションを止めないことを確かめる。
func TestLiveWaitsForSlowStart(t *testing.T) {
	t.Parallel()

	encoder := &fakeEncoder{warmup: make(chan struct{})}
	f := newLiveFixture(t, &fakeOpener{err: nil}, encoder)
	start := f.clock.Now()
	type result struct {
		status int
		body   string
		err    error
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+livePath+kyusu.PlaylistName, nil)
	require.NoError(t, err)
	f.token.Authorize(req)
	done := make(chan result, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- result{status: 0, body: "", err: err}

			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		done <- result{status: resp.StatusCode, body: string(body), err: err}
	}()
	require.Eventually(t, func() bool { return encoder.runs.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	f.live.Reap(start.Add(15 * time.Second))
	close(encoder.warmup)
	got := <-done
	assert.Equal(t, result{status: http.StatusOK, body: fakePlaylist, err: nil}, got)
	assert.Equal(t, int32(1), encoder.running.Load(), "the session keeps running")
}

// TestLiveReplacesStoppingSession は、使われなくなって止めているセッションと同じ組の視聴者が来たら、
// 止まり終わるのを待たずに、新しいセッションを作ることを確かめる。
func TestLiveReplacesStoppingSession(t *testing.T) {
	t.Parallel()

	encoder := &fakeEncoder{stopDelay: 2 * time.Second}
	f := newLiveFixture(t, &fakeOpener{err: nil}, encoder)
	status, _ := f.get(t, livePath+kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status)

	f.live.Reap(f.clock.Now().Add(time.Minute))
	status, body := f.get(t, livePath+kyusu.PlaylistName)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, fakePlaylist, body)
	assert.Equal(t, int32(2), encoder.runs.Load())
}

// TestNewTokenRequiresSecret は、アプリの秘密キーがなければ、トークンを作らないことを確かめる。
func TestNewTokenRequiresSecret(t *testing.T) {
	t.Parallel()

	_, err := kyusu.NewToken("")
	require.ErrorIs(t, err, kyusu.ErrNoKey)
}

// TestLiveErrors は、配信できない理由に応じた状態を返すことを確かめる。
func TestLiveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opener     kyusu.ServiceOpener
		encoder    kyusu.Encoder
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name: "知らない画質", opener: &fakeOpener{err: nil}, encoder: new(fakeEncoder),
			path: kyusu.LivePathPrefix + "3273601024/4k/1/" + kyusu.PlaylistName, wantStatus: http.StatusNotFound, wantBody: "unknown preset",
		},
		{
			name: "知らない音声", opener: &fakeOpener{err: nil}, encoder: new(fakeEncoder),
			path: kyusu.LivePathPrefix + "3273601024/360p/3/" + kyusu.PlaylistName, wantStatus: http.StatusNotFound, wantBody: "unknown audio",
		},
		{
			name: "HLS でないファイル", opener: &fakeOpener{err: nil}, encoder: new(fakeEncoder),
			path: livePath + "..%2F..%2Fetc%2Fpasswd", wantStatus: http.StatusNotFound, wantBody: "404 page not found",
		},
		{
			name: "動いていないセッションのセグメント", opener: &fakeOpener{err: nil}, encoder: new(fakeEncoder),
			path: livePath + "segment00000.ts", wantStatus: http.StatusNotFound, wantBody: "the live session is not running",
		},
		{
			name: "チューナーサーバーを設定していない", opener: nil, encoder: new(fakeEncoder),
			path: livePath + kyusu.PlaylistName, wantStatus: http.StatusServiceUnavailable, wantBody: "tuner server is not configured",
		},
		{
			name: "FFmpeg がない", opener: &fakeOpener{err: nil}, encoder: nil,
			path: livePath + kyusu.PlaylistName, wantStatus: http.StatusServiceUnavailable, wantBody: "FFmpeg or tsreadex is not installed",
		},
		{
			name: "チューナーが空いていない", encoder: new(fakeEncoder),
			opener: &fakeOpener{err: &mirakurun.APIError{Method: http.MethodGet, URL: "", StatusCode: http.StatusServiceUnavailable, Reason: ""}},
			path:   livePath + kyusu.PlaylistName, wantStatus: http.StatusServiceUnavailable, wantBody: "no tuner is available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLiveFixture(t, tt.opener, tt.encoder)
			status, body := f.get(t, tt.path)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(body))
		})
	}
}

// TestLiveRequiresToken は、tenban のトークンのないリクエストを拒むことを確かめる。
func TestLiveRequiresToken(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t, &fakeOpener{err: nil}, new(fakeEncoder))
	other, err := kyusu.NewToken(keyMaterial + "!")
	require.NoError(t, err)
	for _, authorize := range []func(*http.Request){func(*http.Request) {}, other.Authorize} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+livePath+kyusu.PlaylistName, nil)
		require.NoError(t, err)
		authorize(req)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.Zero(t, f.encoder.runs.Load())
}

// TestLiveTranscodesTunerStream は、チューナーサーバーのストリームを、実際の tsreadex と FFmpeg で HLS にして配信し、
// セッションを止めるとチューナーを解放することを確かめる。FFmpeg か tsreadex がなければ飛ばす。
func TestLiveTranscodesTunerStream(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	fake := faketuner.New(faketuner.DefaultOptions(clk.Now(), loadTestTS(t)))
	upstream := httptest.NewServer(fake)
	t.Cleanup(upstream.Close)
	client, err := mirakurun.New(&mirakurun.Options{
		URL: upstream.URL, Username: "", Password: "", UserAgent: "", HTTPClient: nil, RequestTimeout: 0, DetectInterval: 0, Clock: clk,
	})
	require.NoError(t, err)
	f := newLiveFixture(t, client, transcoder)

	status, body := f.get(t, livePath+kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status, body)
	var segment string
	for line := range strings.Lines(body) {
		if strings.HasPrefix(line, "segment") {
			segment = strings.TrimSpace(line)

			break
		}
	}
	require.NotEmpty(t, segment)
	status, data := f.get(t, livePath+segment)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, data)
	assert.Equal(t, byte(0x47), data[0])

	users := func() int {
		n := 0
		for _, tuner := range fake.Tuners() {
			n += len(tuner.Users)
		}

		return n
	}
	assert.Equal(t, 1, users())
	f.live.Reap(f.clock.Now().Add(time.Minute))
	assert.Eventually(t, func() bool { return users() == 0 && f.sessionDirs(t) == 0 }, 10*time.Second, 50*time.Millisecond)
}
