package recording

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

// sessionCache は、最後に読めた予約、録画設定、保存先プールの設定。DB に届かない間は、これで録画を始め、セグメントを作る (#1073)。
type sessionCache struct {
	reservation sqlc.GetReservationForRecordingRow
	// settings と pools は、読めなかったら nil。
	settings *domain.RecordingSettings
	pools    []sqlc.StoragePool
}

// loadSessionCache は、予約 reservation の録画設定と、保存先プールの設定を読む。読めなかったものは nil にしておく。
func (r *Recorder) loadSessionCache(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) *sessionCache {
	c := &sessionCache{reservation: *reservation, settings: nil, pools: nil}
	c.refresh(ctx, r, reservation)

	return c
}

// refresh は、録画設定と保存先プールの設定を読み直す。読めなければ、前に読めたものを残す。
func (c *sessionCache) refresh(ctx context.Context, r *Recorder, reservation *sqlc.GetReservationForRecordingRow) {
	c.reservation = *reservation
	if settings, err := r.settings(ctx, reservation); err == nil {
		c.settings = settings
	}
	if pools, err := sqlc.New(r.pool).ListStoragePools(ctx); err == nil {
		c.pools = pools
	}
}

// cachedSettings は、録画設定を読む。DB に届かなければ、cache に残る、前に読めた録画設定を返す。
func (r *Recorder) cachedSettings(
	ctx context.Context, reservation *sqlc.GetReservationForRecordingRow, cache *sessionCache,
) (*domain.RecordingSettings, error) {
	settings, err := r.settings(ctx, reservation)
	switch {
	case err == nil:
		if cache != nil {
			cache.settings = settings
		}

		return settings, nil
	case database.IsUnavailable(err) && cache != nil && cache.settings != nil:
		return cache.settings, nil
	default:
		return nil, err
	}
}

// cachedPools は、保存先プールの設定を読み、録画を書く候補のプールを返す。DB に届かなければ、cache に残る設定を使う。
func (r *Recorder) cachedPools(ctx context.Context, mounted []string, poolID string, cache *sessionCache) ([]*kago.Pool, error) {
	rows, err := sqlc.New(r.pool).ListStoragePools(ctx)
	switch {
	case err == nil:
		if cache != nil {
			cache.pools = rows
		}
	case database.IsUnavailable(err) && cache != nil && cache.pools != nil:
		rows = cache.pools
	default:
		return nil, fmt.Errorf("failed to list storage pools: %w", err)
	}

	return poolsFrom(rows, mounted, poolID), nil
}

// pendingWrite は、DB に届くようになってから書く、録画の記録。recordingID は、そのときの録画の ID。
type pendingWrite func(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID) error

// pendingSave は、DB に届かずに記録できていない録画とセグメント (#1073)。
// 録画の行がなければ、セグメントの行も書けないので、録画、書きかけのセグメントの片付け、セグメントの作成と終わりの順に記録する。
type pendingSave struct {
	// recording は、Session の録画。記録したら、DB の行で置き換える (引き継ぎや再開なら、ID が変わる)。
	recording *sqlc.Recording
	// start は、録画をまだ記録していないかどうか。
	start bool
	// closeStale は、落ちた mikan の書きかけのセグメントを、まだ閉じていないかどうか。
	closeStale bool
	// startedAt は、録画を始めた時刻。録画は、この時刻で記録する。
	startedAt time.Time
	// handedOver は、止まる mikan から引き継いだかどうか。引き継いだなら、書きかけのセグメントを閉じない。
	handedOver bool
	// writes は、まだ書いていないセグメントの作成と終わりの記録。作った順に書く。
	writes []pendingWrite
	cache  *sessionCache
}

// waiting は、記録できていないものがあるかどうかを返す。
func (p *pendingSave) waiting() bool {
	return p.start || p.closeStale || len(p.writes) > 0
}

// writeOrDefer は、write を書く。記録できていないものが残っていれば、順を守るため、書かずに積む。
// DB に届かなければ、積んで nil を返す。
func (p *pendingSave) writeOrDefer(ctx context.Context, r *Recorder, write pendingWrite) error {
	if p.waiting() {
		p.writes = append(p.writes, write)

		return nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	err := write(writeCtx, sqlc.New(r.pool), p.recording.ID)
	if database.IsUnavailable(err) {
		r.logger.WarnContext(ctx, "failed to save the recording file; keep recording and save it when the database is reachable", slog.Any("error", err))
		p.writes = append(p.writes, write)

		return nil
	}

	return err
}

// save は、記録できていない録画とセグメントを記録する。失敗したら、残りを残して返す。
func (p *pendingSave) save(ctx context.Context, r *Recorder, tracker *endTracker) error {
	if p.start {
		recording, err := r.start(ctx, &p.cache.reservation, p.startedAt)
		if err != nil {
			return err
		}
		*p.recording = recording
		tracker.recordingID = recording.ID
		p.start = false
		p.closeStale = !p.handedOver
	}
	// 落ちた mikan の代わりに取ったなら、書きかけのセグメントを閉じる。まだ記録していない、この mikan のセグメントは閉じない。
	if p.closeStale {
		if err := r.closeStaleFiles(ctx, p.recording.ID, p.startedAt); err != nil {
			return err
		}
		p.closeStale = false
	}
	q := sqlc.New(r.pool)
	for len(p.writes) > 0 {
		if err := p.writes[0](ctx, q, p.recording.ID); err != nil {
			return err
		}
		p.writes = slices.Delete(p.writes, 0, 1)
	}

	return nil
}

// insertSegment は、セグメント seg を記録する書き込みを返す。
func insertSegment(seg *segment) pendingWrite {
	return func(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID) error {
		if err := q.InsertRecordingFile(ctx, segmentParams(seg, recordingID)); err != nil {
			return fmt.Errorf("failed to save the recording file: %w", err)
		}

		return nil
	}
}

// segmentParams は、セグメント seg を、録画 recordingID のファイルとして記録する引数を返す。
func segmentParams(seg *segment, recordingID uuid.UUID) sqlc.InsertRecordingFileParams {
	return sqlc.InsertRecordingFileParams{
		ID: seg.fileID, RecordingID: recordingID, Kind: fileKindSegment, PoolID: seg.poolID, Path: seg.path, StartedAt: seg.startedAt, Now: seg.startedAt,
	}
}

// provisionalRecording は、DB に届かずに記録できていない録画を、予約から仮に作る。記録したら、DB の行で置き換える。
func provisionalRecording(reservation *sqlc.GetReservationForRecordingRow, startedAt time.Time) sqlc.Recording {
	return sqlc.Recording{
		ID: id.New(), APIID: 0, ReservationID: &reservation.ID, ServiceID: reservation.ServiceID, EventID: reservation.EventID,
		Name: reservation.Name, Description: reservation.Description, StartAt: reservation.StartAt, EndAt: reservation.EndAt,
		State: string(StateRecording), FailureReason: nil, Protected: false, Drops: 0, Errors: 0, Scrambled: 0,
		Gaps: []byte("[]"), Audios: []byte("[]"), VideoResolution: nil, Genres: []byte("[]"), RuleID: nil, RuleName: nil,
		ProgramLost: false, BroadcastStartedAt: nil, BroadcastEndedAt: nil, WaitedForBroadcast: false, NotBroadcast: false, AiredEventID: nil,
		RecordedFrom: maxTime(startedAt, *reservation.PlannedStartAt), RecordedUntil: nil, CreatedAt: startedAt, UpdatedAt: startedAt,
	}
}
