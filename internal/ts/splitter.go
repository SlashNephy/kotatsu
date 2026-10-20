package ts

import (
	"errors"
	"io"
	"time"
)

// Splitter は、TS を、映像の I フレームを含む PES の先頭で、決めておいた PTS の位置ごとに区切って書く (設計書 7.3)。
//
// kyusu が録画の映像を変換せずに配信するとき、録画の索引の I フレームの位置でセグメントを切るのに使う (#806)。
// FFmpeg の HLS や segment の muxer は、出力の始まりからの時間で切るので、起動し直すたびに区切りがずれる。
//
// 区切りは、PTS の昇順の並び (cuts) で決める。PTS が cuts[0] 以上の最初の I フレームから最初のセグメントを始め、
// I フレームの PTS が次の区切り以上になったら、その PES の先頭から次のセグメントにする。最初のセグメントより前のパケットは捨てる。
// 区切りを飛び越えた (間の I フレームが欠けた) ときは、飛び越えた区切りの分のセグメントを作らず、届いた区切りの番号にする。
// cuts を使い切った後は、今のセグメントを始めた I フレームから interval 以上進んだ最初の I フレームで切る。
// 録画中の録画では、まだ索引にない先の区切りを、この規則で決める。
// 各セグメントの先頭には、直前の PAT と PMT のパケットを写し、そのセグメントだけで再生を始められるようにする。
type Splitter struct {
	cuts []int64
	// interval は、cuts を使い切った後に切る間隔 (90 kHz)。
	interval int64
	// create は、i 番目の区切り (Cuts の添え字) から始まるセグメントの書き込み先を開く。
	create func(i int) (io.WriteCloser, error)

	// next は、次のセグメントを始める区切りの添え字。
	next int
	// started は、今のセグメントを始めた I フレームの PTS。
	started int64
	current io.WriteCloser
	// pending は、映像の PES の始まりから、I フレームかどうかが分かるまでのパケット。ほかの PID のパケットも、順を保って溜める。
	pending []byte
	// holding は、pending に PES を溜めているかどうか。
	holding bool

	// pat と pmt は、直前の PAT と PMT のパケット。セグメントの先頭に写す。
	pat, pmt   []byte
	programs   *programTable
	sections   map[uint16]*sectionAssembler
	generation int
	video      *iframeDetector

	carry    [PacketSize]byte
	carryLen int
	err      error
}

var _ io.WriteCloser = new(Splitter)

// errNoCut は、区切りがないことを表す。
var errNoCut = errors.New("no cut is given")

// NewSplitter は Splitter を返す。cuts は PTS (90 kHz) の昇順の並びで、空でない。interval は、cuts を使い切った後に切る間隔。
// create は、i 番目の区切りから始まるセグメントの書き込み先を開く。セグメントを書き終えたら、その Close を呼ぶ。
func NewSplitter(cuts []int64, interval time.Duration, create func(i int) (io.WriteCloser, error)) (*Splitter, error) {
	if len(cuts) == 0 {
		return nil, errNoCut
	}

	return &Splitter{
		cuts: cuts, interval: int64(interval * PTSClock / time.Second), create: create, next: 0, started: 0, current: nil, pending: nil, holding: false,
		pat: nil, pmt: nil, programs: newProgramTable(), sections: map[uint16]*sectionAssembler{}, generation: 0, video: nil,
		carry: [PacketSize]byte{}, carryLen: 0, err: nil,
	}, nil
}

// Write は、TS を受け取る。パケットの境界でなくてもよい。
// セグメントを開くか書くのに失敗したら、失敗したパケットまでのバイト数と、そのエラーを返す。以後の Write と Close も、同じエラーを返す。
func (s *Splitter) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	n := len(p)
	if s.carryLen > 0 {
		copied := copy(s.carry[s.carryLen:], p)
		s.carryLen += copied
		p = p[copied:]
		if s.carryLen < PacketSize {
			return n, nil
		}
		s.handle(s.carry[:])
		s.carryLen = 0
	}
	for s.err == nil && len(p) > 0 {
		if p[0] != syncByte {
			// FFmpeg の出力はパケットの境界にそろっているので、ずれたら次の sync byte まで読み飛ばす。
			p = p[1:]

			continue
		}
		if len(p) < PacketSize {
			s.carryLen = copy(s.carry[:], p)

			break
		}
		s.handle(p[:PacketSize])
		p = p[PacketSize:]
	}
	if s.err != nil {
		// 失敗したパケットまでを読んだものとする。
		return n - len(p), s.err
	}

	return n, nil
}

