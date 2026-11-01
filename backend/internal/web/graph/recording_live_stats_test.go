package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordingLiveStats は、録画中の録画だけが、書いているセグメントの様子を返すことを確かめる (#829)。
func TestRecordingLiveStats(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	recording := f.addRecording(t, dir, remainingRecordingName, "recording", time.Hour)
	completed := f.addRecording(t, dir, remainingRecordingName+"-2", "completed", time.Hour)
	// どちらの録画にも、書いているセグメントの様子の行を足す。録画を終えた後に残った行は、返さない。
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_live_stats (recording_id, bytes, drops, errors, scrambled, bitrate, updated_at)
		SELECT id, 1000, 2, 1, 0, 16000000, now() FROM recordings WHERE name IN ($1, $2)`, remainingRecordingName, remainingRecordingName+"-2")
	require.NoError(t, err)

	type stats struct {
		Bitrate int64 `json:"bitrate"`
		Bytes   int64 `json:"bytes"`
		Drops   int64 `json:"drops"`
	}
	read := func(recordingID string) *stats {
		t.Helper()
		var node struct {
			Node struct {
				LiveStats *stats `json:"liveStats"`
			} `json:"node"`
		}
		require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Recording { liveStats { bitrate bytes drops } } } }`,
			map[string]any{varID: recordingID}, &node))

		return node.Node.LiveStats
	}

	assert.Equal(t, &stats{Bitrate: 16000000, Bytes: 1000, Drops: 2}, read(recording))
	// 録画を終えた録画は、行が残っていても返さない。
	assert.Nil(t, read(completed))
}
