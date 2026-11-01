package animerecord_test

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// 候補の選び方のテストに使うサービス。main と sub は、同じネットワークで同じイベントを共有する (メインチャンネルとサブチャンネル)。
const (
	titleA          = "作品A"
	titleB          = "作品B"
	titleC          = "作品C"
	stateWannaWatch = "WANNA_WATCH"
	listCurrent     = "CURRENT"
	stateWatching   = "WATCHING"
)

const (
	serviceMain  int64 = 100_101
	serviceSub   int64 = 100_102
	serviceOther int64 = 200_101
)

// broadcast は、候補の選び方のテストの番組。
func broadcast(tid int32, serviceID, eventID int64, start time.Time, name, related string) sqlc.ListAnimeWishlistProgramsRow {
	return sqlc.ListAnimeWishlistProgramsRow{
		SyobocalTid: tid, ServiceID: serviceID, EventID: eventID, StartAt: start, EndAt: start.Add(30 * time.Minute), Name: name,
		RelatedItems: []byte(related),
	}
}

// pickedBroadcast は、候補の放送を比べやすい形にしたもの。
type pickedBroadcast struct {
	ServiceID int64
	EventID   int64
	Preferred bool
}

type pickedWork struct {
	TID        int32
	Title      string
	Broadcasts []pickedBroadcast
	Reserved   int
	Covered    bool
}

func picked(works []*animerecord.WishlistWork) []pickedWork {
	result := make([]pickedWork, 0, len(works))
	for _, work := range works {
		broadcasts := make([]pickedBroadcast, 0, len(work.Broadcasts))
		for _, b := range work.Broadcasts {
			broadcasts = append(broadcasts, pickedBroadcast{ServiceID: b.ServiceID, EventID: b.EventID, Preferred: b.Preferred})
		}
		result = append(result, pickedWork{TID: work.TID, Title: work.Title, Broadcasts: broadcasts, Reserved: work.ReservedBroadcasts, Covered: work.Covered})
	}

	return result
}

