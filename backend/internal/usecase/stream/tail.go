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

	// open は、PID ごとの、終わりまでに全部が届いていない PES の始まりと、残りのバイト数。
	type pending struct {
		offset int64
		left   int
	}
	open := map[uint16]*pending{}
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
		if packet[1]&0x40 != 0 {
			// PES の始まり。長さを持つ PES だけを追う。
			delete(open, pid)
			if len(payload) >= 6 && payload[0] == 0 && payload[1] == 0 && payload[2] == 1 {
				if length := int(binary.BigEndian.Uint16(payload[4:6])); length > 0 {
					open[pid] = &pending{offset: start + int64(i), left: length - (len(payload) - 6)}
				}
			}
		} else if p, ok := open[pid]; ok {
			p.left -= len(payload)
		}
		if p, ok := open[pid]; ok && p.left <= 0 {
			delete(open, pid)
		}
	}
	end := to
	for _, p := range open {
		end = min(end, p.offset)
	}

	return end, nil
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
