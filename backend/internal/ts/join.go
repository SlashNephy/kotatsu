package ts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// DefaultJoinWindow は、セグメントの重なりを探す範囲 (バイト) の既定値。
	// 引き継ぎで重なるのは数秒なので、20 Mbps で約 25 秒分あれば足りる。
	DefaultJoinWindow = 64 << 20
	// joinProbePackets は、前のセグメントの末尾から取り出して、次のセグメントの中で探すパケットの数。
	joinProbePackets = 64
	// joinChunkPackets は、つなぐときに 1 回で読み書きするパケットの数。
	joinChunkPackets = 2048
)

// ErrNoOverlap は、次のセグメントの先頭に、前のセグメントの末尾と同じパケットの並びが見つからないことを表す。
// 重なりのパケットが、PAT の continuity_counter のほかに一つでも違えば、これを返す。
var ErrNoOverlap = errors.New("the segments do not overlap packet by packet")

// Joiner は、録画のセグメントを、1 つのファイルにつなぐ (設計書 5.4)。
//
// 引き継ぎで新旧の mikan が重なって受け取ったパケットは、チューナーサーバーが同じチャンネルを相乗りさせるので、
// クライアントごとに作り直される PAT の continuity_counter のほかは、同じものが同じ順に並ぶ。
// そこで、前のセグメントの末尾のパケットの並びを次のセグメントの先頭から探し、重なりを除いてつなぐ。
// PAT の continuity_counter は、前のセグメントの続きになるようにずらす。
// こうしてつないだファイルは、1 つのストリームを録り続けたものとバイト単位で同じになる。
type Joiner struct {
	out *os.File
	// written は、out に書いたバイト数。いつもパケットの大きさの倍数にする。
	written int64
	// lastPATCC は、最後に書いた PAT の continuity_counter。hasPAT が false なら、まだ書いていない。
	lastPATCC byte
	hasPAT    bool
	window    int
}

// NewJoiner は、out の先頭からつないだものを書く Joiner を返す。window は重なりを探す範囲 (バイト) で、0 なら既定値にする。
func NewJoiner(out *os.File, window int) *Joiner {
	if window <= 0 {
		window = DefaultJoinWindow
	}

	return &Joiner{out: out, written: 0, lastPATCC: 0, hasPAT: false, window: window}
}

// Written は、書いたバイト数を返す。
func (j *Joiner) Written() int64 {
	return j.written
}

// Append は、r のパケットを、そのまま後ろにつなぐ。最初のセグメントと、前のセグメントと時間が重ならないセグメントに使う。
// 末尾の、パケットの大きさに満たない端は捨てる。
func (j *Joiner) Append(r io.Reader) error {
	return j.copyPackets(r, nil)
}

// AppendOverlapping は、前のセグメントと時間が重なるセグメント r を、重なりを除いてつなぐ。
// 重なりが見つからないか、重なりのパケットが違えば ErrNoOverlap を返す。そのときは何も書かない。
func (j *Joiner) AppendOverlapping(r io.Reader) error {
	tail, err := j.tail()
	if err != nil {
		return err
	}
	head := make([]byte, j.window)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read the next segment: %w", err)
	}
	head = head[:n]
	start := syncOffset(head)
	if start < 0 {
		return fmt.Errorf("%w: the next segment has no TS packets", ErrNoOverlap)
	}
	head = head[start:]

	// 重なりは、head のうち、パケットとしてそろった部分で探す。
	// 端の、パケットに満たない分は捨てずに、続きの読み込みにつなぐ。捨てると、それより後ろのパケットの位置がずれる。
	overlap, ok := findOverlap(tail, head[:len(head)/PacketSize*PacketSize])
	if !ok {
		return ErrNoOverlap
	}

	return j.copyPackets(io.MultiReader(bytes.NewReader(head[overlap:]), r), new(patShift))
}

// tail は、out に書いた末尾の、重なりを探す範囲を読む。
func (j *Joiner) tail() ([]byte, error) {
	size := min(j.written, int64(j.window)/PacketSize*PacketSize)
	tail := make([]byte, size)
	if _, err := j.out.ReadAt(tail, j.written-size); err != nil {
		return nil, fmt.Errorf("failed to read the joined file: %w", err)
	}

	return tail, nil
}

