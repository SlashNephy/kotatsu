package library_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/library"
	"github.com/SlashNephy/kotatsu/internal/segjoin"
)

const (
	orphanName    = "消し忘れ.m2ts"
	orphanSidecar = "消えた録画.m2ts.kotatsu"
	removedByJoin = "つなぐ元.m2ts"
)

// orphanFixture は、録画 1 つと、DB にないファイル、使っているファイル、書いたばかりのファイル、調べないファイルを置いたプールを作る。
func orphanFixture(t *testing.T) *fixture {
	t.Helper()

	f := newFixture(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	f.recording(t, completed, false)
	write := func(rel string) {
		path := filepath.Join(f.dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	}
	for _, rel := range []string{
		orphanName,
		orphanSidecar + "/thumbnail.jpg",
		recordingName + ".m2ts.joining",
		removedByJoin,
		".DS_Store",
		"lost+found/#123",
	} {
		write(rel)
	}
	// 名前を変えた日時 (ctime) は戻せないので、時計を進めて、ここまでに書いたファイルを、書いてから時間が経ったものにする。
	f.clock.Advance(time.Since(f.clock.Now()) + 2*time.Hour)
	// 書いたばかりのファイルは、録画を始めたところで、まだ DB にないことがある。
	write("録画中.m2ts")
	recent := f.clock.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, "録画中.m2ts"), recent, recent))

	// 置き換えるジョブが終わるまで、つなぐ元のファイルは残っている。
	args, err := json.Marshal(segjoin.FinalizeArgs{
		RecordingID: f.userID, PoolID: poolID, Path: recordingName + ".m2ts",
		Removed: []segjoin.File{{PoolID: poolID, Path: removedByJoin}},
	})
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "INSERT INTO river_job (kind, args, max_attempts, queue) VALUES ($1, $2, 25, 'default')",
		new(segjoin.FinalizeArgs).Kind(), args)
	require.NoError(t, err)

	return f
}

type orphan struct {
	path    string
	sidecar bool
}

func (f *fixture) orphans(t *testing.T) []orphan {
	t.Helper()

	found, err := f.library.FindOrphans(t.Context(), f.pool)
	require.NoError(t, err)
	result := make([]orphan, 0, len(found))
	for _, file := range found {
		assert.Equal(t, poolID, file.PoolID)
		result = append(result, orphan{path: file.Entry.Path, sidecar: file.Entry.Sidecar})
	}

	return result
}

// TestFindOrphans は、DB にないファイルと、持ち主のない付属ファイルのディレクトリだけを返し、
// 録画のファイル、つないでいる途中のファイルとつなぐ元のファイル、書いたばかりのファイル、隠しファイルと lost+found は返さないことを確かめる。
func TestFindOrphans(t *testing.T) {
	t.Parallel()

	f := orphanFixture(t)
	assert.ElementsMatch(t, []orphan{{path: orphanName, sidecar: false}, {path: orphanSidecar, sidecar: true}}, f.orphans(t))
}

