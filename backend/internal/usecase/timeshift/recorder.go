package timeshift

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

const (
	// DefaultChunkDuration は、タイムシフトのチャンクの長さ (付録 A)。
	DefaultChunkDuration = 5 * time.Minute
	// DefaultTick は、つなぎ直すまで待つ間に時計を確かめる間隔と、割り振りが消えていないかを確かめる間隔。
	DefaultTick = time.Second
	// chunkTimeLayout は、チャンクのファイル名にする、チャンクの開始の UTC の書式。
	chunkTimeLayout = "20060102T150405Z"
	// chunkExt は、チャンクのファイルの拡張子。
	chunkExt = ".m2ts"
	// readPackets は、1 回に読む TS パケットの数。
	readPackets = 256
)

// DefaultBackoff は、ストリームが切れたときに、開き直すまでの間隔を、切れるたびに延ばしていく列 (付録 A)。最後の間隔を繰り返す。
func DefaultBackoff() []time.Duration {
	return []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
}

// RecorderOptions は Recorder の設定。0 の項目は既定値にする。
type RecorderOptions struct {
	ChunkDuration time.Duration
	// Backoff は、開き直すまでの間隔の列。空なら DefaultBackoff にする。
	Backoff []time.Duration
	// Tick は、時計と割り振りを確かめる間隔。
	Tick time.Duration
}

// Recorder は、タイムシフトの割り振りのサービスを、ChunkDuration ごとのチャンクに分けて録り続ける Session を提供する (設計書 6)。
// チャンクごとに、通常の録画と同じ索引を書き、EIT[p/f] で放送中の番組が替わった時刻を記録する。
type Recorder struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	tuner  recording.StreamOpener
	store  *kago.Store
	logger *slog.Logger
	opts   RecorderOptions
}

// NewRecorder は Recorder を返す。store には、タイムシフトのディレクトリとデータディレクトリが要る。opts は nil でもよい。
func NewRecorder(pool *pgxpool.Pool, clk clock.Clock, tuner recording.StreamOpener, store *kago.Store, logger *slog.Logger, opts *RecorderOptions) *Recorder {
	o := RecorderOptions{ChunkDuration: DefaultChunkDuration, Backoff: DefaultBackoff(), Tick: DefaultTick}
	if opts != nil {
		if opts.ChunkDuration > 0 {
			o.ChunkDuration = opts.ChunkDuration
		}
		if len(opts.Backoff) > 0 {
			o.Backoff = opts.Backoff
		}
		if opts.Tick > 0 {
			o.Tick = opts.Tick
		}
	}

	return &Recorder{pool: pool, clock: clk, tuner: tuner, store: store, logger: logger, opts: o}
}

// Session は、Worker に渡す Session を返す。
func (r *Recorder) Session() recording.Session {
	return r.record
}

// record は、割り振りのサービスを録り続ける。neko が割り振りを消したら、チャンクと番組を閉じて nil を返す。
// ctx が終わったら (リースを失ったか、mikan を止めるとき)、チャンクを閉じて返る。番組は放送中のままなので閉じない。
// ストリームが切れたら、チャンクを閉じ、Backoff の間隔で開き直す。開いてから ChunkDuration 以上録れたら、間隔を最初に戻す。
func (r *Recorder) record(ctx context.Context, assignment *recording.Assignment) error {
	logger := r.logger.With(slog.String("assignment_id", assignment.ID.String()), slog.Int64("service_id", assignment.ServiceID))
	// 落ちた mikan の代わりに取ったなら、書きかけのチャンクと番組を閉じる。
	// 止まる mikan から引き継いだなら、古い mikan がまだ書いていて、自分で閉じるので、閉じない。
	if !assignment.HandedOver {
		if err := r.closeStale(ctx, assignment.ServiceID); err != nil {
			return err
		}
	}
	programs, err := r.newProgramTracker(ctx, logger, assignment.ServiceID)
	if err != nil {
		return err
	}

	attempt := 0
	for {
		openedAt := r.clock.Now()
		deleted := r.recordStream(ctx, logger, assignment, programs)
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if deleted {
			return r.stopDeleted(ctx, logger, programs)
		}
		if r.clock.Now().Sub(openedAt) >= r.opts.ChunkDuration {
			attempt = 0
		}
		interval := r.opts.Backoff[min(attempt, len(r.opts.Backoff)-1)]
		attempt++
		deleted, err := r.waitUntil(ctx, assignment, openedAt.Add(interval))
		if err != nil {
			return err
		}
		if deleted {
			return r.stopDeleted(ctx, logger, programs)
		}
	}
}

