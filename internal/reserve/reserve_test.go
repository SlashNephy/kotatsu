package reserve_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// grNetworkID と grServiceID は、faketuner.DefaultOptions の最初のサービス。
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
	// futureEvent は、テストの時刻より後に放送される番組の eventId。
	futureEvent = 6
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

type fixture struct {
	pool         *pgxpool.Pool
	fake         *faketuner.Server
	reconciler   *epg.Reconciler
	reservations *reserve.Reservations
	user         *audit.Actor
}

// newFixture は、偽チューナーサーバーの EPG を同期した DB と、予約を作るユーザーを用意する。
func newFixture(t *testing.T) *fixture {
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
	clk := clocktest.NewFake(testNow())
	reconciler := epg.NewReconciler(pool, client, clk, func(context.Context, pgx.Tx) error { return nil })
	_, err = reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleMember, PasswordHash: nil,
	})
	require.NoError(t, err)

	return &fixture{
		pool:         pool,
		fake:         fake,
		reconciler:   reconciler,
		reservations: reserve.NewReservations(clk, audit.NewRecorder(clk)),
		user:         audit.User(user.ID),
	}
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
}

func programInput() *reserve.ProgramInput {
	return &reserve.ProgramInput{ServiceID: serviceID(), EventID: futureEvent, FollowEventRelay: true, Overrides: nil}
}

// TestCreateProgramKeepsOneReservationPerProgram は、同じ番組を同時に何度予約しても、予約は 1 件になることを確かめる。
func TestCreateProgramKeepsOneReservationPerProgram(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	const attempts = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ids     = map[uuid.UUID]bool{}
		created int
		errs    = []error{}
	)
	for range attempts {
		wg.Go(func() {
			reservation, ok, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			mu.Lock()
			defer mu.Unlock()
			// require は別の goroutine から呼べないので、エラーは集めて、待ち合わせた後に確かめる。
			if err != nil {
				errs = append(errs, err)

				return
			}
			ids[reservation.ID] = true
			if ok {
				created++
			}
		})
	}
	wg.Wait()

	require.Empty(t, errs)
	assert.Equal(t, 1, created)
	assert.Len(t, ids, 1)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM reservations"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'reservation.create'"))
}

// TestProgramReservationKeepsCopyAfterEPGRemoval は、番組が EPG から消えても、予約の番組情報を取得できることを確かめる。
func TestProgramReservationKeepsCopyAfterEPGRemoval(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	created, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)

	require.NoError(t, f.fake.ForgetProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent)))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	require.Zero(t, f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = $2", serviceID(), futureEvent))

	got, err := f.reservations.Get(t.Context(), f.pool, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "こたつテスト1 テスト番組 6", got.Name)
	assert.Equal(t, "偽のチューナーサーバーが合成した番組です。", got.Description)
	assert.Equal(t, time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC), got.StartAt)
	assert.Equal(t, time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC), got.EndAt)
}

func TestCreateProgramRejectsUnknownProgram(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	in := programInput()
	in.EventID = 9999
	_, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, in)
	require.ErrorIs(t, err, reserve.ErrProgramNotFound)
}

// TestOverridesApplyOnDefaults は、録画設定を、全体の既定値の上に予約ごとに上書きできることを確かめる。
func TestOverridesApplyOnDefaults(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	in := programInput()
	in.Overrides = &reserve.Overrides{
		PoolID:             new("archive"),
		FileNameTemplate:   new(`{{.Name}}`),
		StartMarginSeconds: new(60),
		EndMarginSeconds:   nil,
		Priority:           new(3),
	}
	created, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, in)
	require.NoError(t, err)

	got, err := f.reservations.Get(t.Context(), f.pool, created.ID)
	require.NoError(t, err)
	assert.Equal(t, &reserve.Settings{
		PoolID:             "archive",
		FileNameTemplate:   `{{.Name}}`,
		StartMarginSeconds: 60,
		// 上書きしなかった項目は、全体の既定値のまま。
		EndMarginSeconds: reserve.DefaultMarginSeconds,
		Priority:         3,
	}, got.Settings(reserve.DefaultSettings()))
}

func TestInvalidOverridesAreRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		overrides *reserve.Overrides
		wantErr   string
	}{
		{name: "unknown field in the file name template", overrides: &reserve.Overrides{FileNameTemplate: new(`{{.Title}}`)}, wantErr: "invalid file name template"},
		{name: "broken file name template", overrides: &reserve.Overrides{FileNameTemplate: new(`{{.Name`)}, wantErr: "invalid file name template"},
		{name: "invalid pool ID", overrides: &reserve.Overrides{PoolID: new("../main")}, wantErr: `poolId "../main"`},
		{name: "negative margin", overrides: &reserve.Overrides{EndMarginSeconds: new(-1)}, wantErr: "endMarginSeconds must be between 0 and 3600"},
		{name: "priority out of range", overrides: &reserve.Overrides{Priority: new(11)}, wantErr: "priority must be between -10 and 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.overrides.Validate()
			require.ErrorIs(t, err, reserve.ErrInvalidSettings)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCreateTimeValidatesSchedule(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	start := testNow().Add(time.Hour)
	tests := []struct {
		name    string
		in      *reserve.TimeInput
		wantErr error
	}{
		{name: "valid", in: &reserve.TimeInput{ServiceID: serviceID(), StartAt: start, EndAt: start.Add(time.Hour), Name: "朝の番組", Overrides: nil}},
		{name: "end before start", in: &reserve.TimeInput{ServiceID: serviceID(), StartAt: start, EndAt: start, Name: "", Overrides: nil}, wantErr: reserve.ErrInvalidReservation},
		{name: "longer than a day", in: &reserve.TimeInput{ServiceID: serviceID(), StartAt: start, EndAt: start.Add(25 * time.Hour), Name: "", Overrides: nil}, wantErr: reserve.ErrInvalidReservation},
		{name: "invalid overrides", in: &reserve.TimeInput{ServiceID: serviceID(), StartAt: start, EndAt: start.Add(time.Hour), Name: "", Overrides: &reserve.Overrides{Priority: new(100)}}, wantErr: reserve.ErrInvalidSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reservation, err := f.reservations.CreateTime(t.Context(), f.pool, f.user, tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, reserve.KindTime, reservation.Kind)
			assert.Nil(t, reservation.EventID)
			assert.Equal(t, tt.in.Name, reservation.Name)
		})
	}
}

// TestChangesAreAudited は、予約の作成、変更、削除が、操作者とともに監査ログに残ることを確かめる。
func TestChangesAreAudited(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)

	updated, err := f.reservations.Update(t.Context(), f.pool, f.user, reservation.ID, &reserve.Update{
		Enabled: new(false), FollowEventRelay: nil, Overrides: &reserve.Overrides{Priority: new(5)},
		StartAt: nil, EndAt: nil, Name: nil,
	})
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	assert.Equal(t, new(5), updated.Overrides.Priority)
	assert.Equal(t, &f.user.UserID, updated.UpdatedBy)

	// 番組予約の時刻は EPG に従うので、変えられない。
	_, err = f.reservations.Update(t.Context(), f.pool, f.user, reservation.ID, &reserve.Update{Name: new("別の名前")})
	require.ErrorIs(t, err, reserve.ErrInvalidReservation)

	require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, reservation.ID))
	_, err = f.reservations.Get(t.Context(), f.pool, reservation.ID)
	require.ErrorIs(t, err, reserve.ErrNotFound)

	for _, action := range []string{"reservation.create", "reservation.update", "reservation.delete"} {
		assert.Equal(t, 1, f.count(t,
			"SELECT count(*) FROM audit_events WHERE action = $1 AND target_type = 'reservation' AND target_id = $2 AND actor_user_id = $3",
			action, reservation.ID.String(), f.user.UserID), action)
	}
	assert.Equal(t, 1, f.count(t,
		"SELECT count(*) FROM audit_events WHERE action = 'reservation.update' AND summary -> 'changed' = '[\"enabled\", \"settings\"]'::jsonb"))
}

// TestSystemActorLeavesCreatorEmpty は、neko のようなシステムが作った予約では、作成者を空にすることを確かめる。
func TestSystemActorLeavesCreatorEmpty(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, audit.System("neko"), programInput())
	require.NoError(t, err)
	assert.Nil(t, reservation.CreatedBy)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE actor_type = 'system' AND actor_role = 'neko'"))
}
