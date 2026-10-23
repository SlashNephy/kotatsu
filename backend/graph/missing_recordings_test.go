package graph_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	missingRecordingsQuery = `{ missingRecordings { recording { id name } segments missingSegments } }`
	cleanUpMissingMutation = `mutation($ids: [ID!]!) { cleanUpMissingRecordings(ids: $ids) { deletedRecordings prunedSegments skipped } }`
	missingRecordingName   = "消えた録画"
	remainingRecordingName = "残っている録画"
)

// TestMissingRecordings は、管理者が、ファイルの見つからない録画を探して DB から消せ、ほかのロールは使えないことを確かめる。
func TestMissingRecordings(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	gone := f.addRecording(t, dir, missingRecordingName, "completed", 2*time.Hour)
	kept := f.addRecording(t, dir, remainingRecordingName, "completed", time.Hour)
	require.NoError(t, os.Remove(filepath.Join(dir, missingRecordingName+".m2ts")))

	type missing struct {
		Recording struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"recording"`
		Segments        int `json:"segments"`
		MissingSegments int `json:"missingSegments"`
	}
	var found struct {
		MissingRecordings []missing `json:"missingRecordings"`
	}
	require.Empty(t, admin.query(t, missingRecordingsQuery, nil, &found))
	require.Len(t, found.MissingRecordings, 1)
	assert.Equal(t, gone, found.MissingRecordings[0].Recording.ID)
	assert.Equal(t, 1, found.MissingRecordings[0].MissingSegments)

	var cleaned struct {
		CleanUpMissingRecordings struct {
			DeletedRecordings int `json:"deletedRecordings"`
			PrunedSegments    int `json:"prunedSegments"`
			Skipped           int `json:"skipped"`
		} `json:"cleanUpMissingRecordings"`
	}
	// 見つかる録画を渡しても、確かめ直して消さない。
	require.Empty(t, admin.query(t, cleanUpMissingMutation, map[string]any{varIDs: []string{gone, kept}}, &cleaned))
	assert.Equal(t, 1, cleaned.CleanUpMissingRecordings.DeletedRecordings)
	assert.Zero(t, cleaned.CleanUpMissingRecordings.PrunedSegments)
	require.Empty(t, admin.query(t, missingRecordingsQuery, nil, &found))
	assert.Empty(t, found.MissingRecordings)
	assert.Equal(t, []string{remainingRecordingName}, f.recordings(t, 10, nil, nil).names())

	var ignored struct{}
	assert.Equal(t, []string{codeForbidden}, f.query(t, missingRecordingsQuery, nil, &ignored))
	assert.Equal(t, []string{codeForbidden}, f.query(t, cleanUpMissingMutation, map[string]any{varIDs: []string{kept}}, &ignored))
}
