package rule_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
)

// watchFixture は、視聴ルールの照合を確かめるための、2 人のユーザーを足した engineFixture。
type watchFixture struct {
	*engineFixture

	alice uuid.UUID
	bob   uuid.UUID
}

func newWatchFixture(tb testing.TB, extra ...*faketuner.Program) *watchFixture {
	tb.Helper()

	f := newEngineFixture(tb, extra...)
	users := map[string]uuid.UUID{}
	for _, name := range []string{"alice", "bob"} {
		user, err := auth.NewUsers(clocktest.NewFake(testNow())).Create(tb.Context(), f.pool, &auth.NewUser{
			Username: name, DisplayName: name, Role: auth.RoleViewer, PasswordHash: nil,
		})
		require.NoError(tb, err)
		users[name] = user.ID
	}

	return &watchFixture{engineFixture: f, alice: users["alice"], bob: users["bob"]}
}

// watchConditions は、こたつテスト1 の番組名が pattern に一致する番組の条件。
func watchConditions(pattern string) *match.Conditions {
	return &match.Conditions{
		Keywords:   []*match.Keyword{{Text: pattern, Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		ServiceIDs: []int64{serviceID()},
	}
}

// createWatchRule は、ユーザー userID の、こたつテスト1 の番組名が pattern に一致する番組の視聴ルールを作る。
func (f *watchFixture) createWatchRule(tb testing.TB, userID uuid.UUID, pattern string) *watch.Rule {
	tb.Helper()

	created, err := watch.CreateRule(tb.Context(), f.pool, f.clock, userID, &watch.RuleInput{
		Name: pattern, Enabled: true, Conditions: watchConditions(pattern),
	})
	require.NoError(tb, err)

	return created
}

func (f *watchFixture) programKey(eventID int64) domain.ProgramKey {
	return domain.ProgramKey{ServiceID: serviceID(), EventID: eventID}
}

// watchReserved は、ユーザー userID が視聴予約した、こたつテスト1 の番組の eventId と、視聴ルールが作ったものかどうかを返す。
func (f *watchFixture) watchReserved(t *testing.T, userID uuid.UUID) map[int64]bool {
	t.Helper()

	rows, err := f.pool.Query(t.Context(),
		"SELECT event_id, watch_rule_id IS NOT NULL FROM watch_reservations WHERE user_id = $1 AND service_id = $2", userID, serviceID())
	require.NoError(t, err)
	defer rows.Close()
	reserved := map[int64]bool{}
	for rows.Next() {
		var (
			eventID int64
			byRule  bool
		)
		require.NoError(t, rows.Scan(&eventID, &byRule))
		reserved[eventID] = byRule
	}
	require.NoError(t, rows.Err())

	return reserved
}

func (f *watchFixture) settingsVersion(t *testing.T, userID uuid.UUID) int64 {
	t.Helper()

	version, err := query.New(f.pool).UserSettingsVersion(t.Context(), userID)
	require.NoError(t, err)

	return version
}

// TestEngineWatchRules は、視聴ルールに一致した放送前の番組に、持ち主の視聴予約を付け、
// 視聴ルールが作った視聴予約だけを、一致しなくなったら外すことを確かめる (設計書 4.8)。
// eventId 3 は放送中 (12:00〜)、4 は 12:30、5 は 13:00 に始まる。照合は 12:10 に行う。
func TestEngineWatchRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup は、1 回目の照合の前に行う。
		setup func(t *testing.T, f *watchFixture)
		// change は、1 回目の照合の後、2 回目の照合の前に行う。nil なら 1 回だけ照合する。
		change func(t *testing.T, f *watchFixture)
		// want は、最後の照合の後の alice の視聴予約 (eventId と、視聴ルールが作ったものかどうか)。
		want        map[int64]bool
		wantCreated int
		wantDeleted int
	}{
		{
			name: "一致した放送前の番組に付ける",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			want:        map[int64]bool{4: true, 5: true},
			wantCreated: 2,
		},
		{
			name: "放送中の番組には付けない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 3$`)
			},
			want: map[int64]bool{},
		},
		{
			name: "通知を送る時刻を過ぎた番組には付けない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				// 30 分前に通知するので、12:30 に始まる eventId 4 は、12:00 に通知の時刻を過ぎている。
				_, err := watch.SetReminderMinutes(t.Context(), f.pool, f.clock, f.alice, 30)
				require.NoError(t, err)
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			want:        map[int64]bool{5: true},
			wantCreated: 1,
		},
		{
			name: "手で付けた視聴予約は、手で付けたまま残す",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.Reserve(t.Context(), f.pool, f.clock, f.alice, f.programKey(4)))
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			want:        map[int64]bool{4: false, 5: true},
			wantCreated: 1,
		},
		{
			name: "手で外した番組には付け直さない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.Reserve(t.Context(), f.pool, f.clock, f.alice, f.programKey(4)))
				require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(4), false))
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			want:        map[int64]bool{5: true},
			wantCreated: 1,
		},
		{
			name: "視聴ルールが付けた視聴予約を手で外したら、次の照合でも付け直さない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(4), false))
			},
			want: map[int64]bool{5: true},
		},
		{
			name: "手で外した後に手で付け直したら、手で付けたものにする",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(4), false))
				require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(4), true))
			},
			want: map[int64]bool{4: false, 5: true},
		},
		{
			name: "視聴ルールが付けた視聴予約に手で付け直したら、手で付けたものにする",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(4), true))
				_, err := f.pool.Exec(t.Context(), "UPDATE watch_rules SET enabled = false WHERE user_id = $1", f.alice)
				require.NoError(t, err)
			},
			want:        map[int64]bool{4: false},
			wantDeleted: 1,
		},
		{
			name: "条件を変えて一致しなくなったら、視聴ルールが付けたものだけを外す",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				require.NoError(t, watch.Reserve(t.Context(), f.pool, f.clock, f.alice, f.programKey(4)))
				f.createWatchRule(t, f.alice, `テスト番組 (4|5|6)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				rules, err := watch.ListRules(t.Context(), f.pool, f.alice)
				require.NoError(t, err)
				_, err = watch.UpdateRule(t.Context(), f.pool, f.clock, f.alice, rules[0].ID, &watch.RuleInput{
					Name: "6 だけ", Enabled: true, Conditions: watchConditions(`テスト番組 6$`),
				})
				require.NoError(t, err)
			},
			want:        map[int64]bool{4: false, 6: true},
			wantDeleted: 1,
		},
		{
			name: "視聴ルールを無効にしたら、付けたものを外す",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				rules, err := watch.ListRules(t.Context(), f.pool, f.alice)
				require.NoError(t, err)
				_, err = watch.SetRuleEnabled(t.Context(), f.pool, f.clock, f.alice, rules[0].ID, false)
				require.NoError(t, err)
			},
			want:        map[int64]bool{},
			wantDeleted: 2,
		},
		{
			name: "視聴ルールを消しても、付けたものは手で付けたものとして残す",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				rules, err := watch.ListRules(t.Context(), f.pool, f.alice)
				require.NoError(t, err)
				require.NoError(t, watch.DeleteRule(t.Context(), f.pool, f.clock, f.alice, rules[0].ID))
			},
			want: map[int64]bool{4: false, 5: false},
		},
		{
			name: "ほかの視聴ルールにまだ一致していれば、外さない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
				f.createWatchRule(t, f.alice, `テスト番組 5$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				rules, err := watch.ListRules(t.Context(), f.pool, f.alice)
				require.NoError(t, err)
				_, err = watch.SetRuleEnabled(t.Context(), f.pool, f.clock, f.alice, rules[0].ID, false)
				require.NoError(t, err)
			},
			want:        map[int64]bool{5: true},
			wantDeleted: 1,
		},
		{
			name: "条件を読めない視聴ルールがあるユーザーの視聴予約は、外さない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				_, err := f.pool.Exec(t.Context(),
					`UPDATE watch_rules SET conditions = '{"version": 1, "conditions": {"unknownField": true}}' WHERE user_id = $1`, f.alice)
				require.NoError(t, err)
			},
			want: map[int64]bool{4: true, 5: true},
		},
		{
			name: "終了時刻が未定になった番組の視聴予約は、外さない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.alice, `テスト番組 (4|5)$`)
			},
			change: func(t *testing.T, f *watchFixture) {
				t.Helper()
				_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = true WHERE service_id = $1 AND event_id = 5", serviceID())
				require.NoError(t, err)
			},
			want: map[int64]bool{4: true, 5: true},
		},
		{
			name: "ほかのユーザーの視聴ルールでは付けない",
			setup: func(t *testing.T, f *watchFixture) {
				t.Helper()
				f.createWatchRule(t, f.bob, `テスト番組 (4|5)$`)
			},
			want:        map[int64]bool{},
			wantCreated: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newWatchFixture(t)
			tt.setup(t, f)
			result := f.run(t)
			if tt.change != nil {
				tt.change(t, f)
				result = f.run(t)
			}
			assert.Equal(t, tt.want, f.watchReserved(t, f.alice))
			assert.Equal(t, tt.wantCreated, result.WatchCreated)
			assert.Equal(t, tt.wantDeleted, result.WatchDeleted)
			// 視聴ルールは録画の予約を作らない。
			assert.Empty(t, f.reserved(t))
		})
	}
}

