package graph_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	animeWishlistQuery = `{ animeWishlist { enabled imports { destination entryCount }
		works { tid title reservedBroadcasts covered sources { destination workId status url } broadcasts { preferred program { id name } } } } }`
	createAnimeWishlistRule = `mutation($tid: Int!) { createAnimeWishlistRule(tid: $tid) {
		id name enabled conditions { keywords { text } services { name } } } }`
	refreshAnimeWishlist = `mutation { refreshAnimeWishlist }`
	// wishlistTID は、見たいリストに入れた作品のしょぼいカレンダーの TID。
	wishlistTID = 6776
)

type animeWishlistResult struct {
	AnimeWishlist struct {
		Enabled bool `json:"enabled"`
		Imports []struct {
			Destination string `json:"destination"`
			EntryCount  int    `json:"entryCount"`
		} `json:"imports"`
		Works []struct {
			Tid                int    `json:"tid"`
			Title              string `json:"title"`
			ReservedBroadcasts int    `json:"reservedBroadcasts"`
			Covered            bool   `json:"covered"`
			Sources            []struct {
				Destination string `json:"destination"`
				WorkID      int    `json:"workId"`
				Status      string `json:"status"`
				URL         string `json:"url"`
			} `json:"sources"`
			Broadcasts []struct {
				Preferred bool `json:"preferred"`
				Program   struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"program"`
			} `json:"broadcasts"`
		} `json:"works"`
	} `json:"animeWishlist"`
}

// linkWishlistProgram は、サービス serviceID の start からの番組を、作品 (wishlistTID) の第 5 話にする。
func linkWishlistProgram(t *testing.T, f *fixture, serviceID int64, start time.Time) {
	t.Helper()
	for _, statement := range []string{
		`UPDATE programs SET name = '作品A #5「話」', name_normalized = '作品A #5「話」' WHERE service_id = $1 AND start_at = $2`,
		`INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at) SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`,
		`INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', 6776, 5, 5, NULL, false, false)`,
	} {
		tag, err := f.pool.Exec(t.Context(), statement, serviceID, start)
		require.NoError(t, err)
		require.Equal(t, int64(1), tag.RowsAffected())
	}
}

// connectWishlist は、ユーザー userID と Annict の連携と、見たいリストの作品 (Annict の作品 10) を DB に入れる。
// 連携と取り込みは usecase のテストで確かめるので、ここでは行を直に書く。
func connectWishlist(t *testing.T, f *fixture, userID uuid.UUID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at, wishlist_checked_at, wishlist_imported_at)
		VALUES ($1, 'annict', '\x00', 'annict-user', $2, $2, $2)`, userID, testNow())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, fetched_at)
		VALUES ($1, 'annict', 10, 'wanna_watch', '作品A', $2)`, userID, testNow())
	require.NoError(t, err)
}

