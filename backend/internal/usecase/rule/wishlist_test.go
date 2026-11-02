package rule_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
)

// enableWishlistWatch は、アニメの連携を有効にし、ユーザー userID の見たいリストに、TID 6776 の作品 (Annict の作品 10) を入れる。
// eventId 4 (12:30〜) を第 3 話、eventId 5 (13:00〜) を第 4 話に結びつけ、eventId 6 (13:30〜) を第 4 話の再放送にする。
func (f *watchFixture) enableWishlistWatch(t *testing.T, userID uuid.UUID, progress *int32) {
	t.Helper()
	f.setLinking(t, userID, true)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10, NULL, NULL)`)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at)
		VALUES ($1, 'annict', '\x00', 'user', now())`, userID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, progress, fetched_at)
		VALUES ($1, 'annict', 10, 'watching', '作品', $2, now())`, userID, progress)
	require.NoError(t, err)
	for _, link := range []struct {
		eventID int64
		episode float64
		rerun   bool
	}{{4, 3, false}, {5, 4, false}, {6, 4, true}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
			SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND event_id = $2`, serviceID(), link.eventID)
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end,
			is_first, is_last, is_rerun)
			SELECT service_id, start_at, 0, 'syobocal', 6776, $3, $3, false AS is_first, false AS is_last, $4 FROM programs WHERE service_id = $1 AND event_id = $2`,
			serviceID(), link.eventID, link.episode, link.rerun)
		require.NoError(t, err)
	}
}

// setLinking は、管理者がアニメの連携を有効か無効にする。
func (f *watchFixture) setLinking(t *testing.T, actor uuid.UUID, enabled bool) {
	t.Helper()
	_, err := settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.User(actor), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil, DataBroadcastIPEnabled: nil,
		AnimeLinkingEnabled: new(enabled), DRCSReplacements: nil,
	})
	require.NoError(t, err)
}

// setNotifyWishlist は、ユーザー userID の、見たいリストの作品の放送を通知する設定を変える。
func (f *watchFixture) setNotifyWishlist(t *testing.T, userID uuid.UUID, notify bool) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_record_settings (user_id, notify_wishlist, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET notify_wishlist = EXCLUDED.notify_wishlist`, userID, notify)
	require.NoError(t, err)
}

// wishlistReserved は、ユーザー userID の、見たいリストの通知が付けた視聴予約の eventId を返す。
func (f *watchFixture) wishlistReserved(t *testing.T, userID uuid.UUID) []int64 {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT event_id FROM watch_reservations WHERE user_id = $1 AND from_wishlist ORDER BY event_id`, userID)
	require.NoError(t, err)
	defer rows.Close()
	result := []int64{}
	for rows.Next() {
		var eventID int64
		require.NoError(t, rows.Scan(&eventID))
		result = append(result, eventID)
	}
	require.NoError(t, rows.Err())

	return result
}

// TestEngineWishlistWatch は、見たいリストの作品の放送を通知するユーザーに、見終えた話より後の話ごとに最も早い本放送の視聴予約を付け、
// 当たらなくなったら外すことを確かめる (#1234)。照合は 12:10 に行う。
func TestEngineWishlistWatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		progress *int32
		// change は、1 回目の照合の後、2 回目の照合の前に行う。nil なら 1 回だけ照合する。
		change      func(t *testing.T, f *watchFixture)
		want        []int64
		wantCreated int
		wantDeleted int
	}{
		{name: "見終えていない話の本放送に付け、再放送には付けない", progress: new(int32(2)), want: []int64{4, 5}, wantCreated: 2},
		{name: "見終えた話には付けない", progress: new(int32(3)), want: []int64{5}, wantCreated: 1},
		{
			name: "通知をやめたら外す", progress: nil,
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.setNotifyWishlist(t, f.alice, false)
			},
			want: []int64{}, wantDeleted: 2,
		},
		{
			name: "見たいリストから外れたら外す", progress: nil,
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				_, err := f.pool.Exec(t.Context(), `DELETE FROM anime_wishlist_entries WHERE user_id = $1`, f.alice)
				require.NoError(t, err)
			},
			want: []int64{}, wantDeleted: 2,
		},
		{
			name: "手で外した放送には付け直さない", progress: nil,
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.Cancel(t.Context(), f.pool, f.clock, f.alice, f.programKey(4)))
			},
			want: []int64{5},
		},
		{
			name: "管理者が連携を無効にしたら外す", progress: nil,
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.setLinking(t, f.alice, false)
			},
			want: []int64{}, wantDeleted: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newWatchFixture(t)
			f.enableWishlistWatch(t, f.alice, tt.progress)
			f.setNotifyWishlist(t, f.alice, true)
			result := f.run(t)
			if tt.change != nil {
				tt.change(t, f)
				result = f.run(t)
			}
			assert.Equal(t, tt.want, f.wishlistReserved(t, f.alice))
			if tt.change == nil {
				assert.Equal(t, tt.wantCreated, result.WishlistCreated)
			}
			assert.Equal(t, tt.wantDeleted, result.WishlistDeleted)
			assert.Empty(t, f.wishlistReserved(t, f.bob), "other users are not notified")
		})
	}
}

// TestEngineWishlistWatchManual は、見たいリストの通知が付けた視聴予約に手で付け直したら、手で付けたものにし、
// 通知をやめても消さないことを確かめる (#1234)。
func TestEngineWishlistWatchManual(t *testing.T) {
	t.Parallel()
	f := newWatchFixture(t)
	f.enableWishlistWatch(t, f.alice, nil)
	f.setNotifyWishlist(t, f.alice, true)
	f.run(t)
	require.Equal(t, []int64{4, 5}, f.wishlistReserved(t, f.alice))

	require.NoError(t, watch.Reserve(t.Context(), f.pool, f.clock, f.alice, f.programKey(4)))
	f.setNotifyWishlist(t, f.alice, false)
	result := f.run(t)
	assert.Equal(t, 1, result.WishlistDeleted)
	assert.Empty(t, f.wishlistReserved(t, f.alice))
	assert.Equal(t, map[int64]bool{4: false}, f.watchReserved(t, f.alice), "the reservation added again by hand is kept")
}