// TestDeleteOrphans は、DB にないファイルを確かめ直して消し、監査ログに残すことと、使っているファイルやプールの外は消さないことを確かめる。
func TestDeleteOrphans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		sidecar bool
		deleted bool
	}{
		{name: "DB にないファイルを消す", path: orphanName, sidecar: false, deleted: true},
		{name: "持ち主のない付属ファイルのディレクトリを消す", path: orphanSidecar, sidecar: true, deleted: true},
		{name: "録画のファイルは消さない", path: recordingName + ".m2ts", sidecar: false, deleted: false},
		{name: "録画の付属ファイルのディレクトリは消さない", path: recordingName + ".m2ts.kotatsu", sidecar: true, deleted: false},
		{name: "付属ファイルのディレクトリの中は消さない", path: orphanSidecar + "/thumbnail.jpg", sidecar: false, deleted: false},
		{name: "つないでいる途中のファイルは消さない", path: recordingName + ".m2ts.joining", sidecar: false, deleted: false},
		{name: "つなぐ元のファイルは消さない", path: removedByJoin, sidecar: false, deleted: false},
		{name: "書いたばかりのファイルは消さない", path: "録画中.m2ts", sidecar: false, deleted: false},
		{name: "隠しファイルは消さない", path: ".DS_Store", sidecar: false, deleted: false},
		{name: "ファイルを付属ファイルのディレクトリとしては消さない", path: orphanName, sidecar: true, deleted: false},
		{name: "プールの外は消さない", path: "../" + orphanName, sidecar: false, deleted: false},
		{name: "ないファイル", path: "ない.m2ts", sidecar: false, deleted: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := orphanFixture(t)
			result, err := f.library.DeleteOrphans(t.Context(), f.pool, audit.User(f.userID),
				[]*library.OrphanTarget{{PoolID: poolID, Path: tt.path, Sidecar: tt.sidecar}})
			require.NoError(t, err)
			var events int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM audit_events WHERE action = 'storage_file.deleted' AND target_id = $1 AND summary->>'path' = $2",
				poolID, tt.path).Scan(&events))
			if !tt.deleted {
				assert.Equal(t, &library.DeleteOrphansResult{Deleted: 0, Skipped: 1}, result)
				assert.Zero(t, events)

				return
			}
			assert.Equal(t, &library.DeleteOrphansResult{Deleted: 1, Skipped: 0}, result)
			assert.NoFileExists(t, filepath.Join(f.dir, tt.path))
			assert.NoDirExists(t, filepath.Join(f.dir, tt.path))
			assert.Equal(t, 1, events)
			assert.NotContains(t, f.orphans(t), orphan{path: tt.path, sidecar: tt.sidecar})
			assert.FileExists(t, filepath.Join(f.dir, recordingName+".m2ts"))
		})
	}
}

// TestOrphansInSidecarDir は、付属ファイルをまとめて置くディレクトリでも、持ち主の録画ファイルが DB にないディレクトリだけを探して消せることを確かめる。
func TestOrphansInSidecarDir(t *testing.T) {
	t.Parallel()

	f := orphanFixture(t)
	sidecarDir := t.TempDir()
	lib := library.New(f.clock, audit.NewRecorder(f.clock), kago.New(map[string]string{poolID: f.dir}, nil).WithSidecarDir(sidecarDir),
		slog.New(slog.DiscardHandler))
	owned := filepath.Join(poolID, recordingName+".m2ts.kotatsu")
	gone := filepath.Join(poolID, "番組", "消えた録画.m2ts.kotatsu")
	for _, dir := range []string{owned, gone} {
		require.NoError(t, os.MkdirAll(filepath.Join(sidecarDir, dir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sidecarDir, dir, "thumbnail.jpg"), []byte("jpeg"), 0o600))
	}
	f.clock.Advance(2 * time.Hour)

	found, err := lib.FindOrphans(t.Context(), f.pool)
	require.NoError(t, err)
	inSidecarDir := []string{}
	for _, file := range found {
		if file.PoolID == kago.SidecarPoolID {
			inSidecarDir = append(inSidecarDir, file.Entry.Path)
		}
	}
	assert.Equal(t, []string{filepath.ToSlash(gone)}, inSidecarDir)

	result, err := lib.DeleteOrphans(t.Context(), f.pool, audit.User(f.userID), []*library.OrphanTarget{
		{PoolID: kago.SidecarPoolID, Path: filepath.ToSlash(gone), Sidecar: true},
		{PoolID: kago.SidecarPoolID, Path: filepath.ToSlash(owned), Sidecar: true},
	})
	require.NoError(t, err)
	assert.Equal(t, &library.DeleteOrphansResult{Deleted: 1, Skipped: 1}, result)
	// 空になったディレクトリも消し、持ち主のいるディレクトリは残す。
	assert.NoDirExists(t, filepath.Join(sidecarDir, poolID, "番組"))
	assert.DirExists(t, filepath.Join(sidecarDir, owned))
}
