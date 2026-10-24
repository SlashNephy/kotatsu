package graph_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

const (
	reservationFields = `id kind eventId startAt endAt name enabled followEventRelay service { name }
		settings { priority } assignment { state tuner plannedStartAt plannedEndAt } rules { name }
		createdBy { username } updatedBy { username }`
	createProgramReservation = `mutation($input: ProgramReservationInput!) { createProgramReservation(input: $input) { ` + reservationFields + ` } }`
	createTimeReservation    = `mutation($input: TimeReservationInput!) { createTimeReservation(input: $input) { ` + reservationFields + ` } }`
	updateReservation        = `mutation($id: ID!, $input: ReservationUpdateInput!) { updateReservation(id: $id, input: $input) { ` + reservationFields + ` } }`
	deleteReservation        = `mutation($id: ID!) { deleteReservation(id: $id) { id reservation { ` + reservationFields + ` } } }`
	listReservations         = `query($from: Time!, $to: Time!, $states: [AssignmentState!], $rule: ID, $first: Int!, $after: String) {
		reservations(from: $from, to: $to, states: $states, rule: $rule, first: $first, after: $after) {
			edges { node { ` + reservationFields + ` } } pageInfo { hasNextPage endCursor }
		}
	}`
	// fourteen と tomorrow は、テストの日の 14:00 と、次の日の 0:00。
	fourteen = "2026-10-10T14:00:00Z"
	tomorrow = "2026-10-11T00:00:00Z"
	// GraphQL の変数の名前。
	varProgram          = "program"
	varEndAt            = "endAt"
	varStartAt          = "startAt"
	varService          = "service"
	varName             = "name"
	varFrom             = "from"
	varTo               = "to"
	varStates           = "states"
	varRule             = "rule"
	varAfter            = "after"
	varID               = "id"
	varSettings         = "settings"
	varPriority         = "priority"
	varFollowEventRelay = "followEventRelay"
	// grServiceID は、faketuner.DefaultOptions の最初のサービス (こたつテスト1)。
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
)

type reservationResult struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	EventID *int      `json:"eventId"`
	StartAt time.Time `json:"startAt"`
	EndAt   time.Time `json:"endAt"`
	Name    string    `json:"name"`
	Enabled bool      `json:"enabled"`
	Service struct {
		Name string `json:"name"`
	} `json:"service"`
	Settings struct {
		Priority *int `json:"priority"`
	} `json:"settings"`
	Assignment struct {
		State string `json:"state"`
		Tuner *int   `json:"tuner"`
	} `json:"assignment"`
	Rules []struct {
		Name string `json:"name"`
	} `json:"rules"`
	CreatedBy *username `json:"createdBy"`
	UpdatedBy *username `json:"updatedBy"`
}

