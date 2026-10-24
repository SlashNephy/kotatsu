package stream

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// patPID は、PAT を運ぶパケットの PID。
	patPID = 0x0000
	// psiWindow は、途中から流すときに、PAT と PMT を探す範囲。開始位置の手前をこれだけ読む。
	// 放送の PAT と PMT は 100 ミリ秒ごとに送られるので、24 Mbps でも 300 KB ほどで見つかる。
	psiWindow = 4096 * ts.PacketSize
)

// psiPrelude は、ファイルの offset より手前で最後に送られた PAT と、その PAT が指す PMT のパケットを返す。
//
// TS を途中から流すと、プレイヤーは PAT と PMT を受け取るまで、映像のパケットを捨てる。
// 開始位置の I フレームの手前に PAT と PMT を足せば、最初の GOP から再生できる。
// 1 パケットに収まらない PAT と PMT は足さない。見つからなければ、何も足さない。
func psiPrelude(r io.ReaderAt, offset int64) ([]byte, error) {
	from := max(offset-psiWindow, 0)
	// offset は、索引が指すパケットの境目なので、そこからパケットの大きさずつ戻れば、パケットの境目にそろう。
	from = offset - (offset-from)/ts.PacketSize*ts.PacketSize
	window := make([]byte, offset-from)
	if _, err := r.ReadAt(window, from); err != nil {
		return nil, fmt.Errorf("failed to read the segment before the start: %w", err)
	}

	latest := map[uint16][]byte{}
	for i := 0; i+ts.PacketSize <= len(window); i += ts.PacketSize {
		packet := window[i : i+ts.PacketSize]
		if packet[0] != 0x47 || packet[1]&0x40 == 0 {
			// 同期していないパケットと、セクションの続きのパケットは使わない。
			continue
		}
		latest[pidOf(packet)] = packet
	}
	pat, ok := latest[patPID]
	if !ok {
		return []byte{}, nil
	}
	prelude := append([]byte{}, pat...)
	for _, pmtPID := range pmtPIDs(pat) {
		if pmt, ok := latest[pmtPID]; ok {
			prelude = append(prelude, pmt...)
		}
	}

	return prelude, nil
}

// pidOf は、パケットの PID を返す。
func pidOf(packet []byte) uint16 {
	return binary.BigEndian.Uint16(packet[1:3]) & 0x1FFF
}

// pmtPIDs は、1 パケットに収まった PAT が指す PMT の PID を返す。NIT を指す番組番号 0 は含めない。
func pmtPIDs(packet []byte) []uint16 {
	pids := []uint16{}
	payload := packet[4:]
	if packet[3]&0x20 != 0 {
		// アダプテーションフィールドを飛ばす。
		if len(payload) == 0 || int(payload[0])+1 > len(payload) {
			return pids
		}
		payload = payload[int(payload[0])+1:]
	}
	if len(payload) == 0 || int(payload[0])+1 > len(payload) {
		return pids
	}
	section := payload[int(payload[0])+1:]
	if len(section) < 8 || section[0] != 0x00 {
		return pids
	}
	length := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	// 番組の並びは、8 バイトの見出しの後から、末尾の CRC の手前まで。
	end := 3 + length - 4
	if end > len(section) {
		return pids
	}
	for i := 8; i+4 <= end; i += 4 {
		if binary.BigEndian.Uint16(section[i:i+2]) == 0 {
			continue
		}
		pids = append(pids, binary.BigEndian.Uint16(section[i+2:i+4])&0x1FFF)
	}

	return pids
}
