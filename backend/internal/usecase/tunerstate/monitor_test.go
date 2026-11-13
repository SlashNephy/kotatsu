package tunerstate_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/tunerstate"
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func noGuard(context.Context, pgx.Tx) error { return nil }

// recordingHandler は、ログのメッセージを記録する。
type recordingHandler struct {
	slog.Handler

	mu       sync.Mutex
	messages []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, record.Message)

	return nil
}

func (h *recordingHandler) count(message string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := 0
	for _, m := range h.messages {
		if m == message {
			n++
		}
	}

	return n
}

type fixture struct {
	pool    *pgxpool.Pool
	fake    *faketuner.Server
	server  *httptest.Server
	clock   *clocktest.Fake
	logs    *recordingHandler
	monitor *tunerstate.Monitor
}

// newFixture は、チューナーサーバーの時計が OS の時計より serverAhead だけ進んでいる環境を用意する。
func newFixture(t *testing.T, serverAhead time.Duration) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	options := faketuner.DefaultOptions(testNow(), nil)
	options.Clock = clocktest.NewFake(testNow().Add(serverAhead))
	fake := faketuner.New(options)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	clk := clocktest.NewFake(testNow())
	logs := &recordingHandler{Handler: slog.DiscardHandler, mu: sync.Mutex{}, messages: []string{}}

	return &fixture{
		pool:    pool,
		fake:    fake,
		server:  server,
		clock:   clk,
		logs:    logs,
		monitor: tunerstate.NewMonitor(pool, client, clk, noGuard, slog.New(logs), nil),
	}
}

func (f *fixture) tuners(t *testing.T) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT name || ':' || array_to_string(types, '+') FROM tuners ORDER BY tuner_index")
	require.NoError(t, err)
	tuners, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return tuners
}

func (f *fixture) version(t *testing.T) int64 {
	t.Helper()

	var version int64
	err := f.pool.QueryRow(t.Context(), "SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", tunerstate.NotifyChannel).Scan(&version)
	require.NoError(t, err)

	return version
}

// TestCheckFollowsTunerConfiguration は、チューナーの構成が変わると DB の状態も変わり、変更が通知されることを確かめる。
func TestCheckFollowsTunerConfiguration(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	require.NoError(t, f.monitor.Check(t.Context()))
	assert.Equal(t, []string{"GR0:GR", "GR1:GR", "BSCS0:BS+CS"}, f.tuners(t))
	first := f.version(t)
	assert.Positive(t, first)

	// 構成が変わらなければ、通知しない。
	require.NoError(t, f.monitor.Check(t.Context()))
	assert.Equal(t, first, f.version(t))

	// 1 本で地上波と BS/CS を受信できる機器 (PX-MLT など) に置き換える。
	f.fake.SetTuners([]*faketuner.TunerConfig{{Name: "MLT0", Types: []string{"GR", "BS", "CS"}}})
	require.NoError(t, f.monitor.Check(t.Context()))
	assert.Equal(t, []string{"MLT0:GR+BS+CS"}, f.tuners(t))
	assert.Greater(t, f.version(t), first)
}

// TestRunRefreshesOnTrigger は、チューナーのイベントで Trigger されると、定期の取得を待たずに取得し直すことを確かめる。
func TestRunRefreshesOnTrigger(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- f.monitor.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	require.Eventually(t, func() bool { return len(f.tuners(t)) == 3 }, 10*time.Second, 20*time.Millisecond)

	f.fake.SetTuners([]*faketuner.TunerConfig{{Name: "GR0", Types: []string{"GR"}}})
	f.monitor.Trigger()
	require.Eventually(t, func() bool { return len(f.tuners(t)) == 1 }, 10*time.Second, 20*time.Millisecond)
}

// TestCheckWarnsClockSkew は、チューナーサーバーの時計のずれが閾値を超えると、警告をログに出し、ずれを状態に残すことを確かめる。
func TestCheckWarnsClockSkew(t *testing.T) {
	t.Parallel()

	const warning = "the clock of the tuner server differs from the OS clock; check NTP on both hosts"
	tests := []struct {
		name         string
		serverAhead  time.Duration
		wantSkewMs   int64
		wantWarnings int
	}{
		{name: "within the threshold", serverAhead: 10 * time.Second, wantSkewMs: 10_000, wantWarnings: 0},
		{name: "ahead", serverAhead: time.Minute, wantSkewMs: 60_000, wantWarnings: 1},
		{name: "behind", serverAhead: -time.Minute, wantSkewMs: -60_000, wantWarnings: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.serverAhead)
			// ずれ続けていても、警告は閾値を超えたときに 1 回だけ出す。
			require.NoError(t, f.monitor.Check(t.Context()))
			require.NoError(t, f.monitor.Check(t.Context()))

			var skew int64
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT clock_skew_ms FROM tuner_server_status").Scan(&skew))
			assert.Equal(t, tt.wantSkewMs, skew)
			assert.Equal(t, tt.wantWarnings, f.logs.count(warning))
		})
	}
}