// TestPickWishlistWorks は、見たいリストの作品の放送から、候補を選ぶ規則を確かめる (#928)。
// 予約のある放送を外し、同じイベントを共有する放送を 1 つにし、最初の放送のサービスを録るのに選ぶ。
func TestPickWishlistWorks(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	hour := func(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }
	sources := map[int32][]*animerecord.WishlistSource{
		1: {{Destination: animerecord.DestinationAnnict, WorkID: 10, Status: animerecord.WishlistWannaWatch, Title: titleA}},
		2: {{Destination: animerecord.DestinationAniList, WorkID: 20, Status: animerecord.WishlistWatching, Title: ""}},
	}
	// sharedWithSub は、メインチャンネルの番組 (イベント 5) が、サブチャンネルと共有していることを表す。sharedWithMain はその逆。
	const sharedWithSub = `[{"type":"shared","serviceId":102,"eventId":5}]`
	const sharedWithMain = `[{"type":"shared","serviceId":101,"eventId":5}]`

	tests := []struct {
		name     string
		programs []sqlc.ListAnimeWishlistProgramsRow
		reserved [][2]int64
		// rules は、今あるルールのキーワードとサービス。
		rules map[string][]int64
		want  []pickedWork
	}{
		{
			name: "the service of the earliest broadcast is preferred",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceMain, 1, hour(22), "作品A #2", ""),
				broadcast(1, serviceOther, 1, hour(20), "作品A #2", ""),
				broadcast(1, serviceOther, 2, hour(20+24*7-1), "作品A #3", ""),
			},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 0, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceOther, EventID: 1, Preferred: true},
				{ServiceID: serviceMain, EventID: 1, Preferred: false},
				{ServiceID: serviceOther, EventID: 2, Preferred: true},
			}}},
		},
		{
			name: "at the same time, the smaller service is preferred",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceOther, 1, hour(20), "作品A #2", ""),
				broadcast(1, serviceMain, 1, hour(20), "作品A #2", ""),
			},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 0, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceMain, EventID: 1, Preferred: true},
				{ServiceID: serviceOther, EventID: 1, Preferred: false},
			}}},
		},
		{
			name: "a work with a reserved broadcast is covered, and no other service is promoted",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceOther, 1, hour(20), "作品A #2", ""),
				broadcast(1, serviceMain, 1, hour(22), "作品A #2", ""),
			},
			reserved: [][2]int64{{serviceOther, 1}},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 1, Covered: true, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceMain, EventID: 1, Preferred: false},
			}}},
		},
		{
			name: "a broadcast sharing the event with a reserved program is excluded",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceSub, 5, hour(20), "作品A #2", sharedWithMain),
				broadcast(1, serviceOther, 1, hour(22), "作品A #2", ""),
			},
			reserved: [][2]int64{{serviceMain, 5}},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 1, Covered: true, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceOther, EventID: 1, Preferred: false},
			}}},
		},
		{
			name: "broadcasts sharing an event become the one on the smallest service",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceSub, 5, hour(20), "作品A #2", sharedWithMain),
				broadcast(1, serviceMain, 5, hour(20), "作品A #2", sharedWithSub),
			},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 0, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceMain, EventID: 5, Preferred: true},
			}}},
		},
		{
			name: "a work with a rule of the same keyword and service is covered even before any reservation",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceOther, 1, hour(20), "[新]作品A #2「話」", ""),
				broadcast(1, serviceMain, 1, hour(22), "作品A #2「話」", ""),
			},
			// 全角で書いたキーワードも、照合と同じく正規化して比べる。
			rules: map[string][]int64{"作品Ａ": {serviceOther}},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 0, Covered: true, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceOther, EventID: 1, Preferred: false},
				{ServiceID: serviceMain, EventID: 1, Preferred: false},
			}}},
		},
		{
			name: "a rule of another keyword or another service set does not cover the work",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceOther, 1, hour(20), "作品A #2「話」", ""),
			},
			rules: map[string][]int64{"作品A": {serviceMain}, "ほかの作品": {serviceOther}, titleB: {serviceOther, serviceMain}},
			want: []pickedWork{{TID: 1, Title: titleA, Reserved: 0, Covered: false, Broadcasts: []pickedBroadcast{
				{ServiceID: serviceOther, EventID: 1, Preferred: true},
			}}},
		},
		{
			name: "a work whose broadcasts are all reserved is not a candidate",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceMain, 1, hour(20), "作品A #2", ""),
			},
			reserved: [][2]int64{{serviceMain, 1}},
			want:     []pickedWork{},
		},
		{
			name: "works are ordered by their first broadcast, and a work without a listed title is named from the program",
			programs: []sqlc.ListAnimeWishlistProgramsRow{
				broadcast(1, serviceMain, 1, hour(22), "作品A #2", ""),
				broadcast(2, serviceMain, 2, hour(21), "[新]作品B #1「始まり」", ""),
				broadcast(3, serviceMain, 3, hour(20), "見たいリストにない作品 #1", ""),
			},
			want: []pickedWork{
				{TID: 2, Title: titleB, Reserved: 0, Broadcasts: []pickedBroadcast{{ServiceID: serviceMain, EventID: 2, Preferred: true}}},
				{TID: 1, Title: titleA, Reserved: 0, Broadcasts: []pickedBroadcast{{ServiceID: serviceMain, EventID: 1, Preferred: true}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, picked(animerecord.PickWishlistWorks(sources, tt.programs, tt.reserved, tt.rules)))
		})
	}
}

