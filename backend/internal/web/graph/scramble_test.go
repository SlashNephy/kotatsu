package graph_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/scramble"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

const (
	// clearService と scrambledService は、faketuner の「こたつテスト1」と「こたつテスト2」のサービス ID。
	// 「こたつテスト2」には、映像と音声をスクランブルしたままの TS を流させる。
	clearService     = "3273601024"
	scrambledService = "3273701032"
	scrambledQuery   = `subscription { viewerEvents { liveStreamScrambled { service decidedAt } } }`
)

// scrambleRelay は、faketuner を上流にした、ブラウザ内での変換のライブの中継を立てる。clk で、スクランブルの抑止時間を測る。
func scrambleRelay(t *testing.T, f *fixture, clk *clocktest.Fake) *httptest.Server {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)
	opts := faketuner.DefaultOptions(clk.Now(), data)
	for _, channel := range opts.Channels {
		for _, service := range channel.Services {
			if service.Name == serviceName2 {
				service.TS = faketuner.Scramble(data, 0x0111, 0x0112)
			}
		}
	}
	upstream := httptest.NewServer(faketuner.New(opts))
	t.Cleanup(upstream.Close)
	client, err := mirakurun.New(&mirakurun.Options{
		URL: upstream.URL, Username: "", Password: "", UserAgent: "", HTTPClient: httpclienttest.New(t), RequestTimeout: 0, DetectInterval: 0, Clock: clk,
	})
	require.NoError(t, err)
	mux := http.NewServeMux()
	reporter := scramble.NewReporter(f.pool, clk, slog.New(slog.DiscardHandler))
	mux.Handle("GET /internal/streams/services/{id}", streamapi.NewLiveHandler(client, slog.New(slog.DiscardHandler)).WithScramble(reporter))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// watchLive は、サービス serviceID のライブの中継を開き、時計を進めながら、中継が閉じるか timeout が過ぎるまで読む。
func watchLive(t *testing.T, server *httptest.Server, clk *clocktest.Fake, serviceID string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/internal/streams/services/"+serviceID, nil)
	require.NoError(t, err)
	resp, err := httpclienttest.New(t).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// faketuner は実時間で流すので、読む間に偽の時計を進め、抑止時間を越えさせる。
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clk.Advance(time.Second)
			}
		}
	}()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// subscribeScrambled は、viewerEvents を購読し、始めに届く今の版数の知らせを待ってから返す。
// 応答の頭が届いても、購読がサービスごとに最後に決めた日時を控え終えたとは限らない。控える前に決めた判定は送らないので、
// 待たずに決めると、テストの判定が、控えた日時に紛れて届かないことがある。
func subscribeScrambled(t *testing.T, f *fixture) <-chan json.RawMessage {
	t.Helper()

	events := f.subscribe(t, scrambledQuery)
	next(t, events)

	return events
}

// scrambledServices は、viewerEvents の知らせのうち、スクランブルされたまま届くと決めた知らせのサービスを、within の間に届いた順に返す。
func scrambledServices(t *testing.T, events <-chan json.RawMessage, within time.Duration) []string {
	t.Helper()

	services := []string{}
	deadline := time.After(within)
	for {
		select {
		case event, ok := <-events:
			require.True(t, ok, "the stream ended")
			var got struct {
				ViewerEvents struct {
					LiveStreamScrambled *struct {
						Service string `json:"service"`
					} `json:"liveStreamScrambled"`
				} `json:"viewerEvents"`
			}
			require.NoError(t, json.Unmarshal(event, &got))
			if got.ViewerEvents.LiveStreamScrambled != nil {
				services = append(services, got.ViewerEvents.LiveStreamScrambled.Service)
			}
		case <-deadline:
			return services
		}
	}
}

// TestLiveStreamScrambled は、ブラウザ内での変換の中継が、スクランブルされたまま届くと決めたら、ストリームを閉じ、
// 理由が viewerEvents でプレイヤーに届くことを確かめる (設計書 7.1、#1517)。
func TestLiveStreamScrambled(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	clk := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	server := scrambleRelay(t, f, clk)
	events := subscribeScrambled(t, f)

	// 復号できている局は、閉じずに流し続け、知らせも届かない。
	const watch = 2 * time.Second
	started := time.Now()
	watchLive(t, server, clk, clearService, watch)
	assert.GreaterOrEqual(t, time.Since(started), watch, "the decrypted stream must not be closed")

	// スクランブルされたまま届く局は、抑止時間を越えたら閉じ、その局の知らせだけが届く。
	started = time.Now()
	watchLive(t, server, clk, scrambledService, 10*time.Second)
	assert.Less(t, time.Since(started), 10*time.Second, "the scrambled stream must be closed")
	assert.Equal(t, []string{graph.GlobalID("Service", scrambledService)}, scrambledServices(t, events, 3*time.Second))
}

// TestLiveStreamScrambledSkipsEarlierVerdicts は、購読を始める前に決めた判定を送らないことを確かめる。
// 前に見た人の判定で、カードを直した後の視聴を止めないためである。
func TestLiveStreamScrambledSkipsEarlierVerdicts(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	clk := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	reporter := scramble.NewReporter(f.pool, clk, slog.New(slog.DiscardHandler))
	require.NoError(t, reporter.Report(t.Context(), 3273701032))
	events := subscribeScrambled(t, f)
	// ほかのサービスで決めると版数が上がるが、前に決めたサービスの判定は送らない。
	require.NoError(t, reporter.Report(t.Context(), 3273601024))
	assert.Equal(t, []string{graph.GlobalID("Service", clearService)}, scrambledServices(t, events, 1500*time.Millisecond))

	clk.Advance(time.Minute)
	require.NoError(t, reporter.Report(t.Context(), 3273701032))
	assert.Equal(t, []string{graph.GlobalID("Service", scrambledService)}, scrambledServices(t, events, 1500*time.Millisecond))
}

// TestLiveStreamScrambledWithSkewedClocks は、時計の遅れたホストが後から決めても、新しい判定として届くことを確かめる。
func TestLiveStreamScrambledWithSkewedClocks(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	ahead := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	behind := clocktest.NewFake(ahead.Now().Add(-time.Minute))
	require.NoError(t, scramble.NewReporter(f.pool, ahead, slog.New(slog.DiscardHandler)).Report(t.Context(), 3273701032))
	events := subscribeScrambled(t, f)

	require.NoError(t, scramble.NewReporter(f.pool, behind, slog.New(slog.DiscardHandler)).Report(t.Context(), 3273701032))
	assert.Equal(t, []string{graph.GlobalID("Service", scrambledService)}, scrambledServices(t, events, 1500*time.Millisecond))
}
