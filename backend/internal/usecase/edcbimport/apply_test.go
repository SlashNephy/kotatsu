package edcbimport_test

import (
	"context"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/edcbimport"
)

const (
	serviceID = 3273601024
	eventID   = 100
)

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	userID   uuid.UUID
	importer *edcbimport.Importer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '32736', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, false, true, 'こたつ特集', '', 'こたつ特集', '', '{}', '[]', '[]', $6, $6)`,
		serviceID, eventID, int64(serviceID)*100000+eventID, clk.Now().Add(2*time.Hour), clk.Now().Add(150*time.Minute), clk.Now())
	require.NoError(t, err)
	userID := id.New()
	require.NoError(t, sqlc.New(pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: "mikan", DisplayName: "みかん", Role: "admin", PasswordHash: nil, CreatedAt: clk.Now(),
	}))

	return &fixture{pool: pool, clock: clk, userID: userID, importer: edcbimport.NewImporter(clk, audit.NewRecorder(clk))}
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))

	return n
}

func snapshot(now time.Time) *edcbimport.Snapshot {
	nhkKey := edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: 1024}
	recorded := func(id uint32, title string, status uint32, start time.Time) *edcb.RecordedFile {
		return &edcb.RecordedFile{ServiceKey: nhkKey, ID: id, Title: title, Start: start, Duration: 30 * time.Minute, ServiceName: "", EventID: 1, Status: status, StartEPG: time.Time{}}
	}
	reserveOf := func(id uint32, eventID uint16, comment string, mode uint8) *edcb.Reserve {
		return &edcb.Reserve{
			ServiceKey: nhkKey, Title: "こたつ特集", Start: now.Add(2 * time.Hour), Duration: 30 * time.Minute, StationName: "", EventID: eventID,
			Comment: comment, ID: id, RecSetting: edcb.RecSetting{RecMode: mode, Priority: 2},
		}
	}
	past := now.Add(-24 * time.Hour)
	bsKey := edcb.ServiceKey{ONID: 4, TSID: 16625, SID: 101}
	bs := func(r *edcb.Reserve) *edcb.Reserve {
		r.ServiceKey = bsKey

		return r
	}
	bsRecorded := func(f *edcb.RecordedFile) *edcb.RecordedFile {
		f.ServiceKey = bsKey

		return f
	}

	return &edcbimport.Snapshot{
		AutoAdds: []*edcb.AutoAdd{
			autoAdd(func(*edcb.AutoAdd) {}),
			autoAdd(func(a *edcb.AutoAdd) { a.ID, a.Search.Services = 8, []uint64{} }),
		},
		Reserves: []*edcb.Reserve{
			reserveOf(1, eventID, "", 1),
			// キーワード予約から作った予約は、ルールから作り直すので飛ばす。
			reserveOf(2, eventID, "EPG自動予約(ニュース)", 1),
			reserveOf(3, 0xFFFF, "", 1),
			reserveOf(4, eventID, "", 4),
			// Kotatsu にない局の予約は飛ばす。
			bs(reserveOf(5, eventID, "", 1)),
		},
		Recorded: []*edcb.RecordedFile{
			recorded(1, "こたつ特集 #1", 1, past),
			// 同じ局と開始の録画済みは、先のものだけを使う。
			recorded(2, "こたつ特集 #1", 1, past),
			recorded(3, "こたつ特集 #2", 7, past.Add(time.Hour)),
			recorded(4, "", 1, past.Add(2*time.Hour)),
			// Kotatsu にない局の録画済みは飛ばす。
			bsRecorded(recorded(5, "BS", 1, past)),
		},
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	s := snapshot(f.clock.Now())

	// dry run では、取り込んだ数を数えるが、書き込まない。
	plan, err := f.importer.Apply(t.Context(), f.pool, s, &edcbimport.ApplyOptions{UserID: uuid.Nil(), DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{edcbimport.KindRecordedHistory: 1, edcbimport.KindRule: 1, edcbimport.KindReserve: 1}, plan.Imported)
	for _, table := range []string{"rules", "reservations", "recording_history", "import_sources"} {
		assert.Zero(t, f.count(t, table), table)
	}

	result, err := f.importer.Apply(t.Context(), f.pool, s, &edcbimport.ApplyOptions{UserID: f.userID, DryRun: false})
	require.NoError(t, err)
	assert.Equal(t, plan.Imported, result.Imported)
	assert.Equal(t, 1, result.SkippedAutoReserves)
	skipped := map[string][]int64{}
	for _, sk := range result.Skipped {
		skipped[sk.Kind] = append(skipped[sk.Kind], sk.SourceID)
	}
	assert.Equal(t, map[string][]int64{
		edcbimport.KindRecordedHistory: {3, 4, 5},
		edcbimport.KindRule:            {8},
		edcbimport.KindReserve:         {3, 4, 5},
	}, skipped)
	assert.Equal(t, 1, f.count(t, "rules"))
	assert.Equal(t, 1, f.count(t, "reservations"))
	assert.Equal(t, 1, f.count(t, "recording_history"))

	// もう一度取り込んでも、二重に作らない。
	again, err := f.importer.Apply(t.Context(), f.pool, s, &edcbimport.ApplyOptions{UserID: f.userID, DryRun: false})
	require.NoError(t, err)
	assert.Empty(t, again.Imported)
	assert.Equal(t, plan.Imported, again.AlreadyImported)
	assert.Equal(t, 1, f.count(t, "rules"))
}

// busyReader は、最初の busy 回は忙しいと答える。
type busyReader struct {
	busy  int
	calls int
}

func (r *busyReader) AutoAdds(context.Context) ([]*edcb.AutoAdd, error) {
	r.calls++
	if r.calls <= r.busy {
		return nil, edcb.ErrBusy
	}

	return []*edcb.AutoAdd{}, nil
}

func (*busyReader) Reserves(context.Context) ([]*edcb.Reserve, error) { return []*edcb.Reserve{}, nil }

func (*busyReader) RecordedFiles(context.Context) ([]*edcb.RecordedFile, error) {
	return []*edcb.RecordedFile{}, nil
}

func TestReadRetriesWhileBusy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		busy    int
		wantErr bool
	}{
		{name: "busy for a while", busy: 2, wantErr: false},
		// 送り直しの回数を使い切ったら、理由を返して始めない。
		{name: "busy too long", busy: 4, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := &busyReader{busy: tt.busy, calls: 0}
			_, err := edcbimport.Read(t.Context(), reader, slog.New(slog.DiscardHandler), &edcbimport.ReadOptions{BusyWait: time.Millisecond, BusyRetries: 3})
			if tt.wantErr {
				require.ErrorIs(t, err, edcb.ErrBusy)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.busy+1, reader.calls)
		})
	}
}
