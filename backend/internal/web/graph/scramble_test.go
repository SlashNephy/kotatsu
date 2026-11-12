package graph_test

import (
	"context"
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
	scrambledQuery   = `subscription($service: ID!) { liveStreamScrambled(service: $service) { decidedAt } }`
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
	mux.Handle("GET /streams/services/{id}", streamapi.NewLiveHandler(client, slog.New(slog.DiscardHandler)).WithScramble(reporter))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// watchLive は、サービス serviceID のライブの中継を開き、時計を進めながら、中継が閉じるか timeout が過ぎるまで読む。
func watchLive(t *testing.T, server *httptest.Server, clk *clocktest.Fake, serviceID string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/streams/services/"+serviceID, nil)
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

// TestLiveStreamScrambled は、ブラウザ内での変換の中継が、スクランブルされたまま届くと決めたら、ストリームを閉じ、
// 理由が subscription でプレイヤーに届くことを確かめる (設計書 7.1、#1517)。
func TestLiveStreamScrambled(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	clk := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	server := scrambleRelay(t, f, clk)
	events := f.subscribeWith(t, scrambledQuery, map[string]any{varService: graph.GlobalID("Service", scrambledService)})
	others := f.subscribeWith(t, scrambledQuery, map[string]any{varService: graph.GlobalID("Service", clearService)})

	// 復号できている局は、閉じずに流し続ける。
	const watch = 2 * time.Second
	started := time.Now()
	watchLive(t, server, clk, clearService, watch)
	assert.GreaterOrEqual(t, time.Since(started), watch, "the decrypted stream must not be closed")

	// スクランブルされたまま届く局は、抑止時間を越えたら閉じ、理由が届く。
	started = time.Now()
	watchLive(t, server, clk, scrambledService, 10*time.Second)
	assert.Less(t, time.Since(started), 10*time.Second, "the scrambled stream must be closed")
	event := next(t, events)
	assert.Contains(t, string(event), "decidedAt")
	// ほかのサービスの購読には届かない。
	select {
	case got := <-others:
		t.Fatalf("an event arrived for another service: %s", got)
	case <-time.After(2 * time.Second):
	}
}

// TestLiveStreamScrambledSkipsEarlierVerdicts は、購読を始める前に決めた判定を送らないことを確かめる。
// 前に見た人の判定で、カードを直した後の視聴を止めないためである。
func TestLiveStreamScrambledSkipsEarlierVerdicts(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	clk := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	reporter := scramble.NewReporter(f.pool, clk, slog.New(slog.DiscardHandler))
	require.NoError(t, reporter.Report(t.Context(), 3273701032))
	events := f.subscribeWith(t, scrambledQuery, map[string]any{varService: graph.GlobalID("Service", scrambledService)})
	// ほかのサービスで決めると版数が上がるが、このサービスの前の判定は送らない。
	require.NoError(t, reporter.Report(t.Context(), 3273601024))
	select {
	case got := <-events:
		t.Fatalf("an earlier verdict arrived: %s", got)
	case <-time.After(1500 * time.Millisecond):
	}

	clk.Advance(time.Minute)
	require.NoError(t, reporter.Report(t.Context(), 3273701032))
	assert.JSONEq(t, `{"liveStreamScrambled":{"decidedAt":"2026-11-12T12:01:00Z"}}`, string(next(t, events)))
}

// TestLiveStreamScrambledWithSkewedClocks は、時計の遅れたホストが後から決めても、新しい判定として届くことを確かめる。
func TestLiveStreamScrambledWithSkewedClocks(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	ahead := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	behind := clocktest.NewFake(ahead.Now().Add(-time.Minute))
	require.NoError(t, scramble.NewReporter(f.pool, ahead, slog.New(slog.DiscardHandler)).Report(t.Context(), 3273701032))
	events := f.subscribeWith(t, scrambledQuery, map[string]any{varService: graph.GlobalID("Service", scrambledService)})

	require.NoError(t, scramble.NewReporter(f.pool, behind, slog.New(slog.DiscardHandler)).Report(t.Context(), 3273701032))
	assert.Contains(t, string(next(t, events)), "liveStreamScrambled")
}