// TestEngineWatchRulesNotifyOnce は、通知のジョブが通知して消した視聴予約を、次の照合で付け直さず、二度通知しないことを確かめる。
func TestEngineWatchRulesNotifyOnce(t *testing.T) {
	t.Parallel()

	f := newWatchFixture(t)
	f.createWatchRule(t, f.alice, `テスト番組 4$`)
	require.Equal(t, 1, f.run(t).WatchCreated)

	notifier := watch.NewNotifier(f.pool, f.clock, func(id int64) string { return fmt.Sprintf("/live/%d", id) })
	// 既定の 5 分前 (12:25) に通知する。
	f.clock.Advance(15 * time.Minute)
	notified, _, err := notifier.Notify(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, notified)

	assert.Equal(t, 0, f.run(t).WatchCreated)
	assert.Empty(t, f.watchReserved(t, f.alice))
	notified, _, err = notifier.Notify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, notified)
}

// TestEngineWatchRulesTouchSettings は、照合で視聴予約を付け外ししたユーザーにだけ、ユーザーの設定の変更として知らせることを確かめる。
func TestEngineWatchRulesTouchSettings(t *testing.T) {
	t.Parallel()

	f := newWatchFixture(t)
	f.createWatchRule(t, f.alice, `テスト番組 4$`)
	f.createWatchRule(t, f.bob, `一致しない番組`)
	alice, bob := f.settingsVersion(t, f.alice), f.settingsVersion(t, f.bob)

	f.run(t)
	assert.Greater(t, f.settingsVersion(t, f.alice), alice, "alice's watch reservations changed")
	assert.Equal(t, bob, f.settingsVersion(t, f.bob), "bob's watch reservations did not change")

	alice = f.settingsVersion(t, f.alice)
	f.run(t)
	assert.Equal(t, alice, f.settingsVersion(t, f.alice), "nothing changed in the second run")
}

