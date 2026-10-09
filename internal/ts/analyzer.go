package ts

import (
	"io"
	"maps"
	"slices"
	"sync"
)

// Analyzer は、書き込まれる TS を流し読みし、PID ごとの統計を数える。
//
// 録画ファイルへの書き込みと並べて io.MultiWriter で使う。パケットの途中で区切られた書き込みも受け付ける。
// Write は 1 つの goroutine から呼ぶ。Stats は、Write と並行に別の goroutine から呼んでよい。
type Analyzer struct {
	mu sync.Mutex
	// offset は、これまでに受け取ったバイト数。
	offset int64
	// carry は、前の書き込みの終わりで途切れたパケットの前半。
	carry    [PacketSize]byte
	carryLen int
	// synced は、パケットの境界に合っているかどうか。sync byte が見つからなければ false にし、次の sync byte を探す。
	synced     bool
	syncLosses int64
	packets    int64
	pids       map[uint16]*pidState
}

var _ io.Writer = new(Analyzer)

// pidState は、PID ごとの状態と統計。
type pidState struct {
	stats PIDStats
	// lastCC は、最後にペイロードを持っていたパケットの continuity_counter。まだなければ -1。
	lastCC int
	// duplicated は、直前のパケットが重複 (同じ continuity_counter の再送) だったかどうか。重複は 1 回だけ許される。
	duplicated bool
}

// PIDStats は、PID ごとの統計。
type PIDStats struct {
	PID     uint16
	Packets int64
	// Drops は、continuity_counter が不連続になった回数。抜けたパケットの数ではない。
	// 連続した 10 パケットが抜けても 1 回と数える。
	Drops int64
	// Errors は、transport_error_indicator が立っていたパケットの数。
	Errors int64
	// Scrambled は、transport_scrambling_control が 0 以外だったパケットの数。
	Scrambled int64
}

// Stats は、解析した TS 全体の統計。
type Stats struct {
	// Bytes は、受け取ったバイト数。
	Bytes   int64
	Packets int64
	// SyncLosses は、パケットの先頭に sync byte がなく、次の sync byte を探し直した回数。
	SyncLosses int64
	// PIDs は、PID の昇順に並べた統計。null パケットの PID は含めない。
	PIDs []*PIDStats
}

// NewAnalyzer は Analyzer を作る。
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		mu:         sync.Mutex{},
		offset:     0,
		carry:      [PacketSize]byte{},
		carryLen:   0,
		synced:     true,
		syncLosses: 0,
		packets:    0,
		pids:       map[uint16]*pidState{},
	}
}

// Write は p を解析する。エラーは返さない。
func (a *Analyzer) Write(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	n := len(p)
	// 前の書き込みで途切れたパケットを先に埋める。
	if a.carryLen > 0 {
		copied := copy(a.carry[a.carryLen:], p)
		a.carryLen += copied
		a.offset += int64(copied)
		p = p[copied:]
		if a.carryLen < PacketSize {
			return n, nil
		}
		a.handle(a.carry[:])
		a.carryLen = 0
	}

	for len(p) > 0 {
		if p[0] != syncByte {
			if a.synced {
				a.synced = false
				a.syncLosses++
			}
			p = p[1:]
			a.offset++

			continue
		}
		a.synced = true
		if len(p) < PacketSize {
			a.carryLen = copy(a.carry[:], p)
			a.offset += int64(len(p))

			break
		}
		a.handle(p[:PacketSize])
		p = p[PacketSize:]
		a.offset += PacketSize
	}

	return n, nil
}

// Stats は、ここまでの統計を返す。
func (a *Analyzer) Stats() *Stats {
	a.mu.Lock()
	defer a.mu.Unlock()

	pids := make([]*PIDStats, 0, len(a.pids))
	for _, pid := range slices.Sorted(maps.Keys(a.pids)) {
		copied := a.pids[pid].stats
		pids = append(pids, &copied)
	}

	return &Stats{
		Bytes:      a.offset,
		Packets:    a.packets,
		SyncLosses: a.syncLosses,
		PIDs:       pids,
	}
}

// handle は 1 パケットを処理する。
func (a *Analyzer) handle(b []byte) {
	a.packets++
	pkt := parsePacket(b)
	if pkt.pid == NullPID {
		return
	}

	state := a.pids[pkt.pid]
	if state == nil {
		state = &pidState{stats: PIDStats{PID: pkt.pid, Packets: 0, Drops: 0, Errors: 0, Scrambled: 0}, lastCC: -1, duplicated: false}
		a.pids[pkt.pid] = state
	}
	state.stats.Packets++
	if pkt.transportError {
		// 誤りのあるパケットは、ヘッダーも信用できないので、continuity_counter も見ない。
		// 次のパケットを基準に数え直し、1 つの誤りをドロップとしても数えないようにする。
		state.stats.Errors++
		state.lastCC = -1

		return
	}
	if pkt.scrambling != 0 {
		state.stats.Scrambled++
	}
	state.checkContinuity(&pkt)
}

// checkContinuity は、continuity_counter の不連続を数える。
//
//   - ペイロードを持たないパケットでは continuity_counter は増えないので、見ない
//   - discontinuity_indicator が立っていれば、不連続は意図的なので数えない
//   - 同じ値の再送 (重複) は 1 回だけ許す
func (s *pidState) checkContinuity(pkt *packet) {
	if !pkt.hasPayload {
		return
	}
	cc := int(pkt.continuityCounter)
	defer func() { s.lastCC = cc }()

	if s.lastCC < 0 || pkt.discontinuity {
		s.duplicated = false

		return
	}
	if cc == s.lastCC && !s.duplicated {
		s.duplicated = true

		return
	}
	s.duplicated = false
	if cc != (s.lastCC+1)&0x0F {
		s.stats.Drops++
	}
}
