package reserve_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

// 予約の名前。conflicting は割り当てが競合している予約、partial は一部のみ録画できる予約にするもの。
// disabled は、無効にした時刻予約。
const (
	first       = "event 5"
	conflicting = "event 6"
	partial     = "event 7"
	disabled    = "time"
)

// at は、テストの日の時刻を返す。
func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 10, hour, minute, 0, 0, time.UTC)
}

// listFixture は、13:00、13:30、14:00 に始まる番組予約 (eventId 5、6、7) と、13:00〜13:30 の時刻予約を持つ。
// 13:30 の番組予約だけ、割り当てが競合している。時刻予約だけ、無効にしてある。
type listFixture struct {
	*fixture

	// names は、予約の ID から、確かめやすい名前を引く。
	names map[uuid.UUID]string
	// ids は、名前から予約の ID を引く。
	ids map[string]uuid.UUID
}

func newListFixture(t *testing.T) *listFixture {
	t.Helper()

	f := &listFixture{fixture: newFixture(t), names: map[uuid.UUID]string{}, ids: map[string]uuid.UUID{}}
	for _, event := range []int64{5, 6, 7} {
		reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user,
			&reserve.ProgramInput{ServiceID: serviceID(), EventID: event, FollowEventRelay: true, Overrides: nil})
		require.NoError(t, err)
		f.add(reservation.ID, fmt.Sprintf("event %d", event))
	}
	reservation, err := f.reservations.CreateTime(t.Context(), f.pool, f.user,
		&reserve.TimeInput{ServiceID: serviceID(), StartAt: at(13, 0), EndAt: at(13, 30), Name: "時刻予約", Overrides: nil})
	require.NoError(t, err)
	f.add(reservation.ID, disabled)

	_, err = f.pool.Exec(t.Context(), `UPDATE reservations SET assignment_state = 'conflict' WHERE id = $1`, f.ids[conflicting])
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `UPDATE reservations SET enabled = false WHERE id = $1`, f.ids[disabled])
	require.NoError(t, err)

	return f
}

func (f *listFixture) add(reservationID uuid.UUID, name string) {
	f.names[reservationID] = name
	f.ids[name] = reservationID
}

func (f *listFixture) namesOf(reservations []*reserve.Reservation) []string {
	names := make([]string, 0, len(reservations))
	for _, r := range reservations {
		names = append(names, f.names[r.ID])
	}

	return names
}

func TestList(t *testing.T) {
	t.Parallel()

	f := newListFixture(t)
	// 13:00 に始まる 2 件は ID の順に並ぶ。ID は作った順に大きくなる。
	all := []string{first, disabled, conflicting, partial}

	tests := []struct {
		name    string
		query   reserve.ListQuery
		want    []string
		wantErr error
	}{
		{
			name:  "all reservations in the order of the start time",
			query: reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, Enabled: nil, AfterStartAt: nil, AfterID: nil, Limit: 10},
			want:  all,
		},
		{
			name:  "reservations overlapping the range",
			query: reserve.ListQuery{From: at(13, 30), To: at(14, 0), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 10},
			want:  []string{conflicting},
		},
		{
			name: "reservations in the states",
			query: reserve.ListQuery{
				From: at(0, 0), To: at(23, 0), States: []reserve.AssignmentState{reserve.AssignmentConflict}, AfterStartAt: nil, AfterID: nil, Limit: 10,
			},
			want: []string{conflicting},
		},
		{
			name:  "enabled reservations only",
			query: reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, Enabled: new(true), AfterStartAt: nil, AfterID: nil, Limit: 10},
			want:  []string{first, conflicting, partial},
		},
		{
			name:  "disabled reservations only",
			query: reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, Enabled: new(false), AfterStartAt: nil, AfterID: nil, Limit: 10},
			want:  []string{disabled},
		},
		{
			name:  "limit",
			query: reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 2},
			want:  all[:2],
		},
		{
			name:    "empty range",
			query:   reserve.ListQuery{From: at(13, 0), To: at(13, 0), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 10},
			wantErr: reserve.ErrInvalidReservation,
		},
		{
			name:    "cursor without the ID",
			query:   reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, AfterStartAt: new(at(13, 0)), AfterID: nil, Limit: 10},
			wantErr: reserve.ErrInvalidReservation,
		},
		{
			name:    "zero limit",
			query:   reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 0},
			wantErr: reserve.ErrInvalidReservation,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := f.reservations.List(t.Context(), f.pool, &tt.query)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, f.namesOf(got))
		})
	}
}

