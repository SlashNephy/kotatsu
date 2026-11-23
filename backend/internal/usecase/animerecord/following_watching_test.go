package animerecord_test

import (
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// hana と hanaUsername は、モックのフォロー中のユーザーの名前とユーザー名。
const (
	hana         = "Hana"
	hanaUsername = "hana"
)

// TestFollowingWatchingWorks は、フォロー中のユーザーが「見てる」にした作品を、見たいリストと一緒に Annict と AniList から取り込み、
// これからの放送のある作品を、見ている人の多い順に返すことを確認する (#1968)。「見たい」などほかの状態の作品と、TID に対応しない作品は出さない。
// ほかのユーザーには見せず、連携を外すと、その記録先の分は消える。
func TestFollowingWatchingWorks(t *testing.T) {
	t.Parallel()

	f := newRecordFixture(t)
	otherService := faketuner.ServiceItemID(grNetworkID+1, 0x0408)
	// testTID (Annict の作品 10、AniList の 600) は 13:00 と、別のサービスの 12:30 に、TID 7000 (Annict の作品 21) は 14:00 に放送する。
	link := func(serviceID int64, start time.Time, tid int32) {
		t.Helper()
		_, err := f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
			SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID, start)
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', $3, 5, 5, NULL, false, false)`, serviceID, start, tid)
		require.NoError(t, err)
	}
	link(otherService, at(12, 30), testTID)
	link(serviceID(), at(13, 0), testTID)
	link(serviceID(), at(14, 0), 7000)
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 7000, 0, 21, NULL, NULL)`)
	require.NoError(t, err)

	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.alice, animerecord.DestinationAniList)
	f.connect(t, f.bob, animerecord.DestinationAnnict)
	// 1 ページに 1 人ずつ返し、ページをたどることも確認する。
	f.annict.setFollowing([]fakeFollowing{
		{Username: hanaUsername, Name: hana, WorkID: annictWorkID, Status: stateWatching, Title: titleA},
		{Username: "kai", Name: "Kai", WorkID: annictWorkID, Status: stateWatching, Title: titleA},
		{Username: "kai", Name: "Kai", WorkID: 21, Status: stateWatching, Title: titleC},
		{Username: "mio", Name: "Mio", WorkID: annictWorkID, Status: stateWannaWatch, Title: titleA},
		{Username: "mio", Name: "Mio", WorkID: 20, Status: stateWatching, Title: "対応のない作品"},
	}, 1)
	f.anilist.setFollowing([]fakeFollowing{
		{Username: "rin", WorkID: aniListID, Status: listCurrent, Title: titleWorkA},
		{Username: "sora", WorkID: aniListID, Status: listPlanning, Title: titleWorkA},
	}, 1)
	require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.alice.UserID))

	type want struct {
		tid      int32
		watchers []string
		events   int
	}
	check := func(t *testing.T, userID any, expected []want) {
		t.Helper()
		id := f.alice.UserID
		if userID != nil {
			id = f.bob.UserID
		}
		works, err := f.service.FollowingWatchingWorks(t.Context(), f.pool, id)
		require.NoError(t, err)
		got := make([]want, 0, len(works))
		for _, work := range works {
			watchers := make([]string, 0, len(work.Watchers))
			for _, watcher := range work.Watchers {
				watchers = append(watchers, watcher.Destination+":"+watcher.Name)
			}
			got = append(got, want{tid: work.TID, watchers: watchers, events: len(work.Broadcasts)})
		}
		assert.Equal(t, expected, got)
	}

	check(t, nil, []want{
		{tid: testTID, watchers: []string{"annict:Hana", "annict:Kai", "anilist:rin"}, events: 2},
		{tid: 7000, watchers: []string{"annict:Kai"}, events: 1},
	})
	// bob は取り込んでいないので、alice の取り込んだものは見えない。
	check(t, true, []want{})

	// 取り込んでから保持する期間 (24 時間) を過ぎたものは出さない。利用者の情報を、表示に要る範囲を超えて溜めないため。
	_, err = f.pool.Exec(t.Context(), `UPDATE anime_following_watching SET fetched_at = $1 WHERE user_id = $2`,
		f.clock.Now().Add(-animerecord.FollowingRetention-time.Minute), f.alice.UserID)
	require.NoError(t, err)
	check(t, nil, []want{})
	_, err = f.pool.Exec(t.Context(), `UPDATE anime_following_watching SET fetched_at = $1 WHERE user_id = $2`, f.clock.Now(), f.alice.UserID)
	require.NoError(t, err)

	// 連携を外すと、その記録先の分は消える。
	require.NoError(t, f.service.Disconnect(t.Context(), f.pool, audit.User(f.alice.UserID), f.alice.UserID, animerecord.DestinationAnnict))
	check(t, nil, []want{{tid: testTID, watchers: []string{"anilist:rin"}, events: 2}})
}

// TestImportFollowingWatchingKeepsWishlist は、フォロー中のユーザーが見ている作品を引けなくても、見たいリストの取り込みは成功とし、
// 前に取り込んだ見ている作品を残すことを確認する (#1968)。
func TestImportFollowingWatchingKeepsWishlist(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.annict.setList([]fakeListEntry{{ID: annictWorkID, Title: titleA, Status: stateWannaWatch}}, 0)
	f.annict.setFollowing([]fakeFollowing{{Username: hanaUsername, Name: hana, WorkID: annictWorkID, Status: stateWatching, Title: titleA}}, 0)
	require.NoError(t, f.service.ImportWishlist(t.Context(), f.pool, f.alice.UserID, animerecord.DestinationAnnict))

	f.annict.failNext(opFollowingWatching, 500, 1)
	require.NoError(t, f.service.ImportWishlist(t.Context(), f.pool, f.alice.UserID, animerecord.DestinationAnnict))

	var count int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM anime_following_watching WHERE user_id = $1`, f.alice.UserID).Scan(&count))
	assert.Equal(t, 1, count)
	assert.Equal(t, []string{wishlistRowA}, f.wishlistRows(t, f.alice.UserID))
}

