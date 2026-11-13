package ts

import (
	"errors"
	"io"
	"sync"
	"time"
)

// スクランブルされたまま届いた TS を見分けるパラメーター (#1517、設計書 5.3、7.1、付録 A)。
//
// どれも、チューナーサーバーのカードを抜いた実機で測っていない仮の値である。実機で測ったら直す。
const (
	// ScrambledRatioThreshold は、ES のパケットのうち、スクランブルされているものの割合の閾値。これを超えたら、スクランブルされたままとする。
	// 正常に復号できた TS は 0 に、カードが読めていない TS は 1 に寄ると見込むので、両者から最も離れた真ん中に置く。
	// 映像だけが暗号のまま (音声は平文) のときも、映像のパケットが大半を占めるので、閾値を超える。
	ScrambledRatioThreshold = 0.5
	// MinRecordingScramblePackets は、録画を判定するのに要る、ES のパケットの最少の数。これに満たない録画とセグメントは判定しない。
	// 地上波 (15 Mbps 前後) の約 3 秒分で、選局した直後の 1 秒ほどがスクランブルされたままでも、割合は閾値に届かない。
	// ビットレートの低いラジオ (100 kbps 前後) では約 7 分分になり、それより短い録画は判定しない。
	MinRecordingScramblePackets = 30000
	// DefaultScrambleWindowPackets は、ライブで割合を出す窓 1 つの、ES のパケットの数。
	// 地上波では 0.1 秒に満たず、ラジオでも約 8 秒で埋まる。
	DefaultScrambleWindowPackets = 500
	// DefaultScrambleWindows は、閾値を超えた窓が、これだけ続いたらスクランブルされたままと決める窓の数。
	// 1 だと、ラジオでは窓 1 つで抑止時間を越え、選局した直後の窓 1 つで決めてしまうので、2 以上にする。
	DefaultScrambleWindows = 2
	// DefaultScrambleHold は、閾値を超えた窓が続き始めてから、スクランブルされたままと決めるまでに待つ時間。
	// 選局した直後に鍵がそろうまでの時間と、デスクランブラーが ECM を問い合わせ直す時間より長く取る。
	DefaultScrambleHold = 10 * time.Second
	// DefaultScrambleMinPartialPackets は、判定の前にエンコーダーが降りたとき、埋まりかけの窓を使ってよい ES のパケットの最少の数。
	// 窓の大きさより小さくする。
	DefaultScrambleMinPartialPackets = 200
)

// ScrambleCounts は、ES のパケットのうち、判定に数えるパケットと、そのうちスクランブルされていたパケットの数。
type ScrambleCounts struct {
	Packets   int64
	Scrambled int64
}

// Ratio は、スクランブルされていたパケットの割合を返す。数えたパケットがなければ 0 を返す。
func (c ScrambleCounts) Ratio() float64 {
	if c.Packets <= 0 {
		return 0
	}

	return float64(c.Scrambled) / float64(c.Packets)
}

func (c ScrambleCounts) add(o ScrambleCounts) ScrambleCounts {
	return ScrambleCounts{Packets: c.Packets + o.Packets, Scrambled: c.Scrambled + o.Scrambled}
}

// isESKind は、kind が、PMT に載った ES (映像、音声、字幕・データ) の種類かどうかを返す。
// PAT、PMT、ECM、EIT のようなセクションの PID はスクランブルされないので、数えると割合が薄まる。
func isESKind(kind string) bool {
	return kind == PIDKindVideo || kind == PIDKindAudio || kind == PIDKindOther
}

// ESScrambleCounts は、セグメントの PID ごとの集計 qualities から、ES の PID だけを足し合わせる。
// Payloads を記録する前のセグメントでは、Packets から Errors を引いた数と Scrambled で近似する。
func ESScrambleCounts(qualities []PIDQuality) ScrambleCounts {
	var counts ScrambleCounts
	for _, q := range qualities {
		if !isESKind(q.Kind) {
			continue
		}
		if q.Payloads != nil && q.ScrambledPayloads != nil {
			counts = counts.add(ScrambleCounts{Packets: *q.Payloads, Scrambled: *q.ScrambledPayloads})

			continue
		}
		counts = counts.add(ScrambleCounts{Packets: max(q.Packets-q.Errors, 0), Scrambled: q.Scrambled})
	}

	return counts
}

// RecordingScramble は、録画がスクランブルされたまま録れたかを判定した結果。
type RecordingScramble struct {
	// Judged は、判定できたかどうか。ES のパケットが MinRecordingScramblePackets に満たなければ false。
	Judged bool
	// Ratio は、録画全体の、ES のパケットのうちスクランブルされていたものの割合。
	Ratio float64
	// Warning は、録画全体か、判定できたセグメントのどれかが、閾値を超えたかどうか。
	// セグメントごとにも見るのは、途中からカードが読めなくなった録画を見分けるためである。
	Warning bool
}

