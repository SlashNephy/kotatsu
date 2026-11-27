package ts

import "io"

// 字幕の文を、録画の TS から取り出す (#2216、設計書 7.4)。
//
// 字幕は、PMT の stream_type 0x06 の ES のうち、component_tag が 0x30〜0x37 のもので送られる (ARIB STD-B24 第 1 編 第 3 部)。
// PES (独立 PES、同期型) の中の、データグループの字幕文 (第 1 言語) を読む。字幕管理は読まない。
// 時刻は、字幕文の PES の PTS のまま返す。録画の時間軸への換算と、字幕を出す区間 (次に画面を消すまで) は、使う側で決める。

const (
	// streamTypePrivatePES は、独立 PES を運ぶ stream_type。
	streamTypePrivatePES = 0x06
	// captionComponentTagFirst と captionComponentTagLast は、字幕の component_tag の範囲。0x30 が既定の字幕。
	captionComponentTagFirst = 0x30
	captionComponentTagLast  = 0x37
	// captionDataIdentifier は、同期型 PES の data_identifier。
	captionDataIdentifier = 0x80
	// captionPrivateStreamID は、同期型 PES の private_stream_id。
	captionPrivateStreamID = 0xFF
	// captionStatementLanguage は、読む字幕文のデータグループ (data_group_id の下位 4 ビット)。第 1 言語。
	captionStatementLanguage = 0x01
	// captionUnitSeparator は、データユニットの区切り (unit_separator)。
	captionUnitSeparator = 0x1F
	// データユニットの data_unit_parameter。
	captionUnitStatement = 0x20
	captionUnitDRCS1     = 0x30
	captionUnitDRCS2     = 0x31
	// captionMaxPESSize は、組み立てる PES の大きさの上限。字幕の PES は数 KB に収まるので、壊れたデータで膨らみ続けないようにする。
	captionMaxPESSize = 64 * 1024
)

// Caption は、字幕文 1 つ。
type Caption struct {
	// PTS は、字幕文の PES の PTS (90 kHz、33 ビット)。
	PTS int64
	// Text は、字幕文が書いた文。画面を消してから書いた文だけを含む。書かなければ空。
	Text string
	// Cleared は、字幕文が画面を消した (CS) かどうか。消すだけの字幕文は、Text が空になる。
	Cleared bool
}

// CaptionScanner は、TS を流し読みして、サービスの字幕文を集める。io.Writer として、ファイルを読む経路に挟んで使う。
type CaptionScanner struct {
	serviceID uint16
	programs  *programTable
	sections  map[uint16]*sectionAssembler
	// generation は、programs の世代のうち、字幕の PID を決め直したもの。
	generation int
	pid        uint16
	// pes は、組み立てている PES。continuity は、最後に読んだパケットの continuity_counter。
	pes        []byte
	assembling bool
	continuity uint8
	decoder    *aribDecoder
	captions   []Caption
	carry      [PacketSize]byte
	carryLen   int
}

// NewCaptionScanner は、サービス serviceID (program_number) の字幕文を集める CaptionScanner を返す。
// serviceID が 0 か、PAT にないときは、映像のある番組のうち program_number が最も小さいものを読む (NewCueScanner と同じ)。
// replaceDRCS は、外字の画像の MD5 (小文字の 16 進) から置き換える文字を引く。nil なら、外字はすべて「〓」にする。
func NewCaptionScanner(serviceID uint16, replaceDRCS func(md5 string) (string, bool)) *CaptionScanner {
	return &CaptionScanner{
		serviceID: serviceID, programs: newProgramTable(), sections: map[uint16]*sectionAssembler{},
		generation: -1, pid: NullPID, pes: []byte{}, assembling: false, continuity: 0,
		decoder: newCaptionDecoder(replaceDRCS), captions: []Caption{},
		carry: [PacketSize]byte{}, carryLen: 0,
	}
}

var _ io.Writer = new(CaptionScanner)

// Write は、TS を読む。パケットの区切りは、書き込みの区切りとそろっていなくてよい。同期を失ったら、次の sync byte まで読み飛ばす。
func (s *CaptionScanner) Write(p []byte) (int, error) {
	n := len(p)
	if s.carryLen > 0 {
		copied := copy(s.carry[s.carryLen:], p)
		s.carryLen += copied
		p = p[copied:]
		if s.carryLen < PacketSize {
			return n, nil
		}
		s.handle(s.carry[:])
		s.carryLen = 0
	}
	for len(p) > 0 {
		if p[0] != syncByte {
			p = p[1:]

			continue
		}
		if len(p) < PacketSize {
			s.carryLen = copy(s.carry[:], p)

			break
		}
		s.handle(p[:PacketSize])
		p = p[PacketSize:]
	}

	return n, nil
}

// Captions は、ここまでに集めた字幕文を、届いた順に返す。組み立て終えていない最後の PES も読む。
func (s *CaptionScanner) Captions() []Caption {
	s.flush()

	return s.captions
}

