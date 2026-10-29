package stream

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// tailWindow は、セグメントの終わりで途中で切れた PES を探す範囲。終わりの手前をこれだけ読む。
	// 音声と字幕の PES は数 KB なので、PES の始まりは、この範囲に必ず入る。
	tailWindow = 2048 * ts.PacketSize
	// headWindow は、セグメントの頭で、前から続く PES の残りのパケットを落とす範囲。頭からこれだけ読む。
	headWindow = 2048 * ts.PacketSize
)

// completeTail は、ファイルの [from, to) を流すとき、終わりで途中で切れた PES を流さないよう、終わりの位置を前に動かして返す。
//
// mikan が落ちて書きかけのまま閉じたセグメントや、引き継ぎで切り詰めたセグメントは、PES の途中で終わる。続きのないセグメントを
// 次のセグメントにつなぐと、プレイヤーは途中で切れた PES を 1 つのフレームとして復号に渡し、ブラウザは音声の復号に失敗して
// 再生を止める (#1131)。長さを持つ PES (音声や字幕) のうち、終わりまでに全部が届かないものの始まりで切る。
// 映像の PES は長さを持たない (0) ので、切れているか分からず、切る位置には使わない。
func completeTail(r io.ReaderAt, from, to int64) (int64, error) {
	// 書きかけのまま閉じたセグメントは、パケットの途中で終わることがある。パケットの境目にそろえる。
	to -= (to - from) % ts.PacketSize
	start := max(to-tailWindow, from)
	// from はパケットの境目なので、そこからパケットの大きさずつ進めた位置にそろえる。
	start = from + (start-from+ts.PacketSize-1)/ts.PacketSize*ts.PacketSize
	if start >= to {
		return to, nil
	}
	window := make([]byte, to-start)
	if _, err := r.ReadAt(window, start); err != nil && err != io.EOF {
		return to, fmt.Errorf("failed to read the end of the segment: %w", err)
	}

	spans := pesSpans(window, start)
	// 終わりまでに全部が届かない PES の始まりで切る。PID の違う PES のパケットは交互に並ぶので、切る位置より前に始まり、
	// 後ろまで続く PES があれば、それも途中で切れる。その始まりまで、切る位置を前に動かす。
	end := to
	for _, sp := range spans {
		if !sp.complete {
			end = min(end, sp.first)
		}
	}
	for moved := true; moved; {
		moved = false
		for _, sp := range spans {
			if sp.first < end && sp.last >= end {
				end, moved = sp.first, true
			}
		}
	}

	return end, nil
}

// pesSpan は、長さを持つ PES が占める範囲。last は、最後のパケットの始まり。終わりまでに全部が届いていなければ、complete は false。
type pesSpan struct {
	first, last int64
	left        int
	complete    bool
}

// pesSpans は、window (ファイルの start から読んだもの) の中で始まる、長さを持つ PES の範囲を、始まりの順に返す。
func pesSpans(window []byte, start int64) []*pesSpan {
	spans := []*pesSpan{}
	open := map[uint16]*pesSpan{}
	for i := 0; i+ts.PacketSize <= len(window); i += ts.PacketSize {
		packet := window[i : i+ts.PacketSize]
		if packet[0] != 0x47 {
			continue
		}
		payload, ok := payloadOf(packet)
		if !ok {
			continue
		}
		pid := pidOf(packet)
		offset := start + int64(i)
		if packet[1]&0x40 != 0 {
			// PES の始まり。長さを持つ PES だけを追う。
			delete(open, pid)
			if length, ok := pesLength(payload); ok {
				sp := &pesSpan{first: offset, last: offset, left: length - (len(payload) - 6), complete: false}
				spans = append(spans, sp)
				open[pid] = sp
			}
		} else if sp, ok := open[pid]; ok {
			sp.left -= len(payload)
			sp.last = offset
		}
		if sp, ok := open[pid]; ok && sp.left <= 0 {
			sp.complete = true
			delete(open, pid)
		}
	}

	return spans
}

// pesLength は、PES の始まりのペイロードから、PES の長さを返す。長さを持たない (0) か、PES でなければ false を返す。
func pesLength(payload []byte) (int, bool) {
	if len(payload) < 6 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
		return 0, false
	}
	length := int(binary.BigEndian.Uint16(payload[4:6]))

	return length, length > 0
}

// payloadOf は、パケットのペイロードを返す。ペイロードがなければ false を返す。
func payloadOf(packet []byte) ([]byte, bool) {
	control := packet[3] >> 4 & 0x3
	offset := 4
	if control&0x2 != 0 {
		offset += 1 + int(packet[4])
	}
	if control&0x1 == 0 || offset >= len(packet) {
		return nil, false
	}

	return packet[offset:], true
}

// completeHead は、ファイルを from から end まで流すとき、頭の headWindow の範囲で、PES の始まりより前にある、前から続く PES の
// 残りのパケットを落として返す。consumed は、読んだバイト数で、続きはファイルの from + consumed から流す。
//
// 続きのないセグメントを、I フレームの位置から前のセグメントにつなぐと、その I フレームより前に始まった音声の PES の残りが頭に入る。
// プレイヤーは、それを前のセグメントの最後の PES に足して、長さの合わないフレームを復号に渡す (#1131)。
// PID ごとに、PES の始まり (payload_unit_start_indicator) を見るまでのパケットを落とす。PAT と PMT のような PSI も、
// セクションの途中なら同じく落とす。
func completeHead(r io.ReaderAt, from, end int64) ([]byte, int64, error) {
	size := min(int64(headWindow), end-from)
	size -= size % ts.PacketSize
	if size <= 0 {
		return []byte{}, 0, nil
	}
	window := make([]byte, size)
	if _, err := r.ReadAt(window, from); err != nil && err != io.EOF {
		return nil, 0, fmt.Errorf("failed to read the start of the segment: %w", err)
	}
	started := map[uint16]bool{}
	kept := make([]byte, 0, len(window))
	for i := 0; i+ts.PacketSize <= len(window); i += ts.PacketSize {
		packet := window[i : i+ts.PacketSize]
		if packet[0] != 0x47 {
			continue
		}
		pid := pidOf(packet)
		if packet[1]&0x40 != 0 {
			started[pid] = true
		}
		if started[pid] {
			kept = append(kept, packet...)
		}
	}

	return kept, size, nil
}
