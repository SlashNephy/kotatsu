package nextepisode_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/history"
	"github.com/SlashNephy/kotatsu/internal/usecase/nextepisode"
)

const (
	// 偽のチューナーサーバーの、地上波 (こたつテスト1、こたつテスト3) と BS のネットワークとサービス。
	fakeGRNetwork  = 0x7FE0
	fakeGRService  = 0x0400
	fakeGR3Network = 0x7FE0 + 2
	fakeGR3Service = 0x0410
	fakeBSNetwork  = 4
	fakeBSService  = 101
	// animeTID は、しょぼいカレンダーの作品のテストの TID。
	animeTID = 7001
)

func findNow() time.Time {
	return time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC)
}

// fakeProgram は、findNow から offset 後に始まる、30 分の番組。
func fakeProgram(networkID, serviceID, eventID int, offset time.Duration, name string) *faketuner.Program {
	return &faketuner.Program{
		ID: faketuner.ProgramID(networkID, serviceID, eventID), EventID: eventID, ServiceID: serviceID, NetworkID: networkID,
		StartAt: findNow().Add(offset).UnixMilli(), Duration: (30 * time.Minute).Milliseconds(), IsFree: true, Name: name, Description: "",
		RelatedItems: []*faketuner.RelatedItem{},
	}
}

type findFixture struct {
	pool       *pgxpool.Pool
	reconciler *epg.Reconciler
}

func newFindFixture(t *testing.T, programs []*faketuner.Program) *findFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	opts := faketuner.DefaultOptions(findNow(), nil)
	opts.Programs = append(opts.Programs, programs...)
	tuner := httptest.NewServer(faketuner.New(opts))
	t.Cleanup(tuner.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: tuner.URL})
	require.NoError(t, err)
	reconciler := epg.NewReconciler(pool, client, clocktest.NewFake(findNow()), func(context.Context, pgx.Tx) error { return nil })
	_, err = reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	return &findFixture{pool: pool, reconciler: reconciler}
}

func (f *findFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

// addRecording は、2 日前に地上波で録った、番組名 name の録画を足し、その ID を返す。
func (f *findFixture) addRecording(t *testing.T, name string) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	start := findNow().Add(-48 * time.Hour)
	f.exec(t, `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, $5, 'completed', $4, $4, $4)`,
		recordingID, faketuner.ServiceItemID(fakeGRNetwork, fakeGRService), name, start, start.Add(30*time.Minute))

	return recordingID
}

// addHistory は、番組名 name の録画履歴を足す。シリーズ名は、録画履歴を書くときと同じく、番組名から読む。
func (f *findFixture) addHistory(t *testing.T, name, series string, tid *int32, count *float64) {
	t.Helper()

	start := findNow().Add(-24 * time.Hour)
	f.exec(t, `INSERT INTO recording_history (id, service_id, title_key, name, series, syobocal_tid, anime_count, start_at, end_at, created_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, $7)`,
		id.New(), faketuner.ServiceItemID(fakeGRNetwork, fakeGRService), name, series, tid, count, start, start.Add(30*time.Minute))
}

// reserve は、番組 (networkID, serviceID, eventID) を予約する。
func (f *findFixture) reserve(t *testing.T, networkID, serviceID, eventID int, enabled bool) {
	t.Helper()

	f.exec(t, `INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
		enabled, follow_event_relay, settings, created_at, updated_at)
		SELECT $1, 'program', service_id, event_id, start_at, end_at, name, description, '{}', '[]', $2, false, '{}', $3, $3
		FROM programs WHERE service_id = $4 AND event_id = $5`,
		id.New(), enabled, findNow(), faketuner.ServiceItemID(networkID, serviceID), eventID)
}

func eventIDs(broadcasts []*nextepisode.Broadcast) []int64 {
	out := make([]int64, 0, len(broadcasts))
	for _, b := range broadcasts {
		out = append(out, b.EventID)
	}

	return out
}