// TestEngineWatchRulesPickOneOfSharedEvents は、同じイベントを複数のサービスで放送している番組が一致したとき、
// 視聴予約を 1 つだけ付け、二度通知しないことを確かめる。
func TestEngineWatchRulesPickOneOfSharedEvents(t *testing.T) {
	t.Parallel()

	f := newWatchFixture(t, sharedEvent(grServiceID, subServiceID), sharedEvent(subServiceID, grServiceID))
	conditions := watchConditions(sharedEventName)
	conditions.ServiceIDs = []int64{serviceID(), faketuner.ServiceItemID(grNetworkID, subServiceID)}
	_, err := watch.CreateRule(t.Context(), f.pool, f.clock, f.alice, &watch.RuleInput{Name: "共有", Enabled: true, Conditions: conditions})
	require.NoError(t, err)

	assert.Equal(t, 1, f.run(t).WatchCreated)
	rows, err := f.pool.Query(t.Context(), "SELECT service_id FROM watch_reservations WHERE user_id = $1", f.alice)
	require.NoError(t, err)
	serviceIDs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	assert.Equal(t, []int64{serviceID()}, serviceIDs)
}

// TestEngineWatchRulesPruneDismissals は、手で外した番組の記録を、番組が programs からなくなったら消すことを確かめる。
func TestEngineWatchRulesPruneDismissals(t *testing.T) {
	t.Parallel()

	f := newWatchFixture(t)
	for _, eventID := range []int64{4, 5} {
		require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(eventID), true))
		require.NoError(t, watch.SetReserved(t.Context(), f.pool, f.clock, f.alice, f.programKey(eventID), false))
	}
	assert.Equal(t, 0, f.run(t).DismissalsPruned)

	_, err := f.pool.Exec(t.Context(), "DELETE FROM programs WHERE service_id = $1 AND event_id = 5", serviceID())
	require.NoError(t, err)
	assert.Equal(t, 1, f.run(t).DismissalsPruned)
	var remaining []int64
	rows, err := f.pool.Query(t.Context(), "SELECT event_id FROM watch_rule_dismissals WHERE user_id = $1", f.alice)
	require.NoError(t, err)
	remaining, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	assert.Equal(t, []int64{4}, remaining)
}
