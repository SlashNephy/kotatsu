package backup_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/backup"
)

// 定期のバックアップのファイルの名前。書き出した時刻 (UTC) を名前に入れる。
const (
	firstBackup  = "kotatsu-backup-20261117T030000Z.age"
	manualBackup = "kotatsu-backup-20261117T040100Z.age"
	nextBackup   = "kotatsu-backup-20261118T040100Z.age"
)

func runJob(manual bool) *river.Job[backup.RunArgs] {
	return &river.Job[backup.RunArgs]{JobRow: new(rivertype.JobRow), Args: backup.RunArgs{Manual: manual}}
}

// listBackups は、dir の中のファイルの名前を返す。
func listBackups(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func newWorker(pool *pgxpool.Pool, clk *clocktest.Fake, dir string) *backup.Worker {
	return backup.NewWorker(&backup.WorkerOptions{
		DB: pool, Store: kago.New(nil, nil), SecretKey: testSecretKey, Dir: dir, Clock: clk, Logger: slog.New(slog.DiscardHandler),
	})
}

func saveSettings(t *testing.T, pool *pgxpool.Pool, clk *clocktest.Fake, recipients []string, generations int) {
	t.Helper()

	require.NoError(t, backup.SaveSettings(t.Context(), pool, clk.Now(), &backup.Settings{
		Recipients: recipients, IntervalHours: 24, Generations: generations, UpdatedAt: nil,
	}))
}

// TestWorkerSchedule は、定期のバックアップが、公開鍵を登録したときだけ、間隔ごとに書き出し、残す世代の数より古いファイルを消すことと、
// 頼まれたものは間隔を待たずに書き出すことを確かめる (#1926)。
func TestWorkerSchedule(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	dir := t.TempDir()
	// ほかの名前のファイルは、古い世代として消さない。
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o600))
	worker := newWorker(pool, clk, dir)
	identity := newIdentity(t)

	// 公開鍵がなければ、定期のものは何もしない。
	require.NoError(t, worker.Work(t.Context(), runJob(false)))
	assert.Equal(t, []string{"notes.txt"}, listBackups(t, dir))
	_, err := backup.LatestRun(t.Context(), pool)
	require.ErrorIs(t, err, backup.ErrNoRuns)

	saveSettings(t, pool, clk, []string{identity.Recipient().String()}, 2)
	steps := []struct {
		name    string
		advance time.Duration
		manual  bool
		want    []string
	}{
		{name: "the first check writes a backup", advance: 0, manual: false, want: []string{firstBackup}},
		{name: "a check within the interval writes nothing", advance: time.Hour, manual: false, want: []string{firstBackup}},
		{name: "a manual request does not wait for the interval", advance: time.Minute, manual: true, want: []string{
			firstBackup, manualBackup,
		}},
		{name: "a check after the interval writes one and removes the oldest", advance: 24 * time.Hour, manual: false, want: []string{
			manualBackup, nextBackup,
		}},
	}
	for _, step := range steps {
		clk.Advance(step.advance)
		require.NoError(t, worker.Work(t.Context(), runJob(step.manual)), step.name)
		got := slices.DeleteFunc(listBackups(t, dir), func(name string) bool { return name == "notes.txt" })
		assert.Equal(t, step.want, got, step.name)
	}
	assert.Contains(t, listBackups(t, dir), "notes.txt")

	// 書き出したファイルは、持ち主だけが読め、登録した公開鍵の秘密鍵で読める。
	path := filepath.Join(dir, nextBackup)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	_, err = age.Decrypt(f, identity)
	require.NoError(t, err)

	latest, err := backup.LatestRun(t.Context(), pool)
	require.NoError(t, err)
	assert.False(t, latest.Manual)
	assert.Equal(t, nextBackup, *latest.FileName)
	assert.Equal(t, info.Size(), *latest.SizeBytes)
	assert.Nil(t, latest.Error)
}

// TestWorkerFailure は、書き出す先のディレクトリがなければ、失敗を結果に残し、間隔を待たずに 1 時間後に書き出し直すことを確かめる (#1926)。
func TestWorkerFailure(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	saveSettings(t, pool, clk, []string{newIdentity(t).Recipient().String()}, 7)

	require.ErrorIs(t, newWorker(pool, clk, "").Work(t.Context(), runJob(false)), backup.ErrNoBackupDir)
	latest, err := backup.LatestRun(t.Context(), pool)
	require.NoError(t, err)
	require.NotNil(t, latest.Error)
	assert.Contains(t, *latest.Error, "backup.dir")
	assert.Nil(t, latest.FileName)

	dir := t.TempDir()
	worker := newWorker(pool, clk, dir)
	clk.Advance(30 * time.Minute)
	require.NoError(t, worker.Work(t.Context(), runJob(false)))
	assert.Empty(t, listBackups(t, dir), "a failed backup is retried an hour later")
	clk.Advance(30 * time.Minute)
	require.NoError(t, worker.Work(t.Context(), runJob(false)))
	assert.Len(t, listBackups(t, dir), 1)
}

// TestSaveSettings は、読めない公開鍵と、範囲の外の値を拒むことを確かめる (#1926)。
func TestSaveSettings(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	identity := newIdentity(t)
	key := identity.Recipient().String()
	many := make([]string, 0, 17)
	for range 17 {
		many = append(many, newIdentity(t).Recipient().String())
	}
	tests := []struct {
		name     string
		settings backup.Settings
		wantErr  bool
	}{
		{name: "a public key", settings: backup.Settings{Recipients: []string{key}, IntervalHours: 1, Generations: 1, UpdatedAt: nil}, wantErr: false},
		{name: "no public keys", settings: backup.Settings{Recipients: []string{}, IntervalHours: 720, Generations: 365, UpdatedAt: nil}, wantErr: false},
		{name: "a private key", settings: backup.Settings{Recipients: []string{identity.String()}, IntervalHours: 24, Generations: 7, UpdatedAt: nil}, wantErr: true},
		{name: "two keys in one line", settings: backup.Settings{Recipients: []string{key + "\n" + key}, IntervalHours: 24, Generations: 7, UpdatedAt: nil}, wantErr: true},
		{name: "a comment", settings: backup.Settings{Recipients: []string{"# " + key}, IntervalHours: 24, Generations: 7, UpdatedAt: nil}, wantErr: true},
		{name: "too long an interval", settings: backup.Settings{Recipients: []string{key}, IntervalHours: 721, Generations: 7, UpdatedAt: nil}, wantErr: true},
		{name: "no generations", settings: backup.Settings{Recipients: []string{key}, IntervalHours: 24, Generations: 0, UpdatedAt: nil}, wantErr: true},
		{name: "too many keys", settings: backup.Settings{Recipients: many, IntervalHours: 24, Generations: 7, UpdatedAt: nil}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := backup.SaveSettings(t.Context(), pool, testNow(), &tt.settings)
			if tt.wantErr {
				require.ErrorIs(t, err, backup.ErrInvalidSettings)

				return
			}
			require.NoError(t, err)
		})
	}
}
