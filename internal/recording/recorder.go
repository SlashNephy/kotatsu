package recording

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/ts"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// DefaultTuningTime は、チューニングにかかる時間の見込み (付録 A)。予定の開始よりこれだけ早くストリームを開く。
	DefaultTuningTime = 5 * time.Second
	// DefaultTick は、予定の時刻が来たかを確かめる間隔。予約の予定の時刻も、この間隔で読み直す。
	DefaultTick = 500 * time.Millisecond
	// DefaultRetryInterval は、ストリームを開けなかったときや、切れたときに、開き直すまで待つ時間。
	DefaultRetryInterval = 2 * time.Second
	// MaxHistoryDrops は、重複の回避で「録画済み」とみなすドロップの数の上限 (付録 A)。EDCB と同じく 2 件未満にする。
	MaxHistoryDrops = 1
	// ExtensionStep と MaxExtension は、延長中で終了時刻が未定の番組の、録画の延長幅と上限 (付録 A)。
	ExtensionStep = 5 * time.Minute
	MaxExtension  = 3 * time.Hour
	// writeBufferSize は、録画ファイルへの書き込みをまとめる大きさ (設計書 5.2)。
	writeBufferSize = 4 << 20
	// estimatedBytesPerSecond は、保存先を選ぶときに見込む、録画の 1 秒あたりの大きさ。地上波の HD の TS (約 17 Mbps) に余裕を持たせる。
	estimatedBytesPerSecond = 3_000_000
	// segmentExt は、録画ファイルの拡張子。
	segmentExt = ".m2ts"
	// indexFileName は、付属ファイルのディレクトリに置く、索引のファイルの名前。
	indexFileName = "index"
	// fileKindSegment と fileKindIndex は、録画のファイルの種類 (recording_files.kind)。
	fileKindSegment = "segment"
	fileKindIndex   = "index"
)

// FailureReason は、録画に失敗した理由 (設計書 5.5)。
type FailureReason string

const (
	FailureTunerUnavailable       FailureReason = "tuner_unavailable"
	FailureTunerServerUnreachable FailureReason = "tuner_server_unreachable"
	FailureStreamInterrupted      FailureReason = "stream_interrupted"
	FailureStorageUnavailable     FailureReason = "storage_unavailable"
	FailureStorageFull            FailureReason = "storage_full"
	FailureProgramNotFound        FailureReason = "program_not_found"
	FailureCancelled              FailureReason = "cancelled"
)

// State は、録画の状態 (設計書 5.5)。
type State string

const (
	StateRecording           State = "recording"
	StateCompleted           State = "completed"
	StateCompletedWithErrors State = "completed_with_errors"
	StatePartial             State = "partial"
	StateFailed              State = "failed"
)

// StreamOpener は、サービスのストリームを開く。*mirakurun.Client が満たす。
type StreamOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

var _ StreamOpener = new(mirakurun.Client)

// RecorderOptions は Recorder の設定。0 の項目は既定値にする。
type RecorderOptions struct {
	TuningTime    time.Duration
	Tick          time.Duration
	RetryInterval time.Duration
}

// Recorder は、割り振りの録画を行う Session を提供する (設計書 5.1、5.2、5.3)。
// 予定の開始からチューニングにかかる時間を引いた時刻にサービスストリームを開き、届いたデータをすべて最終的な保存先に書き込む。
// 書きながら TS を解析し、ドロップを数え、索引を作る。予定の終わりは、録画しながら予約を読み直して決める。
type Recorder struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	tuner    StreamOpener
	store    *kago.Store
	defaults assign.DefaultsFunc
	location *time.Location
	logger   *slog.Logger
	opts     RecorderOptions
}

// NewRecorder は Recorder を返す。defaults は録画設定の全体の既定値、location はファイル名に使う表示のタイムゾーン。opts は nil でもよい。
func NewRecorder(
	pool *pgxpool.Pool, clk clock.Clock, tuner StreamOpener, store *kago.Store, defaults assign.DefaultsFunc,
	location *time.Location, logger *slog.Logger, opts *RecorderOptions,
) *Recorder {
	if defaults == nil {
		defaults = assign.FixedDefaults(reserve.DefaultSettings())
	}
	o := RecorderOptions{TuningTime: DefaultTuningTime, Tick: DefaultTick, RetryInterval: DefaultRetryInterval}
	if opts != nil {
		if opts.RetryInterval > 0 {
			o.RetryInterval = opts.RetryInterval
		}
		if opts.TuningTime > 0 {
			o.TuningTime = opts.TuningTime
		}
		if opts.Tick > 0 {
			o.Tick = opts.Tick
		}
	}

	return &Recorder{pool: pool, clock: clk, tuner: tuner, store: store, defaults: defaults, location: location, logger: logger, opts: o}
}

