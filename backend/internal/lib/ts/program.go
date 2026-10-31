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
	// StreamType は stream_type。0x02 は MPEG-2 映像、0x1B は H.264、0x24 は HEVC、0x0F は AAC (ADTS) を表す。
	StreamType uint8
	Kind       StreamKind
	// ComponentTag は、ストリーム識別記述子の component_tag (#827)。EPG の映像と音声のコンポーネントと対応づける。
	// 記述子がなければ nil。
	ComponentTag *uint8
}

// descriptorTagStreamIdentifier は、ストリーム識別記述子 (stream_identifier_descriptor) の記述子タグ。
const descriptorTagStreamIdentifier = 0x52

// componentTag は、PMT の ES_info の記述子の並びから、ストリーム識別記述子の component_tag を読む。なければ nil。
func componentTag(descriptors []byte) *uint8 {
	for len(descriptors) >= 2 {
		tag, length := descriptors[0], int(descriptors[1])
		if 2+length > len(descriptors) {
			return nil
		}
		if tag == descriptorTagStreamIdentifier && length >= 1 {
			return new(descriptors[2])
		}
		descriptors = descriptors[2+length:]
	}

	return nil
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
	// generation は、PAT か PMT を読んで構成が変わるたびに増える。
	generation int
	// transportStreamID は、最後に読んだ PAT の transport_stream_id (#830)。まだ読んでいなければ nil。
	transportStreamID *uint16
}

func newProgramTable() *programTable {
	return &programTable{patVersion: -1, pmtPIDs: map[uint16]uint16{}, programs: map[uint16]*Program{}, generation: 0, transportStreamID: nil}
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
	t.transportStreamID = new(header.tableIDExtension)
	// PAT は 100 ミリ秒ほどの間隔で同じ内容が繰り返し届くので、構成が変わったときだけ世代を進める。
	changed := false
	if int(header.version) != t.patVersion {
		t.patVersion = int(header.version)
		clear(t.pmtPIDs)
		changed = true
	}

	for entry := header.body; len(entry) >= 4; entry = entry[4:] {
		number := uint16(entry[0])<<8 | uint16(entry[1])
		pid := uint16(entry[2]&0x1F)<<8 | uint16(entry[3])
		if number == 0 {
			continue
		}
		if current, ok := t.pmtPIDs[number]; !ok || current != pid {
			t.pmtPIDs[number] = pid
			changed = true
		}
	}
	maps.DeleteFunc(t.programs, func(number uint16, _ *Program) bool {
		_, ok := t.pmtPIDs[number]

		return !ok
	})
	if changed {
		t.generation++
	}
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
		var tag *uint8
		if 5+esInfoLength <= len(entry) {
			tag = componentTag(entry[5 : 5+esInfoLength])
		}
		program.Streams = append(program.Streams, &Stream{
			PID:          uint16(entry[1]&0x1F)<<8 | uint16(entry[2]),
			StreamType:   streamType,
			Kind:         streamKind(streamType),
			ComponentTag: tag,
		})
		if 5+esInfoLength > len(entry) {
			break
		}
		entry = entry[5+esInfoLength:]
	}
	t.programs[number] = program
	t.generation++
}

// firstVideo は、program_number が最も小さい番組の、最初の映像のストリームを返す。
func (t *programTable) firstVideo() (*Stream, bool) {
	for _, number := range slices.Sorted(maps.Keys(t.programs)) {
		for _, s := range t.programs[number].Streams {
			if s.Kind == StreamVideo {
				return s, true
			}
		}
	}

	return nil, false
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
