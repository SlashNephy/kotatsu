package keyrebuild_test

import (
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/usecase/keyrebuild"
)

const serviceID = 3273601024

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	_, err = pool.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 0)`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'テスト', 'テスト', 1, 'GR', '27', now(), now())`, serviceID)
	require.NoError(t, err)

	return pool
}

func TestRunRebuildsHistoryTitleKeys(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	start := time.Date(2026, time.November, 1, 21, 0, 0, 0, time.UTC)
	// 直す前の TitleKey は、[4K] と ⚿ を印とみなさず、キーに残していた (#1977)。
	rows := []struct{ name, stored, want string }{
		{name: "[4K]番組名 #1", stored: "[4K]番組名 #1", want: "番組名 #1"},
		{name: "⚿番組名 #2", stored: "[鍵]番組名 #2", want: "番組名 #2"},
		{name: "[新]番組名 #3", stored: "番組名 #3", want: "番組名 #3"},
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for i, row := range rows {
		id := uuid.New()
		ids = append(ids, id)
		_, err := db.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $5, now())`, id, serviceID, row.stored, row.name, start.Add(time.Duration(i)*time.Hour))
		require.NoError(t, err)
	}

	// 2 回目は何も変えない。
	for range 2 {
		require.NoError(t, keyrebuild.Run(t.Context(), slog.New(slog.DiscardHandler), db))
		for i, row := range rows {
			var got string
			require.NoError(t, db.QueryRow(t.Context(), `SELECT title_key FROM recording_history WHERE id = $1`, ids[i]).Scan(&got))
			assert.Equal(t, row.want, got, row.name)
		}
	}
}

// TestRunRebuildsSyobocalTitleKeys は、しょぼいカレンダーのタイトルの鍵を、今の計算に合わせて書き直すことを確かめる (#2041)。
func TestRunRebuildsSyobocalTitleKeys(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	// 直す前の鍵は、波ダッシュを ~ にそろえていなかった。
	rows := []struct {
		tid                 int32
		title, stored, want string
	}{
		{tid: 1, title: "作品名〜副題〜", stored: "作品名〜副題〜", want: "作品名~副題~"},
		{tid: 2, title: "作品名 第2期", stored: "作品名第2期", want: "作品名第2期"},
	}
	for _, row := range rows {
		_, err := db.Exec(t.Context(), "INSERT INTO syobocal_titles (tid, title, title_key) VALUES ($1, $2, $3)", row.tid, row.title, row.stored)
		require.NoError(t, err)
	}

	// 2 回目は何も変えない。
	for range 2 {
		require.NoError(t, keyrebuild.Run(t.Context(), slog.New(slog.DiscardHandler), db))
		for _, row := range rows {
			var got string
			require.NoError(t, db.QueryRow(t.Context(), "SELECT title_key FROM syobocal_titles WHERE tid = $1", row.tid).Scan(&got))
			assert.Equal(t, row.want, got, row.title)
		}
	}
}

func TestRunRebuildsDramaTitleKeys(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	start := time.Date(2026, time.November, 1, 21, 0, 0, 0, time.UTC)
	// 直す前の WorkTitle は、season 2. の後ろの話数を見落とし、どちらの番組も「作品名 season」と読んでいた (#1978)。
	oldKey := drama.TitleKey("作品名 season")
	programs := []struct {
		eventID int64
		name    string
		stored  string
	}{
		{eventID: 1, name: "作品名 season 1. request 3. サブタイトル", stored: oldKey},
		{eventID: 2, name: "作品名 season 2. request 20. サブタイトル", stored: oldKey},
		{eventID: 3, name: "別の作品 #4", stored: drama.TitleKeyOf("別の作品 #4")},
	}
	for i, p := range programs {
		at := start.Add(time.Duration(i) * time.Hour)
		_, err := db.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
			name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
			VALUES ($1, $2, $2, $3, $3, false, true, $4, '', $4, '', '{}', '[]', '[]', now(), now())`, serviceID, p.eventID, at, p.name)
		require.NoError(t, err)
		_, err = db.Exec(t.Context(), `INSERT INTO program_drama_matches (service_id, start_at, event_id, title_key, matched_at, finalized_at)
			VALUES ($1, $2, $3, $4, now(), now())`, serviceID, at, p.eventID, p.stored)
		require.NoError(t, err)
	}
	// 前の鍵の上書きと、season 2 の今の鍵に管理者が付けた上書き、変わらない鍵の上書き。
	season1, season2 := drama.TitleKeyOf(programs[0].name), drama.TitleKeyOf(programs[1].name)
	require.NotEqual(t, season1, season2)
	for key, series := range map[string]int{oldKey: 100, season2: 200, programs[2].stored: 300} {
		_, err := db.Exec(t.Context(), `INSERT INTO drama_title_overrides (title_key, tmdb_series_id, updated_at) VALUES ($1, $2, now())`, key, series)
		require.NoError(t, err)
	}

	// 2 回目は何も変えない。
	for range 2 {
		require.NoError(t, keyrebuild.Run(t.Context(), slog.New(slog.DiscardHandler), db))

		overrides := map[string]int{}
		rows, err := db.Query(t.Context(), `SELECT title_key, tmdb_series_id FROM drama_title_overrides`)
		require.NoError(t, err)
		for rows.Next() {
			var key string
			var series int
			require.NoError(t, rows.Scan(&key, &series))
			overrides[key] = series
		}
		require.NoError(t, rows.Err())
		// 前の鍵の上書きは season 1 に写り、season 2 は管理者の上書きを残す。前の鍵の上書きは、使う結びつきがないので消える。
		assert.Equal(t, map[string]int{season1: 100, season2: 200, programs[2].stored: 300}, overrides)

		for i, want := range []struct {
			key       string
			finalized bool
		}{{key: season1, finalized: false}, {key: season2, finalized: false}, {key: programs[2].stored, finalized: true}} {
			var key string
			var finalized bool
			require.NoError(t, db.QueryRow(t.Context(),
				`SELECT title_key, finalized_at IS NOT NULL FROM program_drama_matches WHERE service_id = $1 AND event_id = $2`,
				serviceID, programs[i].eventID).Scan(&key, &finalized))
			assert.Equal(t, want.key, key, programs[i].name)
			assert.Equal(t, want.finalized, finalized, programs[i].name)
		}
	}
}