// closeStale は、落ちた mikan が残した、書きかけのチャンクと、放送中のままの番組を、今の時刻で閉じる。
func (r *Recorder) closeStale(ctx context.Context, serviceID int64) error {
	now := r.clock.Now()
	q := sqlc.New(r.pool)
	if _, err := q.CloseStaleTimeshiftChunks(ctx, sqlc.CloseStaleTimeshiftChunksParams{EndedAt: &now, ServiceID: serviceID}); err != nil {
		return fmt.Errorf("failed to close the stale timeshift chunks: %w", err)
	}
	if _, err := q.EndTimeshiftProgram(ctx, sqlc.EndTimeshiftProgramParams{EndedAt: &now, ServiceID: serviceID}); err != nil {
		return fmt.Errorf("failed to end the stale timeshift program: %w", err)
	}

	return nil
}

// stopDeleted は、neko が割り振りを消したので、放送中の番組を閉じて録るのをやめる。録る mikan がいなくなるので、番組の終わりは今にする。
func (r *Recorder) stopDeleted(ctx context.Context, logger *slog.Logger, programs *programTracker) error {
	logger.InfoContext(ctx, "the timeshift assignment was deleted; stopping")
	programs.end(ctx, r.clock.Now())

	return nil
}

// waitUntil は、時計が until になるまで待つ。その間に割り振りが消えたら true を返す。ctx が終わったら、その原因を返す。
func (r *Recorder) waitUntil(ctx context.Context, assignment *recording.Assignment, until time.Time) (bool, error) {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for r.clock.Now().Before(until) {
		select {
		case <-ctx.Done():
			return false, context.Cause(ctx)
		case <-ticker.C:
		}
		if r.deleted(ctx, assignment) {
			return true, nil
		}
	}

	return false, nil
}

// deleted は、割り振りが DB から消えたかどうかを返す。確かめられなければ、消えていないとみなす。
func (r *Recorder) deleted(ctx context.Context, assignment *recording.Assignment) bool {
	exists, err := sqlc.New(r.pool).TimeshiftAssignmentExists(ctx, assignment.ID)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.WarnContext(ctx, "failed to check the timeshift assignment", slog.Any("error", err))
		}

		return false
	}

	return !exists
}

// recordStream は、サービスストリームを開き、切れるまでチャンクに書く。割り振りが消えて止めたなら true を返す。
func (r *Recorder) recordStream(ctx context.Context, logger *slog.Logger, assignment *recording.Assignment, programs *programTracker) bool {
	stream, err := r.tuner.OpenServiceStream(ctx, assignment.ServiceID, &mirakurun.StreamOptions{Decode: true, Priority: Priority})
	if err != nil {
		if ctx.Err() == nil {
			logger.WarnContext(ctx, "failed to open the service stream; retrying", slog.Any("error", err))
		}

		return false
	}
	// 引き継ぎ待ちの古い mikan は、ストリームを開いたことを見て、自分のストリームを閉じる (設計書 5.4)。
	err = sqlc.New(r.pool).MarkTimeshiftStreamOpened(ctx, sqlc.MarkTimeshiftStreamOpenedParams{
		Generation: assignment.Generation, Now: r.clock.Now(), ID: assignment.ID,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to record that the stream was opened", slog.Any("error", err))
	}

	// ctx が終わるか、割り振りが消えたら、ストリームを閉じて、読み込みを止める。
	var deleted atomic.Bool
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		r.watch(watchCtx, assignment, stream, &deleted)
	}()
	err = r.copyChunks(ctx, logger, assignment.ServiceID, stream, programs)
	stopWatch()
	<-watchDone
	_ = stream.Close()
	if ctx.Err() == nil && !deleted.Load() {
		logger.WarnContext(ctx, "the timeshift stream was interrupted; reconnecting", slog.Any("error", err))
	}

	return deleted.Load()
}

// watch は、ctx が終わるか、割り振りが消えるまで待ち、ストリームを閉じる。消えたなら deleted を立てる。
func (r *Recorder) watch(ctx context.Context, assignment *recording.Assignment, stream io.Closer, deleted *atomic.Bool) {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = stream.Close()

			return
		case <-ticker.C:
		}
		if r.deleted(ctx, assignment) {
			deleted.Store(true)
			_ = stream.Close()

			return
		}
	}
}