// Session は、Worker に渡す Session を返す。
func (r *Recorder) Session() Session {
	return r.record
}

// record は、割り振りの予約を録画する。録画しなくてよくなったときと、録画を終えたときに nil を返す。
// 録画の失敗は、録画の状態と理由に記録し、割り振りは終わったことにする。
func (r *Recorder) record(ctx context.Context, assignment *Assignment) error {
	reservation, ok, err := r.waitUntilOpen(ctx, assignment)
	if err != nil || !ok {
		return err
	}
	logger := r.logger.With(slog.String("reservation_id", reservation.ID.String()), slog.String("name", reservation.Name))

	recording, err := r.start(ctx, &reservation)
	if err != nil {
		return err
	}
	logger = logger.With(slog.String("recording_id", recording.ID.String()))
	// 落ちた mikan の代わりに取ったなら、書きかけのセグメントを、最後に書き込んだ時刻で閉じる (設計書 5.4)。
	// 止まる mikan から引き継いだなら、古い mikan がまだ書いているので、閉じない。
	if !assignment.HandedOver {
		if err := r.closeStaleFiles(ctx, recording.ID, r.clock.Now()); err != nil {
			return err
		}
	}

	tracker := newEndTracker(r, &recording, &reservation)
	result := &sessionResult{lastFailure: ""}
	for {
		stream, err := r.tuner.OpenServiceStream(ctx, reservation.ServiceID, &mirakurun.StreamOptions{Decode: true, Priority: assignment.Priority})
		if err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			// チューナーが空くか、チューナーサーバーにつながるまで、予定の終わりまで開き直す。
			result.lastFailure = streamFailure(err)
			logger.WarnContext(ctx, "failed to open the service stream; retrying", slog.Any("error", err))
			if done, err := r.waitToReopen(ctx, logger, assignment, &recording, tracker, result, r.opts.RetryInterval); done {
				return err
			}

			continue
		}
		// 引き継ぎ待ちの古い mikan は、ストリームを開いたことを見て、自分のストリームを閉じる (設計書 5.4)。
		err = sqlc.New(r.pool).MarkRecordingStreamOpened(ctx, sqlc.MarkRecordingStreamOpenedParams{
			Generation: assignment.Generation, Now: r.clock.Now(), ID: assignment.ID,
		})
		if err != nil {
			logger.WarnContext(ctx, "failed to record that the stream was opened", slog.Any("error", err))
		}

		segment, reason, err := r.createSegment(ctx, &recording, &reservation)
		if err != nil {
			_ = stream.Close()
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			logger.WarnContext(ctx, "failed to create a recording file", slog.Any("error", err))

			return r.fail(ctx, assignment, &recording, reason)
		}
		logger.InfoContext(ctx, "started recording", slog.String("pool_id", segment.poolID), slog.String("path", segment.path))
		openedAt := r.clock.Now()

		stats, end, copyErr := r.copyUntilEnd(ctx, tracker, stream, segment)
		_ = stream.Close()
		closeErr := r.closeSegment(context.WithoutCancel(ctx), &recording, segment, stats)
		switch {
		case ctx.Err() != nil:
			// リースを失ったか、止めるときは、録画を録画中のまま残し、続きを録る mikan に任せる。
			return errors.Join(context.Cause(ctx), closeErr)
		case closeErr != nil:
			logger.WarnContext(ctx, "failed to close the recording file", slog.Any("error", closeErr))

			return r.fail(ctx, assignment, &recording, FailureStorageUnavailable)
		case copyErr != nil:
			// ストリームが切れたら、開き直して、新しいセグメントに書く (設計書 5.2)。
			logger.WarnContext(ctx, "the stream was interrupted; reconnecting", slog.Any("error", copyErr))
			result.lastFailure = FailureStreamInterrupted
			// 開いてすぐに切れ続けると、セグメントを作り続けるので、開き直すのは RetryInterval に 1 回までにする。
			// 長く録れていたなら、待たずに開き直し、途切れる区間を短くする。
			wait := r.opts.RetryInterval - r.clock.Now().Sub(openedAt)
			if done, err := r.waitToReopen(ctx, logger, assignment, &recording, tracker, result, wait); done {
				return err
			}

			continue
		}

		return r.finish(ctx, logger, assignment, &recording, end, result, false)
	}
}