// TestCheckRecordsDegradedWhileUnreachable は、接続できない間は縮退中として記録し、接続が戻ったら解除することを確かめる。
func TestCheckRecordsDegradedWhileUnreachable(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	require.NoError(t, f.monitor.Check(t.Context()))
	f.server.Close()

	var (
		since     *time.Time
		lastError string
	)
	status := func() {
		require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT degraded_since, last_error FROM tuner_server_status").Scan(&since, &lastError))
	}
	notified := func() int {
		var n int
		require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM notification_events WHERE kind = 'tuner_server_unreachable'").Scan(&n))

		return n
	}
	// 接続できている間は通知しない。
	assert.Zero(t, notified())

	require.NoError(t, f.monitor.Check(t.Context()))
	status()
	require.NotNil(t, since)
	assert.Equal(t, testNow(), since.UTC())
	assert.Contains(t, lastError, "connection refused")
	assert.Equal(t, 1, notified())
	// 接続できなかった間も、チューナーの構成は最後に分かったものを残す。
	assert.Len(t, f.tuners(t), 3)

	// 縮退が続いている間は、縮退し始めた日時を変えない。
	f.clock.Advance(time.Minute)
	require.NoError(t, f.monitor.Check(t.Context()))
	status()
	assert.Equal(t, testNow(), since.UTC())
	assert.Equal(t, 1, notified())
}

// dateless は、時計を返さないチューナーサーバーを再現する。Date ヘッダーをプロキシが取り除いた場合や、時計を返さない EDCB (#1532)。
type dateless struct {
	tunerstate.Upstream

	err error
}

func (d dateless) ServerDate(context.Context) (time.Time, error) {
	return time.Time{}, d.err
}

// TestCheckRecordsTunersWithoutDate は、時計が取れなくても縮退にせず、チューナーの構成を記録することを確かめる。
// 時計を返さない種類のサーバー (EDCB) では、確かめるたびに警告を出さない。
func TestCheckRecordsTunersWithoutDate(t *testing.T) {
	t.Parallel()

	const warning = "failed to read the clock of the tuner server"
	tests := []struct {
		name         string
		err          error
		wantWarnings int
	}{
		{name: "no Date header", err: errors.New("the tuner server returned no valid Date header"), wantWarnings: 1},
		{name: "clock unavailable", err: tunerserver.ErrClockUnavailable, wantWarnings: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 0)
			client, err := mirakurun.New(&mirakurun.Options{URL: f.server.URL})
			require.NoError(t, err)
			logs := &recordingHandler{Handler: slog.DiscardHandler, mu: sync.Mutex{}, messages: []string{}}
			monitor := tunerstate.NewMonitor(f.pool, dateless{Upstream: client, err: tt.err}, f.clock, noGuard, slog.New(logs), nil)
			require.NoError(t, monitor.Check(t.Context()))

			assert.Len(t, f.tuners(t), 3)
			var (
				since *time.Time
				skew  *int64
			)
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT degraded_since, clock_skew_ms FROM tuner_server_status").Scan(&since, &skew))
			assert.Nil(t, since)
			assert.Nil(t, skew)
			assert.Equal(t, tt.wantWarnings, logs.count(warning))
		})
	}
}

func TestCheckClearsDegradedAfterReconnect(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	_, err := f.pool.Exec(t.Context(), "INSERT INTO tuner_server_status (degraded_since, last_error, checked_at) VALUES ($1, 'connection refused', $1)", testNow().Add(-time.Hour))
	require.NoError(t, err)

	require.NoError(t, f.monitor.Check(t.Context()))
	var (
		since     *time.Time
		lastError string
	)
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT degraded_since, last_error FROM tuner_server_status").Scan(&since, &lastError))
	assert.Nil(t, since)
	assert.Empty(t, lastError)
}

func TestTruncateKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{name: "short enough", in: "接続できません", maxBytes: 100, want: "接続できません"},
		// 「接」は 3 バイトなので、4 バイトで切ると 2 文字目の途中になる。
		{name: "cut in the middle of a character", in: "接続", maxBytes: 4, want: "接"},
		{name: "ascii", in: "connection refused", maxBytes: 10, want: "connection"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tunerstate.Truncate(tt.in, tt.maxBytes))
		})
	}
}

// unidentified は、種類の判定に失敗するチューナーサーバーを再現する。
type unidentified struct {
	tunerstate.Upstream
}

func (unidentified) Identify(context.Context) (*tunerserver.Identity, error) {
	return nil, errors.New("the tuner server returned 500 for /api/version")
}

// TestCheckRecordsServerIdentity は、チューナーサーバーの種類と版を記録し、分からない間は最後に分かったものを残すことを確かめる (#1538)。
func TestCheckRecordsServerIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// then は、一度記録した後に、もう一度確かめる前の変化。
		then func(t *testing.T, f *fixture) *tunerstate.Monitor
	}{
		{
			name: "identified again",
			then: func(_ *testing.T, f *fixture) *tunerstate.Monitor { return f.monitor },
		},
		{
			name: "identification fails",
			then: func(t *testing.T, f *fixture) *tunerstate.Monitor {
				t.Helper()
				client, err := mirakurun.New(&mirakurun.Options{URL: f.server.URL})
				require.NoError(t, err)

				return tunerstate.NewMonitor(f.pool, unidentified{Upstream: client}, f.clock, noGuard, slog.New(slog.DiscardHandler), nil)
			},
		},
		{
			name: "unreachable",
			then: func(_ *testing.T, f *fixture) *tunerstate.Monitor {
				f.server.Close()

				return f.monitor
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 0)
			identity := func() (*string, *string) {
				var kind, version *string
				require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT server_kind, server_version FROM tuner_server_status").Scan(&kind, &version))

				return kind, version
			}

			require.NoError(t, f.monitor.Check(t.Context()))
			kind, version := identity()
			assert.Equal(t, new("mirakurun"), kind)
			assert.Equal(t, new("3.9.0-faketuner"), version)

			require.NoError(t, tt.then(t, f).Check(t.Context()))
			kind, version = identity()
			assert.Equal(t, new("mirakurun"), kind)
			assert.Equal(t, new("3.9.0-faketuner"), version)
		})
	}
}