// TestListPagesThroughAllReservations は、最後の予約の位置から続きを読むと、すべての予約を 1 回ずつ読めることを確かめる。
func TestListPagesThroughAllReservations(t *testing.T) {
	t.Parallel()

	f := newListFixture(t)
	query := reserve.ListQuery{From: at(0, 0), To: at(23, 0), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 1}
	got := []string{}
	for {
		page, err := f.reservations.List(t.Context(), f.pool, &query)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		got = append(got, f.namesOf(page)...)
		last := page[len(page)-1]
		query.AfterStartAt, query.AfterID = &last.StartAt, &last.ID
	}
	assert.Equal(t, []string{first, disabled, conflicting, partial}, got)
}

func TestListCarriesAssignment(t *testing.T) {
	t.Parallel()

	f := newListFixture(t)
	_, err := f.pool.Exec(t.Context(), `UPDATE reservations SET assignment_state = 'partial', assigned_tuner = 2,
		planned_start_at = '2026-10-10T13:59:50Z', planned_end_at = '2026-10-10T14:30:00Z' WHERE id = $1`, f.ids[partial])
	require.NoError(t, err)

	listed, err := f.reservations.List(t.Context(), f.pool, &reserve.ListQuery{
		From: at(14, 0), To: at(14, 30), States: nil, AfterStartAt: nil, AfterID: nil, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, &reserve.Assignment{
		State:          reserve.AssignmentPartial,
		Tuner:          new(int64(2)),
		PlannedStartAt: new(time.Date(2026, 10, 10, 13, 59, 50, 0, time.UTC)),
		PlannedEndAt:   new(at(14, 30)),
	}, listed[0].Assignment)

	// まだ計算していない予約は、割り当てたチューナーと予定の時刻を持たない。
	got, err := f.reservations.Get(t.Context(), f.pool, f.ids[first])
	require.NoError(t, err)
	assert.Equal(t, &reserve.Assignment{State: reserve.AssignmentPending, Tuner: nil, PlannedStartAt: nil, PlannedEndAt: nil}, got.Assignment)
}

func TestListByPrograms(t *testing.T) {
	t.Parallel()

	f := newListFixture(t)
	empty, err := f.reservations.ListByPrograms(t.Context(), f.pool, []reserve.ProgramKey{})
	require.NoError(t, err)
	assert.Empty(t, empty)

	got, err := f.reservations.ListByPrograms(t.Context(), f.pool, []reserve.ProgramKey{
		{ServiceID: serviceID(), EventID: 6},
		{ServiceID: serviceID(), EventID: 7},
		// 予約のない番組は結果に含めない。
		{ServiceID: serviceID(), EventID: 8},
	})
	require.NoError(t, err)

	names := map[int64]string{}
	for key, reservation := range got {
		names[key.EventID] = f.names[reservation.ID]
	}
	assert.Equal(t, map[int64]string{6: conflicting, 7: partial}, names)
}

func TestRuleIDs(t *testing.T) {
	t.Parallel()

	f := newListFixture(t)
	// 後に作ったルールを先に関係づけても、ルールを作った順に返す。
	older, newer := id.New(), id.New()
	for i, ruleID := range []uuid.UUID{older, newer} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates,
			follow_event_relay, created_at, updated_at) VALUES ($1, $3, '', true, '{}', '{}', false, true, $2, $2)`,
			ruleID, testNow().Add(time.Duration(i)*time.Minute), ruleID.String())
		require.NoError(t, err)
	}
	for _, ruleID := range []uuid.UUID{newer, older} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)`, f.ids[conflicting], ruleID)
		require.NoError(t, err)
	}

	got, err := f.reservations.RuleIDs(t.Context(), f.pool, []uuid.UUID{f.ids[first], f.ids[conflicting]})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID][]uuid.UUID{f.ids[conflicting]: {older, newer}}, got)
}
