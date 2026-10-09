package ts_test

import (
	"encoding/binary"
	"hash/crc32"
	"math/bits"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	pmtPID     = 0x01F0
	programNum = 0x0400
)

// crcMPEG は、CRC-32/MPEG-2 を hash/crc32 から求める。
// hash/crc32 はビットを反転する方式なので、入力のビットを反転して計算し、結果を反転し直す。
func crcMPEG(b []byte) uint32 {
	reversed := make([]byte, len(b))
	for i, v := range b {
		reversed[i] = bits.Reverse8(v)
	}

	return bits.Reverse32(^crc32.ChecksumIEEE(reversed))
}

func TestCRCMPEGOracle(t *testing.T) {
	t.Parallel()

	// CRC-32/MPEG-2 の検査値。
	assert.Equal(t, uint32(0x0376E6E7), crcMPEG([]byte("123456789")))
}

// section は、section_syntax_indicator が 1 のセクションを作る。
func section(tableID byte, extension uint16, version byte, body []byte) []byte {
	length := 5 + len(body) + 4
	s := []byte{tableID}
	s = binary.BigEndian.AppendUint16(s, 0xB000|uint16(length&0x0FFF))
	s = binary.BigEndian.AppendUint16(s, extension)
	s = append(s, 0xC1|version<<1, 0x00, 0x00)
	s = append(s, body...)

	return binary.BigEndian.AppendUint32(s, crcMPEG(s))
}

func patSection(version byte, programs map[uint16]uint16) []byte {
	body := make([]byte, 0, 4*len(programs))
	for number, pid := range programs {
		body = binary.BigEndian.AppendUint16(body, number)
		body = binary.BigEndian.AppendUint16(body, 0xE000|pid)
	}

	return section(0x00, 0x7FE0, version, body)
}

type esEntry struct {
	streamType byte
	pid        uint16
	infoLength int
}

// pmtSection は、program_number が programNum の PMT を作る。
func pmtSection(version byte, streams ...esEntry) []byte {
	body := []byte{0xE1, 0x11, 0xF0, 0x00}
	for _, s := range streams {
		body = append(body, s.streamType)
		body = binary.BigEndian.AppendUint16(body, 0xE000|s.pid)
		body = binary.BigEndian.AppendUint16(body, 0xF000|uint16(s.infoLength&0x0FFF))
		// 中身を読まない記述子の代わりに、0xFF で埋める。
		for range s.infoLength {
			body = append(body, 0xFF)
		}
	}

	return section(0x02, programNum, version, body)
}

// psiPackets は、セクションを順に詰めたパケットを作る。各セクションは新しいパケットから始める。
func psiPackets(pid uint16, cc *byte, sections ...[]byte) [][]byte {
	pkts := [][]byte{}
	for _, s := range sections {
		data := append([]byte{0x00}, s...)
		first := true
		for len(data) > 0 {
			p := make([]byte, ts.PacketSize)
			for i := range p {
				p[i] = 0xFF
			}
			p[0] = 0x47
			binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
			if first {
				p[1] |= 0x40
			}
			p[3] = 0x10 | *cc&0x0F
			*cc++
			n := copy(p[4:], data)
			data = data[n:]
			first = false
			pkts = append(pkts, p)
		}
	}

	return pkts
}

func TestAnalyzerProgramsFromTestTS(t *testing.T) {
	t.Parallel()

	analyzer := ts.NewAnalyzer()
	_, err := analyzer.Write(loadTestTS(t))
	require.NoError(t, err)

	assert.Equal(t, []*ts.Program{{
		Number:  programNum,
		PMTPID:  pmtPID,
		PCRPID:  videoPID,
		Version: 0,
		Streams: []*ts.Stream{
			{PID: videoPID, StreamType: 0x02, Kind: ts.StreamVideo},
			{PID: audioPID, StreamType: 0x0F, Kind: ts.StreamAudio},
		},
	}}, analyzer.Stats().Programs)
}