// TestWishlistRuleInput は、候補の作品から作るルールが、最初の放送の作品名とサービスで、ほかの話にも一致することを確かめる (#928)。
// 番組名の話数までキーワードに入れると、その話にしか一致しないので、話数は入れない。
func TestWishlistRuleInput(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC)
	defaults := &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: new(7), FollowEventRelay: false}
	program := func(serviceID int64, name string) *match.Program {
		return match.NewProgram(&match.ProgramInput{
			ServiceID: serviceID, SharedFrom: 0, ChannelType: "GR", Name: name, Description: "", Extended: map[string]string{}, Genres: []*match.Genre{},
			StartAt: start.Add(7 * 24 * time.Hour), Duration: 30 * time.Minute, IsFree: true,
		})
	}

	tests := []struct {
		name        string
		first       string
		wantKeyword string
		// matches と misses は、作ったルールが一致する番組と、一致しない番組。
		matches []*match.Program
		misses  []*match.Program
	}{
		{
			name:        "the title before the episode number",
			first:       "[新]作品A #1「始まり」[字]",
			wantKeyword: titleA,
			matches:     []*match.Program{program(serviceMain, "作品A #2「次の話」"), program(serviceMain, "作品A　第3話")},
			misses:      []*match.Program{program(serviceOther, "作品A #2「次の話」"), program(serviceMain, "ほかの作品 #2")},
		},
		{
			name:        "a name without an episode number, without the marks",
			first:       "[字]劇場版 作品B [デ]",
			wantKeyword: "劇場版 作品B",
			matches:     []*match.Program{program(serviceMain, "劇場版 作品B")},
			misses:      []*match.Program{program(serviceMain, titleB)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			work := &animerecord.WishlistWork{
				TID: 1, Title: "作品の名前", Sources: []*animerecord.WishlistSource{}, ReservedBroadcasts: 0,
				Broadcasts: []*animerecord.WishlistBroadcast{
					{ServiceID: serviceMain, EventID: 1, StartAt: start, Name: tt.first, Preferred: true},
					{ServiceID: serviceOther, EventID: 1, StartAt: start.Add(time.Hour), Name: tt.first, Preferred: false},
				},
			}

			in := animerecord.WishlistRuleInput(work, defaults)
			assert.Equal(t, "作品の名前", in.Name)
			assert.True(t, in.Enabled)
			assert.True(t, in.AvoidDuplicates, "the rule starts from the rule defaults")
			assert.Equal(t, new(7), in.DuplicateWindowDays)
			assert.False(t, in.FollowEventRelay)
			require.NoError(t, in.Conditions.Validate())
			require.Len(t, in.Conditions.Keywords, 1)
			assert.Equal(t, tt.wantKeyword, in.Conditions.Keywords[0].Text)
			assert.Equal(t, []int64{serviceMain}, in.Conditions.ServiceIDs, "the service of the first broadcast")
			matcher, err := match.Compile(in.Conditions, time.UTC)
			require.NoError(t, err)
			for _, p := range tt.matches {
				assert.True(t, matcher.Match(p), p.Name)
			}
			for _, p := range tt.misses {
				assert.False(t, matcher.Match(p), p.Name)
			}
		})
	}
}

