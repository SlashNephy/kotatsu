// Package ts は、録画する MPEG-2 TS をパケット単位で流し読みし、その場で解析する (設計書 5.3)。
//
// 扱うのは、ARIB の知識を必要としないパケット単位の処理 (ドロップの集計、PAT と PMT、I フレームの位置) だけである。
// 録画の書き込みと同じ経路で動くので、パケットごとのメモリ確保をしない。
package ts

const (
	// PacketSize は TS パケットの大きさ。
	PacketSize = 188
	// syncByte は、TS パケットの先頭のバイト。
	syncByte = 0x47
	// NullPID は、帯域を埋めるための null パケットの PID。
	NullPID = 0x1FFF
	// noPCR は、PCR を持たないパケットの PCR の値。
	noPCR = -1
)

// packet は、TS パケットのヘッダーと、アダプテーションフィールドのうち使う項目。
type packet struct {
	pid uint16
	// transportError は transport_error_indicator。伝送路で訂正できない誤りが起きたことを表す。
	transportError bool
	// payloadUnitStart は payload_unit_start_indicator。PES や PSI のセクションがこのパケットから始まることを表す。
	payloadUnitStart bool
	// scrambling は transport_scrambling_control。0 以外ならペイロードがスクランブルされている。
	scrambling uint8
	hasPayload bool
	// continuityCounter は continuity_counter。ペイロードを持つパケットごとに 1 ずつ増える 4 ビットの値。
	continuityCounter uint8
	// discontinuity は discontinuity_indicator。continuity_counter の不連続が意図的であることを表す。
	discontinuity bool
	// pcr は PCR (27 MHz)。持たなければ noPCR。
	pcr int64
	// payload は、ペイロードの部分。b を指すので、b を書き換えたら使えない。
	payload []byte
	// malformed は、アダプテーションフィールドの長さがパケットに収まらないことを表す。
	malformed bool
}

// parsePacket は、188 バイトのパケットを読む。b の先頭が sync byte であることは、呼び出し側が確かめる。
func parsePacket(b []byte) packet {
	p := packet{
		pid:               uint16(b[1]&0x1F)<<8 | uint16(b[2]),
		transportError:    b[1]&0x80 != 0,
		payloadUnitStart:  b[1]&0x40 != 0,
		scrambling:        b[3] >> 6,
		hasPayload:        b[3]&0x10 != 0,
		continuityCounter: b[3] & 0x0F,
		discontinuity:     false,
		pcr:               noPCR,
		payload:           nil,
		malformed:         false,
	}

	offset := 4
	if b[3]&0x20 != 0 {
		length := int(b[4])
		offset = 5 + length
		if offset > PacketSize {
			p.malformed = true

			return p
		}
		if length > 0 {
			flags := b[5]
			p.discontinuity = flags&0x80 != 0
			if flags&0x10 != 0 && length >= 7 {
				p.pcr = parsePCR(b[6:12])
			}
		}
	}
	if p.hasPayload {
		p.payload = b[offset:PacketSize]
	}

	return p
}

// parsePCR は、6 バイトの PCR (33 ビットの base と 9 ビットの extension) を 27 MHz の値にする。
func parsePCR(b []byte) int64 {
	base := int64(b[0])<<25 | int64(b[1])<<17 | int64(b[2])<<9 | int64(b[3])<<1 | int64(b[4])>>7
	extension := int64(b[4]&0x01)<<8 | int64(b[5])

	return base*300 + extension
}
