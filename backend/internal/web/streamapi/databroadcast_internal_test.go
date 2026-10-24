package streamapi

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// siPacket は、PID が pid で、ペイロードを fill で埋めたパケットを作る。
func siPacket(pid uint16, fill byte) []byte {
	p := bytes.Repeat([]byte{fill}, ts.PacketSize)
	p[0] = syncByte
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[3] = 0x10

	return p
}

// TestDataBroadcastWriterSplitsPackets は、パケットの途中で切れた書き込みや、境目のずれがあっても、
// パケットを正しく切り分けて、残すパケットだけを書くことを確かめる。
func TestDataBroadcastWriterSplitsPackets(t *testing.T) {
	t.Parallel()

	// NIT (0x0010) と EIT (0x0012) は残り、PMT を読む前の 0x0111 は捨てられる。
	nit, eit, video := siPacket(0x0010, 0xA1), siPacket(0x0012, 0xA2), siPacket(0x0111, 0xA3)
	stream := bytes.Join([][]byte{nit, video, eit, video, nit}, nil)
	want := bytes.Join([][]byte{nit, eit, nit}, nil)

	tests := []struct {
		name  string
		input []byte
		chunk int
	}{
		{name: "whole packets", input: stream, chunk: ts.PacketSize},
		{name: "one byte at a time", input: stream, chunk: 1},
		{name: "chunks across packets", input: stream, chunk: 100},
		{name: "all at once", input: stream, chunk: len(stream)},
		{name: "garbage before the first packet", input: append([]byte{0x00, 0x01, 0x02}, stream...), chunk: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			w := newDataBroadcastWriter(recorder, 1024)
			for data := tt.input; len(data) > 0; {
				n := min(tt.chunk, len(data))
				written, err := w.Write(data[:n])
				require.NoError(t, err)
				require.Equal(t, n, written)
				data = data[n:]
			}
			assert.Equal(t, want, recorder.Body.Bytes())
		})
	}
}

func TestProgramNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		serviceID int64
		want      uint16
		wantOK    bool
	}{
		{name: "terrestrial", serviceID: 3273601024, want: 1024, wantOK: true},
		{name: "BS", serviceID: 400101, want: 101, wantOK: true},
		{name: "too large for program_number", serviceID: 3273699999, want: 0, wantOK: false},
		{name: "negative", serviceID: -1, want: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := programNumber(tt.serviceID)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
