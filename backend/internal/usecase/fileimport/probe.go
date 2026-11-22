// Package fileimport は、保存先プールにある、Kotatsu が録っていない TS を、録画として取り込む (設計書 6.5、#1734)。
//
// 番組の情報は、ファイルの頭と終わりを読み、PAT と PMT、SDT、EIT[p/f]、TOT から求める。
// Kotatsu にあるサービスの TS だけを取り込み、ほかは理由を付けて飛ばす。
package fileimport

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// bdavPacketSize は、Blu-ray (BDAV) の .m2ts のパケットの大きさ。TS のパケットの前に 4 バイトの時刻 (TP_extra_header) を付ける。
	bdavPacketSize = 192
	// syncByte は、TS のパケットの先頭のバイト。
	syncByte = 0x47
	// syncPackets は、パケットの大きさを見分けるのに、sync byte が続くことを確かめるパケットの数。
	syncPackets = 8
	// headLimit は、頭から読む量の上限。PAT、PMT、SDT、TOT、EIT[p/f] は数秒ごとに送られるので、揃えば、ここまで読まずにやめる。
	headLimit = 32 << 20
	// tailSize は、終わりから読む量。録った終わりの TOT と、終わりの頃の EIT[p/f] を読む。
	tailSize = 16 << 20
	// chunkSize は、1 回に読む量。
	chunkSize = 1 << 20
)

var (
	// ErrBDAV は、192 バイトのパケットの Blu-ray (BDAV) の .m2ts であることを表す。Kotatsu の索引と配信は 188 バイトの TS を前提にする。
	ErrBDAV = errors.New("the file is a 192-byte-packet BDAV stream (Blu-ray .m2ts); only 188-byte-packet TS is supported")
	// ErrNotTS は、MPEG-2 TS でないことを表す。
	ErrNotTS = errors.New("the file is not an MPEG-2 TS")
)

// Probe は、TS の頭と終わりから読んだ、録画の情報。
type Probe struct {
	// NetworkID は、SDT[actual] の original_network_id。SDT を読めなければ nil。
	NetworkID *uint16
	// Services は、映像を持つ番組 (サービス)。program_number の順に並べる。
	Services []*Service
	// RecordedFrom と RecordedUntil は、頭で最初に読んだ TOT と、終わりで最後に読んだ TOT。読めなければ nil。
	RecordedFrom  *time.Time
	RecordedUntil *time.Time
}

// Service は、TS の 1 つのサービスと、EIT[p/f] で放送中か次とされた番組。
type Service struct {
	ServiceID uint16
	// Name は、SDT[actual] のサービス名。読めなければ空。
	Name string
	// Events は、頭と終わりの EIT[p/f] で放送中か次とされた番組。読んだ順に並べ、同じ番組は 1 つにする。
	Events []*Event
}

// Event は、EIT[p/f] で読んだ番組。
type Event struct {
	EventID     uint16
	Name        string
	Description string
	Extended    []*ts.ExtendedItem
	// Start と Duration は、番組の開始と長さ。未定なら nil。
	Start    *time.Time
	Duration *time.Duration
}

// ReadProbe は、大きさ size の TS r の頭と終わりを読み、録画の情報を返す。
// 192 バイトのパケットの BDAV なら ErrBDAV を、TS でなければ ErrNotTS を返す。
func ReadProbe(r io.ReaderAt, size int64) (*Probe, error) {
	if err := CheckPacketSize(r, size); err != nil {
		return nil, err
	}

	probe := &Probe{NetworkID: nil, Services: []*Service{}, RecordedFrom: nil, RecordedUntil: nil}
	names := map[uint16]string{}
	var headLast *time.Time
	head := ts.NewAnalyzer(&ts.AnalyzerOptions{
		Index: nil, CutoffPTS: nil, OnLogo: nil, OnNIT: nil, OnPresentFollowing: nil,
		OnTOT: func(at time.Time) {
			if probe.RecordedFrom == nil {
				probe.RecordedFrom = &at
			}
			headLast = &at
		},
		OnSDT: func(sdt *ts.SDT) {
			if !sdt.Actual {
				return
			}
			probe.NetworkID = new(sdt.OriginalNetworkID)
			for _, s := range sdt.Services {
				if s.HasServiceDescriptor {
					names[s.ServiceID] = s.Name
				}
			}
		},
	})
	headEnd, err := feed(head, r, 0, min(size, headLimit), func() bool { return headComplete(head, probe) })
	if err != nil {
		return nil, err
	}

	analyzers := []*ts.Analyzer{head}
	probe.RecordedUntil = headLast
	if headEnd < size {
		tail := ts.NewAnalyzer(&ts.AnalyzerOptions{
			Index: nil, CutoffPTS: nil, OnLogo: nil, OnNIT: nil, OnSDT: nil, OnPresentFollowing: nil,
			OnTOT: func(at time.Time) { probe.RecordedUntil = &at },
		})
		if _, err := feed(tail, r, max(headEnd, size-tailSize), size, nil); err != nil {
			return nil, err
		}
		analyzers = append(analyzers, tail)
	}

	for _, program := range videoPrograms(head) {
		probe.Services = append(probe.Services, &Service{
			ServiceID: program, Name: names[program], Events: events(analyzers, program),
		})
	}

	return probe, nil
}

// CheckPacketSize は、頭のパケットが 188 バイトごとに sync byte で始まるかを確かめる。
// 192 バイトごとなら BDAV とみなして ErrBDAV を返す。どちらでもなければ ErrNotTS を返す。ライブラリのファイルの再生 (#1734) でも使う。
func CheckPacketSize(r io.ReaderAt, size int64) error {
	buf := make([]byte, min(size, bdavPacketSize*(syncPackets+1)))
	if _, err := r.ReadAt(buf, 0); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read the file: %w", err)
	}
	switch {
	case synced(buf, ts.PacketSize):
		return nil
	case synced(buf, bdavPacketSize):
		return ErrBDAV
	default:
		return ErrNotTS
	}
}

