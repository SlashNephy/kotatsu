package dramarecord_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
)

const (
	// other と unlinked は、テストのほかの TMDB の作品。other は 12:30 の番組に結びつき、unlinked はどこにも結びつかない。
	other    int32 = 9003
	unlinked int32 = 9004
	// seriesWatching と otherWatching は、見ている作品の series と other。
	seriesWatching = "9001:watching"
	otherWatching  = "9003:watching"
)

// importOnce は、時計を進めてから、alice のウォッチリストを取り込む。
func (f *fixture) importOnce(t *testing.T) {
	t.Helper()
	f.clock.Advance(time.Minute)
	require.NoError(t, f.service.ImportWatchlist(t.Context(), f.pool, f.alice.UserID, dramarecord.DestinationSimkl))
}

// setItems は、モックの Simkl のウォッチリストを items にし、変わった日時を今にする。removed なら、作品を外したことにする。
func (f *fixture) setItems(items []*simklItem, removed bool) {
	now := f.clock.Now()
	f.simkl.Lock()
	defer f.simkl.Unlock()
	for _, item := range items {
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = now
		}
	}
	f.simkl.Items = items
	f.simkl.Activity = now.UTC().Format(time.RFC3339)
	if removed {
		f.simkl.RemovedActivity = f.simkl.Activity
	}
}

// allItemsCalls は、モックの Simkl が受け取った /sync/all-items/shows の数を返す。
func (f *fixture) allItemsCalls() int {
	f.simkl.Lock()
	defer f.simkl.Unlock()

	return len(slices.DeleteFunc(slices.Clone(f.simkl.Requests), func(r string) bool { return !strings.HasSuffix(r, "/sync/all-items/shows") }))
}

// TestImportWatchlist は、Simkl の「見ている」「見たい」の作品を、番組か録画に結びついたものだけ、見ている作品にコピーすることと、
// Simkl で状態が変わったときだけコピーし直すことを確認する (#1649)。
func TestImportWatchlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// steps は、取り込みの前に行う操作。順に、1 回ずつ取り込む。
		steps []func(t *testing.T, f *fixture)
		want  []string
	}{
		{
			name: "watching and plan-to-watch works linked to programs are added",
			steps: []func(t *testing.T, f *fixture){func(_ *testing.T, f *fixture) {
				f.setItems([]*simklItem{
					{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching},
					{SimklID: 2, TMDBID: other, Status: dramarecord.StatusPlanToWatch},
					{SimklID: 3, TMDBID: unlinked, Status: dramarecord.StatusWatching},
					{SimklID: 4, TMDBID: 0, Status: dramarecord.StatusWatching},
					{SimklID: 5, TMDBID: 9005, Status: dramarecord.StatusCompleted},
				}, false)
			}},
			want: []string{seriesWatching, "9003:wanna_watch"},
		},
		{
			name: "a work is added once it is linked to a program",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 3, TMDBID: unlinked, Status: dramarecord.StatusWatching}}, false)
				},
				func(t *testing.T, f *fixture) {
					t.Helper()
					f.link(t, 13, 0, unlinked, 1)
				},
			},
			want: []string{"9004:watching"},
		},
		{
			name: "a work removed in Kotatsu is not added again until it changes on Simkl",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}}, false)
				},
				func(t *testing.T, f *fixture) {
					t.Helper()
					require.NoError(t, dramalibrary.SetStatus(t.Context(), f.pool, f.alice.UserID, series, "", f.clock.Now()))
					// ほかの作品が変わって、差分を引き直しても、外した作品は足し直さない。
					f.setItems([]*simklItem{
						{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching, UpdatedAt: at(0, 0)}, {SimklID: 2, TMDBID: other, Status: dramarecord.StatusWatching},
					}, false)
				},
			},
			want: []string{otherWatching},
		},
		{
			name: "a status change on Simkl applies again",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusPlanToWatch}}, false)
				},
				func(t *testing.T, f *fixture) {
					t.Helper()
					require.NoError(t, dramalibrary.SetStatus(t.Context(), f.pool, f.alice.UserID, series, "", f.clock.Now()))
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}}, false)
				},
			},
			want: []string{seriesWatching},
		},
		{
			name: "finishing on Simkl removes the imported work",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}, {SimklID: 2, TMDBID: other, Status: dramarecord.StatusWatching}}, false)
				},
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusCompleted}, {SimklID: 2, TMDBID: other, Status: dramarecord.StatusWatching, UpdatedAt: at(0, 0)}}, false)
				},
			},
			want: []string{otherWatching},
		},
		{
			name: "removing from the Simkl watchlist removes the imported work",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}, {SimklID: 2, TMDBID: other, Status: dramarecord.StatusWatching}}, false)
				},
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 2, TMDBID: other, Status: dramarecord.StatusWatching, UpdatedAt: at(0, 0)}}, true)
				},
			},
			want: []string{otherWatching},
		},
		{
			name: "a work whose status was changed in Kotatsu is kept",
			steps: []func(t *testing.T, f *fixture){
				func(_ *testing.T, f *fixture) {
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}}, false)
				},
				func(t *testing.T, f *fixture) {
					t.Helper()
					require.NoError(t, dramalibrary.SetStatus(t.Context(), f.pool, f.alice.UserID, series, dramalibrary.StatusWannaWatch, f.clock.Now()))
					f.setItems([]*simklItem{}, true)
				},
			},
			want: []string{"9001:wanna_watch"},
		},
		{
			name: "a work added in Kotatsu first is kept when Simkl finishes it",
			steps: []func(t *testing.T, f *fixture){
				func(t *testing.T, f *fixture) {
					t.Helper()
					require.NoError(t, dramalibrary.SetStatus(t.Context(), f.pool, f.alice.UserID, series, dramalibrary.StatusWatching, f.clock.Now()))
					f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusCompleted}}, false)
				},
			},
			want: []string{seriesWatching},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.link(t, 12, 0, series, 3)
			f.link(t, 12, 30, other, 1)
			f.connect(t, f.alice)
			for _, step := range tt.steps {
				step(t, f)
				f.importOnce(t)
			}
			assert.Equal(t, tt.want, f.entries(t, f.alice.UserID))
			assert.Empty(t, f.entries(t, f.bob.UserID), "works are kept per user")
		})
	}
}