// waitToReopen は、ストリームを開き直す前に d だけ待つ。d が 0 以下なら待たない。
// 待つ前か、待つ間に録画を止めるときが来たら、録画を終え、true と、録画を終えた結果を返す。
func (r *Recorder) waitToReopen(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker, result *sessionResult, d time.Duration,
) (bool, error) {
	if d <= 0 {
		// 予定の終わりに、チューナーサーバーがストリームを閉じることもあるので、開き直す前に、止めるときかを確かめる。
		// 切れてすぐなので、途切れた区間は足さない。
		end := tracker.check(ctx)
		if end == endNone {
			return false, nil
		}

		return true, r.finish(ctx, logger, assignment, recording, end, result, false)
	}
	end, err := tracker.sleep(ctx, d)
	if err != nil {
		return true, err
	}
	if end == endNone {
		return false, nil
	}

	// つながらないまま終えるので、最後のセグメントの終わりから今までも、途切れた区間にする。
	return true, r.finish(ctx, logger, assignment, recording, end, result, true)
}

// sessionResult は、この mikan が録画を担当した間の結果。
type sessionResult struct {
	// lastFailure は、最後にストリームを開けなかったか、切れた理由。
	lastFailure FailureReason
}

// closeStaleFiles は、録画の書きかけのファイルのうち、staleBefore より前から書き込みのないものを、落ちた mikan が残したものとして、
// ファイルの大きさと、最後に書き込んだ時刻で閉じる。
func (r *Recorder) closeStaleFiles(ctx context.Context, recordingID uuid.UUID, staleBefore time.Time) error {
	q := sqlc.New(r.pool)
	files, err := q.ListOpenRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the open recording files: %w", err)
	}
	for i := range files {
		// ファイルを読めなければ、書き込んだ時刻も分からないので、記録している大きさのまま、書き始めた時刻で閉じる。
		size, writtenAt := files[i].Size, files[i].StartedAt
		if path, err := r.store.Path(files[i].PoolID, files[i].Path); err == nil {
			if info, err := os.Stat(path); err == nil {
				size, writtenAt = info.Size(), maxTime(info.ModTime(), files[i].StartedAt)
			}
		}
		if !writtenAt.Before(staleBefore) {
			continue
		}
		if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: size, EndedAt: &writtenAt, ID: files[i].ID}); err != nil {
			return fmt.Errorf("failed to close an abandoned recording file: %w", err)
		}
	}

	return nil
}