// findOverlap は、tail の末尾と同じパケットの並びで head が始まるとき、重なりのバイト数を返す。
// PAT の continuity_counter は、クライアントごとに違うので比べない。
func findOverlap(tail, head []byte) (int, bool) {
	probeSize := joinProbePackets * PacketSize
	if len(tail) < probeSize || len(head) < probeSize {
		return 0, false
	}
	normalizedTail := normalizePAT(tail)
	normalizedHead := normalizePAT(head)
	probe := normalizedTail[len(normalizedTail)-probeSize:]

	for from := 0; ; {
		i := bytes.Index(normalizedHead[from:], probe)
		if i < 0 {
			return 0, false
		}
		at := from + i
		from = at + 1
		if at%PacketSize != 0 {
			continue
		}
		// head の先頭から probe の終わりまでが重なり。重なり全体が、tail の末尾と同じでなければならない。
		overlap := at + probeSize
		if overlap > len(normalizedTail) {
			return 0, false
		}
		if bytes.Equal(normalizedTail[len(normalizedTail)-overlap:], normalizedHead[:overlap]) {
			return overlap, true
		}
	}
}

// normalizePAT は、PAT のパケットの continuity_counter を 0 にした複製を返す。b はパケットの並びでなければならない。
func normalizePAT(b []byte) []byte {
	normalized := bytes.Clone(b)
	for i := 0; i+PacketSize <= len(normalized); i += PacketSize {
		p := normalized[i : i+PacketSize]
		if p[0] == syncByte && pidOf(p) == patPID {
			p[3] &^= 0x0F
		}
	}

	return normalized
}

// patShift は、つなぐセグメントの PAT の continuity_counter をずらす量。最初に出会った PAT で決める。
type patShift struct {
	delta   byte
	decided bool
}

// copyPackets は、r のパケットを out の後ろに書く。shift が nil でなければ、PAT の continuity_counter を、
// 前に書いた PAT の続きになるようにずらす。
func (j *Joiner) copyPackets(r io.Reader, shift *patShift) error {
	buf := make([]byte, joinChunkPackets*PacketSize)
	for {
		n, err := io.ReadFull(r, buf)
		n = n / PacketSize * PacketSize
		if n > 0 {
			chunk := buf[:n]
			j.adjustPAT(chunk, shift)
			if _, err := j.out.WriteAt(chunk, j.written); err != nil {
				return fmt.Errorf("failed to write the joined file: %w", err)
			}
			j.written += int64(n)
		}
		switch {
		case err == nil:
			continue
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return nil
		default:
			return fmt.Errorf("failed to read the segment: %w", err)
		}
	}
}

// adjustPAT は、chunk の PAT の continuity_counter をずらし、最後の値を覚える。continuity_counter は 4 ビットなので、下位 4 ビットで数える。
func (j *Joiner) adjustPAT(chunk []byte, shift *patShift) {
	for i := 0; i+PacketSize <= len(chunk); i += PacketSize {
		p := chunk[i : i+PacketSize]
		if p[0] != syncByte || pidOf(p) != patPID {
			continue
		}
		cc := p[3] & 0x0F
		if shift != nil {
			if !shift.decided {
				shift.decided = true
				if j.hasPAT {
					shift.delta = (j.lastPATCC + 1 - cc) & 0x0F
				}
			}
			cc = (cc + shift.delta) & 0x0F
			p[3] = p[3]&0xF0 | cc
		}
		j.lastPATCC, j.hasPAT = cc, true
	}
}

// syncOffset は、b の中で、パケットの並びが始まる位置を返す。見つからなければ -1。
func syncOffset(b []byte) int {
	for offset := range min(PacketSize, len(b)) {
		ok := true
		for i := offset; i < len(b) && i < offset+5*PacketSize; i += PacketSize {
			if b[i] != syncByte {
				ok = false

				break
			}
		}
		if ok {
			return offset
		}
	}

	return -1
}

// pidOf は、パケットの PID を返す。
func pidOf(p []byte) uint16 {
	return uint16(p[1]&0x1F)<<8 | uint16(p[2])
}
