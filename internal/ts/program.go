package ts

import (
	"maps"
	"slices"
)

// StreamKind は、エレメンタリーストリームの大まかな種類。
type StreamKind string

const (
	StreamVideo StreamKind = "video"
	StreamAudio StreamKind = "audio"
	// StreamOther は、字幕、データ放送、その他のストリーム。
	StreamOther StreamKind = "other"
)

// stream_type の値 (ISO/IEC 13818-1、ARIB STD-B10)。
const (
	streamTypeMPEG1Video = 0x01
	streamTypeMPEG2Video = 0x02
	streamTypeMPEG1Audio = 0x03
	streamTypeMPEG2Audio = 0x04
	streamTypeAACADTS    = 0x0F
	streamTypeAACLATM    = 0x11
	streamTypeH264       = 0x1B
	streamTypeHEVC       = 0x24
)

// Program は、PMT が表す番組 (サービス) の構成。
type Program struct {
	// Number は program_number。ARIB ではサービス ID と同じ値になる。
	Number uint16
	PMTPID uint16
	PCRPID uint16
	// Version は PMT の version_number。構成が変わると増える。
	Version uint8
	// Streams は、PMT に書かれた順のエレメンタリーストリーム。
	Streams []*Stream
}

// Stream はエレメンタリーストリーム。
type Stream struct {
	PID uint16
	// StreamType は stream_type。0x02 は MPEG-2 映像、0x0F は AAC (ADTS) を表す。
	StreamType uint8
	Kind       StreamKind
}

// streamKind は、stream_type から大まかな種類を決める。
func streamKind(streamType uint8) StreamKind {
	switch streamType {
	case streamTypeMPEG1Video, streamTypeMPEG2Video, streamTypeH264, streamTypeHEVC:
		return StreamVideo
	case streamTypeMPEG1Audio, streamTypeMPEG2Audio, streamTypeAACADTS, streamTypeAACLATM:
		return StreamAudio
	default:
		return StreamOther
	}
}

// programTable は、PAT と PMT から分かった番組の構成。
type programTable struct {
	// patVersion は、最後に読んだ PAT の version_number。まだ読んでいなければ -1。
	patVersion int
	// pmtPIDs は、program_number から PMT の PID への対応。program_number 0 (NIT) は含めない。
	pmtPIDs map[uint16]uint16
	// programs は、program_number ごとの、最後に読んだ PMT の内容。
	programs map[uint16]*Program
}

func newProgramTable() *programTable {
	return &programTable{patVersion: -1, pmtPIDs: map[uint16]uint16{}, programs: map[uint16]*Program{}}
}

// isPMTPID は、pid が PAT に書かれた PMT の PID かどうかを返す。
func (t *programTable) isPMTPID(pid uint16) bool {
	for _, pmtPID := range t.pmtPIDs {
		if pmtPID == pid {
			return true
		}
	}

	return false
}

// handlePAT は PAT のセクションを読む。版が変わったら、PAT から消えた番組の構成も捨てる。
func (t *programTable) handlePAT(section []byte) {
	header := parseSectionHeader(section)
	if header == nil || header.tableID != tableIDPAT || !header.currentNext {
		return
	}
	if int(header.version) != t.patVersion {
		t.patVersion = int(header.version)
		clear(t.pmtPIDs)
	}

	for entry := header.body; len(entry) >= 4; entry = entry[4:] {
		number := uint16(entry[0])<<8 | uint16(entry[1])
		pid := uint16(entry[2]&0x1F)<<8 | uint16(entry[3])
		if number == 0 {
			continue
		}
		t.pmtPIDs[number] = pid
	}
	maps.DeleteFunc(t.programs, func(number uint16, _ *Program) bool {
		_, ok := t.pmtPIDs[number]

		return !ok
	})
}

// handlePMT は PMT のセクションを読む。
func (t *programTable) handlePMT(pid uint16, section []byte) {
	header := parseSectionHeader(section)
	if header == nil || header.tableID != tableIDPMT || !header.currentNext || len(header.body) < 4 {
		return
	}
	number := header.tableIDExtension
	if t.pmtPIDs[number] != pid {
		return
	}
	if current := t.programs[number]; current != nil && current.Version == header.version {
		return
	}

	body := header.body
	program := &Program{
		Number:  number,
		PMTPID:  pid,
		PCRPID:  uint16(body[0]&0x1F)<<8 | uint16(body[1]),
		Version: header.version,
		Streams: []*Stream{},
	}
	programInfoLength := int(body[2]&0x0F)<<8 | int(body[3])
	if 4+programInfoLength > len(body) {
		return
	}
	for entry := body[4+programInfoLength:]; len(entry) >= 5; {
		streamType := entry[0]
		esInfoLength := int(entry[3]&0x0F)<<8 | int(entry[4])
		program.Streams = append(program.Streams, &Stream{
			PID:        uint16(entry[1]&0x1F)<<8 | uint16(entry[2]),
			StreamType: streamType,
			Kind:       streamKind(streamType),
		})
		if 5+esInfoLength > len(entry) {
			break
		}
		entry = entry[5+esInfoLength:]
	}
	t.programs[number] = program
}

// snapshot は、番組の構成を program_number の昇順に並べて複製する。
func (t *programTable) snapshot() []*Program {
	programs := make([]*Program, 0, len(t.programs))
	for _, number := range slices.Sorted(maps.Keys(t.programs)) {
		p := *t.programs[number]
		streams := make([]*Stream, 0, len(p.Streams))
		for _, s := range p.Streams {
			copied := *s
			streams = append(streams, &copied)
		}
		p.Streams = streams
		programs = append(programs, &p)
	}

	return programs
}