// finish は、録画を止めた理由と、セグメントの時間の範囲から、途切れた区間と録画の最後の状態を決めて記録する (設計書 5.2、5.5)。
// 途切れた区間は、書けたセグメントの時間の範囲のすき間とする。引き継ぎで重なった範囲は、途切れていない。
// ストリームにつながらないまま終えるなら (disconnected)、最後のセグメントの終わりから今までも、途切れた区間とする。
func (r *Recorder) finish(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, end endReason, result *sessionResult, disconnected bool,
) error {
	if end == endCancelled {
		logger.InfoContext(ctx, "the reservation was disabled or deleted while recording")

		return r.fail(ctx, assignment, recording, FailureCancelled)
	}
	// 落ちた mikan が残した書きかけのファイルを閉じる。止まる mikan は、ストリームを開いたのを見てから、RenewInterval ごとに確かめて止まるので、
	// それより長く書き込みのないファイルは、落ちた mikan のものとする。
	now := r.clock.Now()
	if err := r.closeStaleFiles(ctx, recording.ID, now.Add(-2*DefaultRenewInterval)); err != nil {
		return err
	}
	files, err := sqlc.New(r.pool).ListRecordingFiles(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	gaps, recorded := gapsOf(files, now, disconnected)
	if !recorded {
		reason := result.lastFailure
		if reason == "" {
			reason = FailureStreamInterrupted
		}
		logger.WarnContext(ctx, "nothing was recorded", slog.String("reason", string(reason)))

		return r.fail(ctx, assignment, recording, reason)
	}
	encoded, err := json.Marshal(gaps)
	if err != nil {
		return fmt.Errorf("failed to encode the recording gaps: %w", err)
	}
	if err := sqlc.New(r.pool).SetRecordingGaps(ctx, sqlc.SetRecordingGapsParams{Gaps: encoded, Now: now, ID: recording.ID}); err != nil {
		return fmt.Errorf("failed to record the gaps: %w", err)
	}

	// 引き継ぎや再開で、前のセグメントにドロップがあることもあるので、録画全体の集計で決める。
	finished, err := sqlc.New(r.pool).GetRecording(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	finalState := StateCompleted
	switch {
	case len(gaps) > 0:
		finalState = StatePartial
	case finished.Drops > 0:
		finalState = StateCompletedWithErrors
	}
	if err := r.complete(ctx, assignment, recording.ID, finalState, ""); err != nil {
		return err
	}
	logger.InfoContext(ctx, "finished recording", slog.String("state", string(finalState)))

	return nil
}

// gap は、途切れた区間。
type gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// gapsOf は、書けたセグメントの時間の範囲のすき間を、途切れた区間として返す。書けたセグメントがなければ false を返す。
// 書いている途中のセグメントは、now まで書いているものとする。trailing なら、最後のセグメントの終わりから now までも途切れた区間にする。
func gapsOf(files []sqlc.RecordingFile, now time.Time, trailing bool) ([]gap, bool) {
	type span struct{ from, to time.Time }
	spans := []span{}
	writing := false
	for i := range files {
		file := &files[i]
		switch {
		case file.Kind != fileKindSegment:
		case file.EndedAt == nil:
			writing = true
			spans = append(spans, span{from: file.StartedAt, to: now})
		case file.Size > 0:
			spans = append(spans, span{from: file.StartedAt, to: *file.EndedAt})
		}
	}
	if len(spans) == 0 {
		return []gap{}, false
	}
	slices.SortFunc(spans, func(a, b span) int { return a.from.Compare(b.from) })
	gaps := []gap{}
	covered := spans[0].to
	for _, s := range spans[1:] {
		if s.from.After(covered) {
			gaps = append(gaps, gap{From: covered, To: s.from})
		}
		covered = maxTime(covered, s.to)
	}
	if trailing && !writing && covered.Before(now) {
		gaps = append(gaps, gap{From: covered, To: now})
	}

	return gaps, true
}

// waitUntilOpen は、予定の開始からチューニングにかかる時間を引いた時刻まで待ち、そのときの予約を返す。
// 待つ間に予約を無効にしたか、予定の終わりが過ぎたら、録画しないので false を返す。
func (r *Recorder) waitUntilOpen(ctx context.Context, assignment *Assignment) (sqlc.GetReservationForRecordingRow, bool, error) {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for {
		reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, assignment.ReservationID)
		if errors.Is(err, pgx.ErrNoRows) {
			// 録画を始める前に予約が消された。無効にしたときと同じく、録画せずに終える。
			return reservation, false, nil
		}
		if err != nil {
			return reservation, false, fmt.Errorf("failed to get the reservation: %w", err)
		}
		if !reservation.Enabled || reservation.PlannedStartAt == nil || reservation.PlannedEndAt == nil {
			return reservation, false, nil
		}
		now := r.clock.Now()
		if !now.Before(*reservation.PlannedEndAt) {
			return reservation, false, nil
		}
		if !now.Before(reservation.PlannedStartAt.Add(-r.opts.TuningTime)) {
			return reservation, true, nil
		}
		select {
		case <-ctx.Done():
			return reservation, false, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

// segment は、書いている録画ファイルと、その索引。
type segment struct {
	poolID    string
	path      string
	indexPath string
	fileID    uuid.UUID
	indexID   uuid.UUID
	file      *os.File
	indexFile *os.File
	buffer    *bufio.Writer
	analyzer  *ts.Analyzer
	startedAt time.Time
}

// createSegment は、保存先を選んで録画ファイルと索引を作り、DB に記録する。失敗したら、録画の失敗の理由も返す。
func (r *Recorder) createSegment(
	ctx context.Context, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow,
) (*segment, FailureReason, error) {
	settings, err := r.settings(ctx, reservation)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	mounted := r.store.PoolIDs()
	if (settings.PoolID != "" && !slices.Contains(mounted, settings.PoolID)) || len(mounted) == 0 {
		return nil, FailureStorageUnavailable, fmt.Errorf("%w: %q", kago.ErrPoolNotMounted, settings.PoolID)
	}
	pools, err := r.pools(ctx, mounted, settings.PoolID)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	estimated := uint64(max(reservation.PlannedEndAt.Sub(*reservation.PlannedStartAt), 0)/time.Second) * estimatedBytesPerSecond
	pool, err := r.store.Select(pools, estimated)
	if errors.Is(err, kago.ErrNoPoolFits) {
		return nil, FailureStorageFull, err
	}
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}

	tmpl, err := reserve.ParseFileNameTemplate(settings.FileNameTemplate)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	name, err := kago.RenderName(tmpl, &reserve.FileNameData{
		StartAt: reservation.StartAt.In(r.location), EndAt: reservation.EndAt.In(r.location), ServiceName: reservation.ServiceName,
		ChannelType: reservation.ChannelType, Channel: reservation.Channel, Name: reservation.Name, EventID: valueOr(reservation.EventID),
	}, segmentExt)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	file, rel, err := r.store.Create(pool.ID, "", name)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	// 索引は、録画ファイルの横の付属ファイルのディレクトリに置く。作れなくても、録画は続ける。
	analyzer, indexFile, indexPath, indexErr := r.createIndex(pool.ID, rel)
	if indexErr != nil {
		r.logger.WarnContext(ctx, "failed to create the index; recording without it", slog.String("path", rel), slog.Any("error", indexErr))
	}
	seg := &segment{
		poolID: pool.ID, path: rel, indexPath: indexPath, fileID: uuid.Nil, indexID: uuid.Nil, file: file, indexFile: indexFile,
		buffer: bufio.NewWriterSize(file, writeBufferSize), analyzer: analyzer, startedAt: r.clock.Now(),
	}

	q := sqlc.New(r.pool)
	seg.fileID = id.New()
	err = q.InsertRecordingFile(ctx, sqlc.InsertRecordingFileParams{
		ID: seg.fileID, RecordingID: recording.ID, Kind: fileKindSegment, PoolID: pool.ID, Path: rel, StartedAt: seg.startedAt, Now: seg.startedAt,
	})
	if err != nil {
		_ = file.Close()
		if seg.indexFile != nil {
			_ = seg.indexFile.Close()
		}

		return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the recording file: %w", err)
	}
	if seg.indexFile != nil {
		seg.indexID = id.New()
		err = q.InsertRecordingFile(ctx, sqlc.InsertRecordingFileParams{
			ID: seg.indexID, RecordingID: recording.ID, Kind: fileKindIndex, PoolID: pool.ID, Path: seg.indexPath, StartedAt: seg.startedAt, Now: seg.startedAt,
		})
		if err != nil {
			_ = file.Close()
			_ = seg.indexFile.Close()
			// 記録したセグメントは、書いていないものとして閉じておく。
			now := r.clock.Now()
			if closeErr := q.CloseRecordingFile(context.WithoutCancel(ctx), sqlc.CloseRecordingFileParams{Size: 0, EndedAt: &now, ID: seg.fileID}); closeErr != nil {
				r.logger.WarnContext(ctx, "failed to close the recording file", slog.Any("error", closeErr))
			}

			return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the index file: %w", err)
		}
	}

	return seg, "", nil
}

// pools は、録画を書く候補のプールを、優先順位と容量の閾値の設定と一緒に返す (設計書 5.6)。
// 録画設定でプールを指定していれば、そのプールだけを返す。設定のないプールは、優先順位 0、容量の閾値 0 とする。
func (r *Recorder) pools(ctx context.Context, mounted []string, poolID string) ([]*kago.Pool, error) {
	rows, err := sqlc.New(r.pool).ListStoragePools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage pools: %w", err)
	}
	settings := make(map[string]*kago.Pool, len(rows))
	for i := range rows {
		settings[rows[i].ID] = &kago.Pool{ID: rows[i].ID, Priority: int(rows[i].Priority), MinFreeBytes: uint64(max(rows[i].MinFreeBytes, 0))}
	}
	candidates := mounted
	if poolID != "" {
		candidates = []string{poolID}
	}
	pools := make([]*kago.Pool, 0, len(candidates))
	for _, id := range candidates {
		pool, ok := settings[id]
		if !ok {
			pool = &kago.Pool{ID: id, Priority: 0, MinFreeBytes: 0}
		}
		pools = append(pools, pool)
	}

	return pools, nil
}

// createIndex は、録画ファイル rel の付属ファイルのディレクトリに索引を作り、索引を書く解析器を返す。
// 索引を作れなければ、索引を書かない解析器と、そのエラーを返す。
func (r *Recorder) createIndex(poolID, rel string) (*ts.Analyzer, *os.File, string, error) {
	dir, err := r.store.CreateSidecarDir(poolID, rel)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", err
	}
	indexRel := dir + "/" + indexFileName
	path, err := r.store.Path(poolID, indexRel)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", fmt.Errorf("failed to create the index file: %w", err)
	}
	writer, err := ts.NewIndexWriter(file)
	if err != nil {
		_ = file.Close()

		return ts.NewAnalyzer(nil), nil, "", err
	}

	return ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer}), file, indexRel, nil
}

