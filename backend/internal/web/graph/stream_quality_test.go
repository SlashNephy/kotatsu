package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

// TestStreamQuality は、録画のセグメントの PID ごとの集計を、PID ごとに足し合わせて返し、
// 集計のないセグメントは飛ばし、種類の分かったセグメントの種類を使うことを確かめる (#473)。
func TestStreamQuality(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	recordingID := f.addRecording(t, dir, remainingRecordingName, "completed", time.Hour)
	_, err := f.pool.Exec(t.Context(), `UPDATE recording_files SET pid_stats = '[
		{"pid": 256, "kind": "unknown", "packets": 100, "drops": 1, "errors": 0, "scrambled": 0},
		{"pid": 18, "kind": "psi", "packets": 10, "drops": 0, "errors": 0, "scrambled": 0}]'`)
	require.NoError(t, err)
	for _, stats := range []*string{
		new(`[{"pid": 256, "kind": "video", "packets": 50, "drops": 2, "errors": 1, "scrambled": 3, "streamType": 2, "componentTag": 0}]`),
		nil,
	} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, pid_stats, created_at)
			SELECT $1, recording_id, 'segment', pool_id, $2, 2, started_at, ended_at, $3, created_at FROM recording_files LIMIT 1`,
			id.New(), id.New().String()+".m2ts", stats)
		require.NoError(t, err)
	}

	type quality struct {
		PID       int    `json:"pid"`
		Kind      string `json:"kind"`
		Packets   int64  `json:"packets"`
		Drops     int64  `json:"drops"`
		Errors    int64  `json:"errors"`
		Scrambled int64  `json:"scrambled"`
		// StreamType と ComponentTag は、記録していないセグメントがあっても、記録したものから埋める。
		StreamType   *int `json:"streamType"`
		ComponentTag *int `json:"componentTag"`
	}
	var node struct {
		Node struct {
			StreamQuality []quality `json:"streamQuality"`
		} `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Recording { streamQuality { pid kind packets drops errors scrambled streamType componentTag } } } }`,
		map[string]any{varID: recordingID}, &node))
	assert.Equal(t, []quality{
		{PID: 18, Kind: "PSI", Packets: 10, Drops: 0, Errors: 0, Scrambled: 0, StreamType: nil, ComponentTag: nil},
		{PID: 256, Kind: "VIDEO", Packets: 150, Drops: 3, Errors: 1, Scrambled: 3, StreamType: new(2), ComponentTag: new(0)},
	}, node.Node.StreamQuality)
}
