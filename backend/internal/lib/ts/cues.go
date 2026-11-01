package ts

import "io"

// CM の区間を見分ける手掛かりのうち、TS の中にあるもの (#479)。
//
// CueScanner は、1 つのサービスについて、映像の最初と最後の時刻と、音声の構成が変わった時刻を集める。
// CM と本編の境目では、5.1ch とステレオや、デュアルモノとステレオのように、音声の構成が変わることがある。
// 映像と音声をデコードせずに分かる。
//
// 字幕の本文の有無は、手掛かりにしない。字幕の付いた CM が多く、本文が届いたことで本編とみなすと、CM の区間が途切れるため。

const (
	// adtsChannelUnknown は、ADTS のヘッダーを読めていないときの channel_configuration。
	adtsChannelUnknown = -1
	// stableAudioPES は、channel_configuration が変わったとみなすまでに、新しい値が続く PES の数。
	// PES の始まりが ADTS のフレームの始まりとそろっていないと、たまたま同期語に見えるバイトを読み違えるため。
	stableAudioPES = 10
)

// Cues は、CueScanner が集めた手掛かり。時刻はすべて PTS (90 kHz、33 ビット) のまま持つ。
type Cues struct {
	// FirstVideoPTS と LastVideoPTS は、サービスの映像の、最初と最後の PES の PTS。映像がなければ nil。
	FirstVideoPTS *int64
	LastVideoPTS  *int64
	// AudioChanges は、音声の構成 (音声のストリームの数か、最初の音声の channel_configuration) が変わった PTS。
	// 録画の始まりで構成を初めて知ったときは含めない。
	AudioChanges []int64
}

// CueScanner は、TS を流し読みして、サービスの Cues を集める。io.Writer として、ファイルを読む経路に挟んで使う。
type CueScanner struct {
	serviceID uint16
	programs  *programTable
	sections  map[uint16]*sectionAssembler
	// generation は、programs の世代のうち、ストリームの役割を決め直したもの。
	generation int
	videoPID   uint16
	audioPID   uint16
	audioCount int
	// channels は、最初の音声の、続けて読めた ADTS の channel_configuration。
	channels int
	// pending は、channels と違う値を読み始めた PES の PTS と、その値、続いた PES の数。
	pendingPTS      int64
	pendingChannels int
	pendingCount    int
	// layoutKnown は、音声の構成を一度でも読んだかどうか。初めて読んだときは、変わったとみなさない。
	layoutKnown bool
	// layoutChanged は、PMT で音声の構成が変わり、次の音声の PTS で変わった時刻を記録するのを待っていること。
	layoutChanged bool
	carry         [PacketSize]byte
	carryLen      int
	cues          Cues
}

// NewCueScanner は、サービス serviceID (program_number) の手掛かりを集める CueScanner を返す。
// serviceID が 0 か、PAT にないときは、映像のある番組のうち program_number が最も小さいものを読む。
// 録画は録画したサービスだけの TS なので、FFmpeg が読む最初の映像と音声 (0:v:0、0:a:0) と同じものになる。
func NewCueScanner(serviceID uint16) *CueScanner {
	return &CueScanner{
		serviceID: serviceID, programs: newProgramTable(), sections: map[uint16]*sectionAssembler{},
		generation: -1, videoPID: NullPID, audioPID: NullPID, audioCount: 0,
		channels: adtsChannelUnknown, pendingPTS: noPTS, pendingChannels: adtsChannelUnknown, pendingCount: 0,
		layoutKnown: false, layoutChanged: false,
		carry: [PacketSize]byte{}, carryLen: 0,
		cues: Cues{FirstVideoPTS: nil, LastVideoPTS: nil, AudioChanges: []int64{}},
	}
}

var _ io.Writer = new(CueScanner)

// Write は、TS を読む。パケットの区切りは、書き込みの区切りとそろっていなくてよい。同期を失ったら、次の sync byte まで読み飛ばす。
func (s *CueScanner) Write(p []byte) (int, error) {
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

// Cues は、ここまでに集めた手掛かりを返す。
func (s *CueScanner) Cues() *Cues {
	return &s.cues
}

func (s *CueScanner) handle(b []byte) {
	pkt := parsePacket(b)
	if pkt.malformed || pkt.transportError || pkt.scrambling != 0 || pkt.payload == nil {
		return
	}
	if pkt.pid == patPID || s.programs.isPMTPID(pkt.pid) {
		s.handlePSI(&pkt)

		return
	}
	if !pkt.payloadUnitStart {
		return
	}
	switch pkt.pid {
	case NullPID:
	case s.videoPID:
		if _, pts, ok := parsePESHeader(pkt.payload); ok && pts != noPTS {
			if s.cues.FirstVideoPTS == nil {
				s.cues.FirstVideoPTS = new(pts)
			}
			s.cues.LastVideoPTS = new(pts)
		}
	case s.audioPID:
		s.handleAudio(&pkt)
	}
}

func (s *CueScanner) handlePSI(pkt *packet) {
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

// assign は、PMT から、サービスの映像と最初の音声の PID を決め直す。音声のストリームの数が変わったら、構成が変わったとみなす。
func (s *CueScanner) assign() {
	program := s.programs.programs[s.serviceID]
	if program == nil {
		program = s.programs.firstVideoProgram()
	}
	if program == nil {
		return
	}
	s.videoPID, s.audioPID = NullPID, NullPID
	audios := 0
	for _, stream := range program.Streams {
		switch {
		case stream.Kind == StreamVideo && s.videoPID == NullPID:
			s.videoPID = stream.PID
		case stream.Kind == StreamAudio:
			if s.audioPID == NullPID {
				s.audioPID = stream.PID
			}
			audios++
		}
	}
	if s.audioCount != 0 && audios != s.audioCount {
		s.layoutChanged = true
	}
	s.audioCount = audios
}

// handleAudio は、最初の音声の PES の始まりで、ADTS の channel_configuration を読み、構成が変わった時刻を記録する。
// 新しい値が stableAudioPES 個の PES で続いたら、続き始めた PES の PTS で変わったとみなす。
func (s *CueScanner) handleAudio(pkt *packet) {
	data, pts, ok := parsePESHeader(pkt.payload)
	if !ok || pts == noPTS {
		return
	}
	channels := adtsChannels(data)
	if channels == adtsChannelUnknown {
		return
	}
	if channels == s.channels && !s.layoutChanged {
		s.pendingCount = 0

		return
	}
	if s.pendingCount == 0 || channels != s.pendingChannels {
		s.pendingPTS, s.pendingChannels, s.pendingCount = pts, channels, 0
	}
	s.pendingCount++
	if s.pendingCount < stableAudioPES {
		return
	}
	if s.layoutKnown {
		s.cues.AudioChanges = append(s.cues.AudioChanges, s.pendingPTS)
	}
	s.channels, s.layoutKnown, s.layoutChanged, s.pendingCount = channels, true, false, 0
}

// adtsChannels は、ES の先頭の ADTS のヘッダーから channel_configuration を読む。ADTS で始まっていなければ adtsChannelUnknown を返す。
// ARIB のデュアルモノは channel_configuration が 0 (プログラムコンフィギュレーションエレメントで示す) になるので、ステレオと見分けられる。
func adtsChannels(data []byte) int {
	// 同期語 (12 ビット) と、0 に決まっている layer (2 ビット) を確かめる。
	if len(data) < 4 || data[0] != 0xFF || data[1]&0xF6 != 0xF0 {
		return adtsChannelUnknown
	}

	return int(data[2]&0x01)<<2 | int(data[3]>>6)
}
