package rule_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// grNetworkID と grServiceID は、faketuner.DefaultOptions の最初のサービス (こたつテスト1)。subServiceID は同じチャンネルのサブチャンネル。
	grNetworkID  = 0x7FE0
	grServiceID  = 0x0400
	subServiceID = 0x0401
)

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

type engineFixture struct {
	pool         *pgxpool.Pool
	fake         *faketuner.Server
	clock        *clocktest.Fake
	reconciler   *epg.Reconciler
	rules        *rule.Rules
	reservations *reserve.Reservations
	engine       *rule.Engine
	user         *audit.Actor
}

// newEngineFixture は、偽チューナーサーバーの EPG を 12:10 の時点で同期した DB を用意する。
// こたつテスト1 の番組は、eventId 1 が 11:00 に始まり、30 分ごとに並ぶ。eventId 4 以降が、これから放送される番組である。
func newEngineFixture(t *testing.T, extra ...*faketuner.Program) *engineFixture {
	t.Helper()

	pool := migratedPool(t)
	options := faketuner.DefaultOptions(testNow(), nil)
	options.Programs = append(options.Programs, extra...)
	fake := faketuner.New(options)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	clk := clocktest.NewFake(testNow())
	noGuard := func(context.Context, pgx.Tx) error { return nil }
	reconciler := epg.NewReconciler(pool, client, clk, noGuard)
	_, err = reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	recorder := audit.NewRecorder(clk)
	reservations := reserve.NewReservations(clk, recorder)

	return &engineFixture{
		pool:         pool,
		fake:         fake,
		clock:        clk,
		reconciler:   reconciler,
		rules:        rule.NewRules(clk, recorder),
		reservations: reservations,
		engine:       rule.NewEngine(pool, clk, location, func(context.Context, pgx.Tx) error { return nil }, reservations),
		user:         newUser(t, pool),
	}
}

// createRule は、こたつテスト1 の番組名が pattern に一致する番組を予約するルールを作る。
func (f *engineFixture) createRule(t *testing.T, pattern string, modify func(in *rule.Input)) *rule.Rule {
	t.Helper()

	in := &rule.Input{
		Name:    pattern,
		Memo:    "",
		Enabled: true,
		Conditions: &match.Conditions{
			Keywords:   []*match.Keyword{{Text: pattern, Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
			ServiceIDs: []int64{serviceID()},
		},
		Overrides:           nil,
		ValidFrom:           nil,
		ValidUntil:          nil,
		AvoidDuplicates:     false,
		DuplicateWindowDays: nil,
		FollowEventRelay:    true,
	}
	if modify != nil {
		modify(in)
	}
	created, err := f.rules.Create(t.Context(), f.pool, f.user, in)
	require.NoError(t, err)

	return created
}

func (f *engineFixture) run(t *testing.T) *rule.Result {
	t.Helper()

	result, err := f.engine.Run(t.Context())
	require.NoError(t, err)

	return result
}

// reserved は、予約した番組の eventId と、予約が有効かどうかを返す。
func (f *engineFixture) reserved(t *testing.T) map[int64]bool {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT event_id, enabled FROM reservations WHERE service_id = $1", serviceID())
	require.NoError(t, err)
	defer rows.Close()
	reserved := map[int64]bool{}
	for rows.Next() {
		var (
			eventID int64
			enabled bool
		)
		require.NoError(t, rows.Scan(&eventID, &enabled))
		reserved[eventID] = enabled
	}
	require.NoError(t, rows.Err())

	return reserved
}

func (f *engineFixture) reservationOf(t *testing.T, eventID int64) *reserve.Reservation {
	t.Helper()

	var reservationID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM reservations WHERE service_id = $1 AND event_id = $2", serviceID(), eventID).Scan(&reservationID))
	reservation, err := f.reservations.Get(t.Context(), f.pool, reservationID)
	require.NoError(t, err)

	return reservation
}

func (f *engineFixture) linkedRules(t *testing.T, eventID int64) []uuid.UUID {
	t.Helper()

	rows, err := f.pool.Query(t.Context(),
		"SELECT rr.rule_id FROM reservation_rules rr JOIN reservations r ON r.id = rr.reservation_id WHERE r.service_id = $1 AND r.event_id = $2 ORDER BY rr.rule_id",
		serviceID(), eventID)
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)

	return ids
}

func TestEngineIsIdempotent(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	f.createRule(t, `テスト番組 (5|6)$`, nil)

	assert.Equal(t, &rule.Result{Created: 2, Deleted: 0, Linked: 2, Unlinked: 0}, f.run(t))
	want := map[int64]bool{5: true, 6: true}
	assert.Equal(t, want, f.reserved(t))
	assert.Equal(t, reserve.KindRule, f.reservationOf(t, 5).Kind)

	// 同じ EPG とルールで何度照合しても、予約の集合は変わらない。
	for range 3 {
		assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0}, f.run(t))
		assert.Equal(t, want, f.reserved(t))
	}
}