type reservationList struct {
	Reservations struct {
		Edges []struct {
			Node reservationResult `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"reservations"`
}

func (l *reservationList) names() []string {
	names := make([]string, 0, len(l.Reservations.Edges))
	for _, edge := range l.Reservations.Edges {
		names = append(names, edge.Node.Name)
	}

	return names
}

// programID は、こたつテスト1 の eventId の番組の ID を返す。
func programID(eventID int) string {
	return graph.GlobalID("Program", fmt.Sprintf("%d.%d", faketuner.ServiceItemID(grNetworkID, grServiceID), eventID))
}

func (f *fixture) createProgramReservation(t *testing.T, eventID int) reservationResult {
	t.Helper()

	var got struct {
		CreateProgramReservation reservationResult `json:"createProgramReservation"`
	}
	require.Empty(t, f.query(t, createProgramReservation, map[string]any{varInput: map[string]any{varProgram: programID(eventID)}}, &got))

	return got.CreateProgramReservation
}

// TestReservations は、予約を作り、一覧で取得し、変え、消せることを確かめる。
func TestReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)

	var created struct {
		CreateProgramReservation reservationResult `json:"createProgramReservation"`
	}
	require.Empty(t, f.query(t, createProgramReservation, map[string]any{varInput: map[string]any{
		varProgram: programID(6), varFollowEventRelay: false, varSettings: map[string]any{varPriority: 2},
	}}, &created))
	program := created.CreateProgramReservation
	assert.Equal(t, "PROGRAM", program.Kind)
	assert.Equal(t, new(6), program.EventID)
	assert.Equal(t, programName(6), program.Name)
	assert.Equal(t, "こたつテスト1", program.Service.Name)
	assert.Equal(t, new(2), program.Settings.Priority)
	assert.Equal(t, "PENDING", program.Assignment.State)
	assert.Equal(t, &username{Username: testUsername}, program.CreatedBy)
	// 同じ番組をもう一度予約しても、同じ予約が返る。
	assert.Equal(t, program.ID, f.createProgramReservation(t, 6).ID)

	var timeCreated struct {
		CreateTimeReservation reservationResult `json:"createTimeReservation"`
	}
	require.Empty(t, f.query(t, createTimeReservation, map[string]any{varInput: map[string]any{
		varService: services["こたつ BS"], varStartAt: onePM, varEndAt: fourteen, varName: "BS の時刻予約",
	}}, &timeCreated))
	timeReservation := timeCreated.CreateTimeReservation
	assert.Equal(t, "TIME", timeReservation.Kind)
	assert.Nil(t, timeReservation.EventID)

	// 一覧は、開始時刻の順に並ぶ。
	var listed reservationList
	require.Empty(t, f.query(t, listReservations, map[string]any{varFrom: noon, varTo: tomorrow, varFirst: 10}, &listed))
	assert.Equal(t, []string{"BS の時刻予約", programName(6)}, listed.names())

	// 番組表の番組から、予約を引ける。
	var guide struct {
		Guide []struct {
			Programs []struct {
				EventID     int                `json:"eventId"`
				Reservation *reservationResult `json:"reservation"`
			} `json:"programs"`
		} `json:"guide"`
	}
	require.Empty(t, f.query(t, `query($s: [ID!]) { guide(from: "2026-10-10T13:00:00Z", to: "2026-10-10T14:00:00Z", services: $s) {
		programs { eventId reservation { id } } } }`, map[string]any{"s": []string{services["こたつテスト1"]}}, &guide))
	reserved := map[int]string{}
	for _, p := range guide.Guide[0].Programs {
		if p.Reservation != nil {
			reserved[p.EventID] = p.Reservation.ID
		}
	}
	assert.Equal(t, map[int]string{6: program.ID}, reserved)

	// node でも引ける。
	var node struct {
		Node reservationResult `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Reservation { `+reservationFields+` } } }`, map[string]any{varID: program.ID}, &node))
	assert.Equal(t, program, node.Node)

	// 別のメンバーが変える。
	other := f.as(t, "other", auth.RoleMember)
	var updated struct {
		UpdateReservation reservationResult `json:"updateReservation"`
	}
	require.Empty(t, other.query(t, updateReservation, map[string]any{varID: timeReservation.ID, varInput: map[string]any{
		varEnabled: false, varEndAt: "2026-10-10T14:30:00Z", varName: "延長",
	}}, &updated))
	assert.False(t, updated.UpdateReservation.Enabled)
	assert.Equal(t, "延長", updated.UpdateReservation.Name)
	assert.Equal(t, time.Date(2026, 10, 10, 14, 30, 0, 0, time.UTC), updated.UpdateReservation.EndAt)
	assert.Equal(t, &username{Username: "other"}, updated.UpdateReservation.UpdatedBy)

	// 番組予約を消すと、予約はなくなる。
	var deleted struct {
		DeleteReservation struct {
			ID          string             `json:"id"`
			Reservation *reservationResult `json:"reservation"`
		} `json:"deleteReservation"`
	}
	require.Empty(t, f.query(t, deleteReservation, map[string]any{varID: program.ID}, &deleted))
	assert.Equal(t, program.ID, deleted.DeleteReservation.ID)
	assert.Nil(t, deleted.DeleteReservation.Reservation)
	assert.Equal(t, []string{codeNotFound}, f.query(t, `query($id: ID!) { node(id: $id) { id } }`, map[string]any{varID: program.ID}, nil))

	var actions []string
	rows, err := f.pool.Query(t.Context(), `SELECT action FROM audit_events WHERE target_type = 'reservation' AND user_agent = $1`, testUserAgent)
	require.NoError(t, err)
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.ElementsMatch(t, []string{"reservation.create", "reservation.create", "reservation.update", "reservation.delete"}, actions)
}