// TestImportWatchlistChecksActivities は、/sync/activities が変わっていなければ、ウォッチリストを取得しないことを確認する (Simkl の求め)。
func TestImportWatchlistChecksActivities(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.link(t, 12, 0, series, 3)
	f.connect(t, f.alice)
	f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}}, false)
	f.importOnce(t)
	assert.Equal(t, 1, f.allItemsCalls(), "the first import reads the whole watchlist")
	f.importOnce(t)
	assert.Equal(t, 1, f.allItemsCalls(), "nothing changed, so the watchlist is not read")
	f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching, UpdatedAt: at(0, 0)}}, true)
	f.importOnce(t)
	assert.Equal(t, 3, f.allItemsCalls(), "a removal reads the changes and every ID")
}

// TestImportWatchlistFailure は、取り込めなかったら、前に取り込んだ作品を残し、理由と問い合わせ直す日時を残すことを確認する。
// 429 なら Retry-After の長さだけ待つ。
func TestImportWatchlistFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		wantRetry time.Duration
	}{
		{name: "a server error is retried after an hour", status: http.StatusInternalServerError, wantRetry: time.Hour},
		{name: "rate limiting waits for Retry-After", status: http.StatusTooManyRequests, wantRetry: 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.link(t, 12, 0, series, 3)
			f.connect(t, f.alice)
			f.setItems([]*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching}}, false)
			f.importOnce(t)
			f.simkl.Lock()
			f.simkl.FailNext = tt.status
			f.simkl.Unlock()
			f.importOnce(t)
			assert.Equal(t, []string{seriesWatching}, f.entries(t, f.alice.UserID))
			var retryAt time.Time
			var reason string
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT watchlist_retry_at, watchlist_error FROM drama_record_connections`).Scan(&retryAt, &reason))
			assert.Equal(t, f.clock.Now().Add(tt.wantRetry), retryAt.UTC())
			assert.NotEmpty(t, reason)

			// 問い合わせ直す日時までは、取り込み直しを押しても、ジョブを入れない。
			enqueued, err := f.service.RequestWatchlistImport(t.Context(), f.pool, f.inserter, f.alice.UserID, true)
			require.NoError(t, err)
			assert.False(t, enqueued)
		})
	}
}

// TestRequestWatchlistImport は、画面を開いたときは、前に問い合わせてから 30 分たったときだけ取り込み、取り込み直しを押したときは
// 待たずに取り込むことを確認する。Simkl は、タイマーで問い合わせ続けることを認めていない。
func TestRequestWatchlistImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		advance time.Duration
		force   bool
		want    bool
	}{
		{name: "soon after the last check", advance: 10 * time.Minute, want: false},
		{name: "30 minutes after the last check", advance: dramarecord.WatchlistRefreshInterval, want: true},
		{name: "forced", advance: time.Minute, force: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			enqueued, err := f.service.RequestWatchlistImport(t.Context(), f.pool, f.inserter, f.alice.UserID, false)
			require.NoError(t, err)
			assert.False(t, enqueued, "not connected")
			f.connect(t, f.alice)
			enqueued, err = f.service.RequestWatchlistImport(t.Context(), f.pool, f.inserter, f.alice.UserID, false)
			require.NoError(t, err)
			assert.True(t, enqueued, "never checked")
			f.clock.Advance(tt.advance)
			enqueued, err = f.service.RequestWatchlistImport(t.Context(), f.pool, f.inserter, f.alice.UserID, tt.force)
			require.NoError(t, err)
			assert.Equal(t, tt.want, enqueued)
		})
	}
}
