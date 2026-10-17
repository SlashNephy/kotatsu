package ts

const (
	// pictureStartCode は、MPEG-2 映像の picture_start_code (00 00 01 00) の最後のバイト。
	pictureStartCode = 0x00
	// pictureTypeI は、picture_coding_type が I ピクチャであることを表す値。
	pictureTypeI = 1
	noPTS        = -1

	// h264NALSlice と h264NALIDR は、H.264 の nal_unit_type。IDR でないスライスと、IDR のスライスを表す。
	h264NALSlice = 1
	h264NALIDR   = 5
	// h264SliceTypeI は、slice_type を 5 で割った余りが I スライスであることを表す値。
	h264SliceTypeI = 2

	// hevcNALBLAWLP から hevcNALCRA までは、HEVC の IRAP (BLA、IDR、CRA) のピクチャの nal_unit_type。
	hevcNALBLAWLP = 16
	hevcNALCRA    = 21
)

// videoCodec は、I フレームの探し方を決める映像の符号化方式。
type videoCodec int

const (
	codecMPEG2 videoCodec = iota
	codecH264
	codecHEVC
)

// scanState は、start code を探す状態機械の状態。
type scanState int

const (
	// scanSearching は、00 00 01 を探している状態。
	scanSearching scanState = iota
	// scanStartCode は、00 00 01 を読み、start code の値 (H.264 と HEVC では NAL ユニットのヘッダー) を待っている状態。
	scanStartCode
	// scanTemporalReference は、picture_start_code を読み、temporal_reference の上位 8 ビットを待っている状態。
	scanTemporalReference
	// scanPictureType は、temporal_reference の下位 2 ビットと picture_coding_type を含むバイトを待っている状態。
	scanPictureType
	// scanSliceHeader は、H.264 の IDR でないスライスのヘッダーの先頭 2 バイトを待っている状態。
	scanSliceHeader
)

// iframeDetector は、映像の PES から I フレームを探す。
//
// start code はパケットの境界をまたぐことがあるので、バイトごとに進む状態機械で探す。
// MPEG-2 (MPEG-1 を含む) では I ピクチャ、H.264 では IDR か I スライス、HEVC では IRAP のピクチャを I フレームとみなす。
type iframeDetector struct {
	pid        uint16
	streamType uint8
	codec      videoCodec
	// pesOffset は、今の PES の先頭パケットの位置。まだ PES の始まりを見ていなければ -1。
	pesOffset int64
	pesPTS    int64
	// found は、今の PES で I フレームを見つけたかどうか。1 つの PES につき 1 件だけ索引に入れる。
	found bool

	state scanState
	// zeros は、直前に続いた 0x00 の数。
	zeros int
	// sliceHeader と sliceHeaderLen は、scanSliceHeader で読んだスライスのヘッダーのバイトと、その数。
	sliceHeader    uint16
	sliceHeaderLen int
}

// newIFrameDetector は、stream_type の映像から I フレームを探す iframeDetector を作る。
// I フレームの探し方を知らない stream_type なら false を返す。
func newIFrameDetector(pid uint16, streamType uint8) (*iframeDetector, bool) {
	var codec videoCodec
	switch streamType {
	case streamTypeMPEG1Video, streamTypeMPEG2Video:
		codec = codecMPEG2
	case streamTypeH264:
		codec = codecH264
	case streamTypeHEVC:
		codec = codecHEVC
	default:
		return nil, false
	}

	return &iframeDetector{
		pid: pid, streamType: streamType, codec: codec, pesOffset: -1, pesPTS: noPTS, found: false,
		state: scanSearching, zeros: 0, sliceHeader: 0, sliceHeaderLen: 0,
	}, true
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
	d.sliceHeader = 0
	d.sliceHeaderLen = 0
}

// feed は、映像の PID のパケットを受け取る。I フレームを見つけたら、その PES の索引を返す。
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

// step は 1 バイト進め、I フレームだとわかったら true を返す。
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
		return d.startCode(b)
	case scanTemporalReference:
		d.state = scanPictureType
	case scanPictureType:
		d.resetScan()

		return b>>3&0x07 == pictureTypeI
	case scanSliceHeader:
		d.sliceHeader = d.sliceHeader<<8 | uint16(b)
		d.sliceHeaderLen++
		if d.sliceHeaderLen < 2 {
			return false
		}
		header := d.sliceHeader
		d.resetScan()

		return isH264ISliceHeader(header)
	}

	return false
}

// startCode は、00 00 01 に続くバイトを読む。
func (d *iframeDetector) startCode(b byte) bool {
	switch d.codec {
	case codecMPEG2:
		if b == pictureStartCode {
			d.state = scanTemporalReference

			return false
		}
	case codecH264:
		switch b & 0x1F {
		case h264NALIDR:
			d.resetScan()

			return true
		case h264NALSlice:
			d.state = scanSliceHeader

			return false
		}
	case codecHEVC:
		// HEVC の NAL ユニットのヘッダーは 2 バイトで、nal_unit_type は 1 バイト目の 2 ビット目から 6 ビット。
		nalType := b >> 1 & 0x3F
		d.resetScan()

		return nalType >= hevcNALBLAWLP && nalType <= hevcNALCRA
	}
	d.resetScan()

	return false
}

// isH264ISliceHeader は、H.264 のスライスのヘッダーの先頭 2 バイトから、ピクチャの最初の I スライスかどうかを返す。
//
// first_mb_in_slice と slice_type は指数ゴロム符号 (ue(v)) で並ぶ。
// first_mb_in_slice が 0 なら 1 ビット、slice_type は 9 以下なので 7 ビット以下に収まり、先頭 2 バイトで読める。
func isH264ISliceHeader(header uint16) bool {
	// first_mb_in_slice が 0 (ビット列 "1") でなければ、ピクチャの途中のスライス。
	if header>>15 != 1 {
		return false
	}
	rest := header << 1
	leadingZeros := 0
	for leadingZeros < 15 && rest&0x8000 == 0 {
		leadingZeros++
		rest <<= 1
	}
	// 先頭の 1 を含め 2*leadingZeros+1 ビットが、残りの 15 ビットに収まらなければ読めない。
	if 2*leadingZeros+1 > 15 {
		return false
	}
	sliceType := int(rest>>(15-leadingZeros)) - 1

	return sliceType%5 == h264SliceTypeI
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
