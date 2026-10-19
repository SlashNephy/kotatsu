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
func newEngineFixture(tb testing.TB, extra ...*faketuner.Program) *engineFixture {
	tb.Helper()

	pool := migratedPool(tb)
	options := faketuner.DefaultOptions(testNow(), nil)
	options.Programs = append(options.Programs, extra...)
	fake := faketuner.New(options)
	server := httptest.NewServer(fake)
	tb.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(tb, err)

	clk := clocktest.NewFake(testNow())
	noGuard := func(context.Context, pgx.Tx) error { return nil }
	reconciler := epg.NewReconciler(pool, client, clk, noGuard)
	_, err = reconciler.SyncAll(tb.Context())
	require.NoError(tb, err)

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(tb, err)
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
		user:         newUser(tb, pool),
	}
}

// createRule は、こたつテスト1 の番組名が pattern に一致する番組を予約するルールを作る。
func (f *engineFixture) createRule(tb testing.TB, pattern string, modify func(in *rule.Input)) *rule.Rule {
	tb.Helper()

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
	created, err := f.rules.Create(tb.Context(), f.pool, f.user, in)
	require.NoError(tb, err)

	return created
}

func (f *engineFixture) run(tb testing.TB) *rule.Result {
	tb.Helper()

	result, err := f.engine.Run(tb.Context())
	require.NoError(tb, err)

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

	assert.Equal(t, &rule.Result{Created: 2, Deleted: 0, Linked: 2, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, f.run(t))
	want := map[int64]bool{5: true, 6: true}
	assert.Equal(t, want, f.reserved(t))
	assert.Equal(t, reserve.KindRule, f.reservationOf(t, 5).Kind)

	// 同じ EPG とルールで何度照合しても、予約の集合は変わらない。
	for range 3 {
		assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, f.run(t))
		assert.Equal(t, want, f.reserved(t))
	}
}

// TestEngineFollowsEventRelaySettingOfRules は、ルール予約のイベントリレーへの追従を、一致したルールに合わせることを確かめる。
func TestEngineFollowsEventRelaySettingOfRules(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	f.createRule(t, `テスト番組 5$`, func(in *rule.Input) { in.FollowEventRelay = false })
	f.run(t)
	assert.False(t, f.reservationOf(t, 5).FollowEventRelay)

	// 追従するルールも一致すると、追従するようになる。
	f.createRule(t, `テスト番組 (5|6)$`, func(in *rule.Input) { in.FollowEventRelay = true })
	f.run(t)
	assert.True(t, f.reservationOf(t, 5).FollowEventRelay)
}

// unmatchedInput は、ルール r を、どの番組にも一致しない条件に変える入力を返す。
func unmatchedInput(r *rule.Rule) *rule.Input {
	return &rule.Input{
		Name: r.Name, Memo: "", Enabled: true,
		Conditions: &match.Conditions{Keywords: []*match.Keyword{{Text: "一致しない", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}}},
		Overrides:  nil, ValidFrom: nil, ValidUntil: nil, AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true,
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
	in := unmatchedInput(r)
	_, err := f.rules.Update(t.Context(), f.pool, f.user, r.ID, in)
	require.NoError(t, err)

	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
}

// TestEngineKeepsReservationsWhoseRecordingStarted は、放送の前でも、録画が始まった予約は、ルールに一致しなくなっても消さないことを確かめる (#304)。
// 録画は、放送の開始マージンとチューニングの時間だけ前に始まる。
func TestEngineKeepsReservationsWhoseRecordingStarted(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 (5|6)$`, nil)
	f.run(t)

	// eventId 5 の放送の 20 秒前に、録画が始まっている。予定の録画の開始 (15 秒前) より、チューニングの時間だけ前である。
	reservation := f.reservationOf(t, 5)
	f.clock.Advance(reservation.StartAt.Add(-20 * time.Second).Sub(f.clock.Now()))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, $3, 5, '', '', $4, $4, 'recording', $5, $5, $5)`, id.New(), reservation.ID, serviceID(), reservation.StartAt, f.clock.Now())
	require.NoError(t, err)
	// ルールに一致したままなら、何も変えない。録画中の予約を作り直そうとしない。
	assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, f.run(t))

	in := unmatchedInput(r)
	_, err = f.rules.Update(t.Context(), f.pool, f.user, r.ID, in)
	require.NoError(t, err)

	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
}

// TestEngineKeepsRecordingReservationsOfDeletedRules は、ルールを消しても、そのルールで録画中の予約は消さず、録画を最後まで録らせることを確かめる (#471)。
func TestEngineKeepsRecordingReservationsOfDeletedRules(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 (5|6)$`, nil)
	f.run(t)
	reservation := f.reservationOf(t, 5)
	f.clock.Advance(reservation.StartAt.Add(-20 * time.Second).Sub(f.clock.Now()))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, $3, 5, '', '', $4, $4, 'recording', $5, $5, $5)`, id.New(), reservation.ID, serviceID(), reservation.StartAt, f.clock.Now())
	require.NoError(t, err)

	require.NoError(t, f.rules.Delete(t.Context(), f.pool, f.user, r.ID))

	// 録画していない予約 (eventId 6) だけを消す。
	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
}

