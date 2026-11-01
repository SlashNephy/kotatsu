package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// inspectPriority は、チャンネルの TS を調べるときの優先度 (#830)。負なので、ほかの利用者からチューナーを奪わない。
	inspectPriority = -1
	// DefaultInspectDuration は、チャンネルの TS を調べる長さ。PAT と PMT は 0.1 秒ほどの間隔で届くので、数秒あれば揃う。
	DefaultInspectDuration = 5 * time.Second
)

// ErrInspectionInterrupted は、調べる長さより前に、チャンネルのストリームが終わったときのエラー。
var ErrInspectionInterrupted = errors.New("the channel stream ended before the inspection finished")

// ChannelOpener は、チャンネルのストリームを開く。*mirakurun.Client が満たす。
type ChannelOpener interface {
	OpenChannelStream(ctx context.Context, channelType, channel string, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

var _ ChannelOpener = new(mirakurun.Client)

// InspectedStream は、調べた TS の、PID 1 つの様子。
type InspectedStream struct {
	PID uint16
	// Kind は、PMT と決まった PID から分かる種類 (ts.PIDKind*)。
	Kind string
	// StreamType は、PMT の stream_type。PMT に載っていなければ nil。
	StreamType *uint8
	BPS        int64
	Drops      int64
	Errors     int64
	Scrambled  int64
}

// InspectedProgram は、調べた TS の、番組 (サービス) 1 つの様子。
type InspectedProgram struct {
	// Number は program_number (ARIB ではサービス ID)。
	Number  uint16
	BPS     int64
	Streams []InspectedStream
}

// ChannelInspection は、チャンネルの TS を数秒受け取って調べた結果 (#830)。
type ChannelInspection struct {
	// TransportStreamID は、PAT の transport_stream_id。PAT を受け取れなければ nil。
	TransportStreamID *uint16
	// Duration は、実際に受け取った長さ。
	Duration time.Duration
	TotalBPS int64
	Programs []InspectedProgram
	// Others は、どの番組の PMT にも載っていない PID (PAT、EIT などの SI、ECM、EMM など)。
	Others []InspectedStream
}

// ChannelInspector は、物理チャンネルの TS をまるごと数秒受け取り、サービスごとの構成とビットレートを調べる (#830)。
// 調べる間、チューナーを 1 本使うので、管理者が求めたときだけ使う。
type ChannelInspector struct {
	opener ChannelOpener
	clock  clock.Clock
}

// NewChannelInspector は ChannelInspector を返す。opener が nil なら、Inspect は ErrNoTuner を返す。
func NewChannelInspector(opener ChannelOpener, clk clock.Clock) *ChannelInspector {
	return &ChannelInspector{opener: opener, clock: clk}
}

// Inspect は、チャンネル channelType/channel の TS を duration だけ受け取って調べる。
func (i *ChannelInspector) Inspect(ctx context.Context, channelType, channel string, duration time.Duration) (*ChannelInspection, error) {
	if i.opener == nil {
		return nil, ErrNoTuner
	}
	upstream, err := i.opener.OpenChannelStream(ctx, channelType, channel, &mirakurun.StreamOptions{Decode: true, Priority: inspectPriority})
	if err != nil {
		return nil, fmt.Errorf("failed to open the channel stream: %w", err)
	}
	defer func() {
		_ = upstream.Close()
	}()
	// 調べる長さは、開いてから数える。チューニングと応答を待つ時間で、受け取る長さを削らない。
	readCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	// 長さを過ぎたら、ストリームを閉じて読み込みを終わらせる。
	stop := context.AfterFunc(readCtx, func() { _ = upstream.Close() })
	defer stop()
	analyzer := ts.NewAnalyzer(nil)
	stopwatch := i.clock.StartStopwatch()
	_, copyErr := io.Copy(analyzer, upstream)
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case readCtx.Err() == nil:
		// 長さより前に終わった。録画や視聴にチューナーを奪われたか、接続が切れた。途中までの結果は、欠けているので返さない。
		return nil, fmt.Errorf("%w: %w", ErrInspectionInterrupted, copyErr)
	}

	return inspection(analyzer.Stats(), stopwatch.Elapsed()), nil
}

// inspection は、解析の統計 stats を、elapsed の間に受け取ったものとして、サービスごとの様子にまとめる。
func inspection(stats *ts.Stats, elapsed time.Duration) *ChannelInspection {
	bps := func(packets int64) int64 {
		if elapsed <= 0 {
			return 0
		}

		return packets * ts.PacketSize * 8 * int64(time.Second) / int64(elapsed)
	}
	qualities := ts.PIDQualities(stats, stats.PIDs)
	byPID := map[uint16]ts.PIDQuality{}
	for _, q := range qualities {
		byPID[q.PID] = q
	}
	toStream := func(q ts.PIDQuality) InspectedStream {
		return InspectedStream{
			PID: q.PID, Kind: q.Kind, StreamType: q.StreamType, BPS: bps(q.Packets), Drops: q.Drops, Errors: q.Errors, Scrambled: q.Scrambled,
		}
	}
	result := &ChannelInspection{
		TransportStreamID: stats.TransportStreamID, Duration: elapsed, TotalBPS: bps(stats.Packets),
		Programs: []InspectedProgram{}, Others: []InspectedStream{},
	}
	inProgram := map[uint16]bool{}
	for _, program := range stats.Programs {
		p := InspectedProgram{Number: program.Number, BPS: 0, Streams: []InspectedStream{}}
		for _, pid := range append([]uint16{program.PMTPID}, streamPIDs(program)...) {
			q, ok := byPID[pid]
			if !ok {
				continue
			}
			inProgram[pid] = true
			s := toStream(q)
			p.BPS += s.BPS
			p.Streams = append(p.Streams, s)
		}
		result.Programs = append(result.Programs, p)
	}
	for _, q := range qualities {
		if !inProgram[q.PID] {
			result.Others = append(result.Others, toStream(q))
		}
	}

	return result
}

// streamPIDs は、番組の ES の PID を、PMT に書かれた順に返す。
func streamPIDs(program *ts.Program) []uint16 {
	pids := make([]uint16, 0, len(program.Streams))
	for _, s := range program.Streams {
		pids = append(pids, s.PID)
	}

	return pids
}
