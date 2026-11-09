package animerecord_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
)

const (
	listCompleted = "COMPLETED"
	stateWatched  = "WATCHED"
	// フォロー中のユーザーのユーザー名。
	userCurrent = "cur"
	userPlan    = "plan"
	userTwo     = "two"
	userOne     = "someone"
)

// followingRow は、視聴状況を比べやすい形にしたもの。
type followingRow struct {
	Username string
	Name     string
	Avatar   string
	Status   string
	Progress *int32
	Episodes *int32
	Score    *float64
	Notes    string
}

func followingRows(viewings []*animerecord.FollowingViewing) []followingRow {
	rows := make([]followingRow, 0, len(viewings))
	for _, v := range viewings {
		rows = append(rows, followingRow{
			Username: v.Username, Name: v.Name, Avatar: v.AvatarURL, Status: v.Status,
			Progress: v.Progress, Episodes: v.EpisodesCount, Score: v.Score, Notes: v.Notes,
		})
	}

	return rows
}

// TestImportFollowing は、記録先の視聴状況を、ページをたどって取得し、状態の順に、1 人 1 件で返すことを確かめる (#1372)。
func TestImportFollowing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		destination  string
		following    []fakeFollowing
		pageSize     int
		extraWork    bool
		want         []followingRow
		wantRequests int
	}{
		{
			name:        "Annict, following the pages and leaving out the people without a status",
			destination: animerecord.DestinationAnnict,
			following: []fakeFollowing{
				{Username: "wanna", Name: "見たい人", Avatar: "https://example.com/a.png", WorkID: annictWorkID, Status: stateWannaWatch},
				{Username: "none", Name: "状態なし", WorkID: 999_999, Status: stateWatching},
				{Username: "done", Name: "見た人", WorkID: annictWorkID, Status: stateWatched},
			},
			pageSize: 1,
			want: []followingRow{
				{Username: "done", Name: "見た人", Status: animerecord.FollowingWatched},
				{Username: "wanna", Name: "見たい人", Avatar: "https://example.com/a.png", Status: animerecord.FollowingWannaWatch},
			},
			wantRequests: 3,
		},
		{
			name:        "AniList, with the progress, the score and the notes",
			destination: animerecord.DestinationAniList,
			following: []fakeFollowing{
				{Username: userCurrent, WorkID: aniListID, Status: listCurrent, Progress: 3, Score: 7.5, Notes: "メモ", Episodes: 12},
				{Username: userPlan, WorkID: aniListID, Status: listPlanning},
				{Username: "re", WorkID: aniListID, Status: "REPEATING"},
			},
			pageSize: 2,
			want: []followingRow{
				{Username: userCurrent, Name: userCurrent, Status: animerecord.FollowingWatching, Progress: new(int32(3)), Episodes: new(int32(12)), Score: new(7.5), Notes: "メモ"},
				{Username: "re", Name: "re", Status: animerecord.FollowingRepeating},
				{Username: userPlan, Name: userPlan, Status: animerecord.FollowingWannaWatch},
			},
			wantRequests: 2,
		},
		{
			name:        "one person on two works of the TID is merged into the earliest status",
			destination: animerecord.DestinationAniList,
			following: []fakeFollowing{
				{Username: userTwo, WorkID: aniListID, Status: listPlanning},
				{Username: userTwo, WorkID: aniListID + 1, Status: listCompleted},
			},
			extraWork: true,
			want:      []followingRow{{Username: userTwo, Name: userTwo, Status: animerecord.FollowingWatched}},
			// 作品を 1 つずつ引く。
			wantRequests: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, tt.destination)
			if tt.extraWork {
				_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', $1, 1, NULL, $2, NULL)`, testTID, aniListID+1)
				require.NoError(t, err)
			}
			fake := f.annict
			if tt.destination == animerecord.DestinationAniList {
				fake = f.anilist
			}
			fake.setFollowing(tt.following, tt.pageSize)

			before, err := f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
			require.NoError(t, err)
			assert.True(t, before.Enabled)
			assert.True(t, before.NeedsRefresh)

			require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))

			got, err := f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
			require.NoError(t, err)
			assert.False(t, got.NeedsRefresh)
			assert.False(t, got.Failed)
			assert.Equal(t, tt.want, followingRows(got.Viewings))
			assert.Equal(t, tt.wantRequests, fake.followingCount())

			other, err := f.service.FollowingViewings(t.Context(), f.pool, f.bob.UserID, testTID)
			require.NoError(t, err)
			assert.False(t, other.Enabled, "a user who is not connected cannot see it")
			assert.Empty(t, other.Viewings)
		})
	}
}

// TestFollowingAvailability は、管理者が連携を無効にしたときと、作品を引けないときに、出さないことを確かめる (#1372)。
func TestFollowingAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tid     int32
		linking bool
	}{
		{name: "anime linking is disabled", tid: testTID, linking: false},
		{name: "the work is not mapped", tid: 999_999, linking: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, animerecord.DestinationAnnict)
			setLinking(t, f, tt.linking)

			got, err := f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, tt.tid)
			require.NoError(t, err)
			assert.False(t, got.Enabled)
			assert.False(t, got.NeedsRefresh)

			enqueued, err := f.service.RefreshFollowing(t.Context(), f.pool, f.inserter, f.alice.UserID, tt.tid)
			require.NoError(t, err)
			assert.False(t, enqueued)
			require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, tt.tid))
			assert.Zero(t, f.annict.followingCount())
		})
	}
}

