package timeshift_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// setWhileWatching は、視聴している間だけ録る設定を変える。
func (e *env) setWhileWatching(t *testing.T, enabled bool) {
	t.Helper()

	_, err := e.pool.Exec(t.Context(), `
		INSERT INTO system_settings (id, recording_defaults, archive_retention_days, updated_at, timeshift_while_watching, timeshift_while_watching_retention_seconds)
		VALUES (1, '{}', 90, $1, $2, 1800)
		ON CONFLICT (id) DO UPDATE SET timeshift_while_watching = EXCLUDED.timeshift_while_watching`, e.clk.Now(), enabled)
	require.NoError(t, err)
}

// watch は、地上波の i 番目のサービスを今から視聴していることを記録し、視聴が新しく始まったかを返す。
func (e *env) watch(t *testing.T, service int) (bool, error) {
	t.Helper()

	return e.q.TouchTimeshiftWatch(t.Context(), sqlc.TouchTimeshiftWatchParams{
		ServiceID: grService(service), WatchingUntil: e.clk.Now().Add(timeshift.WatchLease), Now: e.clk.Now(),
	})
}

func (e *env) channelExists(t *testing.T, service int) bool {
	t.Helper()

	exists, err := e.q.TimeshiftChannelExists(t.Context(), grService(service))
	require.NoError(t, err)

	return exists
}

func TestSchedulerWatched(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	_, err := e.watch(t, 2)
	require.ErrorIs(t, err, pgx.ErrNoRows, "設定が切れていれば、視聴を記録しない")

	e.setWhileWatching(t, true)
	started, err := e.watch(t, 2)
	require.NoError(t, err)
	assert.True(t, started, "視聴が新しく始まった")
	started, err = e.watch(t, 2)
	require.NoError(t, err)
	assert.False(t, started, "視聴が続いている")

	runSteps(t, e, []step{
		{
			name: "視聴しているサービスの割り振りを書く", prepare: func() {},
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(2)},
		},
		{
			name: "視聴が続いている間は、割り振りを変えない", prepare: func() { e.clk.Advance(time.Minute) },
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(2)},
		},
		{
			name: "視聴が続いているとみなす時間を過ぎたら、録るのをやめる", prepare: func() { e.clk.Advance(2 * time.Minute) },
			wantCreated: 0, wantDeleted: 1, wantChanged: true,
			want: []int64{},
		},
	})
	assert.True(t, e.channelExists(t, 2), "残す時間の間は、バッファを再生できる")
	channels, err := e.q.ListTimeshiftChannels(t.Context())
	require.NoError(t, err)
	assert.Empty(t, channels, "視聴している間だけ録るチャンネルは、管理者の設定に出さない")

	started, err = e.watch(t, 2)
	require.NoError(t, err)
	assert.True(t, started, "残す時間の間に戻ったら、視聴が新しく始まったものとして知らせる")
	_, err = e.scheduler.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{grService(2)}, assignedServices(t, e.pool))

	e.clk.Advance(timeshift.WatchLease + timeshift.WatchKeep + time.Second)
	_, err = e.scheduler.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Empty(t, assignedServices(t, e.pool))
	assert.False(t, e.channelExists(t, 2), "残す時間を過ぎたら、チャンネルを消してバッファを破棄させる")
}

func TestSchedulerWatchedKeepsConfiguredChannels(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.setWhileWatching(t, true)
	e.configure(t, 0)
	_, err := e.watch(t, 0)
	require.NoError(t, err)
	_, err = e.watch(t, 1)
	require.NoError(t, err)

	_, err = e.scheduler.RunOnce(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{grService(0), grService(1)}, assignedServices(t, e.pool))

	e.setWhileWatching(t, false)
	_, err = e.scheduler.RunOnce(t.Context())
	require.NoError(t, err)
	assert.True(t, e.channelExists(t, 0), "管理者の設定したチャンネルは消さない")
	assert.False(t, e.channelExists(t, 1), "設定を切ったら、視聴している間だけ録るチャンネルを消す")
}

func TestWatchesTrack(t *testing.T) {
	t.Parallel()

	serviceRequest := func(t *testing.T, method string, principal *auth.Principal) *http.Request {
		t.Helper()

		request := httptest.NewRequestWithContext(t.Context(), method, "/streams/services/1", http.NoBody)
		request.SetPathValue("id", "1")
		if principal != nil {
			request = request.WithContext(auth.WithPrincipal(request.Context(), principal))
		}

		return request
	}

	tests := []struct {
		name      string
		method    string
		principal *auth.Principal
		want      bool
	}{
		{name: "ログインしたユーザーの配信を記録する", method: http.MethodGet, principal: new(auth.Principal), want: true},
		{name: "署名付き URL の外部プレイヤーは記録しない", method: http.MethodGet, principal: nil, want: false},
		{name: "HEAD は配信を流さないので記録しない", method: http.MethodHead, principal: new(auth.Principal), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			e.setWhileWatching(t, true)
			watches := timeshift.NewWatches(e.pool, e.clk, slog.New(slog.DiscardHandler))
			handler := watches.Track(func(string) (int64, bool) { return grService(1), true }, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

			handler.ServeHTTP(httptest.NewRecorder(), serviceRequest(t, tt.method, tt.principal))

			watched := func() bool {
				// assert.Never は、テストが終わった後にも別の goroutine で呼ぶことがあるので、t.Context の取り消しを引き継がない。
				rows, err := e.q.ListWatchedTimeshiftChannels(context.WithoutCancel(t.Context()), e.clk.Now())

				return err == nil && len(rows) == 1
			}
			if tt.want {
				assert.Eventually(t, watched, 5*time.Second, 10*time.Millisecond)
				assert.Positive(t, notifyVersion(t, e, timeshift.WatchNotifyChannel), "視聴が新しく始まったら、neko に知らせる")

				return
			}
			assert.Never(t, watched, 100*time.Millisecond, 10*time.Millisecond)
		})
	}
}

func notifyVersion(t *testing.T, e *env, channel string) int64 {
	t.Helper()

	var version int64
	require.Eventually(t, func() bool {
		err := e.pool.QueryRow(t.Context(), "SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", channel).Scan(&version)

		return err == nil && version > 0
	}, 5*time.Second, 10*time.Millisecond)

	return version
}
