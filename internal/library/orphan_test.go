package library_test

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/jobs"
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
	// 録画のファイルと、上のファイルは、書いてから時間が経ったものにする。
	old := f.clock.Now().Add(-2 * time.Hour)
	root, err := os.OpenRoot(f.dir)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, fs.WalkDir(root.FS(), ".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return root.Chtimes(path, old, old)
	}))
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

// TestDeleteOrphan は、DB にないファイルを確かめ直して消し、監査ログに残すことと、使っているファイルやプールの外は消さないことを確かめる。
func TestDeleteOrphan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		sidecar bool
		wantErr error
	}{
		{name: "DB にないファイルを消す", path: orphanName, sidecar: false, wantErr: nil},
		{name: "持ち主のない付属ファイルのディレクトリを消す", path: orphanSidecar, sidecar: true, wantErr: nil},
		{name: "録画のファイルは消さない", path: recordingName + ".m2ts", sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "録画の付属ファイルのディレクトリは消さない", path: recordingName + ".m2ts.kotatsu", sidecar: true, wantErr: library.ErrNotOrphan},
		{name: "つないでいる途中のファイルは消さない", path: recordingName + ".m2ts.joining", sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "つなぐ元のファイルは消さない", path: removedByJoin, sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "書いたばかりのファイルは消さない", path: "録画中.m2ts", sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "隠しファイルは消さない", path: ".DS_Store", sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "ファイルを付属ファイルのディレクトリとしては消さない", path: orphanName, sidecar: true, wantErr: library.ErrNotOrphan},
		{name: "プールの外は消さない", path: "../" + orphanName, sidecar: false, wantErr: library.ErrNotOrphan},
		{name: "ないファイル", path: "ない.m2ts", sidecar: false, wantErr: library.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := orphanFixture(t)
			err := f.library.DeleteOrphan(t.Context(), f.pool, audit.User(f.userID), poolID, tt.path, tt.sidecar)
			var events int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM audit_events WHERE action = 'storage_file.deleted' AND target_id = $1 AND summary->>'path' = $2",
				poolID, tt.path).Scan(&events))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, events)

				return
			}
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(f.dir, tt.path))
			assert.NoDirExists(t, filepath.Join(f.dir, tt.path))
			assert.Equal(t, 1, events)
			assert.NotContains(t, f.orphans(t), orphan{path: tt.path, sidecar: tt.sidecar})
			assert.FileExists(t, filepath.Join(f.dir, recordingName+".m2ts"))
		})
	}
}
