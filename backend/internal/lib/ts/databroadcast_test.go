package ts_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	carouselPID = 0x0140
	otherPMTPID = 0x01F1
	nitPID      = 0x0010
	eitPID      = 0x0012
)

// payloadPacket は、ペイロードを 0xAA で埋めたパケットを作る。
func payloadPacket(pid uint16) []byte {
	p := bytes.Repeat([]byte{0xAA}, ts.PacketSize)
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[3] = 0x10

	return p
}

// pcrPacket は、PCR を持つアダプテーションフィールドと、fill で埋めたペイロードを持つパケットを作る。
// withPayload が false なら、アダプテーションフィールドだけのパケットにする。
func pcrPacket(pid uint16, cc byte, withPayload bool, fill byte) []byte {
	p := bytes.Repeat([]byte{fill}, ts.PacketSize)
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[1] |= 0x40
	p[3] = 0x20 | cc&0x0F
	length := byte(7)
	if withPayload {
		p[3] |= 0x10
	} else {
		length = ts.PacketSize - 5
		for i := 12; i < ts.PacketSize; i++ {
			p[i] = 0xFF
		}
	}
	p[4] = length
	// PCR_flag だけを立て、PCR の base と extension を適当な値にする。
	copy(p[5:12], []byte{0x10, 0x12, 0x34, 0x56, 0x78, 0xFE, 0x9A})

	return p
}

// pcrOnlyPacket は、pcrPacket(pid, cc, true, _) からペイロードを除いた、期待するパケットを作る。
func pcrOnlyPacket(pid uint16, cc byte) []byte {
	p := bytes.Repeat([]byte{0xFF}, ts.PacketSize)
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[3] = 0x20 | cc&0x0F
	p[4] = ts.PacketSize - 5
	copy(p[5:12], []byte{0x10, 0x12, 0x34, 0x56, 0x78, 0xFE, 0x9A})

	return p
}

// psiHeader は、PAT と、program_number が programNum の PMT のパケットを作る。
// PMT の PCR の PID は videoPID になる。
func psiHeader(patVersion byte, pmtPIDOfProgram uint16) [][]byte {
	var patCC, pmtCC byte
	pkts := psiPackets(0x0000, &patCC, patSection(patVersion, map[uint16]uint16{programNum: pmtPIDOfProgram, programNum + 1: otherPMTPID}))
	pkts = append(pkts, psiPackets(pmtPIDOfProgram, &pmtCC, pmtSection(0,
		esEntry{streamType: 0x02, pid: videoPID, infoLength: 0},
		esEntry{streamType: 0x0F, pid: audioPID, infoLength: 0},
		esEntry{streamType: 0x0D, pid: carouselPID, infoLength: 3},
	))...)

	return pkts
}

func TestDataBroadcastFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// before は、確かめるパケットの前に流すパケット。
		before [][]byte
		packet []byte
		// want は残るパケット。nil なら捨てる。
		want []byte
	}{
		{
			name:   "SI is kept before the PMT is read",
			before: [][]byte{},
			packet: payloadPacket(eitPID),
			want:   payloadPacket(eitPID),
		},
		{
			name:   "data carousel is dropped before the PMT is read",
			before: [][]byte{},
			packet: payloadPacket(carouselPID),
			want:   nil,
		},
		{
			name:   "NIT is kept",
			before: psiHeader(0, pmtPID),
			packet: payloadPacket(nitPID),
			want:   payloadPacket(nitPID),
		},
		{
			name:   "data carousel is kept after the PMT is read",
			before: psiHeader(0, pmtPID),
			packet: payloadPacket(carouselPID),
			want:   payloadPacket(carouselPID),
		},
		{
			name:   "audio is dropped",
			before: psiHeader(0, pmtPID),
			packet: payloadPacket(audioPID),
			want:   nil,
		},
		{
			name:   "video without a PCR is dropped",
			before: psiHeader(0, pmtPID),
			packet: payloadPacket(videoPID),
			want:   nil,
		},
		{
			name:   "video with a PCR keeps only the adaptation field",
			before: psiHeader(0, pmtPID),
			packet: pcrPacket(videoPID, 5, true, 0xAA),
			want:   pcrOnlyPacket(videoPID, 5),
		},
		{
			name:   "PCR packet without a payload is kept as is",
			before: psiHeader(0, pmtPID),
			packet: pcrPacket(videoPID, 5, false, 0xAA),
			want:   pcrPacket(videoPID, 5, false, 0xAA),
		},
		{
			name:   "PMT of another service is dropped",
			before: psiHeader(0, pmtPID),
			packet: payloadPacket(otherPMTPID),
			want:   nil,
		},
		{
			name:   "old PMT PID is dropped after the PAT changes",
			before: append(psiHeader(0, pmtPID), psiHeader(1, pmtPID+2)...),
			packet: payloadPacket(pmtPID),
			want:   nil,
		},
		{
			name:   "data carousel is kept after the PAT changes",
			before: append(psiHeader(0, pmtPID), psiHeader(1, pmtPID+2)...),
			packet: payloadPacket(carouselPID),
			want:   payloadPacket(carouselPID),
		},
		{
			name:   "packet without the sync byte is dropped",
			before: psiHeader(0, pmtPID),
			packet: append([]byte{0x00}, payloadPacket(carouselPID)[1:]...),
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter := ts.NewDataBroadcastFilter(programNum)
			for _, p := range tt.before {
				_, _ = filter.Filter(p)
			}
			got, keep := filter.Filter(tt.packet)
			if tt.want == nil {
				assert.False(t, keep)

				return
			}
			require.True(t, keep)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDataBroadcastFilterKeepsPSI(t *testing.T) {
	t.Parallel()

	filter := ts.NewDataBroadcastFilter(programNum)
	for _, p := range psiHeader(0, pmtPID) {
		got, keep := filter.Filter(p)
		require.True(t, keep)
		assert.Equal(t, p, got)
	}
}