// JudgeRecordingScramble は、録画のセグメントごとの PID の集計 segments から、スクランブルされたまま録れたかを判定する (#1517、設計書 5.5)。
// PID の集計を記録していないセグメント (nil) は飛ばす。
func JudgeRecordingScramble(segments [][]PIDQuality) RecordingScramble {
	var total ScrambleCounts
	warning := false
	for _, qualities := range segments {
		counts := ESScrambleCounts(qualities)
		total = total.add(counts)
		if counts.Packets >= MinRecordingScramblePackets && counts.Ratio() > ScrambledRatioThreshold {
			warning = true
		}
	}
	if total.Packets < MinRecordingScramblePackets {
		return RecordingScramble{Judged: false, Ratio: 0, Warning: false}
	}

	return RecordingScramble{Judged: true, Ratio: total.Ratio(), Warning: warning || total.Ratio() > ScrambledRatioThreshold}
}

//go:generate go tool go-enum --values --nocomments

// ScrambleVerdict は、ライブの TS がスクランブルされたままかを判定した結果。
//
// ENUM(
// Undecided // まだ決めていないことを表す。
// Clear // スクランブルされていないと決めたことを表す。決めたら変わらない。
// Scrambled // スクランブルされたままと決めたことを表す。決めたら変わらない。
// )
type ScrambleVerdict int

// ErrScrambled は、スクランブルされたままと決めた後に、ScrambleDetector へ書き込んだことを表す。
// io.TeeReader で読むストリームに挟むと、決めた時点で読むのが止まる。
var ErrScrambled = errors.New("the stream is still scrambled")

// ScrambleDetectorOptions は ScrambleDetector の設定。0 の項目は既定値にする。
type ScrambleDetectorOptions struct {
	WindowPackets     int
	Windows           int
	Hold              time.Duration
	MinPartialPackets int
	// Now は今の時刻を返す。抑止時間を測るのに使う。
	Now func() time.Time
	// OnScrambled は、スクランブルされたままと決めたときに、1 度だけ呼ぶ。Write の中で呼び、返すまで Write は ErrScrambled を返さない。
	// 理由を画面に送ってから、ストリームを閉じるのに使う。nil なら呼ばない。
	OnScrambled func()
}

// ScrambleDetector は、ライブで流す TS を流し読みし、スクランブルされたまま届いているかを、連続した窓で判定する (#1517、設計書 7.1)。
//
// 数えるのは、PMT に載った ES の PID の、ペイロードを持ち、transport_error_indicator の立っていないパケットだけである。
// PMT を読むまでは何も数えない。窓 1 つの割合が閾値以下なら、スクランブルされていないと決めて、数えるのをやめる。
// 閾値を超えた窓が Windows 以上続き、続き始めた窓の 1 個目のパケットを数えてから Hold 以上たったら、スクランブルされたままと決める。
// 選局した直後に、鍵がそろう前のパケットが流れても、無料の局を止めないためである。
//
// Write は 1 つの goroutine から呼ぶ。Verdict と Finish は、別の goroutine から呼んでよい。
type ScrambleDetector struct {
	opts ScrambleDetectorOptions

	mu       sync.Mutex
	carry    [PacketSize]byte
	carryLen int
	programs *programTable
	sections map[uint16]*sectionAssembler
	// esPIDs は、PMT に載った ES の PID。programsGeneration の構成から求める。
	esPIDs             map[uint16]bool
	programsGeneration int

	verdict ScrambleVerdict
	// window は、埋めている窓の数。windowStart は、その窓の 1 個目のパケットを数えた時刻。
	window      ScrambleCounts
	windowStart time.Time
	// last は、最後に埋まった窓の数。まだなければ Packets が 0。
	last ScrambleCounts
	// over は、閾値を超えて続いた窓の数。streakStart は、その最初の窓の 1 個目のパケットを数えた時刻。
	over        int
	streakStart time.Time
}

var _ io.Writer = new(ScrambleDetector)

