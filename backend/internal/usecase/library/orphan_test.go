package library_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/library"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
)

const (
	orphanName = "消し忘れ.m2ts"
	// orphanSegmentDir は、持ち主のセグメントのファイルが DB にない、生成物のディレクトリ。
	orphanSegmentDir = "0199f0a0-0000-7000-8000-00000000dead"
	removedByJoin    = "つなぐ元.m2ts"
	// timeshiftChunk は、タイムシフトのディレクトリにある、DB にないチャンクのファイル。
	timeshiftChunk = "1/chunk.m2ts"
	// removedByJoinSegmentDir は、つなぐ元のセグメントの生成物のディレクトリ。
	removedByJoinSegmentDir = "0199f0a0-0000-7000-8000-00000000beef"
)

// orphanFixture は、録画 1 つと、DB にないファイル、使っているファイル、書いたばかりのファイル、調べないファイルを置いたプールを作る。
func orphanFixture(t *testing.T) *fixture {
	t.Helper()

	f := newFixture(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	f.recording(t, completed, false)
	write := func(root, rel string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	}
	for _, rel := range []string{orphanName, recordingName + ".m2ts.joining", removedByJoin, ".DS_Store", "lost+found/#123"} {
		write(f.dir, rel)
	}
	write(f.timeshiftRoot, timeshiftChunk)
	for _, rel := range []string{orphanSegmentDir + "/thumbnail.jpg", removedByJoinSegmentDir + "/index", "main/古い録画.m2ts.kotatsu/index"} {
		write(f.dataRoot, rel)
	}
	// 名前を変えた日時 (ctime) は戻せないので、時計を進めて、ここまでに書いたファイルを、書いてから時間が経ったものにする。
	f.clock.Advance(time.Since(f.clock.Now()) + 2*time.Hour)
	// 書いたばかりのファイルは、録画を始めたところで、まだ DB にないことがある。
	write(f.dir, "録画中.m2ts")
	recent := f.clock.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, "録画中.m2ts"), recent, recent))

	// 置き換えるジョブが終わるまで、つなぐ元のファイルは残っている。
	args, err := json.Marshal(segjoin.FinalizeArgs{
		RecordingID: f.userID, PoolID: poolID, Path: recordingName + ".m2ts",
		Removed: []segjoin.File{{ID: uuid.MustParse(removedByJoinSegmentDir), PoolID: poolID, Path: removedByJoin}},
	})
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "INSERT INTO river_job (kind, args, max_attempts, queue) VALUES ($1, $2, 25, 'default')",
		new(segjoin.FinalizeArgs).Kind(), args)
	require.NoError(t, err)

	return f
}

type orphan struct {
	pool       string
	path       string
	segmentDir bool
}

func (f *fixture) orphans(t *testing.T) []orphan {
	t.Helper()

	found, err := f.library.FindOrphans(t.Context(), f.pool)
	require.NoError(t, err)
	result := make([]orphan, 0, len(found))
	for _, file := range found {
		result = append(result, orphan{pool: file.PoolID, path: file.Entry.Path, segmentDir: file.Entry.SegmentDir})
	}

	return result
}

// TestFindOrphans は、DB にないファイルと、持ち主のない生成物のディレクトリだけを返し、
// 録画のファイルとその生成物、つないでいる途中のファイルとつなぐ元のファイルとその生成物、書いたばかりのファイル、
// 隠しファイルと lost+found は返さないことを確かめる。データディレクトリにある、ID の名前でないものは、ファイルとして返す。
func TestFindOrphans(t *testing.T) {
	t.Parallel()

	f := orphanFixture(t)
	assert.ElementsMatch(t, []orphan{
		{pool: poolID, path: orphanName, segmentDir: false},
		{pool: kago.DataPoolID, path: orphanSegmentDir, segmentDir: true},
		{pool: kago.DataPoolID, path: "main/古い録画.m2ts.kotatsu/index", segmentDir: false},
	}, f.orphans(t))
}

