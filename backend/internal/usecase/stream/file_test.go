package stream_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// TestOpenFile は、ライブラリのファイルを、索引のできた所までの I フレームから流し、索引によらず末尾まで流すことを確かめる (#1734)。
func TestOpenFile(t *testing.T) {
	t.Parallel()

	// 映像の PID (0x100) のパケットを 20 個並べ、k 番目のパケットの 1 バイト目を k にして見分ける。
	const packets = 20
	data := make([]byte, 0, packets*ts.PacketSize)
	for k := range packets {
		packet := make([]byte, ts.PacketSize)
		packet[0], packet[1], packet[2], packet[3] = 0x47, 0x01, 0x00, 0x10|byte(k%16)
		packet[4] = byte(k)
		data = append(data, packet...)
	}
	name := filepath.Join(t.TempDir(), "show.ts")
	require.NoError(t, os.WriteFile(name, data, 0o600))
	open := func(context.Context) (stream.File, error) { return os.Open(name) }
	// 索引は、0.5005 秒ごとの I フレームが、0、5、10 番目のパケットにあるところまでできている。
	index := []ts.IndexEntry{
		{Offset: 0, PTS: 0},
		{Offset: 5 * ts.PacketSize, PTS: 45045},
		{Offset: 10 * ts.PacketSize, PTS: 90090},
	}

	tests := []struct {
		name      string
		indexing  bool
		start     time.Duration
		wantAt    time.Duration
		wantFirst byte
	}{
		{name: "from the beginning", indexing: true, start: 0, wantAt: 0, wantFirst: 0},
		{name: "the keyframe before the position", indexing: true, start: 700 * time.Millisecond, wantAt: 500500 * time.Microsecond, wantFirst: 5},
		// 索引のできた所より先を求めたら、最後の I フレームから流す。
		{name: "beyond the indexed part", indexing: true, start: time.Hour, wantAt: 1001 * time.Millisecond, wantFirst: 10},
		{name: "indexed", indexing: false, start: 700 * time.Millisecond, wantAt: 500500 * time.Microsecond, wantFirst: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recording := stream.OpenFile("show.ts", open, index, tt.indexing)
			assert.Equal(t, tt.indexing, recording.InProgress)
			assert.Equal(t, "show.ts", recording.FileName())
			pos := recording.Seek(tt.start)
			assert.Equal(t, tt.wantAt, pos.At)
			var out bytes.Buffer
			require.NoError(t, recording.Write(t.Context(), &out, pos))
			// 流し始めの I フレームのパケットから、ファイルの末尾までを流す。
			assert.Equal(t, data[int(tt.wantFirst)*ts.PacketSize:], out.Bytes())
		})
	}
}
