package ts

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
)

// maxProbeBytes は、映像の形式を読むのに溜めるエレメンタリーストリームの長さの上限。
// 最初の I フレームの前には、シーケンスヘッダーか SPS が来るので、これだけあれば足りる。
const maxProbeBytes = 512 << 10

// VideoFormat は、映像のシーケンスヘッダーか SPS から読んだ映像の形式 (#1310)。
type VideoFormat struct {
	// Codec は、mpeg2、h264、hevc のどれか。
	Codec string `json:"codec"`
	// Width と Height は、表示する大きさ (クロップを除いた、符号化した画素の数)。
	Width  int `json:"width"`
	Height int `json:"height"`
	// Interlaced は、インターレースかどうか。
	Interlaced bool `json:"interlaced"`
	// FrameRate は、フレームレート (1 秒あたりのフレーム数)。分からなければ 0。
	FrameRate float64 `json:"frameRate"`
}

// Resolution は、映像の解像度を、EPG と同じく「1080i」や「2160p」のように書く。
func (f *VideoFormat) Resolution() string {
	scan := "p"
	if f.Interlaced {
		scan = "i"
	}

	return fmt.Sprintf("%d%s", f.Height, scan)
}

var errBitstreamEnd = errors.New("unexpected end of the bitstream")

// maxPOCCycle は、H.264 の num_ref_frames_in_pic_order_cnt_cycle の上限。
const maxPOCCycle = 255

// videoFormatProbe は、映像のエレメンタリーストリームの先頭を溜め、映像の形式を読む。
type videoFormatProbe struct {
	codec  videoCodec
	buf    []byte
	format *VideoFormat
	done   bool
}

func newVideoFormatProbe(codec videoCodec) *videoFormatProbe {
	return &videoFormatProbe{codec: codec, buf: make([]byte, 0, 64<<10), format: nil, done: false}
}

// feed は、映像のパケットのペイロードを足す。PES の始まりなら、ヘッダーを除く。
// 形式を読めるか、上限まで溜めたら、それ以上は溜めない。
func (p *videoFormatProbe) feed(pkt *packet) {
	if p.done {
		return
	}
	payload := pkt.payload
	if pkt.payloadUnitStart {
		es, _, ok := parsePESHeader(payload)
		if !ok {
			return
		}
		payload = es
	} else if len(p.buf) == 0 {
		// PES の途中からは読まない。
		return
	}
	// 前に溜めた末尾の 2 バイトと合わせて、新しい start code が届いたときだけ読み直す。
	// パケットのたびに溜めた全体を読み直すと、映像の先頭を読むのに時間がかかる。
	tail := p.buf[max(len(p.buf)-2, 0):]
	p.buf = append(p.buf, payload...)
	if !bytes.Contains(append(slices.Clone(tail), payload...), []byte{0, 0, 1}) {
		if len(p.buf) >= maxProbeBytes {
			p.done, p.buf = true, nil
		}

		return
	}
	if format, ok := parseVideoFormat(p.codec, p.buf); ok {
		p.format, p.done = format, true
		p.buf = nil

		return
	}
	if len(p.buf) >= maxProbeBytes {
		p.done = true
		p.buf = nil
	}
}

// parseVideoFormat は、エレメンタリーストリーム es から、符号化方式 codec の映像の形式を読む。
// シーケンスヘッダーや SPS が、まだ揃っていなければ false を返す。
func parseVideoFormat(codec videoCodec, es []byte) (*VideoFormat, bool) {
	switch codec {
	case codecMPEG2:
		return parseMPEG2Format(es)
	case codecH264:
		return parseNALFormat(es, func(header byte) bool { return header&0x1F == 7 }, 1, parseH264SPS)
	case codecHEVC:
		return parseNALFormat(es, func(header byte) bool { return header>>1&0x3F == 33 }, 2, parseHEVCSPS)
	default:
		return nil, false
	}
}