// wishlistRows は、ユーザーの見たいリストの作品を「記録先:作品 ID:状態:作品名」の形で返す。
func (f *recordFixture) wishlistRows(t *testing.T, userID uuid.UUID) []string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT destination || ':' || work_id || ':' || status || ':' || title FROM anime_wishlist_entries WHERE user_id = $1 ORDER BY destination, work_id`, userID)
	require.NoError(t, err)
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var row string
		require.NoError(t, rows.Scan(&row))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())

	return result
}

// importState は、連携の見たいリストを取り込んだ様子。
func (f *recordFixture) importState(t *testing.T, userID uuid.UUID, name string) (*time.Time, *time.Time, *string) {
	t.Helper()
	var checked, imported *time.Time
	var reason *string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		`SELECT wishlist_checked_at, wishlist_imported_at, wishlist_error FROM anime_record_connections WHERE user_id = $1 AND destination = $2`,
		userID, name).Scan(&checked, &imported, &reason))

	return checked, imported, reason
}

// TestImportWishlist は、Annict と AniList の見たいリストを、ページをたどって取り込み、取り込み直すたびに入れ替えることを確かめる (#928)。
// 取り込めなければ、前に取り込んだ作品を残して、理由を残す。本物の Annict と AniList には問い合わせない。
func TestImportWishlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destination string
		// prepare は、取り込む前に、偽の記録先を整える。
		prepare      func(t *testing.T, f *recordFixture, fake *fakeService)
		wantErr      error
		wantRows     []string
		wantRequests int
		wantError    bool
	}{
		{
			name:        "Annict, following the pages",
			destination: animerecord.DestinationAnnict,
			prepare: func(t *testing.T, _ *recordFixture, fake *fakeService) {
				t.Helper()
				fake.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}, {ID: 11, Title: titleB, Status: stateWatching}, {ID: 12, Title: titleC, Status: stateWatching}}, 2)
			},
			wantRows:     []string{"annict:10:wanna_watch:作品A", "annict:11:watching:作品B", "annict:12:watching:作品C"},
			wantRequests: 2,
		},
		{
			name:        "AniList, following the chunks and preferring the native title",
			destination: animerecord.DestinationAniList,
			prepare: func(t *testing.T, _ *recordFixture, fake *fakeService) {
				t.Helper()
				fake.setList([]fakeListEntry{{ID: 600, Title: titleA, Status: "PLANNING"}, {ID: 601, Title: titleB, Status: listCurrent}, {ID: 602, Title: titleC, Status: "PLANNING"}}, 2)
			},
			wantRows:     []string{"anilist:600:wanna_watch:作品A", "anilist:601:watching:作品B", "anilist:602:wanna_watch:作品C"},
			wantRequests: 2,
		},
		{
			name:        "a work removed from the list disappears",
			destination: animerecord.DestinationAnnict,
			prepare: func(t *testing.T, f *recordFixture, fake *fakeService) {
				t.Helper()
				fake.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}, {ID: 11, Title: titleB, Status: stateWatching}}, 0)
				require.NoError(t, f.service.ImportWishlist(t.Context(), f.pool, f.alice.UserID, animerecord.DestinationAnnict))
				fake.setList([]fakeListEntry{{ID: 11, Title: titleB, Status: stateWatching}}, 0)
			},
			wantRows:     []string{"annict:11:watching:作品B"},
			wantRequests: 2,
		},
		{
			name:        "rate limited keeps the previous works",
			destination: animerecord.DestinationAnnict,
			prepare: func(t *testing.T, f *recordFixture, fake *fakeService) {
				t.Helper()
				fake.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}}, 0)
				require.NoError(t, f.service.ImportWishlist(t.Context(), f.pool, f.alice.UserID, animerecord.DestinationAnnict))
				fake.setList([]fakeListEntry{}, 0)
				fake.failNext(opLibrary, http.StatusTooManyRequests, 1)
			},
			wantErr:      animerecord.ErrRateLimited,
			wantRows:     []string{"annict:10:wanna_watch:作品A"},
			wantRequests: 1,
			wantError:    true,
		},
		{
			name:        "nothing is fetched while anime linking is disabled",
			destination: animerecord.DestinationAnnict,
			prepare: func(t *testing.T, f *recordFixture, fake *fakeService) {
				t.Helper()
				fake.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}}, 0)
				setLinking(t, f, false)
			},
			wantRows:     []string{},
			wantRequests: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, tt.destination)
			fake := f.annict
			if tt.destination == animerecord.DestinationAniList {
				fake = f.anilist
			}
			tt.prepare(t, f, fake)

			err := f.service.ImportWishlist(t.Context(), f.pool, f.alice.UserID, tt.destination)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRows, f.wishlistRows(t, f.alice.UserID))
			assert.Equal(t, tt.wantRequests, fake.requests())
			checked, imported, reason := f.importState(t, f.alice.UserID, tt.destination)
			if tt.wantRequests == 0 {
				assert.Nil(t, checked)

				return
			}
			assert.NotNil(t, checked)
			assert.NotNil(t, imported)
			assert.Equal(t, tt.wantError, reason != nil)
		})
	}
}

// setLinking は、管理者がアニメの結びつきを有効か無効にする。
func setLinking(t *testing.T, f *recordFixture, enabled bool) {
	t.Helper()
	_, err := settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.User(f.bob.UserID), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil, DataBroadcastIPEnabled: nil,
		AnimeLinkingEnabled: new(enabled), DRCSReplacements: nil,
	})
	require.NoError(t, err)
}

// TestConnectEnqueuesWishlistImport は、連携したら、定期ジョブを待たずに、そのユーザーの見たいリストを取り込むジョブを入れることを確かめる (#928)。
func TestConnectEnqueuesWishlistImport(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)

	f.connect(t, f.alice, animerecord.DestinationAnnict)

	f.inserter.mu.Lock()
	defer f.inserter.mu.Unlock()
	require.Len(t, f.inserter.wishlists, 1)
	assert.Equal(t, &f.alice.UserID, f.inserter.wishlists[0].UserID)
}

// TestImportDueWishlists は、取り込み直す時期が来た連携だけを取り込み、429 を返した記録先の残りの連携は次の機会に回すことを確かめる (#928)。
func TestImportDueWishlists(t *testing.T) {
	t.Parallel()

	t.Run("only connections that are due", func(t *testing.T) {
		t.Parallel()
		f := newRecordFixture(t)
		f.connect(t, f.alice, animerecord.DestinationAnnict)
		f.annict.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}}, 0)
		requests := f.annict.requests

		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
		assert.Equal(t, 1, requests(), "a connection never checked is due")
		f.clock.Advance(animerecord.WishlistRefreshInterval - time.Minute)
		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
		assert.Equal(t, 1, requests(), "not due before the refresh interval")
		f.clock.Advance(time.Minute)
		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
		assert.Equal(t, 2, requests(), "due after the refresh interval")

		f.annict.failNext(opLibrary, http.StatusInternalServerError, 1)
		f.clock.Advance(animerecord.WishlistRefreshInterval)
		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool), "a failed connection does not fail the job")
		f.clock.Advance(animerecord.WishlistRetryInterval - time.Minute)
		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
		assert.Equal(t, 2, requests(), "a failed connection waits for the retry interval")
		f.clock.Advance(time.Minute)
		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
		assert.Equal(t, 3, requests(), "a failed connection is retried after the retry interval")
		_, _, reason := f.importState(t, f.alice.UserID, animerecord.DestinationAnnict)
		assert.Nil(t, reason)
	})

	t.Run("rate limited destination waits", func(t *testing.T) {
		t.Parallel()
		f := newRecordFixture(t)
		f.connect(t, f.alice, animerecord.DestinationAnnict)
		f.connect(t, f.bob, animerecord.DestinationAnnict)
		f.connect(t, f.bob, animerecord.DestinationAniList)
		f.annict.failNext(opLibrary, http.StatusTooManyRequests, 1)
		f.anilist.setList([]fakeListEntry{{ID: 600, Title: titleA, Status: listCurrent}}, 0)

		require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))

		checked := 0
		for _, user := range []uuid.UUID{f.alice.UserID, f.bob.UserID} {
			if at, _, reason := f.importState(t, user, animerecord.DestinationAnnict); at != nil {
				checked++
				assert.NotNil(t, reason, "the rate limited connection keeps the reason")
			}
		}
		assert.Equal(t, 1, checked, "the other Annict connection waits for the next check")
		assert.Equal(t, []string{"anilist:600:watching:作品A"}, f.wishlistRows(t, f.bob.UserID), "AniList is not affected")
	})
}

// TestWishlistCandidates は、見たいリストの作品を、作品 ID の対応で TID に、番組の結びつきで番組に引いて、
// これから放送される、予約のない番組を候補にすることを確かめる (#928)。候補は、ユーザー自身の見たいリストからだけ求める。
func TestWishlistCandidates(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	otherService := faketuner.ServiceItemID(grNetworkID+1, 0x0408)
	// 12:00 の番組 (放送中) のほか、12:30 (別のサービス)、13:00、13:30 の番組を、testTID に結びつける。
	for _, link := range []struct {
		serviceID int64
		start     time.Time
	}{{otherService, at(12, 30)}, {serviceID(), at(13, 0)}, {serviceID(), at(13, 30)}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
			SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, link.serviceID, link.start)
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', $3, 5, 5, NULL, false, false)`, link.serviceID, link.start, testTID)
		require.NoError(t, err)
	}
	// TID 7000 は、arm では Annict の作品 20 に対応するが、arm-supplementary が 21 に直している。古い対応では引かない。
	for _, statement := range []string{
		`INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 7000, 0, 20, NULL, NULL)`,
		`INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm_supplementary', 7000, 0, 21, NULL, NULL)`,
	} {
		_, err := f.pool.Exec(t.Context(), statement)
		require.NoError(t, err)
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
		SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID(), at(14, 0))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
		VALUES ($1, $2, 0, 'syobocal', 7000, 1, 1, NULL, true, false)`, serviceID(), at(14, 0))
	require.NoError(t, err)
	// 13:30 の番組は、予約してある。
	var reservedEvent int64
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT event_id FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID(), at(13, 30)).Scan(&reservedEvent))
	_, _, err = reserve.NewReservations(f.clock, audit.NewRecorder(f.clock)).CreateRule(t.Context(), f.pool, audit.User(f.bob.UserID), serviceID(), reservedEvent, true)
	require.NoError(t, err)

	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.alice, animerecord.DestinationAniList)
	f.connect(t, f.bob, animerecord.DestinationAnnict)
	f.annict.setList([]fakeListEntry{{ID: annictWorkID, Title: titleA, Status: stateWannaWatch}, {ID: 20, Title: "古い対応の作品", Status: stateWannaWatch}}, 0)
	f.anilist.setList([]fakeListEntry{{ID: aniListID, Title: "Work A", Status: listCurrent}}, 0)
	require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.alice.UserID))
	// bob の見たいリストは、TID 7000 の作品 (arm-supplementary の対応) だけ。
	f.annict.setList([]fakeListEntry{{ID: 21, Title: titleC, Status: stateWatching}}, 0)
	require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.bob.UserID))

	type want struct {
		tid        int32
		title      string
		sources    []string
		broadcasts []pickedBroadcast
		reserved   int
	}
	check := func(t *testing.T, userID uuid.UUID, expected []want) {
		t.Helper()
		wishlist, err := f.service.Wishlist(t.Context(), f.pool, userID)
		require.NoError(t, err)
		got := make([]want, 0, len(wishlist.Works))
		for _, work := range wishlist.Works {
			sources := make([]string, 0, len(work.Sources))
			for _, source := range work.Sources {
				sources = append(sources, source.Destination+":"+source.Status)
			}
			broadcasts := make([]pickedBroadcast, 0, len(work.Broadcasts))
			for _, b := range work.Broadcasts {
				broadcasts = append(broadcasts, pickedBroadcast{ServiceID: b.ServiceID, EventID: b.EventID, Preferred: b.Preferred})
			}
			got = append(got, want{tid: work.TID, title: work.Title, sources: sources, broadcasts: broadcasts, reserved: work.ReservedBroadcasts})
		}
		assert.Equal(t, expected, got)
	}
	event := func(service int64, start time.Time) int64 {
		var id int64
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT event_id FROM programs WHERE service_id = $1 AND start_at = $2`, service, start).Scan(&id))

		return id
	}

	// 放送中の 12:00 と、予約のある 13:30 は外れる。13:30 に予約があるので、録るサービスは選ばない (予約済み)。
	check(t, f.alice.UserID, []want{{
		tid: testTID, title: titleA, sources: []string{"annict:wanna_watch", "anilist:watching"}, reserved: 1,
		broadcasts: []pickedBroadcast{
			{ServiceID: otherService, EventID: event(otherService, at(12, 30)), Preferred: false},
			{ServiceID: serviceID(), EventID: event(serviceID(), at(13, 0)), Preferred: false},
		},
	}})
	check(t, f.bob.UserID, []want{{
		tid: 7000, title: titleC, sources: []string{"annict:watching"}, reserved: 0,
		broadcasts: []pickedBroadcast{{ServiceID: serviceID(), EventID: event(serviceID(), at(14, 0)), Preferred: true}},
	}})

	// 連携を外すと、その記録先の見たいリストも消え、ほかの記録先の分だけが残る。
	require.NoError(t, f.service.Disconnect(t.Context(), f.pool, audit.User(f.alice.UserID), f.alice.UserID, animerecord.DestinationAnnict))
	check(t, f.alice.UserID, []want{{
		tid: testTID, title: "Work A", sources: []string{"anilist:watching"}, reserved: 1,
		broadcasts: []pickedBroadcast{
			{ServiceID: otherService, EventID: event(otherService, at(12, 30)), Preferred: false},
			{ServiceID: serviceID(), EventID: event(serviceID(), at(13, 0)), Preferred: false},
		},
	}})
}