// endReason は、録画を止めた理由。
type endReason int

const (
	// endNone は、まだ止めていないことを表す。
	endNone endReason = iota
	// endPlanned は、予定の終わりが来たことを表す。
	endPlanned
	// endCancelled は、録画中に予約を無効にしたことを表す。
	endCancelled
)

// copyUntilEnd は、予定の終わりが来るか、予約を無効にするまで、ストリームを録画ファイルと解析器に書き、止めた理由を返す。
// 予定の終わりは、予約を読み直して決める。その前にストリームが切れたら、エラーを返す。
func (r *Recorder) copyUntilEnd(ctx context.Context, tracker *endTracker, stream io.ReadCloser, seg *segment) (*ts.Stats, endReason, error) {
	ended := make(chan endReason, 1)
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		if reason := tracker.wait(watchCtx); reason != endNone {
			ended <- reason
			// 読み込み中のストリームを閉じて、書き込みを止める。
			_ = stream.Close()
		}
	}()

	_, err := io.Copy(io.MultiWriter(seg.buffer, seg.analyzer), stream)
	stopWatch()
	<-watchDone
	stats := seg.analyzer.Stats()
	select {
	case reason := <-ended:
		return stats, reason, nil
	default:
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}

	return stats, endNone, err
}

// endTracker は、録画中の予約と番組を読み直して、録画を止める時刻を決める (設計書 5.1)。
//   - 予定の終わりは、neko が EPG に合わせて計算し直す予約の予定の終わりにする。
//   - 番組の終了時刻が未定 (延長中) なら、予定の終わりが来るたびに ExtensionStep ずつ延ばす。延ばすのは、録画を始めたときの予定の終わりから MaxExtension まで。
//   - 番組が EPG から消えたら、予定の終わりまで録り続け、録画に番組を見失ったと記録する。
type endTracker struct {
	recorder      *Recorder
	recordingID   uuid.UUID
	reservationID uuid.UUID
	serviceID     int64
	eventID       *int64
	// extendedUntil は、延長中に延ばした終わり。延ばしていなければゼロ値。
	extendedUntil time.Time
	// limit は、延ばせる終わりの上限。
	limit time.Time
	lost  bool
}