// NewScrambleDetector は ScrambleDetector を作る。
func NewScrambleDetector(opts ScrambleDetectorOptions) *ScrambleDetector {
	if opts.WindowPackets <= 0 {
		opts.WindowPackets = DefaultScrambleWindowPackets
	}
	if opts.Windows <= 0 {
		opts.Windows = DefaultScrambleWindows
	}
	if opts.Hold <= 0 {
		opts.Hold = DefaultScrambleHold
	}
	if opts.MinPartialPackets <= 0 {
		opts.MinPartialPackets = DefaultScrambleMinPartialPackets
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	return &ScrambleDetector{
		opts: opts, mu: sync.Mutex{}, carry: [PacketSize]byte{}, carryLen: 0,
		programs: newProgramTable(), sections: map[uint16]*sectionAssembler{}, esPIDs: map[uint16]bool{}, programsGeneration: 0,
		verdict: ScrambleVerdictUndecided, window: ScrambleCounts{Packets: 0, Scrambled: 0}, windowStart: time.Time{},
		last: ScrambleCounts{Packets: 0, Scrambled: 0}, over: 0, streakStart: time.Time{},
	}
}

// Verdict は、ここまでの判定を返す。
func (d *ScrambleDetector) Verdict() ScrambleVerdict {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.verdict
}

// Finish は、判定の前にストリームが終わった (エンコーダーが降りた) ときに、いちばん新しい窓だけで決めて返す。
// 埋まりかけの窓が MinPartialPackets 以上あればそれを、なければ最後に埋まった窓を使う。窓が 1 つもなければ決めない。
// すでに決めていれば、その判定を返す。Finish で決めたときは、OnScrambled を呼ばない。
func (d *ScrambleDetector) Finish() ScrambleVerdict {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.verdict != ScrambleVerdictUndecided {
		return d.verdict
	}
	counts := d.last
	if d.window.Packets >= int64(d.opts.MinPartialPackets) {
		counts = d.window
	}
	switch {
	case counts.Packets == 0:
	case counts.Ratio() > ScrambledRatioThreshold:
		d.verdict = ScrambleVerdictScrambled
	default:
		d.verdict = ScrambleVerdictClear
	}

	return d.verdict
}

// Write は p を流し読みする。スクランブルされたままと決めた後は ErrScrambled を返す。パケットの途中で区切られた書き込みも受け付ける。
func (d *ScrambleDetector) Write(p []byte) (int, error) {
	n := len(p)
	d.mu.Lock()
	if d.verdict == ScrambleVerdictClear {
		// 決めた後は、中継の邪魔をしないよう、読まない。
		d.mu.Unlock()

		return n, nil
	}
	if d.verdict == ScrambleVerdictScrambled {
		d.mu.Unlock()

		return 0, ErrScrambled
	}
	decided := d.feed(p)
	d.mu.Unlock()
	if !decided {
		return n, nil
	}
	if d.opts.OnScrambled != nil {
		d.opts.OnScrambled()
	}

	return 0, ErrScrambled
}

// feed は p のパケットを数え、スクランブルされたままと決めたら true を返す。
func (d *ScrambleDetector) feed(p []byte) bool {
	if d.carryLen > 0 {
		copied := copy(d.carry[d.carryLen:], p)
		d.carryLen += copied
		p = p[copied:]
		if d.carryLen < PacketSize {
			return false
		}
		d.carryLen = 0
		if d.handle(d.carry[:]) {
			return true
		}
	}
	for len(p) > 0 && d.verdict == ScrambleVerdictUndecided {
		if p[0] != syncByte || (len(p) > PacketSize && p[PacketSize] != syncByte) {
			// 境界がずれていれば、次の sync byte を探す。判定に使うだけなので、Analyzer ほど厳密には合わせ直さない。
			p = p[1:]

			continue
		}
		if len(p) < PacketSize {
			d.carryLen = copy(d.carry[:], p)

			break
		}
		if d.handle(p[:PacketSize]) {
			return true
		}
		p = p[PacketSize:]
	}

	return false
}

// handle は 1 パケットを数え、スクランブルされたままと決めたら true を返す。
func (d *ScrambleDetector) handle(b []byte) bool {
	pkt := parsePacket(b)
	if pkt.pid == NullPID || pkt.transportError || pkt.malformed || !pkt.hasPayload {
		return false
	}
	if pkt.scrambling == 0 && (pkt.pid == patPID || d.programs.isPMTPID(pkt.pid)) {
		d.handlePSI(&pkt)

		return false
	}
	if d.programsGeneration != d.programs.generation {
		d.programsGeneration = d.programs.generation
		clear(d.esPIDs)
		for _, program := range d.programs.programs {
			for _, s := range program.Streams {
				d.esPIDs[s.PID] = true
			}
		}
	}
	if !d.esPIDs[pkt.pid] {
		return false
	}

	return d.count(pkt.scrambling != 0)
}

// handlePSI は、PAT と PMT のパケットからセクションを組み立てて読む。
func (d *ScrambleDetector) handlePSI(pkt *packet) {
	assembler := d.sections[pkt.pid]
	if assembler == nil {
		assembler = newSectionAssembler()
		d.sections[pkt.pid] = assembler
	}
	pid := pkt.pid
	assembler.feed(pkt.payloadUnitStart, pkt.payload, func(section []byte) {
		if pid == patPID {
			d.programs.handlePAT(section)

			return
		}
		d.programs.handlePMT(pid, section)
	})
}

// count は、ES のパケットを窓に数え、窓が埋まったら判定を進める。スクランブルされたままと決めたら true を返す。
func (d *ScrambleDetector) count(scrambled bool) bool {
	now := d.opts.Now()
	if d.window.Packets == 0 {
		d.windowStart = now
	}
	d.window.Packets++
	if scrambled {
		d.window.Scrambled++
	}
	if d.window.Packets < int64(d.opts.WindowPackets) {
		return false
	}
	filled := d.window
	d.last, d.window = filled, ScrambleCounts{Packets: 0, Scrambled: 0}
	if filled.Ratio() <= ScrambledRatioThreshold {
		// スクランブルされていない窓が来たら、そこで決める。閾値を超えた窓は、必ず続いた並びになる。
		d.verdict = ScrambleVerdictClear

		return false
	}
	if d.over == 0 {
		d.streakStart = d.windowStart
	}
	d.over++
	if d.over >= d.opts.Windows && now.Sub(d.streakStart) >= d.opts.Hold {
		d.verdict = ScrambleVerdictScrambled

		return true
	}

	return false
}
