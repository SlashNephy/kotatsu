package recording

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// broadcastObserveTimeout は、遅れた番組を待つかを決めるために、EIT[p/f] を読むのを待つ時間 (#1046)。
	// EIT[p/f] は数秒ごとに届くので、これだけ待っても読めない TS では待たずに書き始め、取りこぼさないようにする。
	broadcastObserveTimeout = 10 * time.Second
	// maxPrerollBytes は、遅れた番組を待つ間にメモリに溜める大きさの上限。MaxPreroll の間に、ビットレートの高い放送でも収まる。
	maxPrerollBytes = 128 << 20
	// waitReadSize は、遅れた番組を待つ間に、ストリームを 1 回に読む大きさ。
	waitReadSize = 64 * ts.PacketSize
)

// prerolledStream は、溜めた分を先に読ませるストリーム。閉じると、元のストリームを閉じる。
type prerolledStream struct {
	io.Reader
	io.Closer
}

// prerollChunk は、遅れた番組を待つ間に読んだ TS の、パケットの区切りで切ったひとかたまり。
type prerollChunk struct {
	at   time.Time
	data []byte
}

// preroll は、遅れた番組を待つ間に読んだ TS の、直近の window の分を溜める (#1046)。
// 放送中になってから書き始めても、番組の前のマージンを残せるようにする。古いものから、パケットの区切りで捨てる。
type preroll struct {
	window time.Duration
	chunks []prerollChunk
	size   int
	// carry は、パケットの途中で読み終えた分。次に読んだ分とつないで、パケットの区切りで切る。
	carry []byte
}

func newPreroll(window time.Duration) *preroll {
	return &preroll{window: window, chunks: []prerollChunk{}, size: 0, carry: []byte{}}
}

// add は、at に読んだ b を溜め、window より前に読んだ分と、maxPrerollBytes を超えた分を捨てる。
func (p *preroll) add(at time.Time, b []byte) {
	p.carry = append(p.carry, b...)
	if whole := len(p.carry) - len(p.carry)%ts.PacketSize; whole > 0 {
		p.chunks = append(p.chunks, prerollChunk{at: at, data: bytes.Clone(p.carry[:whole])})
		p.size += whole
		p.carry = append(p.carry[:0], p.carry[whole:]...)
	}
	for len(p.chunks) > 0 && (p.chunks[0].at.Before(at.Add(-p.window)) || p.size > maxPrerollBytes) {
		p.size -= len(p.chunks[0].data)
		p.chunks = p.chunks[1:]
	}
}

// startedAt は、溜めた分の最初を読んだ時刻を返す。何も溜めていなければ now を返す。
func (p *preroll) startedAt(now time.Time) time.Time {
	if len(p.chunks) == 0 {
		return now
	}

	return p.chunks[0].at
}

// bytes は、溜めた分を、読んだ順につないで返す。
func (p *preroll) bytes() []byte {
	out := make([]byte, 0, p.size+len(p.carry))
	for _, chunk := range p.chunks {
		out = append(out, chunk.data...)
	}

	return append(out, p.carry...)
}

// waitOutcome は、waitBeforeWriting の結果。
type waitOutcome struct {
	// stream と startedAt は、書くストリーム (溜めた分を先に読ませる) と、その最初のデータを読んだ時刻。
	stream    io.ReadCloser
	startedAt time.Time
	// reopen は、ストリームが切れたので、開き直すこと。
	reopen bool
	// done は、録画を終えたか、止められたこと。err は、そのときに record が返すもの。
	done bool
	err  error
}