// TestFindBySeries は、しょぼいカレンダーに結びつかない録画の続きの放送を、EPG の同期で書いたシリーズ名と話数で引き、
// 前の回、別の期、予約のある回、録った回、1 週間より先の回を外すことを確かめる (#1564)。
func TestFindBySeries(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		fakeProgram(fakeGRNetwork, fakeGRService, 500, 2*time.Hour, "霧笛の港 #4"),
		// 全角で書いても、同じシリーズの回とする。
		fakeProgram(fakeBSNetwork, fakeBSService, 501, 3*time.Hour, "霧笛の港　＃４🈑"),
		fakeProgram(fakeGRNetwork, fakeGRService, 502, 5*time.Hour, "霧笛の港 #2"),
		fakeProgram(fakeGR3Network, fakeGR3Service, 503, 6*time.Hour, "霧笛の港 #5"),
		fakeProgram(fakeBSNetwork, fakeBSService, 504, 8*time.Hour, "霧笛の港 #6"),
		fakeProgram(fakeGRNetwork, fakeGRService, 505, 10*time.Hour, "霧笛の港 第2期 #7"),
		fakeProgram(fakeGRNetwork, fakeGRService, 506, 8*24*time.Hour, "霧笛の港 #8"),
		fakeProgram(fakeGRNetwork, fakeGRService, 507, 11*time.Hour, "霧笛の港 #9"),
	})
	recording := f.addRecording(t, "霧笛の港 #3")
	f.reserve(t, fakeGR3Network, fakeGR3Service, 503, true)
	// 取り消した (無効の) 予約も、人が録らないと決めた放送なので外す。
	f.reserve(t, fakeGRNetwork, fakeGRService, 507, false)
	f.addHistory(t, "霧笛の港 #6", harbor, nil, nil)

	got, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)

	assert.Equal(t, []int64{500, 501}, eventIDs(got))
	assert.InDelta(t, 4, got[0].Number, 0)
}

