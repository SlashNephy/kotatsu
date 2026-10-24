package graph_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	orphanFilesQuery = `{ orphanFiles { poolId path segmentDir size } }`
	orphanFileName   = "消し忘れ.m2ts"
	// orphanSegmentDir は、持ち主のセグメントのファイルが DB にない、データディレクトリの生成物のディレクトリ。
	orphanSegmentDir     = "0199f0a0-0000-7000-8000-00000000dead"
	deleteOrphanMutation = `mutation($files: [OrphanFileInput!]!) { deleteOrphanFiles(files: $files) { deleted skipped } }`
	varFiles             = "files"
)

// TestOrphanFiles は、管理者が、DB にないファイルを探して消せ、録画のファイルは消さず、ほかのロールは使えないことを確かめる。
func TestOrphanFiles(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	f.addRecording(t, dir, remainingRecordingName, "completed", time.Hour)
	require.NoError(t, os.WriteFile(filepath.Join(dir, orphanFileName), []byte("orphan"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dataRootOf(dir), orphanSegmentDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataRootOf(dir), orphanSegmentDir, "thumbnail.jpg"), []byte("jpeg"), 0o600))
	// 書くか名前を変えてから時間が経ったファイルだけを、DB にないとみなす。
	f.clock.Advance(time.Since(f.clock.Now()) + 2*time.Hour)
	// ファイルを消すには直近の再認証が要るので、時計を進めた後にログインする。
	admin := f.as(t, adminName, auth.RoleAdmin)

	type orphanFile struct {
		PoolID     *string `json:"poolId"`
		Path       string  `json:"path"`
		SegmentDir bool    `json:"segmentDir"`
		Size       int64   `json:"size"`
	}
	var found struct {
		OrphanFiles []orphanFile `json:"orphanFiles"`
	}
	require.Empty(t, admin.query(t, orphanFilesQuery, nil, &found))
	// データディレクトリはプールではないので、プールの ID を返さない。
	assert.Equal(t, []orphanFile{
		{PoolID: new(poolMain), Path: orphanFileName, SegmentDir: false, Size: 6},
		{PoolID: nil, Path: orphanSegmentDir, SegmentDir: true, Size: 4},
	}, found.OrphanFiles)

	var deleted struct {
		DeleteOrphanFiles struct {
			Deleted int `json:"deleted"`
			Skipped int `json:"skipped"`
		} `json:"deleteOrphanFiles"`
	}
	files := []orphanFileInput{
		{PoolID: new(poolMain), Path: orphanFileName, SegmentDir: false},
		{PoolID: nil, Path: orphanSegmentDir, SegmentDir: true},
		{PoolID: new(poolMain), Path: remainingRecordingName + ".m2ts", SegmentDir: false},
	}
	require.Empty(t, admin.query(t, deleteOrphanMutation, map[string]any{varFiles: files}, &deleted))
	assert.Equal(t, 2, deleted.DeleteOrphanFiles.Deleted)
	assert.Equal(t, 1, deleted.DeleteOrphanFiles.Skipped)
	assert.NoFileExists(t, filepath.Join(dir, orphanFileName))
	assert.NoDirExists(t, filepath.Join(dataRootOf(dir), orphanSegmentDir))
	assert.FileExists(t, filepath.Join(dir, remainingRecordingName+".m2ts"))

	var ignored struct{}
	assert.Equal(t, []string{codeForbidden}, f.query(t, orphanFilesQuery, nil, &ignored))
	assert.Equal(t, []string{codeForbidden}, f.query(t, deleteOrphanMutation, map[string]any{varFiles: files[:1]}, &ignored))
}