func newEndTracker(r *Recorder, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow) *endTracker {
	return &endTracker{
		recorder: r, recordingID: recording.ID, reservationID: reservation.ID, serviceID: reservation.ServiceID, eventID: reservation.EventID,
		extendedUntil: time.Time{}, limit: reservation.PlannedEndAt.Add(MaxExtension), lost: false,
	}
}

// wait は、録画を止めるときまで待ち、その理由を返す。ctx が終わったら endNone を返す。
func (t *endTracker) wait(ctx context.Context) endReason {
	ticker := time.NewTicker(t.recorder.opts.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return endNone
		case <-ticker.C:
		}
		if reason := t.check(ctx); reason != endNone {
			return reason
		}
	}
}

// sleep は、d の間待つ。その間に録画を止めるときが来たら、その理由を返す。
func (t *endTracker) sleep(ctx context.Context, d time.Duration) (endReason, error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	ticker := time.NewTicker(t.recorder.opts.Tick)
	defer ticker.Stop()
	for {
		if reason := t.check(ctx); reason != endNone {
			return reason, nil
		}
		select {
		case <-ctx.Done():
			return endNone, context.Cause(ctx)
		case <-timer.C:
			return endNone, nil
		case <-ticker.C:
		}
	}
}

// check は、今が録画を止めるときかを確かめる。読めなければ、録画を続ける。
func (t *endTracker) check(ctx context.Context) endReason {
	r := t.recorder
	q := sqlc.New(r.pool)
	reservation, err := q.GetReservationForRecording(ctx, t.reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		// 録画中に予約が消された。無効にしたときと同じく、取り消しとして終える。
		return endCancelled
	}
	if err != nil {
		if ctx.Err() == nil {
			r.logger.WarnContext(ctx, "failed to reload the reservation", slog.Any("error", err))
		}

		return endNone
	}
	// 割り当てを消されても (予定の時刻が NULL)、無効にしたのでなければ、取り消しとは扱わない。
	if !reservation.Enabled {
		return endCancelled
	}
	if reservation.PlannedEndAt == nil {
		return endPlanned
	}
	now := r.clock.Now()
	end := *reservation.PlannedEndAt
	if t.eventID != nil {
		program, err := q.GetProgramEnd(ctx, sqlc.GetProgramEndParams{ServiceID: t.serviceID, EventID: *t.eventID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			t.markLost(ctx)
		case err != nil:
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to reload the program", slog.Any("error", err))
			}
		case program.EndUndecided:
			// 延長中は、終わりが来るたびに延ばす。
			if !now.Before(end) && !now.Before(t.extendedUntil) {
				t.extendedUntil = minTime(now.Add(ExtensionStep), t.limit)
				r.logger.InfoContext(ctx, "the program end is undecided; extending the recording",
					slog.String("recording_id", t.recordingID.String()), slog.Time("until", t.extendedUntil))
			}
			end = maxTime(end, t.extendedUntil)
		}
	}
	if !now.Before(end) {
		return endPlanned
	}

	return endNone
}

