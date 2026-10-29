package stream

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	audioPID = 0x110
	videoPID = 0x100
)

// pesPackets は、PID の PES を、長さ length (0 なら長さを持たない映像の PES) で、パケットに分けて返す。
// 中身は、count 個のパケットに収まるだけ詰める。
func pesPackets(pid, length uint16, count int) [][]byte {
	packets := make([][]byte, 0, count)
	for i := range count {
		packet := bytes.Repeat([]byte{0xFF}, ts.PacketSize)
		packet[0] = 0x47
		binary.BigEndian.PutUint16(packet[1:3], pid)
		packet[3] = 0x10
		if i == 0 {
			packet[1] |= 0x40
			copy(packet[4:], []byte{0, 0, 1, 0xC0})
			binary.BigEndian.PutUint16(packet[8:10], length)
		}
		packets = append(packets, packet)
	}

	return packets
}

// TestCompleteTail は、続きのないセグメントの終わりで、途中で切れた長さを持つ PES を流さないことを確かめる (#1131)。
func TestCompleteTail(t *testing.T) {
	t.Parallel()

	// 1 パケットの PES のペイロードは 184 バイトで、PES の長さは、その中の 6 バイトの後ろから数える。
	// 4 パケットに収まる音声の PES の長さは 184*4 - 6。
	const whole = 184*4 - 6
	tests := []struct {
		name    string
		packets [][]byte
		// want は、流す終わりのパケットの数。
		want int
	}{
		{
			name:    "終わりまでに全部が届いた PES は切らない",
			packets: append(pesPackets(videoPID, 0, 2), pesPackets(audioPID, whole, 4)...),
			want:    6,
		},
		{
			name:    "途中で切れた音声の PES は、その始まりで切る",
			packets: append(append(pesPackets(audioPID, whole, 4), pesPackets(videoPID, 0, 1)...), pesPackets(audioPID, whole, 2)...),
			want:    5,
		},
		{
			name:    "長さを持たない映像の PES は、切れているか分からないので切らない",
			packets: append(pesPackets(audioPID, whole, 4), pesPackets(videoPID, 0, 3)...),
			want:    7,
		},
		{
			name: "途中で切れた PES の後のほかの PID のパケットも、その始まりで切る",
			packets: append(append(pesPackets(audioPID, whole, 4), pesPackets(audioPID, whole, 1)...),
				pesPackets(videoPID, 0, 2)...),
			want: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := bytes.Join(tt.packets, nil)
			got, err := completeTail(bytes.NewReader(data), 0, int64(len(data)))
			require.NoError(t, err)
			assert.Equal(t, int64(tt.want*ts.PacketSize), got)
		})
	}
}

// continuation は、PID の、PES の始まりでない (前から続く) パケットを count 個返す。
func continuation(pid uint16, count int) [][]byte {
	packets := pesPackets(pid, 0, count+1)

	return packets[1:]
}

// TestCompleteHead は、続きのないセグメントの頭で、PES の始まりより前にある、前から続く PES の残りのパケットを落とすことを確かめる (#1131)。
func TestCompleteHead(t *testing.T) {
	t.Parallel()

	video := pesPackets(videoPID, 0, 3)
	audio := pesPackets(audioPID, 184*2-6, 2)
	tests := []struct {
		name    string
		packets [][]byte
		want    [][]byte
	}{
		{
			name:    "I フレームの後に、前から続く音声の PES の残りがあれば落とす",
			packets: append(append(append([][]byte{}, video[0]), continuation(audioPID, 2)...), append(video[1:], audio...)...),
			want:    append(append([][]byte{}, video...), audio...),
		},
		{
			name:    "PES の始まりから始まっていれば、そのまま流す",
			packets: append(append([][]byte{}, video...), audio...),
			want:    append(append([][]byte{}, video...), audio...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := bytes.Join(tt.packets, nil)
			got, consumed, err := completeHead(bytes.NewReader(data), 0, int64(len(data)))
			require.NoError(t, err)
			assert.Equal(t, bytes.Join(tt.want, nil), got)
			assert.Equal(t, int64(len(data)), consumed)
		})
	}
}