// startCodes は、es の中の 00 00 01 の続きの位置 (start code の次のバイト) を順に返す。
func startCodes(es []byte) []int {
	positions := []int{}
	for i := 0; i+3 <= len(es); {
		j := bytes.Index(es[i:], []byte{0, 0, 1})
		if j < 0 {
			break
		}
		positions = append(positions, i+j+3)
		i += j + 3
	}

	return positions
}

// mpeg2FrameRate は、MPEG-2 の frame_rate_code のフレームレートを返す (ISO/IEC 13818-2 6.3.3)。知らない値なら 0 を返す。
func mpeg2FrameRate(code byte) float64 {
	rates := [...]float64{0, 24000.0 / 1001, 24, 25, 30000.0 / 1001, 30, 50, 60000.0 / 1001, 60}
	if int(code) >= len(rates) {
		return 0
	}

	return rates[code]
}

// parseMPEG2Format は、MPEG-2 の sequence_header (00 00 01 B3) と sequence_extension (00 00 01 B5、拡張の種類 1) を読む。
// sequence_extension の progressive_sequence が 0 なら、インターレースとする。
func parseMPEG2Format(es []byte) (*VideoFormat, bool) {
	var format *VideoFormat
	for _, pos := range startCodes(es) {
		if pos >= len(es) {
			break
		}
		body := es[pos+1:]
		switch es[pos] {
		case 0xB3:
			if len(body) < 4 {
				return nil, false
			}
			width := int(body[0])<<4 | int(body[1])>>4
			height := int(body[1]&0x0F)<<8 | int(body[2])
			format = &VideoFormat{Codec: "mpeg2", Width: width, Height: height, Interlaced: false, FrameRate: mpeg2FrameRate(body[3] & 0x0F)}
		case 0xB5:
			if format == nil || len(body) < 2 || body[0]>>4 != 1 {
				continue
			}
			// sequence_extension: extension_start_code_identifier (4)、profile_and_level (8)、progressive_sequence (1)
			format.Interlaced = body[1]&0x08 == 0

			return format, true
		}
	}

	return nil, false
}

// parseNALFormat は、NAL ユニットのうち isSPS に当てはまるものを、headerLen バイトのヘッダーを除いて parse で読む。
// SPS が start code の後で途切れていれば、まだ揃っていないとして false を返す。
func parseNALFormat(es []byte, isSPS func(byte) bool, headerLen int, parse func([]byte) (*VideoFormat, error)) (*VideoFormat, bool) {
	positions := startCodes(es)
	for i, pos := range positions {
		if pos >= len(es) || !isSPS(es[pos]) {
			continue
		}
		// 次の start code までを SPS とする。次がなければ、まだ届いていない。
		if i+1 >= len(positions) {
			return nil, false
		}
		end := positions[i+1] - 3
		if end <= pos+headerLen {
			continue
		}
		format, err := parse(removeEmulationPrevention(es[pos+headerLen : end]))
		if err != nil {
			continue
		}

		return format, true
	}

	return nil, false
}

// removeEmulationPrevention は、NAL ユニットの 00 00 03 の 03 (emulation_prevention_three_byte) を除く。
func removeEmulationPrevention(nal []byte) []byte {
	out := make([]byte, 0, len(nal))
	zeros := 0
	for _, b := range nal {
		if zeros >= 2 && b == 0x03 {
			zeros = 0

			continue
		}
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out = append(out, b)
	}

	return out
}

// bitReader は、ビット列を先頭から読む。読み終えた後に読もうとしたら、err に残し、それからは 0 を返す。
// 構文の途中で一つずつエラーを見ずに、読み終えた後に err を見ればよい。
type bitReader struct {
	data []byte
	pos  int
	err  error
}

func (r *bitReader) bit() uint {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.data)*8 {
		r.err = errBitstreamEnd

		return 0
	}
	b := r.data[r.pos/8] >> (7 - r.pos%8) & 1
	r.pos++

	return uint(b)
}

func (r *bitReader) bits(n int) uint {
	var v uint
	for range n {
		v = v<<1 | r.bit()
	}

	return v
}