// TestManualReservationOverridesRuleReservation は、ルール予約がある番組を手動で予約すると、手動の番組予約に切り替わり、
// 手動の設定が残ることと、ルールに一致しなくなっても消えないことを確かめる (設計書 4.1、#306)。
func TestManualReservationOverridesRuleReservation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// disable は、手動で予約する前に、ユーザーがルール予約を消して (無効にして) おくかどうか。
		disable bool
	}{
		{name: "有効なルール予約", disable: false},
		{name: "ユーザーが消したルール予約", disable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			r := f.createRule(t, `テスト番組 5$`, nil)
			f.run(t)
			ruleReservation := f.reservationOf(t, 5)
			require.Equal(t, reserve.KindRule, ruleReservation.Kind)
			if tt.disable {
				require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, ruleReservation.ID))
			}

			got, created, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, &reserve.ProgramInput{
				ServiceID: serviceID(), EventID: 5, FollowEventRelay: false, Overrides: &reserve.Overrides{EndMarginSeconds: new(120)},
			})
			require.NoError(t, err)
			assert.False(t, created)
			assert.Equal(t, ruleReservation.ID, got.ID)
			assert.Equal(t, reserve.KindProgram, got.Kind)
			assert.True(t, got.Enabled)
			assert.False(t, got.FollowEventRelay)
			assert.Equal(t, &reserve.Overrides{EndMarginSeconds: new(120)}, got.Overrides)

			// ルールに一致したまま照合しても、手動の予約のまま変えない。
			assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, f.run(t))
			assert.Equal(t, reserve.KindProgram, f.reservationOf(t, 5).Kind)

			// ルールに一致しなくなっても、手動の予約は残る。
			in := unmatchedInput(r)
			_, err = f.rules.Update(t.Context(), f.pool, f.user, r.ID, in)
			require.NoError(t, err)
			assert.Equal(t, 0, f.run(t).Deleted)
			assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
		})
	}
}

// TestEngineDoesNotRecreateDisabledReservations は、ユーザーが削除したルール予約を無効にして残し、次の照合で作り直さないことを確かめる。
func TestEngineDoesNotRecreateDisabledReservations(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 5$`, nil)
	f.run(t)

	require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, f.reservationOf(t, 5).ID))
	assert.Equal(t, map[int64]bool{5: false}, f.reserved(t))

	assert.Equal(t, &rule.Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, f.run(t))
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

// TestEngineKeepsReservationsWhileProgramIsUncertain は、番組の終了時刻が未定になったときや、番組が EPG から消えたときに、
// ルール予約を消さないことを確かめる (#518)。どちらも一時的なことがあり、消すと録り逃す。
func TestEngineKeepsReservationsWhileProgramIsUncertain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(t *testing.T, f *engineFixture)
	}{
		{name: "終了時刻が未定になった", change: func(t *testing.T, f *engineFixture) {
			t.Helper()

			program := f.reservationOf(t, 6)
			// 延長で終了時刻が未定になると、Mirakurun は長さを 1 ミリ秒にして返す。
			require.NoError(t, f.fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, 6), program.StartAt, time.Millisecond))
		}},
		{name: "番組が EPG から消えた", change: func(t *testing.T, f *engineFixture) {
			t.Helper()

			require.NoError(t, f.fake.ForgetProgram(faketuner.ProgramID(grNetworkID, grServiceID, 6)))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			f.createRule(t, `テスト番組 6$`, nil)
			f.run(t)
			reservation := f.reservationOf(t, 6)

			tt.change(t, f)
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)

			assert.Zero(t, f.run(t).Deleted)
			got := f.reservationOf(t, 6)
			assert.Equal(t, reservation.ID, got.ID)
		})
	}
}

// TestEngineKeepsDisabledReservationsOfExistingRules は、ユーザーが無効にしたルール予約を、ルールがある間は、
// 一時的にルールに一致しなくなっても消さないことを確かめる (#518)。消すと、また一致したときに、有効な予約として作り直してしまう。
func TestEngineKeepsDisabledReservationsOfExistingRules(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 5$`, nil)
	f.run(t)
	require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, f.reservationOf(t, 5).ID))

	// ルールを変えて一致しなくなっても、無効にした予約は残す。
	_, err := f.rules.Update(t.Context(), f.pool, f.user, r.ID, unmatchedInput(r))
	require.NoError(t, err)
	assert.Zero(t, f.run(t).Deleted)
	assert.Equal(t, map[int64]bool{5: false}, f.reserved(t))

	// ルールを戻しても、無効のまま。
	in := unmatchedInput(r)
	in.Conditions.Keywords[0].Text = `テスト番組 5$`
	in.Conditions.Keywords[0].Regexp = true
	in.Conditions.ServiceIDs = []int64{serviceID()}
	_, err = f.rules.Update(t.Context(), f.pool, f.user, r.ID, in)
	require.NoError(t, err)
	assert.Zero(t, f.run(t).Created)
	assert.Equal(t, map[int64]bool{5: false}, f.reserved(t))
}

