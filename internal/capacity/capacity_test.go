package capacity_test

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/capacity"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// diskSize は、プールのファイルシステムの大きさ。空き容量は、これから録画ファイルの大きさを引いたもの。
	diskSize = 10 << 20
	// fileSize は、録画ファイル 1 つの大きさ。
	fileSize = 1 << 20
)

type fixture struct {
	pool    *pgxpool.Pool
	clock   *clocktest.Fake
	dir     string
	manager *capacity.Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	return newFixtureWithFreeSpace(t, nil)
}

// newFixtureWithFreeSpace は、空き容量を freeSpace で測るフィクスチャを作る。nil なら、ディスクの大きさから録画ファイルの大きさを引く。
func newFixtureWithFreeSpace(t *testing.T, freeSpace kago.FreeSpaceFunc) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)

	dir := t.TempDir()
	// 空き容量は、ディスクの大きさから、プールにある録画ファイルの大きさの合計を引いたものとする。索引は小さいので数えない。
	usedSpace := func(path string) (uint64, error) {
		var used int64
		err := filepath.WalkDir(path, func(name string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(name) != ".m2ts" {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			used += info.Size()

			return nil
		})

		return uint64(max(diskSize-used, 0)), err
	}
	if freeSpace == nil {
		freeSpace = usedSpace
	}
	store := kago.New(map[string]string{poolID: dir}, freeSpace)

	return &fixture{
		pool: pool, clock: clk, dir: dir,
		manager: capacity.NewManager(pool, clk, store, audit.NewRecorder(clk), slog.New(slog.DiscardHandler)),
	}
}

// recording は、録画と、その録画ファイルと索引を作る。古い順に作るよう、作るたびに時計を進める。
func (f *fixture) recording(t *testing.T, name, state string, protected bool) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	now := f.clock.Now()
	f.clock.Advance(time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, $3, '', $4, $4, $5, $6, $4, $4, $4)`, recordingID, serviceID, name, now, state, protected)
	require.NoError(t, err)

	path := name + ".m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, path), make([]byte, fileSize), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(f.dir, path+".kotatsu"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, path+".kotatsu", "index"), []byte("index"), 0o600))
	for _, file := range []struct{ kind, path string }{{"segment", path}, {"index", path + ".kotatsu/index"}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)`, id.New(), recordingID, file.kind, poolID, file.path, fileSize, now)
		require.NoError(t, err)
	}

	return recordingID
}

func (f *fixture) setThreshold(t *testing.T, minFreeBytes int64) {
	t.Helper()

	_, err := sqlc.New(f.pool).UpsertStoragePool(t.Context(), sqlc.UpsertStoragePoolParams{ID: poolID, Priority: 0, MinFreeBytes: minFreeBytes, Now: f.clock.Now()})
	require.NoError(t, err)
}

func (f *fixture) names(t *testing.T) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT name FROM recordings ORDER BY recorded_from")
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return names
}

func (f *fixture) storageLowEvents(t *testing.T) []map[string]any {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT payload FROM notification_events WHERE kind = 'storage_low' ORDER BY occurred_at, id")
	require.NoError(t, err)
	payloads, err := pgx.CollectRows(rows, pgx.RowTo[map[string]any])
	require.NoError(t, err)

	return payloads
}

func (f *fixture) runOnce(t *testing.T) []*capacity.PoolResult {
	t.Helper()

	results, err := f.manager.RunOnce(t.Context())
	require.NoError(t, err)

	return results
}

// TestRunOnceDeletesOldRecordings は、空き容量が閾値を下回ったら、保護されていない古い録画から、閾値を上回るまで消し、
// 消したことを監査ログに残して、空き容量の不足を通知することを確かめる (設計書 6.1)。
func TestRunOnceDeletesOldRecordings(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.recording(t, "保護した録画", "completed", true)
	oldest := f.recording(t, "最も古い録画", "completed", false)
	f.recording(t, "失敗した録画", "failed", false)
	f.recording(t, "録画中の録画", "recording", false)
	f.recording(t, "新しい録画", "completed", false)
	// 録画ファイルは 5 つで、空き容量は 5 MiB。閾値を 6.5 MiB にすると、保護されていない古い録画を 2 つ消せば足りる。
	f.setThreshold(t, 6<<20+512<<10)

	results := f.runOnce(t)
	require.Len(t, results, 1)
	assert.Equal(t, &capacity.PoolResult{PoolID: poolID, FreeBytes: 7 << 20, Deleted: 2, Low: false}, results[0])
	assert.Equal(t, []string{"保護した録画", "録画中の録画", "新しい録画"}, f.names(t))
	// 録画ファイルと、付属ファイルのディレクトリを消す。
	_, err := os.Stat(filepath.Join(f.dir, "最も古い録画.m2ts"))
	require.ErrorIs(t, err, fs.ErrNotExist)
	_, err = os.Stat(filepath.Join(f.dir, "最も古い録画.m2ts.kotatsu"))
	require.ErrorIs(t, err, fs.ErrNotExist)

	var actorRole, reason string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT actor_role, summary->>'reason' FROM audit_events WHERE action = 'recording.deleted' AND target_id = $1", oldest.String()).Scan(&actorRole, &reason))
	assert.Equal(t, "nekonote", actorRole)
	assert.Equal(t, "storage_low", reason)
	events := f.storageLowEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, map[string]any{
		"poolId": poolID, "freeBytes": float64(7 << 20), "minFreeBytes": float64(6<<20 + 512<<10), "deletedRecordings": float64(2), "stillLow": false,
	}, events[0])
}