// TestEngineDeletesReservationsThatNoLongerMatch は、どのルールにも一致しなくなった録画前の予約を消し、
// 放送が始まった予約は残すことを確かめる。
func TestEngineDeletesReservationsThatNoLongerMatch(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 (5|6)$`, nil)
	f.run(t)

	// eventId 5 (13:00〜) の放送が始まった後に、ルールがどちらにも一致しなくなる。
	f.clock.Advance(time.Hour)
	in := &rule.Input{
		Name: r.Name, Memo: "", Enabled: true,
		Conditions: &match.Conditions{Keywords: []*match.Keyword{{Text: "一致しない", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}}},
		Overrides:  nil, ValidFrom: nil, ValidUntil: nil, AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true,
	}
	_, err := f.rules.Update(t.Context(), f.pool, f.user, r.ID, in)
	require.NoError(t, err)

	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
}

// TestEngineDoesNotRecreateDisabledReservations は、ユーザーが削除したルール予約を無効にして残し、次の照合で作り直さないことを確かめる。
func TestEngineDoesNotRecreateDisabledReservations(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 5$`, nil)
	f.run(t)

	require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, f.reservationOf(t, 5).ID))
	assert.Equal(t, map[int64]bool{5: false}, f.reserved(t))

	assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0}, f.run(t))
	assert.Equal(t, map[int64]bool{5: false}, f.reserved(t))

	// ルールを消すと、無効にした予約も消える。
	require.NoError(t, f.rules.Delete(t.Context(), f.pool, f.user, r.ID))
	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Empty(t, f.reserved(t))
}

func TestEngineIgnoresProgramsWithUndecidedEnd(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t, &faketuner.Program{
		ID: faketuner.ProgramID(grNetworkID, grServiceID, 100), EventID: 100, ServiceID: grServiceID, NetworkID: grNetworkID,
		// 終了時刻が未定の番組は、Mirakurun が長さを 1 ミリ秒にして返す。
		StartAt: testNow().Add(3 * time.Hour).UnixMilli(), Duration: 1,
		IsFree: true, Name: "延長中の番組", Description: "", RelatedItems: nil,
	})
	f.createRule(t, `延長中の番組`, nil)

	assert.Zero(t, f.run(t).Created)
	assert.Empty(t, f.reserved(t))
}

// TestEngineAvoidsDuplicates は、同じ番組名を期間内に録画済みなら予約しないことを確かめる (設計書 4.4)。
func TestEngineAvoidsDuplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		avoid  bool
		window *int
		want   map[int64]bool
	}{
		{name: "recorded within the window", avoid: true, window: new(7), want: map[int64]bool{6: true}},
		{name: "recorded before the window", avoid: true, window: new(1), want: map[int64]bool{5: true, 6: true}},
		{name: "unlimited window", avoid: true, window: nil, want: map[int64]bool{6: true}},
		{name: "avoidance disabled", avoid: false, window: nil, want: map[int64]bool{5: true, 6: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			// 2 日前に、表記の違う同じ番組名 (囲み文字と全角の数字) を録画した。
			_, err := f.pool.Exec(t.Context(),
				"INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)",
				id.New(), serviceID(), rule.TitleKey("🈟こたつテスト1 テスト番組 ５"), "🈟こたつテスト1 テスト番組 ５",
				testNow().Add(-48*time.Hour), testNow().Add(-47*time.Hour))
			require.NoError(t, err)
			f.createRule(t, `テスト番組 (5|6)$`, func(in *rule.Input) {
				in.AvoidDuplicates = tt.avoid
				in.DuplicateWindowDays = tt.window
			})

			f.run(t)
			assert.Equal(t, tt.want, f.reserved(t))
		})
	}
}

