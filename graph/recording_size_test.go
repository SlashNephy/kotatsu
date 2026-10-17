package graph_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWritingFileSize は、書いている間のファイルは、DB の大きさでなくファイルの今の大きさを返し、
// 書き終えたファイルは DB の大きさを返すことを確かめる (#736)。
func TestWritingFileSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ended bool
		want  int64
	}{
		{name: "書いている間はファイルの大きさ", ended: false, want: 6},
		{name: "書き終えたら DB の大きさ", ended: true, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, dir := newRecordingFixture(t)
			f.addRecording(t, dir, remainingRecordingName, "recording", time.Hour)
			require.NoError(t, os.WriteFile(filepath.Join(dir, remainingRecordingName+".m2ts"), []byte("tstsTS"), 0o600))
			if !tt.ended {
				_, err := f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = NULL")
				require.NoError(t, err)
			}

			page := f.recordings(t, 10, nil, nil)
			require.Len(t, page.Recordings.Edges, 1)
			require.Len(t, page.Recordings.Edges[0].Node.Files, 1)
			assert.Equal(t, tt.want, page.Recordings.Edges[0].Node.Files[0].Size)
		})
	}
}