// markLost は、番組を見失ったことを、録画に一度だけ記録する。
func (t *endTracker) markLost(ctx context.Context) {
	if t.lost {
		return
	}
	r := t.recorder
	if err := sqlc.New(r.pool).MarkRecordingProgramLost(ctx, sqlc.MarkRecordingProgramLostParams{Now: r.clock.Now(), ID: t.recordingID}); err != nil {
		r.logger.WarnContext(ctx, "failed to record that the program was lost", slog.Any("error", err))

		return
	}
	t.lost = true
	r.logger.WarnContext(ctx, "the program disappeared from the EPG; recording until the planned end", slog.String("recording_id", t.recordingID.String()))
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

// closeSegment は、録画ファイルと索引を書き終え、DB に大きさと時間の範囲を記録し、ドロップを録画の集計に足す。
func (r *Recorder) closeSegment(ctx context.Context, recording *sqlc.Recording, seg *segment, stats *ts.Stats) error {
	now := r.clock.Now()
	errs := []error{}
	// 書いたデータを確実に残すため、閉じる前に fsync する (設計書 5.2)。
	if err := seg.buffer.Flush(); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush the recording file: %w", err))
	}
	if err := seg.file.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync the recording file: %w", err))
	}
	if err := seg.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close the recording file: %w", err))
	}
	q := sqlc.New(r.pool)
	if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: stats.Bytes, EndedAt: &now, ID: seg.fileID}); err != nil {
		errs = append(errs, fmt.Errorf("failed to save the recording file size: %w", err))
	}
	if seg.indexFile != nil {
		// 索引は、なくても再生できるので、書けなくても録画の失敗にしない。
		var indexSize int64
		if err := seg.indexFile.Sync(); err != nil {
			r.logger.WarnContext(ctx, "failed to sync the index file", slog.Any("error", err))
		}
		if info, err := seg.indexFile.Stat(); err == nil {
			indexSize = info.Size()
		}
		if err := seg.indexFile.Close(); err != nil {
			r.logger.WarnContext(ctx, "failed to close the index file", slog.Any("error", err))
		}
		if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: indexSize, EndedAt: &now, ID: seg.indexID}); err != nil {
			r.logger.WarnContext(ctx, "failed to save the index file size", slog.Any("error", err))
		}
	}
	var errorsCount, scrambled int64
	for _, pid := range stats.PIDs {
		errorsCount += pid.Errors
		scrambled += pid.Scrambled
	}
	err := q.AddRecordingStats(ctx, sqlc.AddRecordingStatsParams{Drops: drops(stats), Errors: errorsCount, Scrambled: scrambled, Now: now, ID: recording.ID})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to save the recording stats: %w", err))
	}

	return errors.Join(errs...)
}

// fail は、録画を失敗にし、理由を記録する。割り振りは終わったことにするので、nil を返す。
func (r *Recorder) fail(ctx context.Context, assignment *Assignment, recording *sqlc.Recording, reason FailureReason) error {
	return r.complete(ctx, assignment, recording.ID, StateFailed, reason)
}

// start は、予約の録画を始め、新しく始めたときは、録画の開始を通知する (設計書 6.2)。引き継ぎや再開で続きを録るときは通知しない。
func (r *Recorder) start(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) (sqlc.Recording, error) {
	var recording sqlc.Recording
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		started, err := sqlc.New(tx).StartRecording(ctx, sqlc.StartRecordingParams{
			ID: id.New(), ReservationID: &reservation.ID, ServiceID: reservation.ServiceID, EventID: reservation.EventID,
			Name: reservation.Name, Description: reservation.Description, StartAt: reservation.StartAt, EndAt: reservation.EndAt, Now: r.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to start the recording: %w", err)
		}
		recording = started.Recording
		if !started.Inserted {
			return nil
		}

		return notification.Emit(ctx, tx, r.clock, notification.KindRecordingStarted, map[string]any{
			"recordingId": recording.ID.String(), "name": recording.Name, "serviceName": reservation.ServiceName,
			"startAt": recording.StartAt, "endAt": recording.EndAt,
		})
	})

	return recording, err
}

