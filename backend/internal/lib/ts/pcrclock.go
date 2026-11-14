package ts

import "time"

// PCR の時刻の扱い (#1515)。PCR は、基準部 (33 ビット、90 kHz) だけを使い、拡張部 (27 MHz の端数) は捨てる。
const (
	// pcrBaseHz は、PCR の基準部の周波数。
	pcrBaseHz = 90_000
	// pcrBaseCycle は、PCR の基準部が一周する値 (33 ビット、約 26.5 時間)。
	pcrBaseCycle = int64(1) << 33
	// pcrRewindLimit は、PCR がこれ以上戻ったら、選局などで飛んだとみなして組を結び直す量 (基準部の 1 秒)。
	// PCR の揺れ (ジッタ) では結び直さず、選局の飛びは取り逃さない大きさにする。
	pcrRewindLimit = pcrBaseHz
)

// DefaultPCRPairWindow は、最小の組を選び直す区切りの長さ。PCR の送出間隔の上限 (0.1 秒) より十分長くし、
// PCR とサーバーの時計の水晶のずれが、区切り 2 つ分の間で、表示の刻みより十分小さくなる長さにする。
const DefaultPCRPairWindow = 30 * time.Second

// PCRPair は、PCR と、その PCR を含む TS を受け取った時刻の組 (#1515)。
type PCRPair struct {
	// PCR は、PCR の基準部を秒にしたもの。
	PCR float64
	// ReceivedAt は、PCR を含む TS の塊を受け取った時点の、サーバーの時計。
	ReceivedAt time.Time
}

// lag は、受け取った時刻から PCR を引いた量 (秒)。小さいほど、遅れずに読んだ組である。
func (p *PCRPair) lag() float64 {
	return float64(p.ReceivedAt.UnixMicro())/1e6 - p.PCR
}

// PCRClock は、1 つのサービスの TS から、PCR と受け取った時刻の組を作る (#1515)。
//
// 「受け取った時刻 − PCR」が最も小さい組を採る。遅れて読んだ組を採ると、遅れた分だけ「放送から」が小さく出るからである。
// PCR とサーバーの時計は別の水晶でずれていくので、組は区切り (window) ごとに選び直し、今の区切りで最小の組と、
// 1 つ前の区切りで最小の組のうち、小さい方を使う。区切ってすぐに、遅れた組しかないことがあっても困らないようにするためである。
// PCR が pcrRewindLimit 以上戻るか、一周の半分より先へ進んだら、選局などで飛んだとみなし、持っている組を捨てて結び直す。
// 巻き戻りを足し込む作りにはしない。選局の直後の飛びを取り込み、時刻が何時間もずれるほうが危ないからである。
type PCRClock struct {
	serviceID uint16
	window    time.Duration
	table     *programTable
	pat       *sectionAssembler
	pmt       *sectionAssembler
	// pmtPID は、いま組み立てている PMT の PID。PAT を読むまでは 0。
	pmtPID uint16
	// rest は、前の塊の終わりに残った、188 バイトに満たないパケットの頭。
	rest []byte
	// windowStart は、今の区切りを始めた時刻。まだ始めていなければ零値。
	windowStart time.Time
	current     *PCRPair
	previous    *PCRPair
	// last は、最後に読んだ PCR の基準部。まだ読んでいなければ -1。
	last int64
}

// NewPCRClock は、サービス ID (program_number) が serviceID のサービスの PCRClock を返す。
// PMT を読むまでは、どの PID の PCR も受け付ける。window が 0 以下なら DefaultPCRPairWindow を使う。
func NewPCRClock(serviceID uint16, window time.Duration) *PCRClock {
	if window <= 0 {
		window = DefaultPCRPairWindow
	}

	return &PCRClock{
		serviceID: serviceID, window: window, table: newProgramTable(),
		pat: newSectionAssembler(), pmt: newSectionAssembler(), pmtPID: 0,
		rest: make([]byte, 0, PacketSize), windowStart: time.Time{}, current: nil, previous: nil, last: -1,
	}
}

// Write は、receivedAt に受け取った TS の塊 chunk を読む。塊は、パケットの境目で切れていなくてよい。
// 塊の中の PCR は、どれも receivedAt と組にする。塊の最後の PCR ほど「受け取った時刻 − PCR」が小さく出るので、自然に選ばれる。
func (c *PCRClock) Write(chunk []byte, receivedAt time.Time) {
	c.rotate(receivedAt)
	if len(c.rest) > 0 {
		need := PacketSize - len(c.rest)
		if len(chunk) < need {
			c.rest = append(c.rest, chunk...)

			return
		}
		c.rest = append(c.rest, chunk[:need]...)
		c.handle(c.rest, receivedAt)
		c.rest = c.rest[:0]
		chunk = chunk[need:]
	}
	for len(chunk) >= PacketSize {
		// 同期が外れていたら、次の sync byte まで読み飛ばす。
		if chunk[0] != syncByte {
			chunk = chunk[1:]

			continue
		}
		c.handle(chunk[:PacketSize], receivedAt)
		chunk = chunk[PacketSize:]
	}
	c.rest = append(c.rest, chunk...)
}

// Pair は、今使う組を返す。まだ組がなければ false を返す。
func (c *PCRClock) Pair() (PCRPair, bool) {
	best := c.current
	if c.previous != nil && (best == nil || c.previous.lag() < best.lag()) {
		best = c.previous
	}
	if best == nil {
		return PCRPair{PCR: 0, ReceivedAt: time.Time{}}, false
	}

	return *best, true
}

// rotate は、区切りの長さが経ったら、今の区切りの組を 1 つ前の区切りの組にし、新しい区切りを始める。
func (c *PCRClock) rotate(now time.Time) {
	if c.windowStart.IsZero() {
		c.windowStart = now

		return
	}
	if now.Sub(c.windowStart) >= c.window {
		c.previous = c.current
		c.current = nil
		c.windowStart = now
	}
}

func (c *PCRClock) handle(b []byte, receivedAt time.Time) {
	p := parsePacket(b)
	if p.malformed || p.transportError {
		return
	}
	switch {
	case p.pid == patPID:
		c.pat.feed(p.payloadUnitStart, p.payload, c.table.handlePAT)
		if pid := c.table.pmtPIDs[c.serviceID]; pid != c.pmtPID {
			c.pmtPID = pid
			c.pmt.reset()
		}
	case c.pmtPID != 0 && p.pid == c.pmtPID:
		c.pmt.feed(p.payloadUnitStart, p.payload, func(section []byte) {
			c.table.handlePMT(p.pid, section)
		})
	}
	if p.pcr == noPCR {
		return
	}
	// PMT を読んだら、PCR_PID の PCR だけを使う。0x1FFF は「PCR なし」を表す。
	if program := c.table.programs[c.serviceID]; program != nil && (program.PCRPID == NullPID || p.pid != program.PCRPID) {
		return
	}
	c.observe(p.pcr/300, receivedAt)
}

// observe は、PCR の基準部 base と、受け取った時刻の組を足す。
func (c *PCRClock) observe(base int64, receivedAt time.Time) {
	if c.last >= 0 && (base < c.last-pcrRewindLimit || base-c.last > pcrBaseCycle/2) {
		c.current, c.previous = nil, nil
		c.windowStart = receivedAt
	}
	c.last = base
	pair := &PCRPair{PCR: float64(base) / pcrBaseHz, ReceivedAt: receivedAt}
	if c.current == nil || pair.lag() < c.current.lag() {
		c.current = pair
	}
}