func (s *CaptionScanner) handle(b []byte) {
	pkt := parsePacket(b)
	if pkt.malformed || pkt.transportError || pkt.scrambling != 0 || pkt.payload == nil {
		return
	}
	if pkt.pid == patPID || s.programs.isPMTPID(pkt.pid) {
		s.handlePSI(&pkt)

		return
	}
	if pkt.pid != s.pid || s.pid == NullPID {
		return
	}
	if pkt.payloadUnitStart {
		s.flush()
		s.pes = append(s.pes[:0], pkt.payload...)
		s.assembling, s.continuity = true, pkt.continuityCounter
		s.flushIfComplete()

		return
	}
	if !s.assembling {
		return
	}
	if pkt.continuityCounter != (s.continuity+1)&0x0F {
		// パケットが欠けた PES は読まない。
		s.pes, s.assembling = s.pes[:0], false

		return
	}
	s.continuity = pkt.continuityCounter
	s.pes = append(s.pes, pkt.payload...)
	if len(s.pes) > captionMaxPESSize {
		s.pes, s.assembling = s.pes[:0], false

		return
	}
	s.flushIfComplete()
}

// flushIfComplete は、PES_packet_length の分を読み終えていれば、PES を読む。
func (s *CaptionScanner) flushIfComplete() {
	if len(s.pes) < 6 {
		return
	}
	length := int(s.pes[4])<<8 | int(s.pes[5])
	if length != 0 && len(s.pes) >= 6+length {
		s.pes = s.pes[:6+length]
		s.flush()
	}
}

// flush は、組み立てた PES を読み、破棄する。
func (s *CaptionScanner) flush() {
	if !s.assembling {
		return
	}
	s.assembling = false
	data, pts, ok := parsePESHeader(s.pes)
	s.pes = s.pes[:0]
	if !ok || pts == noPTS {
		return
	}
	s.readDataGroup(data, pts)
}

// readDataGroup は、PES のデータ (data_identifier から) を読み、第 1 言語の字幕文を字幕文の一覧に足す。
func (s *CaptionScanner) readDataGroup(data []byte, pts int64) {
	if len(data) < 3 || data[0] != captionDataIdentifier || data[1] != captionPrivateStreamID {
		return
	}
	begin := 3 + int(data[2]&0x0F)
	if begin+5 > len(data) {
		return
	}
	groupID := data[begin] >> 2
	size := int(data[begin+3])<<8 | int(data[begin+4])
	if begin+5+size > len(data) || groupID&0x0F != captionStatementLanguage {
		return
	}
	group := data[begin+5 : begin+5+size]
	// 字幕文: TMD (2 ビット) の後、TMD が 01 か 10 なら STM (5 バイト) が続き、data_unit_loop_length (24 ビット) とデータユニットが続く。
	if len(group) < 1 {
		return
	}
	offset := 1
	if tmd := group[0] >> 6; tmd == 0x01 || tmd == 0x02 {
		offset += 5
	}
	if offset+3 > len(group) {
		return
	}
	loopLength := int(group[offset])<<16 | int(group[offset+1])<<8 | int(group[offset+2])
	offset += 3
	if offset+loopLength > len(group) {
		return
	}
	s.readDataUnits(group[offset:offset+loopLength], pts)
}

// readDataUnits は、データユニットの並びを読む。外字を覚えてから、本文を読む。
func (s *CaptionScanner) readDataUnits(units []byte, pts int64) {
	caption := Caption{PTS: pts, Text: "", Cleared: false}
	found := false
	for offset := 0; offset+5 <= len(units); {
		if units[offset] != captionUnitSeparator {
			break
		}
		parameter := units[offset+1]
		size := int(units[offset+2])<<16 | int(units[offset+3])<<8 | int(units[offset+4])
		offset += 5
		if offset+size > len(units) {
			break
		}
		body := units[offset : offset+size]
		offset += size
		switch parameter {
		case captionUnitStatement:
			text, cleared := s.decoder.statement(body)
			if cleared {
				caption.Text, caption.Cleared = "", true
			}
			caption.Text += text
			found = true
		case captionUnitDRCS1:
			s.decoder.defineDRCS(body, false)
		case captionUnitDRCS2:
			s.decoder.defineDRCS(body, true)
		}
	}
	if found {
		s.captions = append(s.captions, caption)
	}
}

func (s *CaptionScanner) handlePSI(pkt *packet) {
	assembler := s.sections[pkt.pid]
	if assembler == nil {
		assembler = newSectionAssembler()
		s.sections[pkt.pid] = assembler
	}
	assembler.feed(pkt.payloadUnitStart, pkt.payload, func(section []byte) {
		if pkt.pid == patPID {
			s.programs.handlePAT(section)
		} else {
			s.programs.handlePMT(pkt.pid, section)
		}
	})
	if s.generation != s.programs.generation {
		s.generation = s.programs.generation
		s.assign()
	}
}

// assign は、PMT から、サービスの字幕の PID を決め直す。component_tag 0x30 (既定の字幕) を選び、なければ 0x30〜0x37 の最初のものを選ぶ。
func (s *CaptionScanner) assign() {
	program := s.programs.programs[s.serviceID]
	if program == nil {
		program = s.programs.firstVideoProgram()
	}
	if program == nil {
		return
	}
	pid := uint16(NullPID)
	for _, stream := range program.Streams {
		if stream.StreamType != streamTypePrivatePES || stream.ComponentTag == nil {
			continue
		}
		tag := *stream.ComponentTag
		if tag == captionComponentTagFirst {
			pid = stream.PID

			break
		}
		if tag > captionComponentTagFirst && tag <= captionComponentTagLast && pid == NullPID {
			pid = stream.PID
		}
	}
	if pid != s.pid {
		s.pid, s.pes, s.assembling = pid, s.pes[:0], false
	}
}
