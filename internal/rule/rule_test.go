package rule_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

func newUser(t *testing.T, pool *pgxpool.Pool) *audit.Actor {
	t.Helper()

	user, err := auth.NewUsers(clocktest.NewFake(testNow())).Create(t.Context(), pool, &auth.NewUser{
		Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleMember, PasswordHash: nil,
	})
	require.NoError(t, err)

	return audit.User(user.ID)
}

func validInput() *rule.Input {
	return &rule.Input{
		Name:    yurucamp,
		Memo:    "",
		Enabled: true,
		Conditions: &match.Conditions{
			Keywords: []*match.Keyword{{Text: "ゆるキャン", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		},
		Overrides:           &reserve.Overrides{Priority: new(2)},
		ValidFrom:           nil,
		ValidUntil:          new(testNow().Add(30 * 24 * time.Hour)),
		AvoidDuplicates:     true,
		DuplicateWindowDays: new(7),
		FollowEventRelay:    true,
	}
}

// TestRuleLifecycleIsAudited は、ルールを作り、変え、消せて、そのたびに監査ログが残ることを確かめる。
func TestRuleLifecycleIsAudited(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))

	created, err := rules.Create(t.Context(), pool, actor, validInput())
	require.NoError(t, err)
	// 条件と録画設定は、JSONB を通っても元の値に戻る。
	assert.Equal(t, validInput().Conditions, created.Conditions)
	assert.Equal(t, validInput().Overrides, created.Overrides)
	assert.Equal(t, new(7), created.DuplicateWindowDays)
	assert.Equal(t, &actor.UserID, created.CreatedBy)

	in := validInput()
	in.Enabled = false
	in.DuplicateWindowDays = nil
	updated, err := rules.Update(t.Context(), pool, actor, created.ID, in)
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	assert.Nil(t, updated.DuplicateWindowDays)

	listed, err := rules.List(t.Context(), pool)
	require.NoError(t, err)
	assert.Equal(t, []*rule.Rule{updated}, listed)

	require.NoError(t, rules.Delete(t.Context(), pool, actor, created.ID))
	_, err = rules.Get(t.Context(), pool, created.ID)
	require.ErrorIs(t, err, rule.ErrNotFound)
	require.ErrorIs(t, rules.Delete(t.Context(), pool, actor, created.ID), rule.ErrNotFound)
	_, err = rules.Update(t.Context(), pool, actor, created.ID, validInput())
	require.ErrorIs(t, err, rule.ErrNotFound)

	for _, action := range []string{"rule.create", "rule.update", "rule.delete"} {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(),
			"SELECT count(*) FROM audit_events WHERE action = $1 AND target_id = $2 AND actor_user_id = $3",
			action, created.ID.String(), actor.UserID).Scan(&n))
		assert.Equal(t, 1, n, action)
	}
}

func TestCreateRejectsInvalidRules(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	tests := []struct {
		name    string
		modify  func(in *rule.Input)
		wantErr error
	}{
		{name: "empty name", modify: func(in *rule.Input) { in.Name = "" }, wantErr: rule.ErrInvalidRule},
		{name: "blank name", modify: func(in *rule.Input) { in.Name = " \u3000" }, wantErr: rule.ErrInvalidRule},
		{name: "invalid regular expression", modify: func(in *rule.Input) {
			in.Conditions.Keywords[0] = &match.Keyword{Text: "(", Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}}
		}, wantErr: match.ErrInvalidConditions},
		{name: "missing conditions", modify: func(in *rule.Input) { in.Conditions = nil }, wantErr: match.ErrInvalidConditions},
		{name: "invalid overrides", modify: func(in *rule.Input) { in.Overrides = &reserve.Overrides{Priority: new(99)} }, wantErr: reserve.ErrInvalidSettings},
		{name: "empty validity period", modify: func(in *rule.Input) { in.ValidFrom = in.ValidUntil }, wantErr: rule.ErrInvalidRule},
		{name: "zero duplicate window", modify: func(in *rule.Input) { in.DuplicateWindowDays = new(0) }, wantErr: rule.ErrInvalidRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validInput()
			tt.modify(in)
			_, err := rules.Create(t.Context(), pool, audit.System("neko"), in)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
