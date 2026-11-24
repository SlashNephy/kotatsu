package graph_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	libraryArtifactsQuery   = `{ libraryArtifacts { size files } }`
	clearLibraryArtifacts   = `mutation { clearLibraryArtifacts { size files } }`
	actionArtifactsClear    = "library_artifacts.clear"
	artifactsSegmentDirName = "0199f0a0-0000-7000-8000-00000000beef"
)

type libraryArtifactsResult struct {
	Size  int64 `json:"size"`
	Files int   `json:"files"`
}

// TestClearLibraryArtifacts は、管理者が、データディレクトリの libraries/ の下を丸ごと消せ、ほかの生成物、ユーザーごとの再生位置、
// 動いているジョブは残し、まだ動いていないライブラリのジョブを消し、監査ログに残し、消した後に開けばインデックスを作り直すことを確認する (#1896)。
func TestClearLibraryArtifacts(t *testing.T) {
	t.Parallel()

	f, parts := newLibraryFixture(t)
	admin := f.as(t, "artifacts-admin", domain.RoleAdmin)
	member := f.as(t, "artifacts-member", domain.RoleMember)
	variables := map[string]any{varLibrary: animeLibrary, varPath: episodePath}
	var file libraryFileResult
	require.Eventually(t, func() bool {
		require.Empty(t, member.query(t, libraryFileQuery, variables, &file))

		return file.LibraryFile.Index.Complete
	}, 10*time.Second, 20*time.Millisecond)
	ref := filelib.Ref{Library: animeLibrary, Path: episodePath}
	require.NoError(t, parts.indexer.WriteCaptions(ref, &filelib.CaptionsResult{
		FileID: file.LibraryFile.FileID, Model: "ggml-model.bin", GeneratedAt: time.Now(), Cues: []*filelib.CaptionCue{},
	}))
	require.Empty(t, member.query(t, saveLibraryPosition, map[string]any{"id": file.LibraryFile.ID, varPositionMs: 4500}, nil))
	// libraries/ の外の生成物 (録画のセグメントの生成物と、字幕のモデル) は消さない。
	outside := []string{filepath.Join(artifactsSegmentDirName, "index"), filepath.Join("whisper", "ggml-model.bin")}
	for _, name := range outside {
		require.NoError(t, os.MkdirAll(filepath.Join(parts.dataDir, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(parts.dataDir, name), []byte("keep"), 0o600))
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO river_job (args, kind, max_attempts, queue, state, finalized_at) VALUES
		('{}', 'library_images_generate', 3, 'media', 'available', NULL),
		('{}', 'library_cm_detect', 3, 'cm', 'running', NULL),
		('{}', 'library_auto_captions', 3, 'captions', 'scheduled', NULL),
		('{}', 'library_images_generate', 3, 'media', 'completed', now()),
		('{}', 'thumbnail_generate', 3, 'media', 'available', NULL)`)
	require.NoError(t, err)

	var measured struct {
		LibraryArtifacts libraryArtifactsResult `json:"libraryArtifacts"`
	}
	require.Empty(t, admin.query(t, libraryArtifactsQuery, nil, &measured))
	// インデックス (index)、元のファイルの記録 (source.json)、字幕 (captions.json)。
	assert.Equal(t, 3, measured.LibraryArtifacts.Files)
	assert.Positive(t, measured.LibraryArtifacts.Size)

	var ignored struct{}
	assert.Equal(t, []string{codeForbidden}, member.query(t, libraryArtifactsQuery, nil, &ignored))
	assert.Equal(t, []string{codeForbidden}, member.query(t, clearLibraryArtifacts, nil, &ignored))
	assert.DirExists(t, filepath.Join(parts.dataDir, "libraries"), "管理者でなければ消さない")

	var cleared struct {
		ClearLibraryArtifacts libraryArtifactsResult `json:"clearLibraryArtifacts"`
	}
	require.Empty(t, admin.query(t, clearLibraryArtifacts, nil, &cleared))
	assert.Equal(t, measured.LibraryArtifacts, cleared.ClearLibraryArtifacts)
	assert.NoDirExists(t, filepath.Join(parts.dataDir, "libraries"))
	detached, err := filepath.Glob(filepath.Join(parts.dataDir, ".libraries-*"))
	require.NoError(t, err)
	assert.Empty(t, detached, "移した置き場も消す")
	for _, name := range outside {
		assert.FileExists(t, filepath.Join(parts.dataDir, name))
	}

	// まだ動いていないライブラリのジョブだけを消す。動いているジョブ、終えたジョブ、ほかの種類のジョブは残す。
	rows, err := f.pool.Query(t.Context(), `SELECT kind || ':' || state FROM river_job ORDER BY id`)
	require.NoError(t, err)
	var jobs []string
	for rows.Next() {
		var job string
		require.NoError(t, rows.Scan(&job))
		jobs = append(jobs, job)
	}
	require.NoError(t, rows.Err())
	assert.Subset(t, jobs, []string{"library_cm_detect:running", "library_images_generate:completed", "thumbnail_generate:available"})
	assert.NotContains(t, jobs, "library_images_generate:available")
	assert.NotContains(t, jobs, "library_auto_captions:scheduled")

	var progress int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM library_watch_progress`).Scan(&progress))
	assert.Equal(t, 1, progress, "ユーザーごとの再生位置は消さない")

	var events auditEventsResult
	require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{"first": 50}, &events))
	var summaries []string
	for _, edge := range events.AuditEvents.Edges {
		if edge.Node.Action == actionArtifactsClear {
			summaries = append(summaries, edge.Node.Summary)
		}
	}
	require.Len(t, summaries, 1)
	assert.JSONEq(t, `{"files": 3, "jobs": 2, "size": `+strconv.FormatInt(measured.LibraryArtifacts.Size, 10)+`}`, summaries[0])

	// 消すものがなければ、監査ログに残さない。
	require.Empty(t, admin.query(t, clearLibraryArtifacts, nil, &cleared))
	assert.Equal(t, libraryArtifactsResult{Size: 0, Files: 0}, cleared.ClearLibraryArtifacts)
	require.Empty(t, admin.query(t, auditEventsQuery, map[string]any{"first": 50}, &events))
	assert.Equal(t, 1, countActions(events.actions(), actionArtifactsClear))

	// 消した後にファイルを開けば、インデックスを作り直す。字幕は、頼むまで作らない。
	require.Empty(t, member.query(t, libraryFileQuery, variables, &file))
	assert.Empty(t, file.LibraryFile.AutoCaptions)
	require.Eventually(t, func() bool {
		require.Empty(t, member.query(t, libraryFileQuery, variables, &file))

		return file.LibraryFile.Index.Complete
	}, 10*time.Second, 20*time.Millisecond)
	require.NotNil(t, file.LibraryFile.WatchPositionMs)
	assert.Equal(t, int64(4500), *file.LibraryFile.WatchPositionMs)
}

// TestClearLibraryArtifactsWithoutDataDir は、データディレクトリを設定していない tenban では、量を測れず、消せないことを確認する。
// 空のデータディレクトリで消すと、作業ディレクトリの libraries/ を消してしまうためである。
func TestClearLibraryArtifactsWithoutDataDir(t *testing.T) {
	t.Parallel()

	admin := newFixture(t, &graph.Options{}).as(t, adminName, domain.RoleAdmin)
	var ignored struct{}
	assert.Equal(t, []string{codeConflict}, admin.query(t, libraryArtifactsQuery, nil, &ignored))
	assert.Equal(t, []string{codeConflict}, admin.query(t, clearLibraryArtifacts, nil, &ignored))
}

// countActions は、actions のうち action の数を返す。
func countActions(actions []string, action string) int {
	count := 0
	for _, a := range actions {
		if a == action {
			count++
		}
	}

	return count
}