// complete は、録画を state で終え、正常に終わってドロップが閾値以下なら録画履歴に記録し (設計書 4.4)、
// 録画の完了か失敗を通知する (設計書 6.2)。完了の通知には、Webhook のためにセグメントのプール内のパスを載せる。
func (r *Recorder) complete(ctx context.Context, assignment *Assignment, recordingID uuid.UUID, state State, reason FailureReason) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// 引き継ぎの最中に予定の終わりが来ると、古い mikan と新しい mikan の両方が録画を終えようとする。
		// 録画を終えるのは、今の世代の mikan だけにする。
		generation, err := q.LockRecordingAssignmentGeneration(ctx, assignment.ID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("failed to lock the recording assignment: %w", err)
		case generation != assignment.Generation:
			return errSuperseded
		}
		now := r.clock.Now()
		var failure *string
		if reason != "" {
			failure = new(string(reason))
		}
		if err := q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(state), FailureReason: failure, Now: &now, ID: recordingID}); err != nil {
			return fmt.Errorf("failed to finish the recording: %w", err)
		}
		row, err := q.GetRecordingWithService(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		recording := row.Recording
		payload := map[string]any{
			"recordingId": recording.ID.String(), "name": recording.Name, "serviceName": row.ServiceName,
			"startAt": recording.StartAt, "endAt": recording.EndAt, "state": string(state),
		}
		if state == StateFailed {
			payload["reason"] = string(reason)

			return notification.Emit(ctx, tx, r.clock, notification.KindRecordingFailed, payload)
		}

		if (state == StateCompleted || state == StateCompletedWithErrors) && recording.Drops <= MaxHistoryDrops {
			err := q.InsertRecordingHistory(ctx, sqlc.InsertRecordingHistoryParams{
				ID: id.New(), ServiceID: recording.ServiceID, TitleKey: rule.TitleKey(recording.Name), Name: recording.Name,
				StartAt: recording.StartAt, EndAt: recording.EndAt, Now: now,
			})
			if err != nil {
				return fmt.Errorf("failed to save the recording history: %w", err)
			}
		}
		segments, err := q.ListRecordingSegments(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to list the recording files: %w", err)
		}
		files := make([]map[string]string, 0, len(segments))
		for _, segment := range segments {
			files = append(files, map[string]string{"poolId": segment.PoolID, "path": segment.Path})
		}
		payload["files"] = files

		return notification.Emit(ctx, tx, r.clock, notification.KindRecordingCompleted, payload)
	})
	if errors.Is(err, errSuperseded) {
		r.logger.InfoContext(ctx, "another mikan took over the recording; leaving it to finish", slog.String("recording_id", recordingID.String()))

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to complete the recording: %w", err)
	}

	return nil
}

// errSuperseded は、ほかの mikan が割り振りを取ったので、録画を終えないことを表す。
var errSuperseded = errors.New("the recording assignment was taken over")

// settings は、予約の録画設定を、全体の既定値に、ルール予約ならルールの上書き、さらに予約自体の上書きを重ねて求める。
func (r *Recorder) settings(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) (*reserve.Settings, error) {
	q := sqlc.New(r.pool)
	settings, err := r.defaults(ctx, r.pool)
	if err != nil {
		return nil, err
	}
	if reserve.Kind(reservation.Kind) == reserve.KindRule {
		rows, err := q.ListFirstRuleSettings(ctx, []uuid.UUID{reservation.ID})
		if err != nil {
			return nil, fmt.Errorf("failed to list rule settings: %w", err)
		}
		for _, row := range rows {
			overrides, err := reserve.DecodeOverrides(row.Settings)
			if err != nil {
				return nil, err
			}
			settings = settings.Resolve(overrides)
		}
	}
	overrides, err := reserve.DecodeOverrides(reservation.Settings)
	if err != nil {
		return nil, err
	}

	return settings.Resolve(overrides), nil
}

// streamFailure は、ストリームを開けなかったエラーを、録画の失敗の理由にする。
func streamFailure(err error) FailureReason {
	if apiErr, ok := errors.AsType[*mirakurun.APIError](err); ok {
		if apiErr.StatusCode == http.StatusServiceUnavailable {
			return FailureTunerUnavailable
		}

		return FailureStreamInterrupted
	}

	return FailureTunerServerUnreachable
}

// drops は、全 PID のドロップの合計を返す。
func drops(stats *ts.Stats) int64 {
	var total int64
	for _, pid := range stats.PIDs {
		total += pid.Drops
	}

	return total
}

func valueOr(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}