// copyChunks は、ストリームが切れるまで、チャンクに書く。ChunkDuration ごとに、TS パケットの区切りでチャンクを閉じて次を始める。
// チャンクは、データが届いてから作る。開いてすぐ切れ続けても、空のチャンクを作らない。返る前に、書いているチャンクを閉じる。
func (r *Recorder) copyChunks(ctx context.Context, logger *slog.Logger, serviceID int64, stream io.Reader, programs *programTracker) error {
	var current *chunk
	defer func() {
		if current != nil {
			r.closeChunk(context.WithoutCancel(ctx), logger, current, r.clock.Now())
		}
	}()
	buf := make([]byte, readPackets*ts.PacketSize)
	// carried は、前の読み込みの終わりで途切れた、パケットの前半の長さ。buf の先頭に置いておく。
	carried := 0
	for {
		n, readErr := stream.Read(buf[carried:])
		data := buf[:carried+n]
		whole := len(data) - len(data)%ts.PacketSize
		if whole > 0 {
			var err error
			if current, err = r.writePackets(ctx, logger, serviceID, current, data[:whole]); err != nil {
				return err
			}
			programs.observe(ctx, current.analyzer, r.clock.Now())
		}
		carried = copy(buf, data[whole:])
		if readErr != nil {
			return readErr
		}
	}
}

// writePackets は、TS パケットの区切りで揃えた packets を、チャンクに書き、書いたチャンクを返す。
// 書いているチャンクが ChunkDuration に達していれば、閉じて次を始める。current が nil なら、新しく始める。
// 書けなければ、書いていたチャンクを nil にせずに返すので、呼び出し元が閉じる。
func (r *Recorder) writePackets(ctx context.Context, logger *slog.Logger, serviceID int64, current *chunk, packets []byte) (*chunk, error) {
	now := r.clock.Now()
	if current != nil && now.Sub(current.startedAt) >= r.opts.ChunkDuration {
		// ctx が終わっても、書き終えたチャンクを閉じずに残さない。
		r.closeChunk(context.WithoutCancel(ctx), logger, current, now)
		current = nil
	}
	if current == nil {
		c, err := r.createChunk(ctx, logger, serviceID, now)
		if err != nil {
			return nil, err
		}
		current = c
	}

	return current, current.write(packets)
}

// chunk は、書いているチャンクのファイルと、その索引。
type chunk struct {
	id        uuid.UUID
	file      *os.File
	indexFile *os.File
	analyzer  *ts.Analyzer
	startedAt time.Time
	size      int64
}

// createChunk は、タイムシフトのディレクトリの、サービスのディレクトリにチャンクのファイルを作り、索引を作って、DB に記録する。
// ファイル名は、チャンクの開始の UTC にする。
func (r *Recorder) createChunk(ctx context.Context, logger *slog.Logger, serviceID int64, now time.Time) (*chunk, error) {
	file, rel, err := r.store.Create(kago.TimeshiftPoolID, strconv.FormatInt(serviceID, 10), now.UTC().Format(chunkTimeLayout)+chunkExt)
	if err != nil {
		return nil, fmt.Errorf("failed to create a timeshift chunk: %w", err)
	}
	// 索引は、データディレクトリの、チャンクの ID のディレクトリに置く。作れなくても、録り続ける。
	chunkID := id.New()
	analyzer, indexFile, indexErr := recording.CreateIndex(r.store, chunkID)
	if indexErr != nil {
		logger.WarnContext(ctx, "failed to create the index; recording without it", slog.String("path", rel), slog.Any("error", indexErr))
	}
	c := &chunk{id: chunkID, file: file, indexFile: indexFile, analyzer: analyzer, startedAt: now, size: 0}
	err = sqlc.New(r.pool).InsertTimeshiftChunk(ctx, sqlc.InsertTimeshiftChunkParams{
		ID: chunkID, ServiceID: serviceID, Path: rel, Size: 0, StartedAt: now, Now: now,
	})
	if err != nil {
		_ = c.file.Close()
		if c.indexFile != nil {
			_ = c.indexFile.Close()
		}
		// DB に記録のないファイルは、破棄の対象にならないので、消しておく。
		if path, pathErr := r.store.Path(kago.TimeshiftPoolID, rel); pathErr == nil {
			_ = os.Remove(path)
		}

		return nil, fmt.Errorf("failed to save the timeshift chunk: %w", err)
	}

	return c, nil
}

// write は、チャンクのファイルと解析器に p を書く。
func (c *chunk) write(p []byte) error {
	n, err := c.file.Write(p)
	c.size += int64(n)
	if err != nil {
		return fmt.Errorf("failed to write the timeshift chunk: %w", err)
	}
	// 解析器は、エラーを返さない。索引の書き込みのエラーは、解析器の中に残る。
	_, _ = c.analyzer.Write(p)

	return nil
}