// waitBeforeWriting は、遅れた番組を待つときは、放送中になるまで書かずに待つ (#1046)。待った後は、溜めた直近の分から書く。
// 待たないときは、stream をそのまま、今の時刻から書く。created は、録画の書いたものがあるセグメントの数。
func (r *Recorder) waitBeforeWriting(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker, result *sessionResult,
	stream io.ReadCloser, created int,
) waitOutcome {
	outcome := waitOutcome{stream: stream, startedAt: r.clock.Now(), reopen: false, done: false, err: nil}
	if !tracker.mustWait(created, outcome.startedAt) {
		return outcome
	}
	buffered, end, err := r.waitForBroadcast(ctx, logger, tracker, stream)
	switch {
	case err != nil:
		_ = stream.Close()
		if ctx.Err() != nil {
			outcome.done, outcome.err = true, context.Cause(ctx)

			return outcome
		}
		logger.WarnContext(ctx, "the stream was interrupted while waiting for the program; reconnecting", slog.Any("error", err))
		result.lastFailure = FailureStreamInterrupted
		outcome.done, outcome.err = r.waitToReopen(ctx, logger, assignment, recording, tracker, result, r.opts.RetryInterval)
		outcome.reopen = !outcome.done

		return outcome
	case end != endNone:
		_ = stream.Close()
		if end == endPlanned {
			// 予定の終わり (延長の上限) まで、予約した番組が放送中にならなかった。前の番組の TS は残さない。
			result.lastFailure = FailureProgramNotFound
		}
		outcome.done, outcome.err = true, r.finish(ctx, logger, assignment, recording, end, result, false)

		return outcome
	}

	outcome.startedAt = buffered.startedAt(r.clock.Now())
	if tracker.waited {
		err := sqlc.New(r.pool).SetRecordingRecordedFrom(ctx, sqlc.SetRecordingRecordedFromParams{At: outcome.startedAt, Now: r.clock.Now(), ID: recording.ID})
		if err != nil {
			logger.WarnContext(ctx, "failed to record when the recording started", slog.Any("error", err))
		}
	}
	outcome.stream = &prerolledStream{Reader: io.MultiReader(bytes.NewReader(buffered.bytes()), stream), Closer: stream}

	return outcome
}

// readResult は、ストリームを 1 回読んだ結果。
type readResult struct {
	data []byte
	err  error
}

// waitForBroadcast は、ストリームを書かずに読み、予約した番組が放送中になるまで待つ (#1046)。待っている間は、直近の分を溜める。
// 次のときに待つのをやめ、溜めた分を返す。
//   - 予約した番組が放送中になったとき
//   - 予約した番組が、放送中にも次の番組にも出なくなったとき (eventId の差し替えなど)。取りこぼさないよう、その時点から書く
//   - broadcastObserveTimeout の間、EIT[p/f] を読めないとき
//
// 予定の終わり (延長の上限) が来たときと、予約を無効にしたときは、止めた理由を返す。ストリームが切れたら、エラーを返す。
func (r *Recorder) waitForBroadcast(
	ctx context.Context, logger *slog.Logger, tracker *endTracker, stream io.ReadCloser,
) (*preroll, endReason, error) {
	buffered := newPreroll(min(tracker.startMargin, MaxPreroll))
	analyzer := ts.NewAnalyzer(nil)
	tracker.watch(analyzer)
	since := r.clock.Now()

	// 読むのを別のゴルーチンにして、読めない間も、予定の終わりと予約の無効を確かめる。
	// 1 回ずつ読ませるので、待つのをやめた後は、ストリームを読む者はいない。
	requests := make(chan struct{})
	results := make(chan readResult)
	defer close(requests)
	go func() {
		for range requests {
			buf := make([]byte, waitReadSize)
			n, err := stream.Read(buf)
			results <- readResult{data: buf[:n], err: err}
		}
	}()

	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	requests <- struct{}{}
	for {
		select {
		case <-ctx.Done():
			// 読んでいる最中なら、閉じて読み終えさせる。
			_ = stream.Close()
			<-results

			return nil, endNone, context.Cause(ctx)
		case <-ticker.C:
			if end := tracker.check(ctx); end != endNone {
				_ = stream.Close()
				<-results

				return nil, end, nil
			}
			if r.stopWaiting(ctx, logger, tracker, since) {
				// 読んでいる最中の分も溜めてから返す。
				result := <-results
				buffered.add(r.clock.Now(), result.data)

				return buffered, endNone, nil
			}
		case result := <-results:
			now := r.clock.Now()
			buffered.add(now, result.data)
			if _, err := analyzer.Write(result.data); err != nil {
				return nil, endNone, fmt.Errorf("failed to analyze the stream: %w", err)
			}
			if result.err != nil {
				return nil, endNone, result.err
			}
			tracker.observeBroadcast(ctx, now)
			if r.stopWaiting(ctx, logger, tracker, since) {
				return buffered, endNone, nil
			}
			requests <- struct{}{}
		}
	}
}