// TestWishlistRetryAfter は、記録先が 429 で Retry-After を示したら、その長さ (1 時間まで) だけ待ってから問い合わせ直すことを確かめる (#928)。
// Retry-After がなければ、1 時間待つ。
func TestWishlistRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		retryAfter func(now time.Time) string
		want       time.Duration
	}{
		{name: "seconds", retryAfter: func(time.Time) string { return "120" }, want: 2 * time.Minute},
		{name: "HTTP date", retryAfter: func(now time.Time) string { return now.Add(5 * time.Minute).UTC().Format(http.TimeFormat) }, want: 5 * time.Minute},
		{name: "capped at the retry interval", retryAfter: func(time.Time) string { return "7200" }, want: animerecord.WishlistRetryInterval},
		{name: "without Retry-After", retryAfter: func(time.Time) string { return "" }, want: animerecord.WishlistRetryInterval},
		{name: "unreadable Retry-After", retryAfter: func(time.Time) string { return "soon" }, want: animerecord.WishlistRetryInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, animerecord.DestinationAnnict)
			f.annict.setList([]fakeListEntry{{ID: 10, Title: titleA, Status: stateWannaWatch}}, 0)
			// HTTP の日時は、壁の時計で読むので、テストの時計ではなく、今の時刻から作る。
			f.annict.rateLimitWith(opLibrary, tt.retryAfter(time.Now()))

			require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
			_, _, reason := f.importState(t, f.alice.UserID, animerecord.DestinationAnnict)
			require.NotNil(t, reason, "the rate limited import keeps the reason")
			// 秒の切り捨てで早まらないよう、1 分の余裕を見る。
			f.clock.Advance(tt.want - time.Minute)
			require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
			assert.Equal(t, 0, f.annict.requests(), "not retried before Retry-After")
			f.clock.Advance(time.Minute)
			require.NoError(t, f.service.ImportDueWishlists(t.Context(), f.pool))
			assert.Equal(t, 1, f.annict.requests(), "retried after Retry-After")
		})
	}
}