func TestAnalyzerPrograms(t *testing.T) {
	t.Parallel()

	video := esEntry{streamType: 0x02, pid: 0x0111}
	audio := esEntry{streamType: 0x0F, pid: 0x0112}
	caption := esEntry{streamType: 0x06, pid: 0x0130}
	pat := map[uint16]uint16{programNum: pmtPID}

	tests := []struct {
		name string
		// build は、入力のパケットを作る。cc は PID ごとの continuity_counter。
		build       func(cc map[uint16]*byte) [][]byte
		wantStreams []ts.StreamKind
		wantVersion uint8
	}{
		{
			name: "PAT and PMT",
			build: func(cc map[uint16]*byte) [][]byte {
				return append(psiPackets(0, cc[0], patSection(0, pat)), psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, audio))...)
			},
			wantStreams: []ts.StreamKind{ts.StreamVideo, ts.StreamAudio},
		},
		{
			name: "PMT version update",
			build: func(cc map[uint16]*byte) [][]byte {
				return append(psiPackets(0, cc[0], patSection(0, pat)),
					psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, audio), pmtSection(1, video, audio, caption))...)
			},
			wantStreams: []ts.StreamKind{ts.StreamVideo, ts.StreamAudio, ts.StreamOther},
			wantVersion: 1,
		},
		{
			name: "PMT spanning several packets",
			build: func(cc map[uint16]*byte) [][]byte {
				long := esEntry{streamType: 0x0F, pid: 0x0113, infoLength: 400}
				// PMT は 3 パケットにまたがる。
				return append(psiPackets(0, cc[0], patSection(0, pat)), psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, long, audio))...)
			},
			wantStreams: []ts.StreamKind{ts.StreamVideo, ts.StreamAudio, ts.StreamAudio},
		},
		{
			name: "PMT with a broken CRC is ignored",
			build: func(cc map[uint16]*byte) [][]byte {
				pmt := pmtSection(0, video, audio)
				pmt[len(pmt)-1] ^= 0xFF

				return append(psiPackets(0, cc[0], patSection(0, pat)), psiPackets(pmtPID, cc[pmtPID], pmt)...)
			},
			wantStreams: nil,
		},
		{
			name: "PMT on a PID not listed in PAT is ignored",
			build: func(cc map[uint16]*byte) [][]byte {
				return append(psiPackets(0, cc[0], patSection(0, pat)), psiPackets(0x01F1, cc[0x01F1], pmtSection(0, video, audio))...)
			},
			wantStreams: nil,
		},
		{
			name: "PMT before PAT is ignored",
			build: func(cc map[uint16]*byte) [][]byte {
				return append(psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, audio)), psiPackets(0, cc[0], patSection(0, pat))...)
			},
			wantStreams: nil,
		},
		{
			name: "a program removed from a new PAT is forgotten",
			build: func(cc map[uint16]*byte) [][]byte {
				pkts := append(psiPackets(0, cc[0], patSection(0, pat)), psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, audio))...)

				return append(pkts, psiPackets(0, cc[0], patSection(1, map[uint16]uint16{0x0408: 0x01F8}))...)
			},
			wantStreams: nil,
		},
		{
			name: "a duplicated packet in the middle of a PMT is read once",
			build: func(cc map[uint16]*byte) [][]byte {
				long := esEntry{streamType: 0x0F, pid: 0x0113, infoLength: 400}
				pmt := psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, long, audio))

				return append(psiPackets(0, cc[0], patSection(0, pat)), pmt[0], pmt[1], pmt[1], pmt[2])
			},
			wantStreams: []ts.StreamKind{ts.StreamVideo, ts.StreamAudio, ts.StreamAudio},
		},
		{
			name: "PMT missing a middle packet is ignored",
			build: func(cc map[uint16]*byte) [][]byte {
				long := esEntry{streamType: 0x0F, pid: 0x0113, infoLength: 400}
				pmt := psiPackets(pmtPID, cc[pmtPID], pmtSection(0, video, long, audio))

				return append(psiPackets(0, cc[0], patSection(0, pat)), pmt[0], pmt[2])
			},
			wantStreams: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := map[uint16]*byte{0: new(byte), pmtPID: new(byte), 0x01F1: new(byte)}
			analyzer := ts.NewAnalyzer()
			_, err := analyzer.Write(join(tt.build(cc)))
			require.NoError(t, err)

			programs := analyzer.Stats().Programs
			if tt.wantStreams == nil {
				assert.Empty(t, programs)

				return
			}
			require.Len(t, programs, 1)
			assert.Equal(t, uint16(programNum), programs[0].Number)
			assert.Equal(t, uint16(pmtPID), programs[0].PMTPID)
			assert.Equal(t, tt.wantVersion, programs[0].Version)
			kinds := []ts.StreamKind{}
			for _, s := range programs[0].Streams {
				kinds = append(kinds, s.Kind)
			}
			assert.Equal(t, tt.wantStreams, kinds)
		})
	}
}