// TestFindFillsProgramsWithoutSeries は、シリーズ名の列を足す前に書いた番組 (列が NULL) も、全量の突き合わせで埋まり、
// 続きの放送に出ることを確かめる (#1564)。
func TestFindFillsProgramsWithoutSeries(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{fakeProgram(fakeGRNetwork, fakeGRService, 500, 2*time.Hour, "霧笛の港 #4")})
	recording := f.addRecording(t, "霧笛の港 #3")
	f.exec(t, `UPDATE programs SET series = NULL, episode_number = NULL`)

	before, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)
	assert.Empty(t, before)

	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	after, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)
	assert.Equal(t, []int64{500}, eventIDs(after))

	var unfilled int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM programs WHERE series IS NULL`).Scan(&unfilled))
	assert.Zero(t, unfilled)
}

// TestFindByAnime は、しょぼいカレンダーの話に結びついた録画では、番組名でなく TID と話数で続きの放送を引くことを確かめる (#1564)。
func TestFindByAnime(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		// 番組名が違っても、同じ TID の後の話に結びついていれば出す。
		fakeProgram(fakeGRNetwork, fakeGRService, 600, 2*time.Hour, "アニメ・灯台の地図 第2話"),
		// 番組名のシリーズ名と話数が合っていても、TID に結びついていなければ出さない。
		fakeProgram(fakeBSNetwork, fakeBSService, 601, 3*time.Hour, "灯台の地図 #2"),
		// 録った話数は外す。
		fakeProgram(fakeGRNetwork, fakeGRService, 602, 4*time.Hour, "灯台の地図 #3"),
		fakeProgram(fakeGRNetwork, fakeGRService, 603, 5*time.Hour, "灯台の地図 #4"),
	})
	recording := f.addRecording(t, "灯台の地図 #1")
	f.exec(t, `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, $2)`, recording, findNow())
	f.exec(t, `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
		VALUES ($1, 0, 'syobocal', $2, 1, 1, false, false)`, recording, animeTID)
	for eventID, count := range map[int]float64{600: 2, 602: 3, 603: 4} {
		serviceID := faketuner.ServiceItemID(fakeGRNetwork, fakeGRService)
		f.exec(t, `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
			SELECT service_id, start_at, event_id, $3 FROM programs WHERE service_id = $1 AND event_id = $2`, serviceID, eventID, findNow())
		f.exec(t, `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			SELECT service_id, start_at, 0, 'syobocal', $3, $4, $4, false, false FROM programs WHERE service_id = $1 AND event_id = $2`,
			serviceID, eventID, animeTID, count)
	}
	f.addHistory(t, "灯台の地図 #3", "灯台の地図", new(int32(animeTID)), new(float64(3)))

	got, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)

	assert.Equal(t, []int64{600, 603}, eventIDs(got))
}

// TestFindWithoutEpisode は、話数を読めない録画 (ニュースなど) と、ない録画では、続きの放送を探さないことを確かめる。
func TestFindWithoutEpisode(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{fakeProgram(fakeGRNetwork, fakeGRService, 700, 2*time.Hour, "ニュース7")})
	tests := []struct {
		name      string
		recording uuid.UUID
	}{
		{name: "話数を読めない録画", recording: f.addRecording(t, "ニュース7")},
		{name: "ない録画", recording: id.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := nextepisode.Find(t.Context(), f.pool, tt.recording, findNow())
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}
}

// TestFindReadsArchivedSeriesEpisode は、番組名に話数のない録画の話数を、録画の番組 (アーカイブ) の記述子の話数から読み、
// eventId を使い回した番組表の後の番組の記述子の話数では読まないことを確かめる。
func TestFindReadsArchivedSeriesEpisode(t *testing.T) {
	t.Parallel()

	reused := fakeProgram(fakeGRNetwork, fakeGRService, 900, time.Hour, "別の番組")
	reused.Series = &faketuner.Series{ID: 1, Repeat: 0, Pattern: 0, ExpiresAt: 0, Episode: 9, LastEpisode: 0, Name: ""}
	f := newFindFixture(t, []*faketuner.Program{
		reused,
		fakeProgram(fakeGRNetwork, fakeGRService, 901, 2*time.Hour, "霧笛の港 #4"),
		fakeProgram(fakeGRNetwork, fakeGRService, 902, 3*time.Hour, "霧笛の港 #10"),
	})
	serviceID := faketuner.ServiceItemID(fakeGRNetwork, fakeGRService)
	start := findNow().Add(-48 * time.Hour)
	recording := id.New()
	f.exec(t, `INSERT INTO recordings (id, service_id, event_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 900, '霧笛の港', '', $3, $4, 'completed', $3, $3, $3)`, recording, serviceID, start, start.Add(30*time.Minute))
	f.exec(t, `INSERT INTO program_archives (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description,
		name_normalized, description_normalized, extended, genres, related_items, series_episode, created_at, updated_at, archived_at)
		VALUES ($1, 900, 0, $2, $3, false, true, '霧笛の港', '', '霧笛の港', '', '{}', '[]', '[]', 3, $2, $2, $2)`,
		serviceID, start, start.Add(30*time.Minute))

	got, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)

	assert.Equal(t, []int64{901, 902}, eventIDs(got))
}

// dramaSeries と otherDramaSeries は、TMDb の作品のテストの ID。
const (
	dramaSeries      = 9001
	otherDramaSeries = 9002
)

// matchDrama は、地上波の番組 eventID を、TMDb の作品 seriesID のシーズン season の話 episode に結びつける。
func (f *findFixture) matchDrama(t *testing.T, eventID int, seriesID, season, episode int32) {
	t.Helper()

	f.exec(t, `INSERT INTO program_drama_matches (service_id, start_at, event_id, title_key, tmdb_series_id, season_number, episode_number,
		series_source, episode_source, matched_at)
		SELECT service_id, start_at, event_id, name, $3, $4, $5, 'search', 'air_date', $6 FROM programs WHERE service_id = $1 AND event_id = $2`,
		faketuner.ServiceItemID(fakeGRNetwork, fakeGRService), eventID, seriesID, season, episode, findNow())
}

// linkDrama は、録画 recordingID を、TMDb の作品 seriesID のシーズン 1 の話 episode に結びつける。
func (f *findFixture) linkDrama(t *testing.T, recordingID uuid.UUID, seriesID, episode int32) {
	t.Helper()

	f.exec(t, `INSERT INTO recording_drama_matches (recording_id, tmdb_series_id, season_number, episode_number, series_source, episode_source,
		matched_at) VALUES ($1, $2, 1, $3, 'search', 'air_date', $4)`, recordingID, seriesID, episode, findNow())
}

// TestFindByDrama は、番組名から話数を読めないドラマの録画では、TMDb の作品とシーズンと話の番号で続きの放送を引き、
// 前の話、別のシーズン、別の作品、録った話、TMDb に結びつかない番組を外すことを確かめる (#1564)。
func TestFindByDrama(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		fakeProgram(fakeGRNetwork, fakeGRService, 800, 2*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 801, 3*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 802, 4*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 803, 5*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 804, 6*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 805, 7*time.Hour, "夜明けの診療所"),
		// TMDb の話に結びつかない番組は、番組名が同じでも出さない。
		fakeProgram(fakeGRNetwork, fakeGRService, 806, 8*time.Hour, "夜明けの診療所"),
	})
	f.matchDrama(t, 800, dramaSeries, 1, 4)
	f.matchDrama(t, 801, dramaSeries, 1, 2)
	f.matchDrama(t, 802, dramaSeries, 2, 5)
	f.matchDrama(t, 803, dramaSeries, 1, 5)
	f.matchDrama(t, 804, otherDramaSeries, 1, 6)
	f.matchDrama(t, 805, dramaSeries, 1, 6)
	recording := f.addRecording(t, "夜明けの診療所")
	f.linkDrama(t, recording, dramaSeries, 3)

	// 第 5 話を録り終えて録画履歴に書き、録画を消す。録画の結びつきは消えても、履歴に写した話で、録った話として外す。
	recorded := f.addRecording(t, "夜明けの診療所")
	f.linkDrama(t, recorded, dramaSeries, 5)
	q := sqlc.New(f.pool)
	row, err := q.GetRecording(t.Context(), recorded)
	require.NoError(t, err)
	require.NoError(t, history.Record(t.Context(), q, &row, findNow()))
	f.exec(t, `DELETE FROM recordings WHERE id = $1`, recorded)

	got, err := nextepisode.Find(t.Context(), f.pool, recording, findNow())
	require.NoError(t, err)

	assert.Equal(t, []int64{800, 805}, eventIDs(got))
	assert.InDelta(t, 4, got[0].Number, 0)
}
