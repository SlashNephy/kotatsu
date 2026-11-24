package cli_test

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/power"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// callCancel は、起こす予約を消したことを表す、記録の名前。
const callCancel = "cancel"

// recordingSleepSystem は、OS に伝えたことを記録する SleepSystem。
type recordingSleepSystem struct {
	mu    sync.Mutex
	calls []string
	// scheduled は、起こす予約を置いたときに閉じる。
	scheduled chan struct{}
}

var _ cli.SleepSystem = new(recordingSleepSystem)

func (s *recordingSleepSystem) SetInhibit(on bool) error {
	if on {
		s.record("inhibit")
	} else {
		s.record("release")
	}

	return nil
}

func (s *recordingSleepSystem) ScheduleWake(time.Time) error {
	s.record("schedule")
	close(s.scheduled)

	return nil
}

func (s *recordingSleepSystem) CancelWake() error {
	s.record(callCancel)

	return nil
}

func (s *recordingSleepSystem) Close() error {
	s.record("close")

	return nil
}

func (s *recordingSleepSystem) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string{}, s.calls...)
}

func (s *recordingSleepSystem) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

// TestSleepControl は、all の構成のときだけ、OS のスリープを扱うことと、近くに始まる予約で、
// スリープさせず起こす予約を置き、止めるときに外すことを確認する (#1522、設計書 11.7)。
func TestSleepControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		// disabled は、管理者が画面で OS のスリープを扱わないようにしているかどうか (#1869)。
		disabled bool
		// want は、OS に伝えること。nil なら、口を作らない。
		want []string
	}{
		{
			name: "all keeps the system awake for the near recording and releases it on stop", command: allCommand, disabled: false,
			// 起動したときに前の予約を消し、止めるときに抑止を外して予約を消す。
			want: []string{callCancel, "inhibit", "schedule", "release", callCancel, "close"},
		},
		{
			// 画面で切っていれば、近くに始まる予約があっても、スリープを止めず、起こす予約も置かない。
			// 判定のたびに設定を読むので、口は作り、前のプロセスが残した予約は消す。
			name: "disabled on the admin screen", command: allCommand, disabled: true,
			want: []string{callCancel, callCancel, "close"},
		},
		{name: "a single role does not manage sleep", command: roleTenban, disabled: false, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			database := databasetest.New(t)
			dataDir := t.TempDir()
			env := testEnv()
			env["KOTATSU_DATABASE_URL"] = database.URL.Value()
			env["KOTATSU_STORAGE_DATA_DIR"] = dataDir
			opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
			opts.LookupEnv = func(key string) (string, bool) {
				v, ok := env[key]

				return v, ok
			}
			// ロールの本体は動かさず、HTTP サーバーとスリープの扱いだけを動かす。
			for _, role := range opts.Roles {
				role.Run = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps) error {
					<-ctx.Done()

					return nil
				}
			}
			var (
				mu      sync.Mutex
				created *power.Options
			)
			system := &recordingSleepSystem{mu: sync.Mutex{}, calls: []string{}, scheduled: make(chan struct{})}
			opts.SleepSystem = func(o *power.Options) (cli.SleepSystem, error) {
				mu.Lock()
				defer mu.Unlock()
				created = o

				return system, nil
			}
			migrate := cli.NewCommand(opts)
			migrate.SetArgs([]string{subcommandMigrate})
			require.NoError(t, migrate.ExecuteContext(t.Context()))
			insertNearReservation(t, database.URL.Value(), tt.disabled)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			registered := make(chan struct{}, 1)
			opts.RunRegistry = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps, _ []string, _ string) error {
				registered <- struct{}{}
				<-ctx.Done()

				return nil
			}
			cmd := cli.NewCommand(opts)
			cmd.SetArgs([]string{tt.command})
			done := make(chan error, 1)
			go func() {
				done <- cmd.ExecuteContext(ctx)
			}()
			wait := registered
			switch {
			case tt.disabled:
				// 最初の判定を終えても、スリープを止めず、起こす予約も置かないことを確認する。
				require.Eventually(t, func() bool { return slices.Contains(system.Calls(), callCancel) }, 30*time.Second, 10*time.Millisecond)
				require.Never(t, func() bool {
					calls := system.Calls()

					return slices.Contains(calls, "inhibit") || slices.Contains(calls, "schedule")
				}, 2*time.Second, 50*time.Millisecond)
				wait = make(chan struct{}, 1)
				wait <- struct{}{}
			case tt.want != nil:
				// 最初の判定で、近くに始まる予約の前に起こす予約を置くまで待つ。
				wait = make(chan struct{}, 1)
				go func() {
					<-system.scheduled
					wait <- struct{}{}
				}()
			}
			select {
			case <-wait:
			case <-time.After(30 * time.Second):
				t.Fatal("the command did not start")
			}
			cancel()
			require.NoError(t, <-done)

			mu.Lock()
			defer mu.Unlock()
			if tt.want == nil {
				assert.Nil(t, created)

				return
			}
			require.NotNil(t, created)
			assert.Equal(t, dataDir, created.DataDir)
			assert.Equal(t, tt.want, system.Calls())
		})
	}
}

// insertNearReservation は、8 分後に始まる予約を DB に置く。録画する予定の開始は抑止の前倒し (10 分) の内にあり、起こす時刻 (5 分前) はまだ過ぎていない。
// sleepDisabled なら、全体の設定で OS のスリープを扱わないようにする (#1869)。
func insertNearReservation(t *testing.T, databaseURL string, sleepDisabled bool) {
	t.Helper()

	pool, err := pgxpool.New(t.Context(), databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	now := time.Now().UTC()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{sql: "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', 'GR', 10)", args: nil},
		{sql: `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES (3273601024, 32736, 1024, 'test', 'test', 1, 'GR', '27', $1, $1)`, args: []any{now}},
		{
			sql: `INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
			enabled, follow_event_relay, settings, created_at, updated_at, assignment_state, planned_start_at, planned_end_at)
			VALUES ($1, 'time', 3273601024, NULL, $2, $3, '', '', '{}', '[]', true, false, '{}', $4, $4, 'ok', $2, $3)`,
			args: []any{id.New(), now.Add(8 * time.Minute), now.Add(time.Hour), now},
		},
		{
			sql:  "INSERT INTO system_settings (id, recording_defaults, archive_retention_days, sleep_control_enabled, updated_at) VALUES (1, '{}', 90, $1, $2)",
			args: []any{!sleepDisabled, now},
		},
	} {
		_, err := pool.Exec(t.Context(), stmt.sql, stmt.args...)
		require.NoError(t, err)
	}
}