// TestDeleteOrphans は、DB にないファイルを確かめ直して消し、監査ログに残すことと、使っているファイルやプールの外は消さないことを確かめる。
func TestDeleteOrphans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pool       string
		path       string
		segmentDir bool
		deleted    bool
	}{
		{name: "DB にないファイルを消す", pool: poolID, path: orphanName, segmentDir: false, deleted: true},
		{name: "持ち主のない生成物のディレクトリを消す", pool: kago.DataPoolID, path: orphanSegmentDir, segmentDir: true, deleted: true},
		{name: "録画のファイルは消さない", pool: poolID, path: recordingName + ".m2ts", segmentDir: false, deleted: false},
		{name: "録画の生成物のディレクトリは消さない", pool: kago.DataPoolID, path: "", segmentDir: true, deleted: false},
		{name: "生成物のディレクトリの中は消さない", pool: kago.DataPoolID, path: orphanSegmentDir + "/thumbnail.jpg", segmentDir: false, deleted: false},
		{name: "プールの中は生成物のディレクトリとしては消さない", pool: poolID, path: orphanSegmentDir, segmentDir: true, deleted: false},
		{name: "つないでいる途中のファイルは消さない", pool: poolID, path: recordingName + ".m2ts.joining", segmentDir: false, deleted: false},
		{name: "つなぐ元のファイルは消さない", pool: poolID, path: removedByJoin, segmentDir: false, deleted: false},
		{name: "つなぐ元の生成物のディレクトリは消さない", pool: kago.DataPoolID, path: removedByJoinSegmentDir, segmentDir: true, deleted: false},
		{name: "書いたばかりのファイルは消さない", pool: poolID, path: "録画中.m2ts", segmentDir: false, deleted: false},
		{name: "隠しファイルは消さない", pool: poolID, path: ".DS_Store", segmentDir: false, deleted: false},
		{name: "ファイルを生成物のディレクトリとしては消さない", pool: poolID, path: orphanName, segmentDir: true, deleted: false},
		{name: "プールの外は消さない", pool: poolID, path: "../" + orphanName, segmentDir: false, deleted: false},
		{name: "タイムシフトのバッファは消さない", pool: kago.TimeshiftPoolID, path: timeshiftChunk, segmentDir: false, deleted: false},
		{name: "ないファイル", pool: poolID, path: "ない.m2ts", segmentDir: false, deleted: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := orphanFixture(t)
			path := tt.path
			if path == "" {
				// 録画の生成物のディレクトリ。ID は fixture を作るまで分からない。
				path = f.segmentID.String()
			}
			result, err := f.library.DeleteOrphans(t.Context(), f.pool, audit.User(f.userID),
				[]*library.OrphanTarget{{PoolID: tt.pool, Path: path, SegmentDir: tt.segmentDir}})
			require.NoError(t, err)
			var events int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM audit_events WHERE action = 'storage_file.deleted' AND target_id = $1 AND summary->>'path' = $2",
				tt.pool, path).Scan(&events))
			if !tt.deleted {
				assert.Equal(t, &library.DeleteOrphansResult{Deleted: 0, Skipped: 1}, result)
				assert.Zero(t, events)
				assert.FileExists(t, filepath.Join(f.timeshiftRoot, timeshiftChunk))

				return
			}
			root := f.dir
			if tt.pool == kago.DataPoolID {
				root = f.dataRoot
			}
			assert.Equal(t, &library.DeleteOrphansResult{Deleted: 1, Skipped: 0}, result)
			assert.NoFileExists(t, filepath.Join(root, path))
			assert.NoDirExists(t, filepath.Join(root, path))
			assert.Equal(t, 1, events)
			assert.NotContains(t, f.orphans(t), orphan{pool: tt.pool, path: path, segmentDir: tt.segmentDir})
			assert.FileExists(t, filepath.Join(f.dir, recordingName+".m2ts"))
			assert.DirExists(t, filepath.Join(f.dataRoot, f.segmentID.String()))
		})
	}
}