// TestCreateFollowingWatchingRule は、フォロー中のユーザーが見ている作品から、見たいリストと同じ決まりでルールを作り、
// 作った後は予約済みとして、もう作らせないことを確認する (#1968)。
func TestCreateFollowingWatchingRule(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET name = '作品A #5「話」', name_normalized = '作品A #5「話」' WHERE service_id = $1 AND start_at = $2`, serviceID(), at(13, 0))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
		SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID(), at(13, 0))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
		VALUES ($1, $2, 0, 'syobocal', $3, 5, 5, NULL, false, false)`, serviceID(), at(13, 0), testTID)
	require.NoError(t, err)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.annict.setFollowing([]fakeFollowing{{Username: hanaUsername, Name: hana, WorkID: annictWorkID, Status: stateWatching, Title: titleA}}, 0)
	require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.alice.UserID))
	rules := rule.NewRules(f.clock, audit.NewRecorder(f.clock))

	created, err := f.service.CreateFollowingWatchingRule(t.Context(), f.pool, rules, audit.User(f.alice.UserID), f.alice.UserID, testTID)
	require.NoError(t, err)
	assert.Equal(t, titleA, created.Name)
	_, err = f.service.CreateFollowingWatchingRule(t.Context(), f.pool, rules, audit.User(f.alice.UserID), f.alice.UserID, testTID)
	require.ErrorIs(t, err, animerecord.ErrWishlistWorkCovered)
	_, err = f.service.CreateFollowingWatchingRule(t.Context(), f.pool, rules, audit.User(f.alice.UserID), f.alice.UserID, 7000)
	require.ErrorIs(t, err, animerecord.ErrWishlistWorkNotFound)
}

// TestImportFollowingWatchingRateLimited は、フォロー中のユーザーが見ている作品を取得するときに記録先が 429 を返したら、見たいリストは
// 取り込めたものとして残し、その記録先の残りの連携を、次の機会に回すことを確認する (#1968)。
func TestImportFollowingWatchingRateLimited(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.bob, animerecord.DestinationAnnict)
	f.annict.setList([]fakeListEntry{{ID: annictWorkID, Title: titleA, Status: stateWannaWatch}}, 0)
	f.annict.failNext(opFollowingWatching, http.StatusTooManyRequests, 1)

	require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))

	imported, checked := 0, 0
	for _, user := range []uuid.UUID{f.alice.UserID, f.bob.UserID} {
		at, importedAt, reason := f.importState(t, user, animerecord.DestinationAnnict)
		if at != nil {
			checked++
			assert.Nil(t, reason, "the wishlist itself was imported")
		}
		if importedAt != nil {
			imported++
		}
	}
	assert.Equal(t, 1, checked, "the other Annict connection waits for the next check")
	assert.Equal(t, 1, imported)
}

// TestFollowingWatchingWatchersAreUnique は、1 つの TID に同じ記録先の作品が複数対応していても、同じ人を 1 度だけ数えることを確認する (#1968)。
func TestFollowingWatchingWatchersAreUnique(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
		SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID(), at(13, 0))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
		VALUES ($1, $2, 0, 'syobocal', $3, 5, 5, NULL, false, false)`, serviceID(), at(13, 0), testTID)
	require.NoError(t, err)
	// testTID に、Annict の作品 10 (fixture) のほか、分割して登録された作品 11 も対応する。
	_, err = f.pool.Exec(t.Context(),
		`INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', $1, 1, 11, NULL, NULL)`, testTID)
	require.NoError(t, err)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.annict.setFollowing([]fakeFollowing{
		{Username: hanaUsername, Name: hana, WorkID: annictWorkID, Status: stateWatching, Title: titleA},
		{Username: hanaUsername, Name: hana, WorkID: 11, Status: stateWatching, Title: titleA},
	}, 0)
	require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.alice.UserID))

	works, err := f.service.FollowingWatchingWorks(t.Context(), f.pool, f.alice.UserID)
	require.NoError(t, err)
	require.Len(t, works, 1)
	assert.Len(t, works[0].Watchers, 1)
}