// stopWaiting は、遅れた番組を待つのをやめて書き始めるかどうかを返す。予約した番組が次の番組にいると初めて分かったら、待ったことを記録する。
func (r *Recorder) stopWaiting(ctx context.Context, logger *slog.Logger, tracker *endTracker, since time.Time) bool {
	b := tracker.broadcast
	switch {
	case b.startedAt != nil:
		logger.InfoContext(ctx, "the delayed program is on the air; starting to write")

		return true
	case b.pending():
		if !tracker.waited {
			r.markWaited(ctx, logger, tracker)
		}

		return false
	case b.observed:
		logger.WarnContext(ctx, "the program is neither on the air nor following; starting to write")

		return true
	case !r.clock.Now().Before(since.Add(broadcastObserveTimeout)):
		logger.WarnContext(ctx, "no EIT[p/f] was found; starting to write")

		return true
	}

	return false
}

// markWaited は、録画に、遅れた番組を待ったことを記録する。予約した番組がすでに放送中になっていれば、待たない。
func (r *Recorder) markWaited(ctx context.Context, logger *slog.Logger, tracker *endTracker) {
	n, err := sqlc.New(r.pool).MarkRecordingWaitedForBroadcast(ctx, sqlc.MarkRecordingWaitedForBroadcastParams{Now: r.clock.Now(), ID: tracker.recordingID})
	if err != nil {
		logger.WarnContext(ctx, "failed to record that the recording waited for the program", slog.Any("error", err))

		return
	}
	tracker.waited = n > 0
	if tracker.waited {
		logger.InfoContext(ctx, "the program is delayed; waiting for it to go on the air")
	}
}

// discardSegments は、遅れた番組を待つ前に、それまでに書いたセグメントを消し、残ったセグメントの数 (0) を返す (#1046)。
// 予約した番組はまだ放送中になっていないので、セグメントには前の番組しか入っていない。
// ほかの mikan の記録で放送中になっていれば、消さずに created をそのまま返す。DB に書けなかったときも、消さずに created を返す。
// 次のセグメントで、また遅れを検知したときにやり直す。
func (r *Recorder) discardSegments(ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, tracker *endTracker, created int) int {
	now := r.clock.Now()
	marked := false
	var removed []sqlc.DeleteRecordingSegmentsRow
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		n, err := q.MarkRecordingWaitedForBroadcast(ctx, sqlc.MarkRecordingWaitedForBroadcastParams{Now: now, ID: recording.ID})
		if err != nil {
			return fmt.Errorf("failed to record that the recording waits for the program: %w", err)
		}
		if n == 0 {
			return nil
		}
		marked = true
		removed, err = q.DeleteRecordingSegments(ctx, recording.ID)
		if err != nil {
			return fmt.Errorf("failed to delete the recording segments: %w", err)
		}
		if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
			return fmt.Errorf("failed to notify the recording change: %w", err)
		}

		return nil
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to discard the segments of the previous program", slog.Any("error", err))

		return created
	}
	if !marked {
		// ほかの mikan が、予約した番組を放送中と記録した。この mikan のセグメントにも番組が入るので、待たずに書く。
		tracker.startedBefore = true

		return created
	}
	tracker.waited = true
	for i := range removed {
		// ファイルを消せなくても、記録は消したので、録画には含まれない。残ったファイルは、DB にないファイルとして見つかる (設計書 5.6)。
		if err := r.store.Remove(removed[i].PoolID, removed[i].Path, removed[i].ID); err != nil {
			logger.WarnContext(ctx, "failed to remove a discarded segment", slog.String("path", removed[i].Path), slog.Any("error", err))
		}
	}
	logger.InfoContext(ctx, "the program is delayed; discarded the segments of the previous program", slog.Int("segments", len(removed)))

	return 0
}