// synced は、buf の先頭のどこかから、packetSize ごとに sync byte が syncPackets 回続くかを返す。
func synced(buf []byte, packetSize int) bool {
	if len(buf) < packetSize*syncPackets {
		return false
	}
	for start := range packetSize {
		if start+packetSize*(syncPackets-1) >= len(buf) {
			return false
		}
		ok := true
		for i := range syncPackets {
			if buf[start+packetSize*i] != syncByte {
				ok = false

				break
			}
		}
		if ok {
			return true
		}
	}

	return false
}

// feed は、r の from から to までを analyzer に流す。done が true を返したら、そこでやめる。読み終えた位置を返す。
func feed(analyzer *ts.Analyzer, r io.ReaderAt, from, to int64, done func() bool) (int64, error) {
	buf := make([]byte, chunkSize)
	offset := from
	for offset < to {
		n, err := r.ReadAt(buf[:min(int64(len(buf)), to-offset)], offset)
		if n > 0 {
			_, _ = analyzer.Write(buf[:n])
			offset += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return offset, fmt.Errorf("failed to read the file: %w", err)
		}
		if done != nil && done() {
			break
		}
	}

	return offset, nil
}

// headComplete は、頭で読むもの (PAT と PMT、SDT[actual]、TOT、映像を持つ番組の EIT[p/f] の放送中と次) が揃ったかを返す。
func headComplete(analyzer *ts.Analyzer, probe *Probe) bool {
	if probe.NetworkID == nil || probe.RecordedFrom == nil {
		return false
	}
	stats := analyzer.Stats()
	if len(stats.Programs) == 0 {
		return false
	}
	for _, program := range stats.Programs {
		if len(program.Streams) == 0 {
			return false
		}
	}
	for _, program := range videoPrograms(analyzer) {
		pf := analyzer.PresentFollowing(program)
		if pf == nil || !pf.PresentSeen || !pf.FollowingSeen {
			return false
		}
	}

	return true
}

// videoPrograms は、映像を持つ番組の program_number (サービス ID) を、昇順に返す。
func videoPrograms(analyzer *ts.Analyzer) []uint16 {
	programs := []uint16{}
	for _, program := range analyzer.Stats().Programs {
		if slices.ContainsFunc(program.Streams, func(s *ts.Stream) bool { return s.Kind == ts.StreamKindVideo }) {
			programs = append(programs, program.Number)
		}
	}

	return programs
}

// events は、analyzers が読んだ EIT[p/f] で、サービス serviceID の放送中か次とされた番組を、読んだ順に返す。
// 同じ番組を後の analyzer でも読めば、前に読めなかった項目を埋める。番組名や開始は、放送の直前まで載らないことがあるので、
// 頭で次の番組として読んだときには空でも、終わりで放送中として読めば埋まる。
func events(analyzers []*ts.Analyzer, serviceID uint16) []*Event {
	result := []*Event{}
	for _, analyzer := range analyzers {
		pf := analyzer.PresentFollowing(serviceID)
		if pf == nil {
			continue
		}
		for _, eventID := range []*uint16{pf.Present, pf.Following} {
			if eventID == nil {
				continue
			}
			event := &Event{EventID: *eventID, Name: "", Description: "", Extended: []*ts.ExtendedItem{}, Start: nil, Duration: nil}
			if info := analyzer.EventInfo(serviceID, *eventID); info != nil {
				event.Name, event.Description, event.Extended = info.Name, info.Description, info.Extended
			}
			if et := analyzer.EventTime(serviceID, *eventID); et != nil {
				event.Start, event.Duration = et.Start, et.Duration
			}
			if i := slices.IndexFunc(result, func(e *Event) bool { return e.EventID == *eventID }); i >= 0 {
				mergeEvent(result[i], event)

				continue
			}
			result = append(result, event)
		}
	}

	return result
}

// mergeEvent は、dst の空の項目を、後で読んだ同じ番組 src の項目で埋める。
func mergeEvent(dst, src *Event) {
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if len(dst.Extended) == 0 {
		dst.Extended = src.Extended
	}
	if dst.Start == nil {
		dst.Start = src.Start
	}
	if dst.Duration == nil {
		dst.Duration = src.Duration
	}
}

// Program は、サービスの番組のうち、録った範囲 (from から until) に最も長く重なるものを返す。
// 範囲が分からなければ、頭で放送中とされた番組を返す。どれも重ならなければ nil を返す。開始か長さが未定の番組は選ばない。
// 録画は、番組の前にマージンを取って始まるので、頭で放送中の番組は、前の番組であることが多い。
func (s *Service) Program(from, until *time.Time) *Event {
	if from == nil || until == nil {
		if len(s.Events) == 0 {
			return nil
		}

		return s.Events[0]
	}
	var best *Event
	var bestOverlap time.Duration
	for _, event := range s.Events {
		if event.Start == nil || event.Duration == nil {
			continue
		}
		start, end := *event.Start, event.Start.Add(*event.Duration)
		if !until.After(*from) {
			// 範囲が一瞬なら (TOT を 1 度しか読めなかったなど)、その時に放送していた番組にする。
			if !from.Before(start) && from.Before(end) {
				return event
			}

			continue
		}
		if from.After(start) {
			start = *from
		}
		if until.Before(end) {
			end = *until
		}
		overlap := end.Sub(start)
		if overlap > bestOverlap {
			best, bestOverlap = event, overlap
		}
	}

	return best
}