// TestRefreshFollowing は、問い合わせる時期が来たときだけジョブを入れることを確かめる (#1372)。
func TestRefreshFollowing(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)

	enqueued, err := f.service.RefreshFollowing(t.Context(), f.pool, f.inserter, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.True(t, enqueued)
	f.inserter.mu.Lock()
	require.Len(t, f.inserter.followings, 1)
	assert.Equal(t, animerecord.FollowingArgs{UserID: f.alice.UserID, TID: testTID}, f.inserter.followings[0])
	f.inserter.mu.Unlock()

	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))
	enqueued, err = f.service.RefreshFollowing(t.Context(), f.pool, f.inserter, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.False(t, enqueued, "a fresh result is not fetched again")

	f.clock.Advance(animerecord.FollowingRefreshInterval)
	enqueued, err = f.service.RefreshFollowing(t.Context(), f.pool, f.inserter, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.True(t, enqueued)
}

// TestImportFollowingFailure は、取得できなければ、前の視聴状況を残し、問い合わせ直す日時まで待つことを確かめる (#1372)。
func TestImportFollowingFailure(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.annict.setFollowing([]fakeFollowing{{Username: userOne, Name: "人", WorkID: annictWorkID, Status: stateWatching}}, 0)
	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))

	f.clock.Advance(animerecord.FollowingRefreshInterval)
	f.annict.setFollowing([]fakeFollowing{}, 0)
	f.annict.failNext(opFollowing, http.StatusInternalServerError, 2)
	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))

	got, err := f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.True(t, got.Failed)
	assert.False(t, got.NeedsRefresh, "it waits until the retry time")
	assert.Len(t, got.Viewings, 1, "the previous result stays")

	f.clock.Advance(animerecord.WishlistRetryInterval)
	got, err = f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.True(t, got.NeedsRefresh)

	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))
	f.clock.Advance(animerecord.WishlistRetryInterval)
	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))
	got, err = f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.False(t, got.Failed)
	assert.Empty(t, got.Viewings, "a successful fetch replaces the previous result")
}

// TestImportFollowingRateLimited は、429 を返した記録先には、示された長さの間、ほかのユーザーの分も問い合わせないことを確かめる (#1372)。
func TestImportFollowingRateLimited(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.bob, animerecord.DestinationAnnict)
	f.annict.mu.Lock()
	f.annict.retryAfter = "60"
	f.annict.mu.Unlock()
	f.annict.failNext(opFollowing, http.StatusTooManyRequests, 1)

	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))
	require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.bob.UserID, testTID))

	assert.Zero(t, f.annict.followingCount(), "the rate limited request is not answered and the next one is not sent")
	got, err := f.service.FollowingViewings(t.Context(), f.pool, f.bob.UserID, testTID)
	require.NoError(t, err)
	assert.True(t, got.Failed)
}

// TestFollowingRetention は、保持する期間を過ぎた視聴状況を返さず消すことと、連携を外すと消えることを確かめる (#1372)。
func TestFollowingRetention(t *testing.T) {
	t.Parallel()

	count := func(t *testing.T, f *recordFixture) int {
		t.Helper()
		var rows int
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM anime_following_viewings) + (SELECT count(*) FROM anime_following_fetches)`).Scan(&rows))

		return rows
	}
	prepare := func(t *testing.T) *recordFixture {
		t.Helper()
		f := newRecordFixture(t)
		f.connect(t, f.alice, animerecord.DestinationAnnict)
		f.annict.setFollowing([]fakeFollowing{{Username: userOne, Name: "人", WorkID: annictWorkID, Status: stateWatching}}, 0)
		require.NoError(t, f.service.ImportFollowing(t.Context(), f.pool, f.alice.UserID, testTID))
		require.Equal(t, 2, count(t, f))

		return f
	}

	t.Run("expired rows are hidden and pruned", func(t *testing.T) {
		t.Parallel()
		f := prepare(t)
		f.clock.Advance(animerecord.FollowingRetention + time.Minute)

		got, err := f.service.FollowingViewings(t.Context(), f.pool, f.alice.UserID, testTID)
		require.NoError(t, err)
		assert.Empty(t, got.Viewings, "hidden before the prune job runs")

		require.NoError(t, animerecord.NewPruneWorker(f.service, f.pool).Work(t.Context(), nil))
		assert.Zero(t, count(t, f))
	})

	t.Run("disconnecting removes the rows", func(t *testing.T) {
		t.Parallel()
		f := prepare(t)
		_, err := f.pool.Exec(t.Context(), `DELETE FROM anime_record_connections WHERE user_id = $1`, f.alice.UserID)
		require.NoError(t, err)
		assert.Zero(t, count(t, f))
	})
}