// TestEngineDeletesUncertainReservationsOfDeletedRules は、番組の終了時刻が未定でも、ルールを消したら、そのルール予約を消すことを確かめる。
// 残すと、消したルールの予約で録画してしまう。
func TestEngineDeletesUncertainReservationsOfDeletedRules(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	r := f.createRule(t, `テスト番組 6$`, nil)
	f.run(t)
	program := f.reservationOf(t, 6)
	require.NoError(t, f.fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, 6), program.StartAt, time.Millisecond))
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	require.NoError(t, f.rules.Delete(t.Context(), f.pool, f.user, r.ID))
	assert.Equal(t, 1, f.run(t).Deleted)
	assert.Empty(t, f.reserved(t))
}

// TestEngineSkipsUnreadableRules は、条件を読めないルールがあっても、ほかのルールの照合を続け、読めないルールの予約は
// 消さずに残すことを確かめる (#529)。ローリング更新の途中で、新しい版が足した条件を古い版が読めなくても、照合が止まらないようにする。
func TestEngineSkipsUnreadableRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// update は、ルールの条件を読めないように書き換える SQL。$1 はルールの ID。
		update string
	}{
		{
			// 新しい版が、古い版の知らない条件を書いた。
			name:   "unknown condition",
			update: `UPDATE rules SET conditions = '{"version": 1, "conditions": {"unknownField": true}}' WHERE id = $1`,
		},
		{
			// 条件は読めるが、照合の準備ができない。
			name:   "invalid pattern",
			update: `UPDATE rules SET conditions = jsonb_set(conditions, '{conditions,keywords,0,text}', '"("') WHERE id = $1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			unreadable := f.createRule(t, `テスト番組 5$`, nil)
			f.run(t)
			require.Equal(t, map[int64]bool{5: true}, f.reserved(t))
			_, err := f.pool.Exec(t.Context(), tt.update, unreadable.ID)
			require.NoError(t, err)
			f.createRule(t, `テスト番組 6$`, nil)

			result := f.run(t)
			assert.Equal(t, 1, result.Created)
			assert.Zero(t, result.Deleted)
			assert.Equal(t, 1, result.Linked)
			assert.Zero(t, result.Unlinked)
			require.Len(t, result.Skipped, 1)
			require.Error(t, result.Skipped[unreadable.ID])
			assert.Equal(t, map[int64]bool{5: true, 6: true}, f.reserved(t))
			assert.Equal(t, []uuid.UUID{unreadable.ID}, f.linkedRules(t, 5))
		})
	}
}

// TestEngineMatchesNormalizedExtended は、詳細を、同期で正規化して持った列で照合することを確かめる (#530)。
// 列を足す前に書いた番組 (NULL) は、上流の詳細から正規化し直して照合する。
func TestEngineMatchesNormalizedExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// update は、eventId 5 の番組の詳細を書き換える SQL。$1 はサービスの ID。
		update string
	}{
		{
			name:   "normalized column",
			update: `UPDATE programs SET extended_normalized = '内容 ABC 特集' WHERE service_id = $1 AND event_id = 5`,
		},
		{
			name:   "written before the column",
			update: `UPDATE programs SET extended = '{"内容": "ＡＢＣ　特集"}', extended_normalized = NULL WHERE service_id = $1 AND event_id = 5`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			_, err := f.pool.Exec(t.Context(), tt.update, serviceID())
			require.NoError(t, err)
			f.createRule(t, "ABC 特集", func(in *rule.Input) {
				in.Conditions.Keywords[0].Regexp = false
				in.Conditions.Keywords[0].Fields = []match.Field{match.FieldExtended}
			})

			f.run(t)
			assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
		})
	}
}

// TestEngineMatchesMarksFromRawName は、印を、正規化する前の番組名から取り出して照合することを確かめる。
// 囲み文字を括弧の書き方にする前に正規化した番組名では、🈟 が「新」になり、番組名の文字と見分けられない。
func TestEngineMatchesMarksFromRawName(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	for _, update := range []string{
		`UPDATE programs SET name = '🈟番組', name_normalized = '新番組' WHERE service_id = $1 AND event_id = 5`,
		`UPDATE programs SET name = '新番組', name_normalized = '新番組' WHERE service_id = $1 AND event_id = 6`,
	} {
		_, err := f.pool.Exec(t.Context(), update, serviceID())
		require.NoError(t, err)
	}
	f.createRule(t, "番組", func(in *rule.Input) {
		in.Conditions.Marks = []string{"新"}
	})

	f.run(t)
	assert.Equal(t, map[int64]bool{5: true}, f.reserved(t))
}
