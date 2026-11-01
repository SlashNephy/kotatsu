package ts_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// secondAudioPID は、合成した TS の、2 つ目の音声の PID。
const secondAudioPID = 0x0113

// encodePTS は、PES のヘッダーの PTS の 5 バイトの符号化を返す。
// 33 ビットを、上から順に 3 ビットと 15 ビットずつに分け、それぞれの後ろに marker_bit を置く。
func encodePTS(pts uint64) []byte {
	high := binary.BigEndian.AppendUint64(nil, pts>>29)
	mid := binary.BigEndian.AppendUint16(nil, uint16(pts>>15&0x7FFF)<<1|0x01)
	low := binary.BigEndian.AppendUint16(nil, uint16(pts&0x7FFF)<<1|0x01)

	return []byte{0x21 | high[7]&0x0E, mid[0], mid[1], low[0], low[1]}
}

// pesPacket は、PTS が pts で、中身が data の PES を、1 つのパケットに入れて作る。
func pesPacket(pid uint16, cc *byte, streamID byte, pts uint64, data []byte) []byte {
	p := make([]byte, ts.PacketSize)
	for i := range p {
		p[i] = 0xFF
	}
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[1] |= 0x40
	p[3] = 0x10 | *cc&0x0F
	*cc++
	pes := append([]byte{0x00, 0x00, 0x01, streamID, 0x00, 0x00, 0x80, 0x80, 0x05}, encodePTS(pts)...)
	copy(p[4:], append(pes, data...))

	return p
}

// adts は、channel_configuration が channels の ADTS のヘッダーを返す。
func adts(channels byte) []byte {
	return []byte{0xFF, 0xF1, 0x50 | channels>>2&0x01, channels << 6, 0x00, 0x1F, 0xFC}
}

func TestCueScanner(t *testing.T) {
	t.Parallel()

	var patCC, pmtCC, videoCC, audioCC byte
	streams := make([]esEntry, 0, 3)
	streams = append(streams, esEntry{streamType: 0x02, pid: videoPID}, esEntry{streamType: 0x0F, pid: audioPID})
	pkts := psiPackets(0x0000, &patCC, patSection(0, map[uint16]uint16{programNum: pmtPID}))
	pkts = append(pkts, psiPackets(pmtPID, &pmtCC, pmtSection(0, streams...))...)
	pkts = append(pkts, pesPacket(videoPID, &videoCC, 0xE0, 1000, nil))
	audio := func(pts uint64, channels byte, n uint64) {
		for i := range n {
			pkts = append(pkts, pesPacket(audioPID, &audioCC, 0xC0, pts+i, adts(channels)))
		}
	}
	// 録画の始まりで初めて知った構成は、変わったとみなさない。
	audio(4000, 2, 12)
	// ステレオから 5.1ch に変わったら、新しい値が続き始めた PES の PTS を記録する。
	audio(10000, 6, 12)
	// 1 つの PES だけ違う値を読んでも (ADTS の始まりとそろっていないときなど)、変わったとみなさない。
	audio(15000, 2, 1)
	pkts = append(pkts, pesPacket(audioPID, &audioCC, 0xC0, 15500, []byte{0x00, 0x00, 0x00, 0x00}))
	audio(16000, 6, 12)
	// PMT で音声のストリームが増えたら、次に続いた音声の PTS を記録する。
	pkts = append(pkts, psiPackets(pmtPID, &pmtCC, pmtSection(1, append(streams, esEntry{streamType: 0x0F, pid: secondAudioPID})...))...)
	audio(30000, 6, 12)
	pkts = append(pkts, pesPacket(videoPID, &videoCC, 0xE0, 40000, nil))

	scanner := ts.NewCueScanner(programNum)
	// パケットの区切りとそろわない大きさで書いても、同じに読める。
	data := join(pkts)
	for len(data) > 0 {
		n := min(len(data), 100)
		_, err := scanner.Write(data[:n])
		require.NoError(t, err)
		data = data[n:]
	}

	cues := scanner.Cues()
	require.NotNil(t, cues.FirstVideoPTS)
	require.NotNil(t, cues.LastVideoPTS)
	assert.Equal(t, int64(1000), *cues.FirstVideoPTS)
	assert.Equal(t, int64(40000), *cues.LastVideoPTS)
	assert.Equal(t, []int64{10000, 30000}, cues.AudioChanges)
}

func TestCueScannerServiceFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		serviceID uint16
	}{
		{name: "PAT にあるサービスは、そのまま読む", serviceID: programNum},
		// FFmpeg が読む最初の映像 (0:v:0) と同じ番組を読む。
		{name: "サービス ID が 0 なら、映像のある最初の番組を読む", serviceID: 0},
		{name: "PAT にないサービスなら、映像のある最初の番組を読む", serviceID: programNum + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var patCC, pmtCC, videoCC byte
			pkts := psiPackets(0x0000, &patCC, patSection(0, map[uint16]uint16{programNum: pmtPID}))
			pkts = append(pkts, psiPackets(pmtPID, &pmtCC, pmtSection(0, esEntry{streamType: 0x02, pid: videoPID}))...)
			pkts = append(pkts, pesPacket(videoPID, &videoCC, 0xE0, 1000, nil))

			scanner := ts.NewCueScanner(tt.serviceID)
			_, err := scanner.Write(join(pkts))
			require.NoError(t, err)
			require.NotNil(t, scanner.Cues().FirstVideoPTS)
			assert.Equal(t, int64(1000), *scanner.Cues().FirstVideoPTS)
		})
	}
}