// TestRuleReservations は、ルール予約が一致したルールを持ち、消すと無効になることを確かめる。
func TestRuleReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	var createdRule struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, ruleVariables("", testProgramRule(f.serviceIDs(t))), &createdRule))
	_, rawRuleID, err := graph.ParseGlobalID(createdRule.CreateRule.ID)
	require.NoError(t, err)

	// neko の照合が作るルール予約を、照合を走らせずに作る。
	clk := clocktest.NewFake(testNow())
	reservation, _, err := reserve.NewReservations(clk, audit.NewRecorder(clk)).CreateRule(t.Context(), f.pool, audit.System("neko"),
		faketuner.ServiceItemID(grNetworkID, grServiceID), 7, true)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)`, reservation.ID, rawRuleID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `UPDATE reservations SET assignment_state = 'conflict' WHERE id = $1`, reservation.ID)
	require.NoError(t, err)
	reservationID := graph.GlobalID("Reservation", reservation.ID.String())
	f.createProgramReservation(t, 5)

	// 競合している予約だけを引く。
	var listed reservationList
	require.Empty(t, f.query(t, listReservations, map[string]any{
		varFrom: noon, varTo: tomorrow, varStates: []string{"CONFLICT"}, varFirst: 10,
	}, &listed))
	require.Len(t, listed.Reservations.Edges, 1)
	got := listed.Reservations.Edges[0].Node
	assert.Equal(t, reservationID, got.ID)
	assert.Equal(t, "RULE", got.Kind)
	assert.Equal(t, "CONFLICT", got.Assignment.State)
	assert.Equal(t, "テスト番組", got.Rules[0].Name)
	assert.Nil(t, got.CreatedBy)

	// ルールで絞ると、番組予約を除いて、ルールに一致した予約だけを引く。
	var byRule reservationList
	require.Empty(t, f.query(t, listReservations, map[string]any{
		varFrom: noon, varTo: tomorrow, varRule: createdRule.CreateRule.ID, varFirst: 10,
	}, &byRule))
	require.Len(t, byRule.Reservations.Edges, 1)
	assert.Equal(t, reservationID, byRule.Reservations.Edges[0].Node.ID)

	// ルールに、一致した予約の件数を出す。
	var counted struct {
		Rules []struct {
			ID                       string `json:"id"`
			UpcomingReservationCount int    `json:"upcomingReservationCount"`
		} `json:"rules"`
	}
	require.Empty(t, f.query(t, `{ rules { id upcomingReservationCount } }`, nil, &counted))
	require.Len(t, counted.Rules, 1)
	assert.Equal(t, 1, counted.Rules[0].UpcomingReservationCount)

	// ルール予約を消すと、無効になって残る。
	var deleted struct {
		DeleteReservation struct {
			Reservation *reservationResult `json:"reservation"`
		} `json:"deleteReservation"`
	}
	require.Empty(t, f.query(t, deleteReservation, map[string]any{varID: reservationID}, &deleted))
	require.NotNil(t, deleted.DeleteReservation.Reservation)
	assert.False(t, deleted.DeleteReservation.Reservation.Enabled)
}

func TestReservationPages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	for _, eventID := range []int{4, 5, 6, 7, 8} {
		f.createProgramReservation(t, eventID)
	}

	got := []string{}
	variables := map[string]any{varFrom: noon, varTo: tomorrow, varFirst: 2}
	pages := 0
	for {
		var page reservationList
		require.Empty(t, f.query(t, listReservations, variables, &page))
		got = append(got, page.names()...)
		pages++
		if !page.Reservations.PageInfo.HasNextPage {
			break
		}
		variables[varAfter] = *page.Reservations.PageInfo.EndCursor
	}
	assert.Equal(t, []string{programName(4), programName(5), programName(6), programName(7), programName(8)}, got)
	assert.Equal(t, 3, pages)
}

func TestReservationErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	reservation := f.createProgramReservation(t, 6)
	viewer := f.as(t, "child", auth.RoleViewer)
	timeInput := func(startAt, endAt string) map[string]any {
		return map[string]any{varInput: map[string]any{varService: services["こたつテスト1"], varStartAt: startAt, varEndAt: endAt}}
	}

	tests := []struct {
		name      string
		f         *fixture
		query     string
		variables map[string]any
		want      string
	}{
		{name: "viewer cannot create a program reservation", f: viewer, query: createProgramReservation, variables: map[string]any{varInput: map[string]any{varProgram: programID(7)}}, want: codeForbidden},
		{name: "viewer cannot create a time reservation", f: viewer, query: createTimeReservation, variables: timeInput(onePM, fourteen), want: codeForbidden},
		{name: "viewer cannot update", f: viewer, query: updateReservation, variables: map[string]any{varID: reservation.ID, varInput: map[string]any{varEnabled: false}}, want: codeForbidden},
		{name: "viewer cannot delete", f: viewer, query: deleteReservation, variables: map[string]any{varID: reservation.ID}, want: codeForbidden},
		{name: "unknown program", f: f, query: createProgramReservation, variables: map[string]any{varInput: map[string]any{varProgram: programID(99)}}, want: codeNotFound},
		{name: "invalid program ID", f: f, query: createProgramReservation, variables: map[string]any{varInput: map[string]any{varProgram: services["こたつテスト1"]}}, want: codeBadUserInput},
		{name: "end before start", f: f, query: createTimeReservation, variables: timeInput(fourteen, onePM), want: codeBadUserInput},
		{name: "longer than a day", f: f, query: createTimeReservation, variables: timeInput(onePM, "2026-10-11T14:00:00Z"), want: codeBadUserInput},
		{name: "program reservation cannot be renamed", f: f, query: updateReservation, variables: map[string]any{varID: reservation.ID, varInput: map[string]any{varName: "x"}}, want: codeBadUserInput},
		{name: "invalid priority", f: f, query: updateReservation, variables: map[string]any{varID: reservation.ID, varInput: map[string]any{varSettings: map[string]any{varPriority: 11}}}, want: codeBadUserInput},
		{name: "too many reservations", f: f, query: listReservations, variables: map[string]any{varFrom: noon, varTo: fourteen, varFirst: 101}, want: codeBadUserInput},
		{name: "empty range", f: f, query: listReservations, variables: map[string]any{varFrom: noon, varTo: noon, varFirst: 1}, want: codeBadUserInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, tt.f.query(t, tt.query, tt.variables, nil))
		})
	}

	// 視聴者も予約の一覧は見られる。
	t.Run("viewer can read", func(t *testing.T) {
		t.Parallel()

		var listed reservationList
		require.Empty(t, viewer.query(t, listReservations, map[string]any{varFrom: noon, varTo: tomorrow, varFirst: 10}, &listed))
		assert.Len(t, listed.Reservations.Edges, 1)
	})
}

// TestBulkReservations は、予約をまとめて無効、有効にし、まとめて消せることと、録画中の予約は無効にも削除にもせず、
// 見つからない予約と一緒に飛ばして数えることを確かめる (#471)。
func TestBulkReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	first, second, recordingNow := f.createProgramReservation(t, 6), f.createProgramReservation(t, 7), f.createProgramReservation(t, 8)
	_, rawRecording, err := graph.ParseGlobalID(recordingNow.ID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, recorded_from, created_at, updated_at)
		SELECT gen_random_uuid(), id, service_id, event_id, name, '', start_at, end_at, 'recording', now(), created_at, updated_at
		FROM reservations WHERE id = $1`,
		rawRecording)
	require.NoError(t, err)
	ids := []string{first.ID, second.ID, recordingNow.ID, graph.GlobalID("Reservation", uuid.New().String())}
	type payload struct {
		Processed int `json:"processed"`
		Skipped   int `json:"skipped"`
	}
	const (
		setEnabled = `mutation($ids: [ID!]!, $enabled: Boolean!) { setReservationsEnabled(ids: $ids, enabled: $enabled) { processed skipped } }`
		deleteMany = `mutation($ids: [ID!]!) { deleteReservations(ids: $ids) { processed skipped } }`
	)
	enabledOf := func() map[string]bool {
		rows, err := f.pool.Query(t.Context(), "SELECT name, enabled FROM reservations")
		require.NoError(t, err)
		defer rows.Close()
		got := map[string]bool{}
		for rows.Next() {
			var name string
			var enabled bool
			require.NoError(t, rows.Scan(&name, &enabled))
			got[name] = enabled
		}

		return got
	}

	assert.Equal(t, []string{codeForbidden}, f.as(t, "viewer", auth.RoleViewer).query(t, deleteMany, map[string]any{varIDs: ids}, nil))

	var disabled struct {
		SetReservationsEnabled payload `json:"setReservationsEnabled"`
	}
	require.Empty(t, f.query(t, setEnabled, map[string]any{varIDs: ids, varEnabled: false}, &disabled))
	assert.Equal(t, payload{Processed: 2, Skipped: 2}, disabled.SetReservationsEnabled, "the recording one and the missing one are skipped")
	assert.Equal(t, map[string]bool{first.Name: false, second.Name: false, recordingNow.Name: true}, enabledOf())

	var enabled struct {
		SetReservationsEnabled payload `json:"setReservationsEnabled"`
	}
	require.Empty(t, f.query(t, setEnabled, map[string]any{varIDs: ids[:2], varEnabled: true}, &enabled))
	assert.Equal(t, payload{Processed: 2, Skipped: 0}, enabled.SetReservationsEnabled)

	var deleted struct {
		DeleteReservations payload `json:"deleteReservations"`
	}
	require.Empty(t, f.query(t, deleteMany, map[string]any{varIDs: ids}, &deleted))
	assert.Equal(t, payload{Processed: 2, Skipped: 2}, deleted.DeleteReservations)
	assert.Equal(t, map[string]bool{recordingNow.Name: true}, enabledOf(), "only the recording one is left")
}