// closeChunk は、チャンクのファイルと索引を書き終え、DB に大きさと終わりの時刻を記録する。
// 閉じられなくても、次のチャンクに書き続けるので、ログに残すだけにする。
func (r *Recorder) closeChunk(ctx context.Context, logger *slog.Logger, c *chunk, endedAt time.Time) {
	errs := []error{}
	if err := c.file.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync the timeshift chunk: %w", err))
	}
	if err := c.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close the timeshift chunk: %w", err))
	}
	if c.indexFile != nil {
		if err := c.indexFile.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync the index file: %w", err))
		}
		if err := c.indexFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close the index file: %w", err))
		}
	}
	_, err := sqlc.New(r.pool).CloseTimeshiftChunk(ctx, sqlc.CloseTimeshiftChunkParams{Size: c.size, EndedAt: &endedAt, ID: c.id})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to save the timeshift chunk size: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		logger.WarnContext(ctx, "failed to close the timeshift chunk", slog.String("chunk_id", c.id.String()), slog.Any("error", err))
	}
}

// programTracker は、EIT[p/f] の放送中の番組を追い、替わった時刻を timeshift_programs に記録する。
// チャンクを替えるたびと、つなぎ直すたびに解析器は替わるので、放送中の番組は、ここで持ち続ける。
type programTracker struct {
	recorder  *Recorder
	logger    *slog.Logger
	serviceID int64
	// sid は、EIT[p/f] の service_id。ok が false なら、サービスの ID が EIT の範囲に収まらないので、追わない。
	sid uint16
	ok  bool
	// current は、放送中として記録している番組の eventId。まだなければ nil。
	current *uint16
}

// newProgramTracker は、programTracker を返す。放送中として記録している番組があれば、その続きから追う。
// 引き継いだ mikan が、同じ番組を 2 つの行に分けないようにする。
func (r *Recorder) newProgramTracker(ctx context.Context, logger *slog.Logger, serviceID int64) (*programTracker, error) {
	sid, ok := ts.BroadcastServiceID(serviceID)
	t := &programTracker{recorder: r, logger: logger, serviceID: serviceID, sid: sid, ok: ok, current: nil}
	eventID, err := sqlc.New(r.pool).GetOpenTimeshiftProgram(ctx, serviceID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("failed to get the timeshift program on air: %w", err)
	case eventID >= 0 && eventID <= 0xFFFF:
		t.current = new(uint16(eventID))
	}

	return t, nil
}

// observe は、解析器の EIT[p/f] の放送中の番組が替わっていたら、前の番組を now で閉じ、新しい番組を now から始める。
// 番組名は programs から引き、なければ空にする。記録できなくても、録り続ける。
func (t *programTracker) observe(ctx context.Context, analyzer *ts.Analyzer, now time.Time) {
	if !t.ok {
		return
	}
	pf := analyzer.PresentFollowing(t.sid)
	if pf == nil || pf.Present == nil || (t.current != nil && *t.current == *pf.Present) {
		return
	}
	// 記録できなくても、同じ番組で何度もやり直さないよう、替わったものとする。
	t.current = new(*pf.Present)
	t.end(ctx, now)
	q := sqlc.New(t.recorder.pool)
	name := ""
	program, err := q.GetProgramForReservation(ctx, sqlc.GetProgramForReservationParams{ServiceID: t.serviceID, EventID: int64(*pf.Present)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		t.logger.WarnContext(ctx, "failed to get the program name", slog.Any("error", err))
	default:
		name = program.Name
	}
	err = q.StartTimeshiftProgram(ctx, sqlc.StartTimeshiftProgramParams{ServiceID: t.serviceID, StartedAt: now, EventID: int64(*pf.Present), Name: name})
	if err != nil {
		t.logger.WarnContext(ctx, "failed to record the timeshift program", slog.Any("error", err))
	}
}

// end は、放送中として記録している番組を now で閉じる。
func (t *programTracker) end(ctx context.Context, now time.Time) {
	_, err := sqlc.New(t.recorder.pool).EndTimeshiftProgram(ctx, sqlc.EndTimeshiftProgramParams{EndedAt: &now, ServiceID: t.serviceID})
	if err != nil {
		t.logger.WarnContext(ctx, "failed to end the timeshift program", slog.Any("error", err))
	}
}
