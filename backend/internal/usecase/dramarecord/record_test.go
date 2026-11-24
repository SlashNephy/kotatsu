package dramarecord_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/simkltest"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
)

// thirdRecorded、thirdFailed、thirdPending は、シーズン 1 第 3 話の記録の状態。
const (
	thirdRecorded = "1:3:recorded"
	thirdFailed   = "1:3:failed"
	thirdPending  = "1:3:pending"
)

// TestRecord は、ドラマの番組を、見たと判定する割合まで見たら、結びついた話を Simkl の /sync/history に TMDB の ID で送り、
// 結果を見たユーザーに通知することを確認する (#1649)。
func TestRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(s *simkltest.Server)
		// last は、最後の試行かどうか。
		last         bool
		wantRetry    bool
		wantRecords  []string
		wantHistory  int
		wantNotified []string
	}{
		{
			name: "records the watched episode", prepare: func(*simkltest.Server) {},
			wantRecords: []string{thirdRecorded}, wantHistory: 1,
			wantNotified: []string{"drama_recorded:シーズン1 第3話:"},
		},
		{
			name: "an episode unknown to Simkl fails without retrying", prepare: func(s *simkltest.Server) { s.UnknownEpisodes = []string{"S1E3"} },
			wantRecords: []string{thirdFailed}, wantHistory: 0,
			wantNotified: []string{"drama_record_failed:シーズン1 第3話:Simkl に話が見つかりません"},
		},
		{
			name: "a series unknown to Simkl fails without retrying", prepare: func(s *simkltest.Server) { s.UnknownSeries = []string{"9001"} },
			wantRecords: []string{thirdFailed}, wantHistory: 0,
			wantNotified: []string{"drama_record_failed:シーズン1 第3話:Simkl に話が見つかりません"},
		},
		{
			name: "a server error is retried", prepare: func(s *simkltest.Server) { s.FailNext = http.StatusInternalServerError },
			wantRetry: true, wantRecords: []string{thirdPending}, wantHistory: 0, wantNotified: []string{},
		},
		{
			name: "a server error on the last attempt gives up", prepare: func(s *simkltest.Server) { s.FailNext = http.StatusServiceUnavailable },
			last: true, wantRecords: []string{thirdFailed}, wantHistory: 0,
			wantNotified: []string{"drama_record_failed:シーズン1 第3話:/sync/history returned 503"},
		},
		{
			name: "rate limiting is retried", prepare: func(s *simkltest.Server) { s.FailNext = http.StatusTooManyRequests },
			wantRetry: true, wantRecords: []string{thirdPending}, wantHistory: 0, wantNotified: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.link(t, 12, 0, series, 3)
			f.connect(t, f.alice)
			f.simkl.Lock()
			tt.prepare(f.simkl)
			f.simkl.Unlock()

			f.watch(t, f.alice, 0, at(12, 7))
			jobs := f.inserter.takeRecords()
			require.Len(t, jobs, 1)
			err := f.service.Record(t.Context(), f.pool, &jobs[0], tt.last)
			if tt.wantRetry {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRecords, f.records(t, f.alice.UserID))
			f.simkl.Lock()
			history := append([]*simkltest.Watched{}, f.simkl.History...)
			f.simkl.Unlock()
			require.Len(t, history, tt.wantHistory)
			if tt.wantHistory > 0 {
				assert.Equal(t, &simkltest.Watched{TMDBID: "9001", Season: 1, Episode: 3, WatchedAt: testNow().Format(time.RFC3339)}, history[0],
					"the episode is sent by its TMDB ID, with the time it was judged as watched")
			}
			assert.Equal(t, tt.wantNotified, f.notifications(t))
		})
	}
}

