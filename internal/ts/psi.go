package ts

const (
	patPID = 0x0000

	tableIDPAT = 0x00
	tableIDPMT = 0x02

	// maxSectionSize は、PSI のセクションの大きさの上限 (ISO/IEC 13818-1 では PAT と PMT は 1024 バイトまで)。
	// 私的なセクションは 4096 バイトまでなので、そちらに合わせる。
	maxSectionSize = 4096
	// stuffingByte は、セクションの後ろを埋めるバイト。これが来たら、そのパケットにはもうセクションがない。
	stuffingByte = 0xFF
)

// sectionAssembler は、1 つの PID のパケットからセクションを組み立てる。
// セクションは複数のパケットにまたがることがあり、1 つのパケットに複数のセクションが入ることもある。
type sectionAssembler struct {
	buf []byte
	// active は、セクションの途中を受け取っているかどうか。始まりを見ていなければ、続きのパケットは捨てる。
	active bool
}

func newSectionAssembler() *sectionAssembler {
	return &sectionAssembler{buf: make([]byte, 0, 1024), active: false}
}

// reset は、組み立て中のセクションを捨てる。ドロップや誤りでパケットが欠けたときに呼ぶ。
func (s *sectionAssembler) reset() {
	s.buf = s.buf[:0]
	s.active = false
}

// feed はパケットのペイロードを受け取り、組み立て終えたセクションごとに emit を呼ぶ。
// emit に渡すスライスは、次の feed の呼び出しまでしか使えない。
func (s *sectionAssembler) feed(payloadUnitStart bool, payload []byte, emit func(section []byte)) {
	if !payloadUnitStart {
		if s.active {
			s.append(payload, emit)
		}

		return
	}
	if len(payload) == 0 {
		s.reset()

		return
	}

	// pointer_field は、このパケットで始まる最初のセクションの位置を表す。その前は、前のセクションの続き。
	pointer := int(payload[0])
	rest := payload[1:]
	if pointer > len(rest) {
		s.reset()

		return
	}
	if s.active {
		s.append(rest[:pointer], emit)
	}
	s.reset()
	s.active = true
	s.append(rest[pointer:], emit)
}

// append は data をつなぎ、セクションが揃ったら emit を呼ぶ。続けて次のセクションが始まっていれば、それも組み立てる。
func (s *sectionAssembler) append(data []byte, emit func(section []byte)) {
	for len(data) > 0 {
		if len(s.buf) == 0 && data[0] == stuffingByte {
			s.reset()

			return
		}
		if len(s.buf)+len(data) > maxSectionSize {
			s.reset()

			return
		}
		s.buf = append(s.buf, data...)
		data = nil

		for len(s.buf) >= 3 {
			total := 3 + (int(s.buf[1]&0x0F)<<8 | int(s.buf[2]))
			if len(s.buf) < total {
				break
			}
			emit(s.buf[:total])
			remaining := s.buf[total:]
			if len(remaining) == 0 || remaining[0] == stuffingByte {
				s.buf = s.buf[:0]
				s.active = false

				return
			}
			// 次のセクションが同じパケットで始まっている。
			s.buf = append(s.buf[:0], remaining...)
		}
	}
}

// crc32MPEG は、MPEG-2 の CRC-32 (多項式 0x04C11DB7、初期値 0xFFFFFFFF、反転なし) を計算する。
// 末尾の CRC を含めたセクション全体に対して計算すると、壊れていなければ 0 になる。
// hash/crc32 はビットを反転する方式なので使えない。セクションは小さいので、表を使わずにビットごとに計算する。
func crc32MPEG(b []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, v := range b {
		crc ^= uint32(v) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

// sectionHeader は、section_syntax_indicator が 1 のセクションの共通のヘッダー。
type sectionHeader struct {
	tableID uint8
	// tableIDExtension は、PAT では transport_stream_id、PMT では program_number。
	tableIDExtension uint16
	version          uint8
	currentNext      bool
	// body は、ヘッダーの後から CRC の前までの部分。
	body []byte
}

// parseSectionHeader は、セクションのヘッダーを読み、CRC を確かめる。壊れていれば nil を返す。
func parseSectionHeader(section []byte) *sectionHeader {
	// table_id から last_section_number までの 8 バイトと、CRC の 4 バイトは必ずある。
	if len(section) < 12 || section[1]&0x80 == 0 || crc32MPEG(section) != 0 {
		return nil
	}

	return &sectionHeader{
		tableID:          section[0],
		tableIDExtension: uint16(section[3])<<8 | uint16(section[4]),
		version:          section[5] >> 1 & 0x1F,
		currentNext:      section[5]&0x01 != 0,
		body:             section[8 : len(section)-4],
	}
}