// TestCreateWishlistRule は、候補の作品からルールを作るとき、同じ作品のルールを重ねて作らないことを確かめる (#928)。
// ルールの照合が予約を作るのは後なので、予約だけでなく、同じキーワードとサービスのルールがあれば予約済みとして断る。
func TestCreateWishlistRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, f *recordFixture, create func() error)
	}{
		{
			name: "two concurrent creates make one rule",
			run: func(t *testing.T, _ *recordFixture, create func() error) {
				t.Helper()
				errs := make(chan error, 2)
				var start sync.WaitGroup
				start.Add(1)
				for range 2 {
					go func() {
						start.Wait()
						errs <- create()
					}()
				}
				start.Done()
				succeeded, refused := 0, 0
				for range 2 {
					switch err := <-errs; {
					case err == nil:
						succeeded++
					case errors.Is(err, animerecord.ErrWishlistWorkCovered):
						refused++
					default:
						t.Errorf("unexpected error: %v", err)
					}
				}
				assert.Equal(t, 1, succeeded)
				assert.Equal(t, 1, refused)
			},
		},
		{
			name: "a second create before the rule is matched is refused",
			run: func(t *testing.T, _ *recordFixture, create func() error) {
				t.Helper()
				require.NoError(t, create())
				require.ErrorIs(t, create(), animerecord.ErrWishlistWorkCovered)
			},
		},
		{
			name: "an existing disabled rule of the same keyword and service covers the work",
			run: func(t *testing.T, f *recordFixture, create func() error) {
				t.Helper()
				wishlist, err := f.service.Wishlist(t.Context(), f.pool, f.alice.UserID)
				require.NoError(t, err)
				require.Len(t, wishlist.Works, 1)
				require.False(t, wishlist.Works[0].Covered)
				in := animerecord.WishlistRuleInput(wishlist.Works[0], &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: nil, FollowEventRelay: true})
				in.Name, in.Enabled = "家族が前に作ったルール", false
				_, err = rule.NewRules(f.clock, audit.NewRecorder(f.clock)).Create(t.Context(), f.pool, audit.User(f.bob.UserID), in)
				require.NoError(t, err)

				wishlist, err = f.service.Wishlist(t.Context(), f.pool, f.alice.UserID)
				require.NoError(t, err)
				require.Len(t, wishlist.Works, 1)
				assert.True(t, wishlist.Works[0].Covered, "covered without any reservation")
				assert.False(t, wishlist.Works[0].Broadcasts[0].Preferred)
				require.ErrorIs(t, create(), animerecord.ErrWishlistWorkCovered)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			for _, start := range []time.Time{at(13, 0), at(13, 30)} {
				_, err := f.pool.Exec(t.Context(), `UPDATE programs SET name = '作品A #5「話」', name_normalized = '作品A #5「話」' WHERE service_id = $1 AND start_at = $2`, serviceID(), start)
				require.NoError(t, err)
				_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
					SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`, serviceID(), start)
				require.NoError(t, err)
				_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
					VALUES ($1, $2, 0, 'syobocal', $3, 5, 5, NULL, false, false)`, serviceID(), start, testTID)
				require.NoError(t, err)
			}
			f.connect(t, f.alice, animerecord.DestinationAnnict)
			f.annict.setList([]fakeListEntry{{ID: annictWorkID, Title: titleA, Status: stateWannaWatch}}, 0)
			require.NoError(t, f.service.ImportUserWishlists(t.Context(), f.pool, f.alice.UserID))
			rules := rule.NewRules(f.clock, audit.NewRecorder(f.clock))
			create := func() error {
				_, err := f.service.CreateWishlistRule(t.Context(), f.pool, rules, audit.User(f.alice.UserID), f.alice.UserID, testTID)

				return err
			}

			tt.run(t, f, create)

			var count int
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM rules`).Scan(&count))
			assert.Equal(t, 1, count, "the work has exactly one rule")
		})
	}
}
