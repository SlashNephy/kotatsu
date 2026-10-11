package ts

// SI を運ぶパケットの PID (ARIB STD-B10)。データ放送の描画には、番組の情報と時刻と放送事業者の情報が要る。
const (
	nitPID = 0x0010
	// sdtPID は、SDT と BAT を運ぶ。
	sdtPID = 0x0011
	eitPID = 0x0012
	// totPID は、TDT と TOT を運ぶ。
	totPID = 0x0014
	bitPID = 0x0024
)

// データカルーセルを運ぶ stream_type (ISO/IEC 13818-1、ARIB STD-B24)。
const (
	streamTypeDSMCCTypeB = 0x0B
	streamTypeDSMCCTypeC = 0x0C
	streamTypeDSMCCTypeD = 0x0D
)

// adaptationOnly は、ペイロードを持たず、アダプテーションフィールドだけを持つことを表す adaptation_field_control。
const adaptationOnly = 0x20

// DataBroadcastFilter は、1 つのサービスの TS から、データ放送の描画に要るパケットだけを残す (設計書 7.4)。
//
// 残すのは、PAT、サービスの PMT、NIT、SDT、EIT、TOT、BIT、データカルーセルの ES と、PCR を持つパケットである。
// PCR の PID が映像などの ES を兼ねていれば、PCR のパケットからペイロードを除き、帯域を抑える。
// PMT を読むまでは、データカルーセルの PID が分からないので、PSI と SI だけを残す。
type DataBroadcastFilter struct {
	serviceID uint16
	table     *programTable
	pat       *sectionAssembler
	pmt       *sectionAssembler
	// pmtPID は、いま組み立てている PMT の PID。PAT を読むまでは 0。
	pmtPID uint16
	// pcrOnly は、パケットからペイロードを除くときに書き込む先。Filter の戻り値として返す。
	pcrOnly [PacketSize]byte
}

// NewDataBroadcastFilter は、サービス ID (program_number) が serviceID のサービスの DataBroadcastFilter を返す。
func NewDataBroadcastFilter(serviceID uint16) *DataBroadcastFilter {
	return &DataBroadcastFilter{
		serviceID: serviceID,
		table:     newProgramTable(),
		pat:       newSectionAssembler(),
		pmt:       newSectionAssembler(),
		pmtPID:    0,
		pcrOnly:   [PacketSize]byte{},
	}
}

// Filter は、188 バイトのパケット b を読み、残すパケットと true を返す。捨てるなら false を返す。
// 返すスライスは、b か、Filter の中のバッファを指すので、次に Filter を呼ぶまでしか使えない。
func (f *DataBroadcastFilter) Filter(b []byte) ([]byte, bool) {
	if len(b) != PacketSize || b[0] != syncByte {
		return nil, false
	}
	p := parsePacket(b)
	if p.malformed || p.transportError {
		return nil, false
	}

	switch {
	case p.pid == patPID:
		f.readPAT(&p)

		return b, true
	case p.pid == nitPID, p.pid == sdtPID, p.pid == eitPID, p.pid == totPID, p.pid == bitPID:
		return b, true
	case f.pmtPID != 0 && p.pid == f.pmtPID:
		f.pmt.feed(p.payloadUnitStart, p.payload, func(section []byte) {
			f.table.handlePMT(p.pid, section)
		})

		return b, true
	}

	program := f.table.programs[f.serviceID]
	if program == nil {
		return nil, false
	}
	if isCarouselPID(program, p.pid) {
		return b, true
	}
	if p.pid == program.PCRPID && p.pcr != noPCR {
		return f.stripPayload(b), true
	}

	return nil, false
}

// readPAT は、PAT を読み、サービスの PMT の PID が変わったら、組み立て中の PMT を捨てる。
func (f *DataBroadcastFilter) readPAT(p *packet) {
	f.pat.feed(p.payloadUnitStart, p.payload, f.table.handlePAT)
	if pid := f.table.pmtPIDs[f.serviceID]; pid != f.pmtPID {
		f.pmtPID = pid
		f.pmt.reset()
	}
}

// stripPayload は、b のアダプテーションフィールドだけを残し、残りを詰め物で埋めたパケットを返す。
// PCR の PID が専用の PID なら、もともとペイロードがないので、そのまま返す。
func (f *DataBroadcastFilter) stripPayload(b []byte) []byte {
	if b[3]&0x10 == 0 {
		return b
	}
	out := f.pcrOnly[:]
	// ペイロードがなくなるので、payload_unit_start_indicator を下ろす。
	out[0], out[1], out[2] = b[0], b[1]&^0x40, b[2]
	// transport_scrambling_control と continuity_counter は保つ。ペイロードのないパケットでは、continuity_counter は進まない。
	out[3] = b[3]&0xCF | adaptationOnly
	length := int(b[4])
	out[4] = PacketSize - 5
	copy(out[5:5+length], b[5:5+length])
	for i := 5 + length; i < PacketSize; i++ {
		out[i] = stuffingByte
	}

	return out
}

// isCarouselPID は、pid が program のデータカルーセルの ES かどうかを返す。
func isCarouselPID(program *Program, pid uint16) bool {
	for _, s := range program.Streams {
		if s.PID != pid {
			continue
		}
		switch s.StreamType {
		case streamTypeDSMCCTypeB, streamTypeDSMCCTypeC, streamTypeDSMCCTypeD:
			return true
		}
	}

	return false
}
