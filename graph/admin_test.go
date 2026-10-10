package graph_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/id"
)

const (
	auditEventsQuery = `query($first: Int!, $after: String, $filter: AuditEventFilter) {
		auditEvents(first: $first, after: $after, filter: $filter) {
			edges { cursor node { action targetType targetId summary sourceIp actor { type role user { username } } } }
			pageInfo { hasNextPage endCursor }
		}
	}`
	systemStatusQuery = `{ systemStatus {
		instances { role address alive }
		leases { name holderInstanceId generation valid }
		tunerServer { degraded lastError clockSkewMs }
		tuners { index name types available remote fault }
		pools { id path }
	} }`
	varFilter = "filter"

	actionRuleCreate   = "rule.create"
	actionReserve      = "reservation.create"
	actionUserCreated  = "user.created"
	actionUserDisabled = "user.disabled"
	targetUser         = "user"
	animeLabel         = "アニメ"
)

type auditEventsResult struct {
	AuditEvents struct {
		Edges []struct {
			Cursor string `json:"cursor"`
			Node   struct {
				Action     string  `json:"action"`
				TargetType string  `json:"targetType"`
				TargetID   *string `json:"targetId"`
				Summary    string  `json:"summary"`
				SourceIP   *string `json:"sourceIp"`
				Actor      struct {
					Type string  `json:"type"`
					Role *string `json:"role"`
					User *struct {
						Username string `json:"username"`
					} `json:"user"`
				} `json:"actor"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"auditEvents"`
}

func (r *auditEventsResult) actions() []string {
	actions := make([]string, 0, len(r.AuditEvents.Edges))
	for _, edge := range r.AuditEvents.Edges {
		actions = append(actions, edge.Node.Action)
	}

	return actions
}

// record は、監査ログに記録する。時計を 1 分ずつ進め、記録の日時をずらす。
func (f *fixture) record(t *testing.T, event *audit.Event) {
	t.Helper()

	f.clock.Advance(time.Minute)
	require.NoError(t, pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
		return audit.NewRecorder(f.clock).Record(t.Context(), tx, event)
	}))
}

// TestAuditEvents は、管理者が監査ログを、新しい順に、絞り込んでページを送りながら読めることを確かめる。
func TestAuditEvents(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, adminName, auth.RoleAdmin)
	since := f.clock.Now()
	events := []*audit.Event{
		{Actor: audit.UserFrom(f.userID, netip.MustParseAddr("192.168.1.10"), "kotatsu-test/1.0"), Action: actionRuleCreate, TargetType: "rule", TargetID: "r1", Summary: map[string]any{"name": animeLabel}},
		{Actor: audit.System("neko"), Action: actionReserve, TargetType: "reservation", TargetID: "v1", Summary: map[string]any{}},
		{Actor: audit.User(admin.userID), Action: actionUserCreated, TargetType: targetUser, TargetID: f.userID.String(), Summary: map[string]any{}},
		{Actor: audit.User(admin.userID), Action: actionUserDisabled, TargetType: targetUser, TargetID: f.userID.String(), Summary: map[string]any{}},
	}
	for _, event := range events {
		f.record(t, event)
	}
	// 消したユーザーの操作も、操作者のユーザーを null にして返す。
	gone, err := auth.NewUsers(f.clock).Create(t.Context(), f.pool, &auth.NewUser{Username: "gone", DisplayName: "gone", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	f.record(t, &audit.Event{Actor: audit.User(gone.ID), Action: "guide_view.create", TargetType: "guide_view", TargetID: "g1", Summary: map[string]any{}})
	_, err = f.pool.Exec(t.Context(), "DELETE FROM users WHERE id = $1", gone.ID)
	require.NoError(t, err)
	var orphan auditEventsResult
	require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{varFirst: 1}, &orphan))
	require.Len(t, orphan.AuditEvents.Edges, 1)
	assert.Equal(t, "user", orphan.AuditEvents.Edges[0].Node.Actor.Type)
	assert.Nil(t, orphan.AuditEvents.Edges[0].Node.Actor.User)
	_, err = f.pool.Exec(t.Context(), "DELETE FROM audit_events WHERE action = 'guide_view.create'")
	require.NoError(t, err)

	assert.Equal(t, []string{codeForbidden}, f.query(t, auditEventsQuery, map[string]any{varFirst: 10}, nil), "members cannot read the audit log")

	var page auditEventsResult
	require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{varFirst: 2}, &page))
	assert.Equal(t, []string{actionUserDisabled, actionUserCreated}, page.actions())
	assert.True(t, page.AuditEvents.PageInfo.HasNextPage)
	require.NotNil(t, page.AuditEvents.PageInfo.EndCursor)

	var next auditEventsResult
	require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{varFirst: 2, varAfter: *page.AuditEvents.PageInfo.EndCursor}, &next))
	assert.Equal(t, []string{actionReserve, actionRuleCreate}, next.actions())
	assert.False(t, next.AuditEvents.PageInfo.HasNextPage)
	rule := next.AuditEvents.Edges[1].Node
	require.NotNil(t, rule.Actor.User)
	assert.Equal(t, testUsername, rule.Actor.User.Username)
	assert.JSONEq(t, `{"name":"`+animeLabel+`"}`, rule.Summary)
	require.NotNil(t, rule.SourceIP)
	assert.Equal(t, "192.168.1.10", *rule.SourceIP)
	system := next.AuditEvents.Edges[0].Node.Actor
	assert.Equal(t, "system", system.Type)
	require.NotNil(t, system.Role)
	assert.Equal(t, "neko", *system.Role)
	assert.Nil(t, system.User)

	tests := []struct {
		name   string
		filter map[string]any
		want   []string
	}{
		{name: "action prefix", filter: map[string]any{"actionPrefix": "user."}, want: []string{actionUserDisabled, actionUserCreated}},
		{name: "actor", filter: map[string]any{"actorUserId": graph.GlobalID("User", f.userID.String())}, want: []string{actionRuleCreate}},
		{name: "target", filter: map[string]any{"targetType": targetUser, "targetId": f.userID.String()}, want: []string{actionUserDisabled, actionUserCreated}},
		{
			name: "time range", want: []string{actionUserCreated, actionReserve},
			filter: map[string]any{"since": since.Add(2 * time.Minute), "until": since.Add(4 * time.Minute)},
		},
	}
	for _, tt := range tests {
		var filtered auditEventsResult
		require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{varFirst: 10, varFilter: tt.filter}, &filtered), tt.name)
		assert.Equal(t, tt.want, filtered.actions(), tt.name)
	}

	for _, variables := range []map[string]any{
		{varFirst: 0},
		{varFirst: 201},
		{varFirst: 10, varAfter: "not-a-cursor"},
		{varFirst: 10, varFilter: map[string]any{"actorUserId": "not-an-id"}},
	} {
		assert.Equal(t, []string{codeBadUserInput}, admin.query(t, auditEventsQuery, variables, nil), "%v", variables)
	}
}

