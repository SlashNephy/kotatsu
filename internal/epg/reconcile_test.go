package epg_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/lease"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// fixtureServices と fixturePrograms は、faketuner.DefaultOptions のサービスと番組の数。
	fixtureServices = 5
	fixturePrograms = fixtureServices * 14
	// grNetworkID と grServiceID は、DefaultOptions の最初のサービス (GR 27 の「こたつテスト1」)。
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
)

// testNow は、番組の境目から 10 分過ぎた時刻。DefaultOptions は、境目の 1 時間前 (11:00) から 30 分ごとに番組を並べる。
// eventId 1 と 2 は放送が終わり、3 は放送中で、4 以降はこれから放送される。
func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

type fixture struct {
	pool       *pgxpool.Pool
	fake       *faketuner.Server
	client     *mirakurun.Client
	reconciler *epg.Reconciler
}

func newFixture(t *testing.T, guard epg.Guard) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	fake := faketuner.New(faketuner.DefaultOptions(testNow(), nil))
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	if guard == nil {
		guard = func(context.Context, pgx.Tx) error { return nil }
	}

	return &fixture{
		pool:       pool,
		fake:       fake,
		client:     client,
		reconciler: epg.NewReconciler(pool, client, clocktest.NewFake(testNow()), guard),
	}
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
}

func programID(eventID int) int64 {
	return faketuner.ProgramID(grNetworkID, grServiceID, eventID)
}

// programStart は、DefaultOptions の eventId の番組の開始時刻を返す。eventId 1 は 11:00 に始まる。
func programStart(eventID int) time.Time {
	return time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC).Add(time.Duration(eventID-1) * 30 * time.Minute)
}

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

func TestSyncAllInsertsEverythingFirst(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	assert.Equal(t, fixtureServices, result.ServicesUpserted)
	assert.Equal(t, fixturePrograms, result.ProgramsUpserted)
	assert.Zero(t, result.ProgramsDeleted)
	assert.Positive(t, result.Version)
	assert.Equal(t, fixturePrograms, f.count(t, "SELECT count(*) FROM programs"))
	// 種別の既定値は、同期で見つけた種別についてだけ足す。
	assert.Equal(t, 2, f.count(t, "SELECT count(*) FROM channel_types WHERE (type, display_name, sort_order) IN (('GR', '地上波', 10), ('BS', 'BS', 20))"))
	// 検索用の列は NFKC で正規化する。
	assert.Equal(t, fixtureServices, f.count(t, "SELECT count(*) FROM services WHERE name_normalized = normalize(name, NFKC)"))
	// 変更の記録は、変わったサービスごとに 1 行。
	assert.Equal(t, fixtureServices, f.count(t, "SELECT count(*) FROM epg_changes WHERE version = $1", result.Version))
}

func TestSyncAllWithoutChangesDoesNotNotify(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 0, ProgramsDeleted: 0, Version: 0}, result)
}

// TestSyncAllReflectsChangesWithoutEvents は、イベントで届かない変更も、全量の突き合わせで反映することを確かめる (設計書 3.3)。
func TestSyncAllReflectsChangesWithoutEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// change は、上流の EPG を変える。
		change      func(t *testing.T, fake *faketuner.Server)
		want        *epg.Result
		wantProgram func(t *testing.T, f *fixture)
		// wantRange は、変更の記録に残る時間帯。
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name: "future program removed by GC is deleted",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.ForgetProgram(programID(5)))
			},
			want: &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 0, ProgramsDeleted: 1},
			wantProgram: func(t *testing.T, f *fixture) {
				t.Helper()
				assert.Zero(t, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 5", serviceID()))
			},
			wantStart: programStart(5),
			wantEnd:   programStart(6),
		},
		{
			name: "program on air is kept until archived",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.ForgetProgram(programID(3)))
			},
			want: &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 0, ProgramsDeleted: 0},
			wantProgram: func(t *testing.T, f *fixture) {
				t.Helper()
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 3", serviceID()))
			},
		},
		{
			name: "rescheduled program is updated in place",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.RescheduleProgram(programID(6), testNow().Add(3*time.Hour), time.Hour))
			},
			want: &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 1, ProgramsDeleted: 0},
			wantProgram: func(t *testing.T, f *fixture) {
				t.Helper()
				assert.Equal(t, 1, f.count(t,
					"SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 6 AND start_at = $2 AND end_at = $3",
					serviceID(), testNow().Add(3*time.Hour), testNow().Add(4*time.Hour)))
			},
			// 変更の前 (13:30 からの 30 分) と後 (15:10 からの 1 時間) の両方を含む。
			wantStart: programStart(6),
			wantEnd:   testNow().Add(4 * time.Hour),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)
			tt.change(t, f.fake)

			result, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)
			tt.want.Version = result.Version
			assert.Equal(t, tt.want, result)
			tt.wantProgram(t, f)
			if tt.wantStart.IsZero() {
				assert.Zero(t, result.Version, "nothing changed, so it must not notify")

				return
			}
			assert.Equal(t, 1, f.count(t,
				"SELECT count(*) FROM epg_changes WHERE version = $1 AND service_id = $2 AND start_at = $3 AND end_at = $4",
				result.Version, serviceID(), tt.wantStart, tt.wantEnd))
		})
	}
}

