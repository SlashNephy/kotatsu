package graph_test

import (
	"log/slog"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	backupQuery          = `{ backup { settings { recipients intervalHours generations updatedAt } latestRun { manual fileName error } } }`
	updateBackupSettings = `mutation($input: BackupSettingsInput!) { updateBackupSettings(input: $input) { settings { recipients intervalHours generations } } }`
	runBackup            = `mutation { runBackup { settings { recipients } } }`
	keyRecipients        = "recipients"
)

type backupStatus struct {
	Settings struct {
		Recipients    []string `json:"recipients"`
		IntervalHours int      `json:"intervalHours"`
		Generations   int      `json:"generations"`
		UpdatedAt     *string  `json:"updatedAt"`
	} `json:"settings"`
	LatestRun *struct {
		Manual   bool    `json:"manual"`
		FileName *string `json:"fileName"`
		Error    *string `json:"error"`
	} `json:"latestRun"`
}

// TestBackup は、管理者が、定期のバックアップの公開鍵と間隔と世代の数を変え、今すぐバックアップするよう頼めることと、
// 読めない公開鍵や範囲の外の値を拒むこと、管理者でなければ使えないことを確認する (#1926)。
func TestBackup(t *testing.T) {
	t.Parallel()

	inserter := &lateInserter{client: nil}
	f := newFixture(t, &graph.Options{Jobs: inserter})
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	inserter.client = client
	admin := f.as(t, adminName, domain.RoleAdmin)
	status := func() backupStatus {
		t.Helper()

		var got struct {
			Backup backupStatus `json:"backup"`
		}
		require.Empty(t, admin.query(t, backupQuery, nil, &got))

		return got.Backup
	}

	// 一度も変えていなければ、既定値で、公開鍵はなく、まだバックアップしていない。
	initial := status()
	assert.Empty(t, initial.Settings.Recipients)
	assert.Equal(t, 24, initial.Settings.IntervalHours)
	assert.Equal(t, 7, initial.Settings.Generations)
	assert.Nil(t, initial.Settings.UpdatedAt)
	assert.Nil(t, initial.LatestRun)

	// 公開鍵がなければ、今すぐバックアップするよう頼めない。
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, runBackup, nil, nil))

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	key := identity.Recipient().String()
	tests := []struct {
		name  string
		input map[string]any
		want  []string
	}{
		{name: "a key that is not an age public key", input: map[string]any{keyRecipients: []string{"ssh-ed25519 AAAA"}}, want: []string{codeBadUserInput}},
		{name: "a private key", input: map[string]any{keyRecipients: []string{identity.String()}}, want: []string{codeBadUserInput}},
		{name: "an interval out of range", input: map[string]any{"intervalHours": 0}, want: []string{codeBadUserInput}},
		{name: "generations out of range", input: map[string]any{"generations": 366}, want: []string{codeBadUserInput}},
		{name: "a valid key with spaces and duplicates", input: map[string]any{keyRecipients: []string{" " + key + " ", key, ""}, "intervalHours": 6}, want: []string{}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, admin.query(t, updateBackupSettings, map[string]any{"input": tt.input}, nil), tt.name)
	}
	saved := status()
	assert.Equal(t, []string{key}, saved.Settings.Recipients)
	assert.Equal(t, 6, saved.Settings.IntervalHours)
	assert.Equal(t, 7, saved.Settings.Generations, "an omitted field is kept")
	assert.NotNil(t, saved.Settings.UpdatedAt)

	// 頼んだバックアップは、待っている間は重ねて頼めない。
	require.Empty(t, admin.query(t, runBackup, nil, nil))
	assert.Equal(t, []string{codeConflict}, admin.query(t, runBackup, nil, nil))

	// 管理者でなければ、見ることも操作もできない。
	for _, role := range []domain.Role{domain.RoleMember, domain.RoleViewer} {
		other := f.as(t, string(role), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, backupQuery, nil, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, updateBackupSettings, map[string]any{"input": map[string]any{"generations": 3}}, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, runBackup, nil, nil), role)
	}

	// 設定を変えたことと、頼んだことを監査ログに残す。
	var actions []string
	rows, err := f.pool.Query(t.Context(), "SELECT action FROM audit_events WHERE target_type = 'backup' ORDER BY occurred_at")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"backup.settings.update", "backup.run"}, actions)
}