// TestSystemStatus は、管理者が、インスタンス、リース、チューナー、保存先プールの状態を読めることを確かめる。
func TestSystemStatus(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{Pools: map[string]string{"sub": "/mnt/sub", "main": "/mnt/main"}})
	admin := f.as(t, adminName, auth.RoleAdmin)
	now := f.clock.Now()
	alive, stopped := id.New(), id.New()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO instances (id, role, address, started_at, heartbeat_at) VALUES ($1, 'neko', '10.0.0.1:8080', $2, $2)", []any{alive, now}},
		{"INSERT INTO instances (id, role, address, started_at, heartbeat_at) VALUES ($1, 'mikan', '10.0.0.2:8080', $2, $2)", []any{stopped, now.Add(-time.Hour)}},
		{"INSERT INTO leases (name, holder, generation, acquired_at, renewed_at, expires_at) VALUES ('neko', $1, 3, $2, $2, $3)", []any{alive, now, now.Add(15 * time.Second)}},
		{"INSERT INTO tuner_server_status (degraded_since, last_error, clock_skew_ms, checked_at) VALUES ($1, 'connection refused', 120, $1)", []any{now}},
		{"INSERT INTO tuners (tuner_index, name, types, is_available, is_remote, is_fault, updated_at) VALUES (0, 'PX-Q3PE', '{GR,BS}', $1, $2, $3, $4)", []any{true, false, false, now}},
	} {
		_, err := f.pool.Exec(t.Context(), stmt.sql, stmt.args...)
		require.NoError(t, err, stmt.sql)
	}

	assert.Equal(t, []string{codeForbidden}, f.query(t, systemStatusQuery, nil, nil), "members cannot read the system status")

	var got struct {
		SystemStatus struct {
			Instances []struct {
				Role    string `json:"role"`
				Address string `json:"address"`
				Alive   bool   `json:"alive"`
			} `json:"instances"`
			Leases []struct {
				Name             string `json:"name"`
				HolderInstanceID string `json:"holderInstanceId"`
				Generation       int    `json:"generation"`
				Valid            bool   `json:"valid"`
			} `json:"leases"`
			TunerServer struct {
				Degraded    bool   `json:"degraded"`
				LastError   string `json:"lastError"`
				ClockSkewMs *int   `json:"clockSkewMs"`
			} `json:"tunerServer"`
			Tuners []struct {
				Index int      `json:"index"`
				Name  string   `json:"name"`
				Types []string `json:"types"`
			} `json:"tuners"`
			Pools []struct {
				ID   string `json:"id"`
				Path string `json:"path"`
			} `json:"pools"`
		} `json:"systemStatus"`
	}
	require.Empty(t, admin.query(t, systemStatusQuery, nil, &got))
	status := got.SystemStatus
	require.Len(t, status.Instances, 2)
	assert.Equal(t, "mikan", status.Instances[0].Role)
	assert.False(t, status.Instances[0].Alive, "the instance stopped writing heartbeats an hour ago")
	assert.Equal(t, "neko", status.Instances[1].Role)
	assert.True(t, status.Instances[1].Alive)
	require.Len(t, status.Leases, 1)
	assert.Equal(t, alive.String(), status.Leases[0].HolderInstanceID, "the neko leader is the alive instance")
	assert.Equal(t, 3, status.Leases[0].Generation)
	assert.True(t, status.Leases[0].Valid)
	assert.True(t, status.TunerServer.Degraded)
	assert.Equal(t, "connection refused", status.TunerServer.LastError)
	require.NotNil(t, status.TunerServer.ClockSkewMs)
	assert.Equal(t, 120, *status.TunerServer.ClockSkewMs)
	require.Len(t, status.Tuners, 1)
	assert.Equal(t, []string{"GR", "BS"}, status.Tuners[0].Types)
	require.Len(t, status.Pools, 2)
	assert.Equal(t, "main", status.Pools[0].ID)
	assert.Equal(t, "/mnt/main", status.Pools[0].Path)
}

// TestSystemStatusBeforeChecks は、まだ何も確かめていないときも、空の状態を返すことを確かめる。
func TestSystemStatusBeforeChecks(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, adminName, auth.RoleAdmin)
	var got struct {
		SystemStatus struct {
			Instances   []any `json:"instances"`
			Leases      []any `json:"leases"`
			TunerServer *any  `json:"tunerServer"`
			Tuners      []any `json:"tuners"`
			Pools       []any `json:"pools"`
		} `json:"systemStatus"`
	}
	require.Empty(t, admin.query(t, systemStatusQuery, nil, &got))
	assert.Empty(t, got.SystemStatus.Instances)
	assert.Empty(t, got.SystemStatus.Leases)
	assert.Nil(t, got.SystemStatus.TunerServer, "the tuner server has not been checked")
	assert.Empty(t, got.SystemStatus.Tuners)
	assert.NotNil(t, got.SystemStatus.Pools)
	assert.Empty(t, got.SystemStatus.Pools)
}