func (r *bitReader) skip(n int) {
	r.bits(n)
}

// ue は、符号なしの指数ゴロム符号を読む。
func (r *bitReader) ue() uint {
	zeros := 0
	for r.err == nil && r.bit() == 0 {
		zeros++
		if zeros > 31 {
			r.err = errBitstreamEnd

			return 0
		}
	}

	return 1<<zeros - 1 + r.bits(zeros)
}

// se は、符号ありの指数ゴロム符号を読む。
func (r *bitReader) se() int {
	v := r.ue()
	if v%2 == 1 {
		return int((v + 1) / 2)
	}

	return -int(v / 2)
}

// isH264HighProfile は、SPS に chroma_format_idc などを持つ profile_idc かを返す (ITU-T H.264 7.3.2.1.1)。
func isH264HighProfile(profile uint) bool {
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

// parseH264SPS は、H.264 の SPS (NAL ヘッダーを除いたもの) から、大きさとインターレースかどうかと、VUI のフレームレートを読む。
func parseH264SPS(sps []byte) (*VideoFormat, error) {
	r := &bitReader{data: sps, pos: 0, err: nil}
	profile := r.bits(8)
	r.skip(16) // constraint_set フラグ (8) と level_idc (8)
	r.ue()     // seq_parameter_set_id
	chromaFormat := uint(1)
	if isH264HighProfile(profile) {
		chromaFormat = readH264ChromaInfo(r)
	}
	r.ue() // log2_max_frame_num_minus4
	skipH264POC(r)
	r.ue()    // max_num_ref_frames
	r.skip(1) // gaps_in_frame_num_value_allowed_flag
	widthMBs, heightMapUnits := r.ue(), r.ue()
	frameMBsOnly := r.bit()
	if frameMBsOnly == 0 {
		r.skip(1) // mb_adaptive_frame_field_flag
	}
	r.skip(1) // direct_8x8_inference_flag
	width := int(widthMBs+1) * 16
	height := int(heightMapUnits+1) * 16 * int(2-frameMBsOnly)
	if r.bit() == 1 { // frame_cropping_flag
		left, right, top, bottom := r.ue(), r.ue(), r.ue(), r.ue()
		cropX, cropY := 2, 2*int(2-frameMBsOnly)
		if chromaFormat == 0 || chromaFormat == 3 {
			cropX, cropY = 1, int(2-frameMBsOnly)
		}
		width -= cropX * int(left+right)
		height -= cropY * int(top+bottom)
	}
	if r.err != nil {
		return nil, r.err
	}

	return &VideoFormat{Codec: "h264", Width: width, Height: height, Interlaced: frameMBsOnly == 0, FrameRate: h264VUIFrameRate(r)}, nil
}

// readH264ChromaInfo は、High 系の profile の SPS の、色の形式から scaling_matrix までを読み、chroma_format_idc を返す。
func readH264ChromaInfo(r *bitReader) uint {
	chromaFormat := r.ue()
	if chromaFormat == 3 {
		r.skip(1) // separate_colour_plane_flag
	}
	r.ue()            // bit_depth_luma_minus8
	r.ue()            // bit_depth_chroma_minus8
	r.skip(1)         // qpprime_y_zero_transform_bypass_flag
	if r.bit() == 1 { // seq_scaling_matrix_present_flag
		lists := 8
		if chromaFormat == 3 {
			lists = 12
		}
		for i := range lists {
			if r.bit() == 1 {
				size := 16
				if i >= 6 {
					size = 64
				}
				skipScalingList(r, size)
			}
		}
	}

	return chromaFormat
}

// skipH264POC は、H.264 の SPS の pic_order_cnt_type とその続きを読み飛ばす。
func skipH264POC(r *bitReader) {
	switch r.ue() {
	case 0:
		r.ue() // log2_max_pic_order_cnt_lsb_minus4
	case 1:
		r.skip(1) // delta_pic_order_always_zero_flag
		r.se()    // offset_for_non_ref_pic
		r.se()    // offset_for_top_to_bottom_field
		// num_ref_frames_in_pic_order_cnt_cycle は 0〜255 (ITU-T H.264 7.4.2.1.1)。壊れた値で、長く回さない。
		cycle := r.ue()
		if cycle > maxPOCCycle {
			r.err = errBitstreamEnd

			return
		}
		for range cycle {
			r.se()
		}
	}
}

// skipScalingList は、H.264 の scaling_list を読み飛ばす。
func skipScalingList(r *bitReader, size int) {
	last, next := 8, 8
	for range size {
		if next != 0 {
			next = (last + r.se() + 256) % 256
		}
		if next != 0 {
			last = next
		}
	}
}

// h264VUIFrameRate は、H.264 の VUI の timing_info からフレームレートを読む。読めなければ 0 を返す。
func h264VUIFrameRate(r *bitReader) float64 {
	if r.bit() == 0 { // vui_parameters_present_flag
		return 0
	}
	if r.bit() == 1 && r.bits(8) == 255 { // aspect_ratio_info_present_flag、aspect_ratio_idc が Extended_SAR
		r.skip(32)
	}
	if r.bit() == 1 { // overscan_info_present_flag
		r.skip(1)
	}
	if r.bit() == 1 { // video_signal_type_present_flag
		r.skip(4)
		if r.bit() == 1 { // colour_description_present_flag
			r.skip(24)
		}
	}
	if r.bit() == 1 { // chroma_loc_info_present_flag
		r.ue()
		r.ue()
	}
	if r.bit() == 0 { // timing_info_present_flag
		return 0
	}
	units, scale := r.bits(32), r.bits(32)
	if r.err != nil || units == 0 {
		return 0
	}

	return float64(scale) / float64(2*units)
}

// parseHEVCSPS は、HEVC の SPS (NAL ヘッダーを除いたもの) から、大きさを読む。
// HEVC の放送 (BS4K) はプログレッシブなので、インターレースは false とする。フレームレートは VUI の手前の構文が長いので読まない。
func parseHEVCSPS(sps []byte) (*VideoFormat, error) {
	r := &bitReader{data: sps, pos: 0, err: nil}
	r.skip(4) // sps_video_parameter_set_id
	maxSubLayers := int(r.bits(3))
	r.skip(1) // sps_temporal_id_nesting_flag
	skipHEVCProfileTierLevel(r, maxSubLayers)
	r.ue() // sps_seq_parameter_set_id
	chromaFormat := r.ue()
	if chromaFormat == 3 {
		r.skip(1) // separate_colour_plane_flag
	}
	width, height := int(r.ue()), int(r.ue())
	if r.bit() == 1 { // conformance_window_flag
		left, right, top, bottom := r.ue(), r.ue(), r.ue(), r.ue()
		subW, subH := 2, 2
		switch chromaFormat {
		case 0, 3:
			subW, subH = 1, 1
		case 2:
			subH = 1
		}
		width -= subW * int(left+right)
		height -= subH * int(top+bottom)
	}
	if r.err != nil {
		return nil, r.err
	}

	return &VideoFormat{Codec: "hevc", Width: width, Height: height, Interlaced: false, FrameRate: 0}, nil
}

// skipHEVCProfileTierLevel は、HEVC の profile_tier_level を読み飛ばす。
func skipHEVCProfileTierLevel(r *bitReader, maxSubLayers int) {
	// general の 88 ビットと general_level_idc の 8 ビット。
	r.skip(96)
	profilePresent := make([]uint, maxSubLayers)
	levelPresent := make([]uint, maxSubLayers)
	for i := range maxSubLayers {
		profilePresent[i], levelPresent[i] = r.bit(), r.bit()
	}
	if maxSubLayers > 0 {
		r.skip((8 - maxSubLayers) * 2)
	}
	for i := range maxSubLayers {
		if profilePresent[i] == 1 {
			r.skip(88)
		}
		if levelPresent[i] == 1 {
			r.skip(8)
		}
	}
}
