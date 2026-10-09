package ts

import (
	"io"
	"maps"
	"slices"
	"sync"
)

// Analyzer は、書き込まれる TS を流し読みし、PID ごとの統計と、PAT と PMT が表す番組の構成を記録する。
// 索引の書き込み先を渡せば、MPEG-2 映像の I フレームの位置と PTS を索引に追記していく。
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
	programs   *programTable
	// sections は、PAT と PMT の PID ごとの、組み立て中のセクション。
	sections map[uint16]*sectionAssembler

	index *IndexWriter
	// indexErr は、索引の書き込みで起きたエラー。起きたら、それ以降は索引を書かない。
	// 録画の書き込みを止めないよう、Write からは返さない。
	indexErr error
	iframes  int64
	// video は、I フレームを探す映像の PID の状態。MPEG-2 の映像がまだ見つかっていなければ nil。
	video *iframeDetector
	// programsGeneration は、video を選んだときの番組の構成の世代。
	programsGeneration int
}

// AnalyzerOptions は Analyzer の設定。
type AnalyzerOptions struct {
	// Index は索引の書き込み先。nil なら索引を作らない。
	Index *IndexWriter
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
	// Programs は、最後に読んだ PAT と PMT が表す番組の構成。program_number の昇順に並べる。
	Programs []*Program
	// IFrames は、見つけた I フレームの数。
	IFrames int64
	// IndexError は、索引の書き込みで起きたエラー。起きていなければ nil。
	IndexError error
}

// NewAnalyzer は Analyzer を作る。opts は nil でもよい。
func NewAnalyzer(opts *AnalyzerOptions) *Analyzer {
	var index *IndexWriter
	if opts != nil {
		index = opts.Index
	}

	return &Analyzer{
		mu:         sync.Mutex{},
		offset:     0,
		carry:      [PacketSize]byte{},
		carryLen:   0,
		synced:     true,
		syncLosses: 0,
		packets:    0,
		pids:       map[uint16]*pidState{},
		programs:   newProgramTable(),
		sections:   map[uint16]*sectionAssembler{},

		index:              index,
		indexErr:           nil,
		iframes:            0,
		video:              nil,
		programsGeneration: -1,
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
		a.handle(a.carry[:], a.offset-PacketSize)
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
		if !a.synced && len(p) > PacketSize && p[PacketSize] != syncByte {
			// 同期を失っている間は、次のパケットの先頭も sync byte であることを確かめてから合わせ直す。
			// ペイロードの中の 0x47 を、パケットの先頭と取り違えないためである。
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
		a.handle(p[:PacketSize], a.offset)
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
		Programs:   a.programs.snapshot(),
		IFrames:    a.iframes,
		IndexError: a.indexErr,
	}
}

// handle は 1 パケットを処理する。offset は、パケットの先頭の位置 (受け取ったバイト列の先頭からのバイト数)。
func (a *Analyzer) handle(b []byte, offset int64) {
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
		a.discardPayload(pkt.pid)

		return
	}
	if pkt.malformed {
		// アダプテーションフィールドの長さが壊れていて、ペイロードの位置もフラグも分からない。
		// 誤りと同じく、次のパケットを基準に数え直す。
		state.lastCC = -1
		a.discardPayload(pkt.pid)

		return
	}
	result := state.checkContinuity(&pkt)
	if pkt.scrambling != 0 {
		// スクランブルされたペイロードは読めない。
		state.stats.Scrambled++
		a.discardPayload(pkt.pid)

		return
	}
	if pkt.payload == nil || result == continuityDuplicate {
		// 重複したパケットのペイロードは、直前のパケットで読んでいる。
		return
	}
	a.handlePSI(&pkt, result == continuityDropped)
	a.handleVideo(&pkt, offset, result == continuityDropped)
}

// discardPayload は、ペイロードを読めなかった PID の、組み立て中のセクションと I フレームの探索を捨てる。
// 欠けたペイロードをまたいでつなぐと、壊れたセクションや、別の PES の位置を索引に書いてしまうからである。
func (a *Analyzer) discardPayload(pid uint16) {
	if assembler := a.sections[pid]; assembler != nil {
		assembler.reset()
	}
	if a.video != nil && a.video.pid == pid {
		a.video.reset()
	}
}

// handleVideo は、映像のパケットから I フレームを探し、見つけたら索引に追記する。
func (a *Analyzer) handleVideo(pkt *packet, offset int64, dropped bool) {
	if a.programsGeneration != a.programs.generation {
		a.programsGeneration = a.programs.generation
		pid, ok := a.programs.mpeg2VideoPID()
		switch {
		case !ok:
			a.video = nil
		case a.video == nil || a.video.pid != pid:
			a.video = newIFrameDetector(pid)
		}
	}
	if a.video == nil || a.video.pid != pkt.pid {
		return
	}
	if dropped {
		a.video.reset()
	}

	entry, ok := a.video.feed(pkt, offset)
	if !ok {
		return
	}
	a.iframes++
	if a.index != nil && a.indexErr == nil {
		a.indexErr = a.index.Append(entry)
	}
}

// handlePSI は、PAT と PMT のパケットからセクションを組み立てて読む。
func (a *Analyzer) handlePSI(pkt *packet, dropped bool) {
	pid := pkt.pid
	if pid != patPID && !a.programs.isPMTPID(pid) {
		return
	}

	assembler := a.sections[pid]
	if assembler == nil {
		assembler = newSectionAssembler()
		a.sections[pid] = assembler
	}
	if dropped {
		// 途中のパケットが欠けたセクションは、組み立てても CRC で弾かれるので、捨てて次の始まりを待つ。
		assembler.reset()
	}
	assembler.feed(pkt.payloadUnitStart, pkt.payload, func(section []byte) {
		if pid == patPID {
			a.programs.handlePAT(section)
		} else {
			a.programs.handlePMT(pid, section)
		}
	})
}

// continuity は、continuity_counter を調べた結果。
type continuity int

const (
	// continuityOK は、連続しているか、調べようがない (最初のパケット、ペイロードがない、意図的な不連続) ことを表す。
	continuityOK continuity = iota
	// continuityDuplicate は、直前のパケットの再送であることを表す。ペイロードは二重に読んではいけない。
	continuityDuplicate
	// continuityDropped は、パケットが欠けたことを表す。
	continuityDropped
)

// checkContinuity は、continuity_counter の不連続を数える。
//
//   - ペイロードを持たないパケットでは continuity_counter は増えないので、見ない
//   - discontinuity_indicator が立っていれば、不連続は意図的なので数えない。
//     ペイロードを持たないパケットで立っていれば、次のパケットを基準に数え直す
//   - 同じ値の再送 (重複) は 1 回だけ許す
func (s *pidState) checkContinuity(pkt *packet) continuity {
	if !pkt.hasPayload {
		if pkt.discontinuity {
			s.lastCC = -1
		}

		return continuityOK
	}
	cc := int(pkt.continuityCounter)
	defer func() { s.lastCC = cc }()

	if s.lastCC < 0 || pkt.discontinuity {
		s.duplicated = false

		return continuityOK
	}
	if cc == s.lastCC && !s.duplicated {
		s.duplicated = true

		return continuityDuplicate
	}
	s.duplicated = false
	if cc != (s.lastCC+1)&0x0F {
		s.stats.Drops++

		return continuityDropped
	}

	return continuityOK
}
