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

const renameRecordingsMutation = `mutation($ids: [ID!]!) { renameRecordingFiles(ids: $ids) { renamed skipped failed } }`

// orphanFileInput は、deleteOrphanFiles に渡す、DB にないファイル。
type orphanFileInput struct {
	PoolID     *string `json:"poolId"`
	Path       string  `json:"path"`
	SegmentDir bool    `json:"segmentDir"`
}

// TestFileOperationsDoNotRequireRecentAuthentication は、録画やファイルを消したり動かしたりする管理者の操作が、
// 第三者に影響しないので、直近の認証から時間が経っても再認証を求めないことを確かめる (設計書 9.5)。
func TestFileOperationsDoNotRequireRecentAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutation  string
		variables func(recordingID string) map[string]any
	}{
		{
			name:      "見つからない録画の片付け",
			mutation:  cleanUpMissingMutation,
			variables: func(recordingID string) map[string]any { return map[string]any{varIDs: []string{recordingID}} },
		},
		{
			name:     "DBにないファイルの削除",
			mutation: deleteOrphanMutation,
			variables: func(string) map[string]any {
				return map[string]any{varFiles: []*orphanFileInput{{PoolID: new(poolMain), Path: orphanFileName, SegmentDir: false}}}
			},
		},
		{
			name:      "ファイル名の付け直し",
			mutation:  renameRecordingsMutation,
			variables: func(recordingID string) map[string]any { return map[string]any{varIDs: []string{recordingID}} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, dir := newRecordingFixture(t)
			// DB にないファイルを消すと、River のジョブを入れるので、River のテーブルを作っておく。
			require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
			admin := f.as(t, adminName, auth.RoleAdmin)
			missing := f.addRecording(t, dir, missingRecordingName, "completed", 2*time.Hour)
			require.NoError(t, os.Remove(filepath.Join(dir, missingRecordingName+".m2ts")))
			require.NoError(t, os.WriteFile(filepath.Join(dir, orphanFileName), []byte("orphan"), 0o600))
			f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)

			assert.Empty(t, admin.query(t, tt.mutation, tt.variables(missing), nil))
		})
	}
}
