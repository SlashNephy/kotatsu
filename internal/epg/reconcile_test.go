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
	// 詳細も、照合のために正規化して持つ (#530)。
	assert.Equal(t, fixturePrograms, f.count(t, "SELECT count(*) FROM programs WHERE extended_normalized IS NOT NULL"))
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

// TestSyncAllStoresProgramAudios は、番組の音声の構成を保存し、変わらなければ書き直さないことを確かめる (#284)。
func TestSyncAllStoresProgramAudios(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	require.NoError(t, f.fake.ChangeProgramAudios(programID(6), []*faketuner.Audio{
		{ComponentType: 3, ComponentTag: new(16), IsMain: true, SamplingRate: 48000, Langs: []string{"jpn"}},
		{ComponentType: 3, ComponentTag: new(17), IsMain: false, SamplingRate: 48000, Langs: []string{"eng"}},
	}))

	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.ProgramsUpserted)
	var languages []string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT ARRAY(SELECT a->'langs'->>0 FROM jsonb_array_elements(audios) a) FROM programs WHERE service_id = $1 AND event_id = 6",
		serviceID()).Scan(&languages))
	assert.Equal(t, []string{"jpn", "eng"}, languages)

	// 音声の構成を DB から読み直しても、上流と同じとみなす。
	result, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 0, ProgramsDeleted: 0, Version: 0}, result)
}

// TestSyncAllStoresProgramVideoResolution は、番組の映像の解像度を保存し、変わらなければ書き直さないことを確かめる (#806)。
// BS4K と 8K の番組を見分けるのに使う。
func TestSyncAllStoresProgramVideoResolution(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	resolution := func() *string {
		var got *string
		require.NoError(t, f.pool.QueryRow(t.Context(),
			"SELECT video_resolution FROM programs WHERE service_id = $1 AND event_id = 6", serviceID()).Scan(&got))

		return got
	}
	assert.Equal(t, new("1080i"), resolution())

	// Mahiron は BS4K の番組の type を mpeg2 と返すので、解像度だけを持つ。
	require.NoError(t, f.fake.ChangeProgramVideo(programID(6), &faketuner.Video{Type: "mpeg2", Resolution: "2160p"}))
	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.ProgramsUpserted)
	assert.Equal(t, new("2160p"), resolution())

	// 解像度を DB から読み直しても、上流と同じとみなす。
	result, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, &epg.Result{ServicesUpserted: 0, ProgramsUpserted: 0, ProgramsDeleted: 0, Version: 0}, result)

	// 上流が映像を返さなくなったら、分からないものとして NULL にする。
	require.NoError(t, f.fake.ChangeProgramVideo(programID(6), nil))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Nil(t, resolution())
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

// TestSyncSkipsArchivablePrograms は、アーカイブへ移す時期を過ぎた番組を、上流が返しても programs に足さず、
// programs に残っていても消さない (Archiver に移させる) ことを確かめる。
func TestSyncSkipsArchivablePrograms(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	add := func(eventID int, ago time.Duration) {
		f.fake.AddProgram(&faketuner.Program{
			ID: programID(eventID), EventID: eventID, ServiceID: grServiceID, NetworkID: grNetworkID,
			StartAt: testNow().Add(-ago).UnixMilli(), Duration: int64(time.Hour / time.Millisecond),
			IsFree: true, Name: "終わった番組", Description: "",
		})
	}
	// 3 日前に終わった番組は、上流が返しても足さない。猶予 (24 時間) の内に終わった番組は足す。
	add(300, 3*24*time.Hour)
	add(301, 2*time.Hour)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	countOf := func(eventID int) int {
		return f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = $2", serviceID(), eventID)
	}
	assert.Equal(t, 0, countOf(300))
	assert.Equal(t, 1, countOf(301))

	// programs に残っている、移す時期を過ぎた番組は、上流から消えても消さない。
	_, err = f.pool.Exec(t.Context(), `UPDATE programs SET start_at = start_at - interval '2 days', end_at = end_at - interval '2 days'
		WHERE service_id = $1 AND event_id = 301`, serviceID())
	require.NoError(t, err)
	require.NoError(t, f.fake.ForgetProgram(programID(301)))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, countOf(301))
}