// TestSyncAllKeepsAdministratorSettings は、管理者が画面から変えた値を同期が上書きしないことを確かめる。
func TestSyncAllKeepsAdministratorSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE channel_types SET display_name = 'テレビ', sort_order = 1 WHERE type = 'GR'")
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE services SET hidden = true, sort_order = 5, name = '古い名前' WHERE id = $1", serviceID())
	require.NoError(t, err)

	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	// サービス名は上流の値に戻すが、非表示の印と並び順は残す。
	assert.Equal(t, 1, result.ServicesUpserted)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM channel_types WHERE type = 'GR' AND display_name = 'テレビ' AND sort_order = 1"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM services WHERE id = $1 AND hidden AND sort_order = 5 AND name = 'こたつテスト1'", serviceID()))
}

func TestSyncServicesOnlyTouchesGivenServices(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	other := faketuner.ProgramID(grNetworkID+1, 0x0408, 5)
	require.NoError(t, f.fake.ForgetProgram(programID(5)))
	require.NoError(t, f.fake.ForgetProgram(other))
	require.NoError(t, f.fake.ExtendProgram(programID(4), 15*time.Minute))

	// 同じサービスを重ねて指定しても、1 回だけ突き合わせる。DB に無いサービスは飛ばす。
	result, err := f.reconciler.SyncServices(t.Context(), []int64{serviceID(), faketuner.ServiceItemID(1, 1), serviceID()})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ProgramsUpserted)
	assert.Equal(t, 1, result.ProgramsDeleted)
	// 指定しなかったサービスの番組は、上流から消えていても残す。
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM programs WHERE mirakurun_id = $1", other))
}

// TestSyncRejectsWritesFromOldLeader は、リーダーでなくなった neko の同期が DB を書き換えないことを確かめる (設計書 1.4)。
func TestSyncRejectsWritesFromOldLeader(t *testing.T) {
	t.Parallel()

	var manager *lease.Manager
	var generation int64
	f := newFixture(t, func(ctx context.Context, tx pgx.Tx) error {
		return manager.Guard(ctx, tx, generation)
	})
	clk := clocktest.NewFake(testNow())
	manager = lease.NewManager(f.pool, clk, "neko", [16]byte{1}, 15*time.Second)
	var acquired bool
	var err error
	generation, acquired, err = manager.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, acquired)

	// 別のインスタンスがリースを奪い、世代が進む。
	clk.Advance(time.Minute)
	other := lease.NewManager(f.pool, clk, "neko", [16]byte{2}, 15*time.Second)
	_, acquired, err = other.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, acquired)

	_, err = f.reconciler.SyncAll(t.Context())
	require.ErrorIs(t, err, lease.ErrLost)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM programs"))
	assert.Zero(t, f.count(t, "SELECT count(*) FROM services"))
}

// TestSyncAllMarksUndecidedEnd は、終了時刻が未定の番組を、上流ごとの表し方によらず未定として記録することを確かめる。
// 長さは、Mirakurun が 1 ミリ秒、Mahiron が 0 で返す (#23 の実測)。
func TestSyncAllMarksUndecidedEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		duration      int64
		wantUndecided bool
	}{
		{name: "Mirakurun returns 1 ms", duration: 1, wantUndecided: true},
		{name: "Mahiron returns 0", duration: 0, wantUndecided: true},
		{name: "decided end", duration: int64(time.Hour / time.Millisecond), wantUndecided: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.fake.AddProgram(&faketuner.Program{
				ID: programID(200), EventID: 200, ServiceID: grServiceID, NetworkID: grNetworkID,
				StartAt: testNow().Add(-time.Hour).UnixMilli(), Duration: tt.duration,
				IsFree: true, Name: "延長中の番組", Description: "",
			})
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)

			assert.Equal(t, 1, f.count(t,
				"SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 200 AND end_undecided = $2",
				serviceID(), tt.wantUndecided))
		})
	}
}
