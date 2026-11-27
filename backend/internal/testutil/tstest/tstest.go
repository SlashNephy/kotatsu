// Package tstest は、テストで読ませる、映像と ARIB 字幕を持つ合成の TS を作る。
package tstest

import (
	"encoding/binary"
	"hash/crc32"
	"math/bits"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// pmtPID は、合成した TS の PMT の PID。
	pmtPID = 0x01F0
	// videoPID は、合成した TS の映像 (MPEG-2) の PID。
	videoPID = 0x0111
	// captionPID は、合成した TS の字幕 (component_tag 0x30) の PID。
	captionPID = 0x0130
)

// Stream は、1 つの番組の TS を、パケットを足しながら作る。
type Stream struct {
	serviceID uint16
	packets   [][]byte
	// cc は、PID ごとの continuity_counter。
	cc map[uint16]byte
}

// NewStream は、program_number が serviceID で、映像と字幕を持つ番組の PAT と PMT を書いた Stream を返す。
func NewStream(serviceID uint16) *Stream {
	s := &Stream{serviceID: serviceID, packets: [][]byte{}, cc: map[uint16]byte{}}
	pat := binary.BigEndian.AppendUint16(nil, serviceID)
	pat = binary.BigEndian.AppendUint16(pat, 0xE000|pmtPID)
	s.section(0x0000, section(0x00, 0x7FE0, pat))
	pmt := []byte{0xE0 | videoPID>>8, videoPID & 0xFF, 0xF0, 0x00}
	pmt = append(pmt, 0x02)
	pmt = binary.BigEndian.AppendUint16(pmt, 0xE000|videoPID)
	pmt = append(pmt, 0xF0, 0x00)
	pmt = append(pmt, 0x06)
	pmt = binary.BigEndian.AppendUint16(pmt, 0xE000|captionPID)
	// stream_identifier_descriptor で component_tag を 0x30 にする。
	pmt = append(pmt, 0xF0, 0x03, 0x52, 0x01, 0x30)
	s.section(pmtPID, section(0x02, serviceID, pmt))

	return s
}

// Keyframe は、PTS が pts の、MPEG-2 の I フレームの PES を足す。
func (s *Stream) Keyframe(pts uint64) *Stream {
	s.pes(videoPID, 0xE0, pts, []byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x08})

	return s
}

// Frame は、PTS が pts の、MPEG-2 の P フレームの PES を足す。
func (s *Stream) Frame(pts uint64) *Stream {
	s.pes(videoPID, 0xE0, pts, []byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x10})

	return s
}

// Management は、PTS が pts の、日本語の 1 言語のグループ A の字幕管理を足す。
func (s *Stream) Management(pts uint64) *Stream {
	s.captionGroup(pts, 0x00, []byte{0x00, 0x01, 0x00, 'j', 'p', 'n', 0x00, 0x00, 0x00, 0x00})

	return s
}

// Statement は、PTS が pts の、グループ A の第 1 言語の字幕文を足す。body は字幕文の本文 (8 単位符号)。
func (s *Stream) Statement(pts uint64, body []byte) *Stream {
	unit := make([]byte, 0, 5+len(body))
	unit = append(unit, 0x1F, 0x20, byte(len(body)>>16&0xFF), byte(len(body)>>8&0xFF), byte(len(body)&0xFF))
	unit = append(unit, body...)
	group := append([]byte{0x00, byte(len(unit) >> 16 & 0xFF), byte(len(unit) >> 8 & 0xFF), byte(len(unit) & 0xFF)}, unit...)
	s.captionGroup(pts, 0x01, group)

	return s
}

// Bytes は、足したパケットをつないで返す。
func (s *Stream) Bytes() []byte {
	return slices.Concat(s.packets...)
}

// captionGroup は、data_group_id が groupID のデータグループを入れた字幕の PES を足す。
func (s *Stream) captionGroup(pts uint64, groupID byte, group []byte) {
	dataGroup := append([]byte{groupID << 2, 0x00, 0x00, byte(len(group) >> 8 & 0xFF), byte(len(group) & 0xFF)}, group...)
	// CRC_16 は読まないので、0 で埋める。
	dataGroup = append(dataGroup, 0x00, 0x00)
	s.pes(captionPID, 0xBD, pts, append([]byte{0x80, 0xFF, 0xF0}, dataGroup...))
}

// pes は、PTS が pts で、内容が data の PES を、184 バイトずつのパケットに分けて足す。
func (s *Stream) pes(pid uint16, streamID byte, pts uint64, data []byte) {
	header := append([]byte{0x80, 0x80, 0x05}, encodePTS(pts)...)
	pes := []byte{0x00, 0x00, 0x01, streamID}
	pes = binary.BigEndian.AppendUint16(pes, uint16((len(header)+len(data))&0xFFFF))
	pes = append(append(pes, header...), data...)
	s.split(pid, pes)
}

// section は、セクション sec を、pointer_field を付けてパケットに分けて足す。
func (s *Stream) section(pid uint16, sec []byte) {
	s.split(pid, append([]byte{0x00}, sec...))
}

// split は、data を、最初のパケットに payload_unit_start_indicator を立てて、184 バイトずつのパケットに分けて足す。
func (s *Stream) split(pid uint16, data []byte) {
	for first := true; len(data) > 0; first = false {
		p := make([]byte, ts.PacketSize)
		for i := range p {
			p[i] = 0xFF
		}
		p[0] = 0x47
		binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
		if first {
			p[1] |= 0x40
		}
		p[3] = 0x10 | s.cc[pid]&0x0F
		s.cc[pid]++
		n := copy(p[4:], data)
		data = data[n:]
		s.packets = append(s.packets, p)
	}
}

// section は、section_syntax_indicator が 1 で、CRC を付けたセクションを作る。
func section(tableID byte, extension uint16, body []byte) []byte {
	length := 5 + len(body) + 4
	s := []byte{tableID}
	s = binary.BigEndian.AppendUint16(s, 0xB000|uint16(length&0x0FFF))
	s = binary.BigEndian.AppendUint16(s, extension)
	s = append(s, 0xC1, 0x00, 0x00)
	s = append(s, body...)

	return binary.BigEndian.AppendUint32(s, crcMPEG(s))
}

// crcMPEG は、CRC-32/MPEG-2 を返す。
func crcMPEG(b []byte) uint32 {
	reversed := make([]byte, len(b))
	for i, v := range b {
		reversed[i] = bits.Reverse8(v)
	}

	return bits.Reverse32(^crc32.ChecksumIEEE(reversed))
}

// encodePTS は、PES のヘッダーの PTS の 5 バイトの符号化を返す。
func encodePTS(pts uint64) []byte {
	high := binary.BigEndian.AppendUint64(nil, pts>>29)
	mid := binary.BigEndian.AppendUint16(nil, uint16(pts>>15&0x7FFF)<<1|0x01)
	low := binary.BigEndian.AppendUint16(nil, uint16(pts&0x7FFF)<<1|0x01)

	return []byte{0x21 | high[7]&0x0E, mid[0], mid[1], low[0], low[1]}
}