// TestAnimeWishlist は、見たいリストの作品の放送の候補を、ユーザーごとに自分の分だけ返し、
// 予約できるユーザーだけが、候補の作品からルールを作れることを確かめる (#928、設計書 9.7)。
func TestAnimeWishlist(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	viewer := f.as(t, "viewer", auth.RoleViewer)
	other := f.as(t, "other", auth.RoleMember)
	serviceID := int64(3273601024)
	// こたつテスト1 の 13:00 からの番組と、別の局 (こたつテスト2) の 13:30 からの番組を、作品の第 5 話にする。
	start := time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC)
	for _, program := range []struct {
		serviceID int64
		start     time.Time
	}{{serviceID, start}, {3273701032, start.Add(30 * time.Minute)}} {
		linkWishlistProgram(t, f, program.serviceID, program.start)
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10, NULL, NULL)`)
	require.NoError(t, err)
	connectWishlist(t, f, f.userID)
	connectWishlist(t, f, viewer.userID)
	tid := map[string]any{"tid": wishlistTID}

	var got animeWishlistResult
	require.Empty(t, viewer.query(t, animeWishlistQuery, nil, &got))
	assert.False(t, got.AnimeWishlist.Enabled, "the linking is disabled by default")
	assert.Empty(t, got.AnimeWishlist.Works)
	assert.Equal(t, []string{codeConflict}, f.query(t, createAnimeWishlistRule, tid, nil))
	assert.Equal(t, []string{codeConflict}, viewer.query(t, refreshAnimeWishlist, nil, nil))
	require.Empty(t, admin.query(t, enableAnimeLinking, nil, nil))

	// 視聴者も、自分の候補は見られる。
	require.Empty(t, viewer.query(t, animeWishlistQuery, nil, &got))
	require.True(t, got.AnimeWishlist.Enabled)
	require.Len(t, got.AnimeWishlist.Imports, 1)
	assert.Equal(t, 1, got.AnimeWishlist.Imports[0].EntryCount)
	require.Len(t, got.AnimeWishlist.Works, 1)
	work := got.AnimeWishlist.Works[0]
	assert.Equal(t, wishlistTID, work.Tid)
	assert.Equal(t, "作品A", work.Title)
	require.Len(t, work.Sources, 1)
	assert.Equal(t, "https://annict.com/works/10", work.Sources[0].URL)
	assert.Equal(t, "WANNA_WATCH", work.Sources[0].Status)
	assert.False(t, work.Covered)
	require.Len(t, work.Broadcasts, 2)
	assert.True(t, work.Broadcasts[0].Preferred, "the earliest broadcast is the one to record")
	assert.False(t, work.Broadcasts[1].Preferred)
	assert.Equal(t, "作品A #5「話」", work.Broadcasts[0].Program.Name)
	require.Empty(t, viewer.query(t, refreshAnimeWishlist, nil, nil), "viewers can refresh their own list")

	// ほかのユーザーの見たいリストは見えず、その作品からルールも作れない。
	var others animeWishlistResult
	require.Empty(t, other.query(t, animeWishlistQuery, nil, &others))
	assert.True(t, others.AnimeWishlist.Enabled)
	assert.Empty(t, others.AnimeWishlist.Imports)
	assert.Empty(t, others.AnimeWishlist.Works)
	assert.Equal(t, []string{codeNotFound}, other.query(t, createAnimeWishlistRule, tid, nil))
	// 視聴者は、自分の候補からもルールを作れない。
	assert.Equal(t, []string{codeForbidden}, viewer.query(t, createAnimeWishlistRule, tid, nil))

	var created struct {
		CreateAnimeWishlistRule struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Enabled    bool   `json:"enabled"`
			Conditions struct {
				Keywords []struct {
					Text string `json:"text"`
				} `json:"keywords"`
				Services []struct {
					Name string `json:"name"`
				} `json:"services"`
			} `json:"conditions"`
		} `json:"createAnimeWishlistRule"`
	}
	require.Empty(t, f.query(t, createAnimeWishlistRule, tid, &created))
	assert.Equal(t, "作品A", created.CreateAnimeWishlistRule.Name)
	assert.True(t, created.CreateAnimeWishlistRule.Enabled)
	require.Len(t, created.CreateAnimeWishlistRule.Conditions.Keywords, 1)
	assert.Equal(t, "作品A", created.CreateAnimeWishlistRule.Conditions.Keywords[0].Text, "the keyword has no episode number")
	require.Len(t, created.CreateAnimeWishlistRule.Conditions.Services, 1)
	assert.Equal(t, "こたつテスト1", created.CreateAnimeWishlistRule.Conditions.Services[0].Name)

	// ルールの照合が予約を作る前でも、作品は予約済みになり、二度押しても重ねて作らない。
	require.Empty(t, f.query(t, animeWishlistQuery, nil, &got))
	require.Len(t, got.AnimeWishlist.Works, 1)
	assert.True(t, got.AnimeWishlist.Works[0].Covered, "covered by the rule before it is matched")
	assert.Equal(t, 0, got.AnimeWishlist.Works[0].ReservedBroadcasts)
	assert.Equal(t, []string{codeConflict}, f.query(t, createAnimeWishlistRule, tid, nil))

	// 作ったルールの照合で、候補の番組が予約され、候補から外れる。
	clk := clocktest.NewFake(testNow())
	reservations := reserve.NewReservations(clk, audit.NewRecorder(clk))
	_, err = rule.NewEngine(f.pool, clk, time.UTC, func(context.Context, pgx.Tx) error { return nil }, reservations).Run(t.Context())
	require.NoError(t, err)
	require.Empty(t, f.query(t, animeWishlistQuery, nil, &got))
	// 作ったルールの局の放送は予約され、ほかの局の放送だけが残る。作品は予約済みになり、録る局を選び直さない。
	require.Empty(t, f.query(t, animeWishlistQuery, nil, &got))
	require.Len(t, got.AnimeWishlist.Works, 1)
	covered := got.AnimeWishlist.Works[0]
	assert.True(t, covered.Covered)
	assert.Equal(t, 1, covered.ReservedBroadcasts)
	require.Len(t, covered.Broadcasts, 1)
	assert.False(t, covered.Broadcasts[0].Preferred, "no other service is promoted once the work has a reservation")
	// 二重押しや古いタブからでも、ルールを重ねて作らない。
	assert.Equal(t, []string{codeConflict}, f.query(t, createAnimeWishlistRule, tid, nil))
	var rules int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM rules`).Scan(&rules))
	assert.Equal(t, 1, rules)
}