// onAirUpstream は、faketuner の番組表に、mirakc の /api/onair の代わりの放送中と次の番組を足した上流。
type onAirUpstream struct {
	*mirakurun.Client

	onair []*mirakurun.OnAirProgram
}

var _ epg.Upstream = new(onAirUpstream)

func (u *onAirUpstream) OnAirPrograms(context.Context) ([]*mirakurun.OnAirProgram, error) {
	return u.onair, nil
}

// TestSyncReflectsOnAirPrograms は、番組表 (EIT[schedule]) に、放送中と次の番組 (EIT[p/f]) を重ねて反映することを確かめる (設計書 5.1)。
// mirakc の /api/programs は、延長や繰り上げを、番組表を取り直すまで反映しない。
func TestSyncReflectsOnAirPrograms(t *testing.T) {
	t.Parallel()

	// 放送中の eventId 3 が延長して終了時刻が未定になり、その後に、番組表にない eventId 900 が差し込まれた。
	extended := &mirakurun.Program{
		ID: programID(3), EventID: 3, ServiceID: grServiceID, NetworkID: grNetworkID,
		StartAt: programStart(3).UnixMilli(), Duration: 1, IsFree: true, Name: "終了時刻が未定の中継",
	}
	inserted := &mirakurun.Program{
		ID: programID(900), EventID: 900, ServiceID: grServiceID, NetworkID: grNetworkID,
		StartAt: programStart(4).UnixMilli(), Duration: int64(5 * time.Minute / time.Millisecond), IsFree: true, Name: "差し込まれた番組",
	}
	// 同期しないサービスの放送中の番組は、SyncServices では反映しない。
	otherService := faketuner.ServiceItemID(grNetworkID, grServiceID+1)
	other := &mirakurun.Program{
		ID: faketuner.ProgramID(grNetworkID, grServiceID+1, 901), EventID: 901, ServiceID: grServiceID + 1, NetworkID: grNetworkID,
		StartAt: programStart(3).UnixMilli(), Duration: 1, IsFree: true, Name: "ほかのサービスの番組",
	}

	tests := []struct {
		name      string
		sync      func(t *testing.T, r *epg.Reconciler)
		wantOther int
	}{
		{
			name: "SyncAll",
			sync: func(t *testing.T, r *epg.Reconciler) {
				t.Helper()
				_, err := r.SyncAll(t.Context())
				require.NoError(t, err)
			},
			wantOther: 1,
		},
		{
			name: "SyncServices",
			sync: func(t *testing.T, r *epg.Reconciler) {
				t.Helper()
				_, err := r.SyncServices(t.Context(), []int64{serviceID()})
				require.NoError(t, err)
			},
			wantOther: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			// サービスと番組表を先に入れておく。
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)

			upstream := &onAirUpstream{Client: f.client, onair: []*mirakurun.OnAirProgram{
				{ServiceID: serviceID(), Current: extended, Next: inserted},
				{ServiceID: otherService, Current: other, Next: nil},
			}}
			tt.sync(t, epg.NewReconciler(f.pool, upstream, clocktest.NewFake(testNow()), func(context.Context, pgx.Tx) error { return nil }))

			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 3 AND end_undecided", serviceID()))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 900", serviceID()))
			assert.Equal(t, tt.wantOther, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 901", otherService))
		})
	}
}

// dataServiceUpstream は、faketuner のサービスに、映像も音声も流さないデータ放送のサービス (種別 0xC0) を足した上流。
type dataServiceUpstream struct {
	*mirakurun.Client

	data *mirakurun.Service
}

var _ epg.Upstream = new(dataServiceUpstream)

func (u *dataServiceUpstream) Services(ctx context.Context) ([]*mirakurun.Service, error) {
	services, err := u.Client.Services(ctx)
	if err != nil {
		return nil, err
	}

	return append(services, u.data), nil
}