// Close は、溜めているパケットを書き、最後のセグメントを閉じる。入力を最後まで書いたときだけ呼ぶ。
func (s *Splitter) Close() error {
	if s.err != nil {
		return s.err
	}
	s.flushPending()
	if s.err != nil {
		return s.err
	}
	if s.current == nil {
		return nil
	}
	err := s.current.Close()
	s.current = nil

	return err
}

// handle は 1 パケットを処理する。
func (s *Splitter) handle(b []byte) {
	pkt := parsePacket(b)
	if pkt.malformed || pkt.transportError {
		s.emit(b)

		return
	}
	s.handlePSI(&pkt, b)
	s.updateVideo()
	if s.video == nil || pkt.pid != s.video.pid || pkt.payload == nil {
		s.emit(b)

		return
	}
	if pkt.payloadUnitStart {
		// 前の PES は、I フレームでなかった。
		s.flushPending()
		s.holding = true
	}
	if !s.holding {
		s.emit(b)

		return
	}
	s.pending = append(s.pending, b...)
	entry, ok := s.video.feed(&pkt, 0)
	if !ok {
		return
	}
	s.keyframe(entry.PTS)
}

// handlePSI は、PAT と PMT のパケットを控え、番組の構成を読む。
func (s *Splitter) handlePSI(pkt *packet, b []byte) {
	if pkt.payload == nil || (pkt.pid != patPID && !s.programs.isPMTPID(pkt.pid)) {
		return
	}
	if pkt.payloadUnitStart {
		if pkt.pid == patPID {
			s.pat = s.pat[:0]
		} else {
			s.pmt = s.pmt[:0]
		}
	}
	if pkt.pid == patPID {
		s.pat = append(s.pat, b...)
	} else {
		s.pmt = append(s.pmt, b...)
	}
	assembler := s.sections[pkt.pid]
	if assembler == nil {
		assembler = newSectionAssembler()
		s.sections[pkt.pid] = assembler
	}
	pid := pkt.pid
	assembler.feed(pkt.payloadUnitStart, pkt.payload, func(section []byte) {
		if pid == patPID {
			s.programs.handlePAT(section)
		} else {
			s.programs.handlePMT(pid, section)
		}
	})
}

// updateVideo は、番組の構成が変わったら、I フレームを探す映像を選び直す。
func (s *Splitter) updateVideo() {
	if s.generation == s.programs.generation {
		return
	}
	s.generation = s.programs.generation
	video, ok := s.programs.firstVideo()
	switch {
	case !ok:
		s.flushPending()
		s.video = nil
	case s.video == nil || s.video.pid != video.PID || s.video.streamType != video.StreamType:
		s.flushPending()
		// I フレームの探し方を知らない映像なら nil になり、区切らずに書く。
		s.video = newIFrameDetector(video.PID, video.StreamType)
	}
}

// keyframe は、溜めている PES が、PTS が pts の I フレームだと分かったときに呼ぶ。区切りに届いていれば、新しいセグメントを始める。
func (s *Splitter) keyframe(pts int64) {
	reached := -1
	for i := s.next; i < len(s.cuts) && PTSDiff(s.cuts[i], pts) >= 0; i++ {
		reached = i
	}
	if reached < 0 && s.next >= len(s.cuts) && PTSDiff(s.started, pts) >= s.interval {
		reached = s.next
	}
	if reached >= 0 {
		s.start(reached, pts)
	}
	s.flushPending()
}

// start は、今のセグメントを閉じ、i 番目の区切りから、PTS が pts の I フレームで新しいセグメントを始める。先頭に PAT と PMT を写す。
func (s *Splitter) start(i int, pts int64) {
	if s.current != nil {
		if err := s.current.Close(); err != nil {
			s.err = err

			return
		}
		s.current = nil
	}
	w, err := s.create(i)
	if err != nil {
		s.err = err

		return
	}
	s.current = w
	s.next = i + 1
	s.started = pts
	s.write(s.pat)
	s.write(s.pmt)
}

// flushPending は、溜めているパケットを今のセグメントに書く。
func (s *Splitter) flushPending() {
	s.holding = false
	if len(s.pending) == 0 {
		return
	}
	s.write(s.pending)
	s.pending = s.pending[:0]
}

// emit は、映像の PES を溜めていればその後ろに、そうでなければ今のセグメントに、パケットを書く。
func (s *Splitter) emit(b []byte) {
	if s.holding {
		s.pending = append(s.pending, b...)

		return
	}
	s.write(b)
}

// write は、今のセグメントに書く。最初のセグメントを始める前なら捨てる。
func (s *Splitter) write(b []byte) {
	if s.err != nil || s.current == nil || len(b) == 0 {
		return
	}
	if _, err := s.current.Write(b); err != nil {
		s.err = err
	}
}