// TestEngineKeepsOneReservationForMultipleRules は、複数のルールに一致した番組の予約を 1 件にまとめ、
// 一致したルールを reservation_rules に残すことを確かめる。手動の予約は、手動のまま関係だけを持つ。
func TestEngineKeepsOneReservationForMultipleRules(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	manual, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, &reserve.ProgramInput{
		ServiceID: serviceID(), EventID: 6, FollowEventRelay: false, Overrides: &reserve.Overrides{Priority: new(5)},
	})
	require.NoError(t, err)
	first := f.createRule(t, `テスト番組 (5|6)$`, nil)
	second := f.createRule(t, `テスト番組 5$`, nil)

	result := f.run(t)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 3, result.Linked)
	assert.Len(t, f.reserved(t), 2)
	assert.ElementsMatch(t, []uuid.UUID{first.ID, second.ID}, f.linkedRules(t, 5))
	assert.Equal(t, []uuid.UUID{first.ID}, f.linkedRules(t, 6))
	// 手動の予約は、手動の設定のまま残る。
	got := f.reservationOf(t, 6)
	assert.Equal(t, manual.ID, got.ID)
	assert.Equal(t, reserve.KindProgram, got.Kind)
	assert.Equal(t, new(5), got.Overrides.Priority)

	// ルールが一致しなくなっても、手動の予約は消さず、関係だけを除く。
	require.NoError(t, f.rules.Delete(t.Context(), f.pool, f.user, first.ID))
	result = f.run(t)
	assert.Equal(t, 0, result.Deleted)
	assert.Equal(t, map[int64]bool{5: true, 6: true}, f.reserved(t))
	assert.Equal(t, []uuid.UUID{second.ID}, f.linkedRules(t, 5))
	assert.Empty(t, f.linkedRules(t, 6))
}

// TestEnginePicksOneOfSharedEvents は、同じイベントを複数のサービスで放送している番組が一致したとき、1 つだけを予約することを確かめる。
func TestEnginePicksOneOfSharedEvents(t *testing.T) {
	t.Parallel()

	start := testNow().Add(5 * time.Hour).UnixMilli()
	shared := func(serviceID, otherServiceID int) *faketuner.Program {
		return &faketuner.Program{
			ID: faketuner.ProgramID(grNetworkID, serviceID, 200), EventID: 200, ServiceID: serviceID, NetworkID: grNetworkID,
			StartAt: start, Duration: time.Hour.Milliseconds(), IsFree: true, Name: "共有イベント", Description: "",
			RelatedItems: []*faketuner.RelatedItem{{Type: "shared", NetworkID: 0, ServiceID: otherServiceID, EventID: 200}},
		}
	}
	f := newEngineFixture(t, shared(grServiceID, subServiceID), shared(subServiceID, grServiceID))
	f.createRule(t, `共有イベント`, func(in *rule.Input) {
		in.Conditions.ServiceIDs = []int64{serviceID(), faketuner.ServiceItemID(grNetworkID, subServiceID)}
	})

	assert.Equal(t, 1, f.run(t).Created)
	var serviceIDs []int64
	rows, err := f.pool.Query(t.Context(), "SELECT service_id FROM reservations")
	require.NoError(t, err)
	serviceIDs, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	assert.Equal(t, []int64{serviceID()}, serviceIDs)
}