// TestSyncSkipsNonMediaServices は、映像も音声も流さないサービスとその番組を取り込まず、
// すでに DB にあるそのようなサービスの番組も、消さずに残すことを確かめる。
func TestSyncSkipsNonMediaServices(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	services, err := f.client.Services(t.Context())
	require.NoError(t, err)
	dataServiceID := grServiceID + 0x40
	data := &mirakurun.Service{
		ID: faketuner.ServiceItemID(grNetworkID, dataServiceID), ServiceID: dataServiceID, NetworkID: grNetworkID,
		Name: "データ放送", Type: 0xC0, LogoID: nil, RemoteControlKeyID: nil, Channel: services[0].Channel,
	}
	f.fake.AddProgram(&faketuner.Program{
		ID: faketuner.ProgramID(grNetworkID, dataServiceID, 500), EventID: 500, ServiceID: dataServiceID, NetworkID: grNetworkID,
		StartAt: testNow().Add(time.Hour).UnixMilli(), Duration: int64(time.Hour / time.Millisecond),
		IsFree: true, Name: "データ放送の番組", Description: "",
	})
	reconciler := epg.NewReconciler(f.pool, &dataServiceUpstream{Client: f.client, data: data}, clocktest.NewFake(testNow()),
		func(context.Context, pgx.Tx) error { return nil })
	_, err = reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM services WHERE id = $1", data.ID))
	assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1", data.ID))

	// 取り込みをやめる前に入ったサービスと番組は、同期しても消さない。
	_, err = f.pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel,
		remote_control_key_id, logo_id, created_at, updated_at)
		SELECT $1, network_id, $2, 'データ放送', 'データ放送', 192, channel_type, channel, remote_control_key_id, logo_id, created_at, updated_at
		FROM services LIMIT 1`, data.ID, dataServiceID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		SELECT $1, 501, 0, start_at, end_at, false, true, 'データ放送の番組', '', 'データ放送の番組', '', '{}', '[]', '[]', created_at, updated_at
		FROM programs WHERE end_at > $2 LIMIT 1`, data.ID, testNow())
	require.NoError(t, err)
	_, err = reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	_, err = reconciler.SyncServices(t.Context(), []int64{data.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 501", data.ID))
	assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 500", data.ID))
}

// TestSyncAllKeepsProgramsWhenUpstreamReturnsNone は、上流がサービスの番組を 1 つも返さなかったときは、
// そのサービスの番組を消さないことを確かめる (#518)。チューナーサーバーが一時的に空の一覧を返すと、未来の番組とルール予約が全部消えていた。
func TestSyncAllKeepsProgramsWhenUpstreamReturnsNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	before := f.count(t, "SELECT count(*) FROM programs")
	require.Positive(t, before)

	programs, err := f.client.Programs(t.Context(), nil)
	require.NoError(t, err)
	for _, program := range programs {
		require.NoError(t, f.fake.ForgetProgram(program.ID))
	}
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, f.count(t, "SELECT count(*) FROM programs"))
}

// TestSyncAllDeletesSomeVanishedPrograms は、サービスの番組の一部だけが上流から消えたときは、今までどおり消すことを確かめる。
func TestSyncAllDeletesSomeVanishedPrograms(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	before := f.count(t, "SELECT count(*) FROM programs")

	require.NoError(t, f.fake.ForgetProgram(programID(6)))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before-1, f.count(t, "SELECT count(*) FROM programs"))
}

// TestSyncServicesKeepsProgramsWhenUpstreamReturnsNone は、イベントでサービスの番組を突き合わせるときも、
// 上流がその番組を 1 つも返さなければ消さないことを確かめる (#518)。
func TestSyncServicesKeepsProgramsWhenUpstreamReturnsNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	before := f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1", serviceID())
	require.Positive(t, before)

	programs, err := f.client.Programs(t.Context(), nil)
	require.NoError(t, err)
	for _, program := range programs {
		if program.ServiceID == grServiceID && program.NetworkID == grNetworkID {
			require.NoError(t, f.fake.ForgetProgram(program.ID))
		}
	}
	_, err = f.reconciler.SyncServices(t.Context(), []int64{serviceID()})
	require.NoError(t, err)
	assert.Equal(t, before, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1", serviceID()))
}
