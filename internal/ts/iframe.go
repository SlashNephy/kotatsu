package ts

const (
	// pictureStartCode は、MPEG-2 映像の picture_start_code (00 00 01 00) の最後のバイト。
	pictureStartCode = 0x00
	// pictureTypeI は、picture_coding_type が I ピクチャであることを表す値。
	pictureTypeI = 1
	noPTS        = -1
)

// scanState は、start code を探す状態機械の状態。
type scanState int

const (
	// scanSearching は、00 00 01 を探している状態。
	scanSearching scanState = iota
	// scanStartCode は、00 00 01 を読み、start code の値を待っている状態。
	scanStartCode
	// scanTemporalReference は、picture_start_code を読み、temporal_reference の上位 8 ビットを待っている状態。
	scanTemporalReference
	// scanPictureType は、temporal_reference の下位 2 ビットと picture_coding_type を含むバイトを待っている状態。
	scanPictureType
)

// iframeDetector は、MPEG-2 映像の PES から I ピクチャを探す。
//
// start code はパケットの境界をまたぐことがあるので、バイトごとに進む状態機械で探す。
// H.264 と HEVC の映像は扱わない。
type iframeDetector struct {
	pid uint16
	// pesOffset は、今の PES の先頭パケットの位置。まだ PES の始まりを見ていなければ -1。
	pesOffset int64
	pesPTS    int64
	// found は、今の PES で I ピクチャを見つけたかどうか。1 つの PES につき 1 件だけ索引に入れる。
	found bool

	state scanState
	// zeros は、直前に続いた 0x00 の数。
	zeros int
}

func newIFrameDetector(pid uint16) *iframeDetector {
	return &iframeDetector{pid: pid, pesOffset: -1, pesPTS: noPTS, found: false, state: scanSearching, zeros: 0}
}

// reset は、探索の途中の状態を捨てる。ドロップでパケットが欠けたときに呼ぶ。
func (d *iframeDetector) reset() {
	d.pesOffset = -1
	d.pesPTS = noPTS
	d.found = false
	d.resetScan()
}

func (d *iframeDetector) resetScan() {
	d.state = scanSearching
	d.zeros = 0
}

// feed は、映像の PID のパケットを受け取る。I ピクチャを見つけたら、その PES の索引を返す。
func (d *iframeDetector) feed(pkt *packet, offset int64) (IndexEntry, bool) {
	payload := pkt.payload
	if pkt.payloadUnitStart {
		d.reset()
		es, pts, ok := parsePESHeader(payload)
		if !ok {
			return IndexEntry{Offset: 0, PTS: 0}, false
		}
		d.pesOffset = offset
		d.pesPTS = pts
		payload = es
	}
	if d.pesOffset < 0 || d.found {
		return IndexEntry{Offset: 0, PTS: 0}, false
	}

	for _, b := range payload {
		if d.step(b) {
			d.found = true
			if d.pesPTS == noPTS {
				return IndexEntry{Offset: 0, PTS: 0}, false
			}

			return IndexEntry{Offset: d.pesOffset, PTS: d.pesPTS}, true
		}
	}

	return IndexEntry{Offset: 0, PTS: 0}, false
}

// step は 1 バイト進め、I ピクチャの picture_coding_type を読んだら true を返す。
func (d *iframeDetector) step(b byte) bool {
	switch d.state {
	case scanSearching:
		switch {
		case b == 0x00:
			d.zeros++
		case b == 0x01 && d.zeros >= 2:
			d.state = scanStartCode
			d.zeros = 0
		default:
			d.zeros = 0
		}
	case scanStartCode:
		if b == pictureStartCode {
			d.state = scanTemporalReference
		} else {
			d.resetScan()
		}
	case scanTemporalReference:
		d.state = scanPictureType
	case scanPictureType:
		d.resetScan()

		return b>>3&0x07 == pictureTypeI
	}

	return false
}

// parsePESHeader は、PES のヘッダーを読み、エレメンタリーストリームの部分と PTS を返す。
// PTS がなければ noPTS を返す。PES の始まりでなければ false を返す。
func parsePESHeader(payload []byte) ([]byte, int64, bool) {
	// packet_start_code_prefix (00 00 01)、stream_id、PES_packet_length、フラグ 2 バイト、PES_header_data_length。
	if len(payload) < 9 || payload[0] != 0x00 || payload[1] != 0x00 || payload[2] != 0x01 {
		return nil, noPTS, false
	}
	headerEnd := 9 + int(payload[8])
	if headerEnd > len(payload) {
		return nil, noPTS, false
	}

	pts := int64(noPTS)
	if payload[7]&0x80 != 0 && headerEnd >= 14 {
		b := payload[9:14]
		pts = int64(b[0]>>1&0x07)<<30 | int64(b[1])<<22 | int64(b[2]>>1)<<15 | int64(b[3])<<7 | int64(b[4]>>1)
	}

	return payload[headerEnd:], pts, true
}