// TestRecordOnce は、同じユーザー、番組、話、記録先の組を 1 回だけ記録することを確認する。見続けても、見直しても送り直さない。
// 連携していないユーザーの番組は、記録しない。
func TestRecordOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.link(t, 12, 0, series, 3)
	f.connect(t, f.alice)

	f.watch(t, f.bob, 0, at(12, 10))
	assert.Empty(t, f.inserter.takeRecords(), "bob is not connected")
	f.watch(t, f.alice, 0, at(12, 3))
	assert.Empty(t, f.inserter.takeRecords(), "3 minutes do not reach 20%")
	f.watch(t, f.alice, 3, at(12, 7))
	f.runRecords(t, false)
	f.watch(t, f.alice, 7, at(12, 10))
	f.watch(t, f.alice, 0, at(12, 10))
	assert.Empty(t, f.inserter.takeRecords(), "watching more does not record again")
	f.simkl.Lock()
	sent := len(f.simkl.History)
	f.simkl.Unlock()
	assert.Equal(t, 1, sent)
	assert.Equal(t, []string{thirdRecorded}, f.records(t, f.alice.UserID))
}

// TestRecordRefreshesTokens は、アクセストークンが切れていたら更新してから送り、断られたら更新して 1 回だけ送り直すことを確認する。
// 更新のトークンが断られたら、連携し直すまで送らない。
func TestRecordRefreshesTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// advance は、連携してから記録するまでに進める時間。
		advance time.Duration
		// expire なら、Kotatsu の知る期限によらず、Simkl でアクセストークンを切らす。
		expire        bool
		rejectRefresh bool
		wantRecords   []string
		wantRefreshes int
		wantReauth    bool
	}{
		{name: "a fresh token is used as is", wantRecords: []string{thirdRecorded}, wantRefreshes: 0},
		{name: "a token close to its expiry is refreshed first", advance: 6*24*time.Hour + time.Hour, wantRecords: []string{thirdRecorded}, wantRefreshes: 1},
		{name: "a rejected token is refreshed and retried once", expire: true, wantRecords: []string{thirdRecorded}, wantRefreshes: 1},
		{
			name: "a rejected refresh token requires authorizing again", expire: true, rejectRefresh: true,
			wantRecords: []string{thirdFailed}, wantRefreshes: 0, wantReauth: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.link(t, 12, 0, series, 3)
			f.connect(t, f.alice)
			f.watch(t, f.alice, 0, at(12, 7))
			f.clock.Advance(tt.advance)
			if tt.expire {
				f.simkl.ExpireAccessTokens()
			}
			f.simkl.Lock()
			f.simkl.RejectRefresh = tt.rejectRefresh
			f.simkl.Unlock()
			f.runRecords(t, false)
			assert.Equal(t, tt.wantRecords, f.records(t, f.alice.UserID))
			f.simkl.Lock()
			refreshes := f.simkl.Refreshes
			f.simkl.Unlock()
			assert.Equal(t, tt.wantRefreshes, refreshes)
			connections, err := f.service.Connections(t.Context(), f.pool, f.alice.UserID)
			require.NoError(t, err)
			require.Len(t, connections, 1)
			assert.Equal(t, tt.wantReauth, connections[0].ReauthorizationRequired)
			if tt.wantReauth {
				assert.Equal(t, []string{"drama_record_failed:シーズン1 第3話:連携し直してください"}, f.notifications(t))
			}
		})
	}
}

// TestRecordedSeriesIsNotImported は、Kotatsu から記録したことで Simkl が作品を「見ている」に入れても、次の取り込みで、
// 見ただけの作品を見ている作品に足さないことを確認する (設計書 3.9 の、見ている作品に入れていない作品の話も見た話にする決まり)。
func TestRecordedSeriesIsNotImported(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.link(t, 12, 0, series, 3)
	f.connect(t, f.alice)
	require.NoError(t, f.service.ImportWatchlist(t.Context(), f.pool, f.alice.UserID, dramarecord.DestinationSimkl))
	f.clock.Advance(time.Minute)
	f.watch(t, f.alice, 0, at(12, 7))
	f.runRecords(t, false)
	f.clock.Advance(time.Minute)
	require.NoError(t, f.service.ImportWatchlist(t.Context(), f.pool, f.alice.UserID, dramarecord.DestinationSimkl))
	assert.Empty(t, f.entries(t, f.alice.UserID))
}
