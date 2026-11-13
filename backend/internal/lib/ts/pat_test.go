package ts_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// patOf は、transport_stream_id が tsid で、programs を持つ PAT のパケットを作る。0 は NIT を指す。
func patOf(cc *byte, tsid uint16, programs ...uint16) []byte {
	body := []byte{}
	for _, number := range programs {
		body = binary.BigEndian.AppendUint16(body, number)
		body = binary.BigEndian.AppendUint16(body, 0xE000|0x100)
	}

	return bytes.Join(psiPackets(0, cc, section(0x00, tsid, 0, body)), nil)
}

func TestPATWatcher(t *testing.T) {
	t.Parallel()

	var cc byte
	first := patOf(&cc, 0x7FE0, 0, 1024)
	second := patOf(&cc, 0x4010, 0, 101)
	stream := append(append(append([]byte{}, first...), payloadPacket(0x100)...), second...)
	tests := []struct {
		name   string
		chunks func() [][]byte
	}{
		{name: "whole", chunks: func() [][]byte { return [][]byte{stream} }},
		// パケットの途中で区切って渡しても読む。
		{name: "split", chunks: func() [][]byte {
			chunks := [][]byte{}
			for i := 0; i < len(stream); i += 100 {
				chunks = append(chunks, stream[i:min(i+100, len(stream))])
			}

			return chunks
		}},
		// 先頭に半端なバイトがあっても、同期バイトで合わせ直す。
		{name: "garbage first", chunks: func() [][]byte { return [][]byte{append([]byte{0x00, 0x01, 0x02}, stream...)} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := []*ts.PAT{}
			w := ts.NewPATWatcher(func(pat *ts.PAT) { got = append(got, pat) })
			for _, chunk := range tt.chunks() {
				n, err := w.Write(chunk)
				require.NoError(t, err)
				assert.Equal(t, len(chunk), n)
			}
			assert.Equal(t, []*ts.PAT{
				{TransportStreamID: 0x7FE0, Programs: []uint16{1024}},
				{TransportStreamID: 0x4010, Programs: []uint16{101}},
			}, got)
		})
	}
}