// TestRunOnceSkipsPools は、閾値を設定していないプールと、空き容量が閾値以上のプールでは、何も消さず、通知しないことを確かめる。
func TestRunOnceSkipsPools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		threshold *int64
	}{
		{name: "閾値を設定していない", threshold: nil},
		{name: "閾値が 0", threshold: new(int64(0))},
		{name: "空き容量が閾値以上", threshold: new(int64(9 << 20))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			f.recording(t, "録画", "completed", false)
			if tt.threshold != nil {
				f.setThreshold(t, *tt.threshold)
			}
			for _, result := range f.runOnce(t) {
				assert.Zero(t, result.Deleted)
				assert.False(t, result.Low)
			}
			assert.Equal(t, []string{"録画"}, f.names(t))
			assert.Empty(t, f.storageLowEvents(t))
		})
	}
}

// TestRunOnceReportsLowStorageOnce は、消せる録画がなく、空き容量が閾値を下回ったままのときは、下回り始めたときだけ通知することを確かめる。
func TestRunOnceReportsLowStorageOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	protected := f.recording(t, "保護した録画", "completed", true)
	f.setThreshold(t, 9<<20+512<<10)

	for range 2 {
		results := f.runOnce(t)
		require.Len(t, results, 1)
		assert.Equal(t, &capacity.PoolResult{PoolID: poolID, FreeBytes: 9 << 20, Deleted: 0, Low: true}, results[0])
	}
	events := f.storageLowEvents(t)
	require.Len(t, events, 1, "it is reported only when the pool becomes low")
	assert.Equal(t, true, events[0]["stillLow"])

	// 保護を外して空きができると、不足を解消したことにし、次に下回ったら、また通知する。
	_, err := f.pool.Exec(t.Context(), "UPDATE recordings SET protected = false WHERE id = $1", protected)
	require.NoError(t, err)
	assert.Equal(t, 1, f.runOnce(t)[0].Deleted)
	f.recording(t, "新しい録画", "completed", true)
	f.runOnce(t)
	assert.Len(t, f.storageLowEvents(t), 3)
}

// TestDeleteSkipsRecordingsProtectedMeanwhile は、消す候補に選んだ後に保護されたか、録画を始めた録画を、消さずに数えないことを確かめる。
func TestDeleteSkipsRecordingsProtectedMeanwhile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change string
	}{
		{name: "保護された", change: "UPDATE recordings SET protected = true WHERE id = $1"},
		{name: "録画を始めた", change: "UPDATE recordings SET state = 'recording' WHERE id = $1"},
		{name: "ほかで消された", change: "DELETE FROM recordings WHERE id = $1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, "録画", "completed", false)
			_, err := f.pool.Exec(t.Context(), tt.change, recordingID)
			require.NoError(t, err)

			deleted, err := f.manager.Delete(t.Context(), recordingID, poolID)
			require.NoError(t, err)
			assert.False(t, deleted)
			_, err = os.Stat(filepath.Join(f.dir, "録画.m2ts"))
			require.NoError(t, err, "the file is kept")
		})
	}
}

// TestRunOnceKeepsRecordingsWithoutFiles は、録画ファイルが見つからない録画を、消しても空きが増えないので消さないことを確かめる。
// プールの実体がマウントされず、空のディレクトリを測っているときに、録画の記録を消し続けないようにする (#513)。
func TestRunOnceKeepsRecordingsWithoutFiles(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.recording(t, "古い録画", "completed", false)
	f.recording(t, "後の録画", "completed", false)
	// マウントに失敗して、プールのディレクトリが空になった。
	entries, err := os.ReadDir(f.dir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(f.dir, entry.Name())))
	}
	f.setThreshold(t, diskSize+1)

	results := f.runOnce(t)
	require.Len(t, results, 1)
	assert.Equal(t, &capacity.PoolResult{PoolID: poolID, FreeBytes: diskSize, Deleted: 0, Low: true}, results[0])
	assert.Equal(t, []string{"古い録画", "後の録画"}, f.names(t))
}

// TestRunOnceStopsWhenFreeSpaceDoesNotGrow は、録画を消しても空き容量が増えなければ、その回は消すのをやめることを確かめる。
// スナップショットがあるときや、ファイルを開いたままの視聴者がいるときに、録画を次々に消さないようにする (#513)。
func TestRunOnceStopsWhenFreeSpaceDoesNotGrow(t *testing.T) {
	t.Parallel()

	f := newFixtureWithFreeSpace(t, func(string) (uint64, error) { return 1 << 20, nil })
	f.recording(t, "古い録画", "completed", false)
	f.recording(t, "次の録画", "completed", false)
	f.recording(t, "後の録画", "completed", false)
	f.setThreshold(t, 2<<20)

	results := f.runOnce(t)
	require.Len(t, results, 1)
	assert.Equal(t, &capacity.PoolResult{PoolID: poolID, FreeBytes: 1 << 20, Deleted: 1, Low: true}, results[0])
	assert.Equal(t, []string{"次の録画", "後の録画"}, f.names(t))
}
