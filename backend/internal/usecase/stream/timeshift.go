package stream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// ErrTimeshiftNotFound は、サービスをタイムシフトで録っていないことを表す。
var ErrTimeshiftNotFound = errors.New("the service is not recorded for timeshift")

// TimeshiftSource は、タイムシフトのバッファのチャンクを、録画のセグメントと同じく、索引を使って連結して流す (設計書 5.7、7.2)。
// 同じストリームを区切っただけのチャンクは、バイトをそのままつなぎ、引き継ぎで重なったチャンクは切り詰める。
// バッファは終わらないので、書いている途中のチャンクの末尾まで読んだら、伸びるのと、次のチャンクができるのを待ちながら流し続ける。
type TimeshiftSource struct {
	db       sqlc.DBTX
	streamer *streamer
}

// NewTimeshiftSource は TimeshiftSource を返す。store は、このインスタンスのタイムシフトのディレクトリとデータディレクトリ。
// pollInterval が 0 なら 500 ミリ秒にする。
func NewTimeshiftSource(db sqlc.DBTX, store *kago.Store, pollInterval time.Duration) *TimeshiftSource {
	return &TimeshiftSource{db: db, streamer: newStreamer(store, pollInterval)}
}

// Open は、サービスのバッファを、from の時刻を再生の時間の起点にして開く。from より前に終わったチャンクは流さない。
// from がバッファの始まりより前か、チャンクの間の穴にあれば、その後の最初のチャンクの始まりを起点にする。
// タイムシフトのディレクトリがなければ ErrNotMounted、サービスを録っていなければ ErrTimeshiftNotFound、
// from より後のチャンクがなければ ErrNoSegment を返す。
func (s *TimeshiftSource) Open(ctx context.Context, serviceID int64, from time.Time) (*Recording, error) {
	if !s.streamer.store.HasTimeshiftDir() {
		return nil, ErrNotMounted
	}
	exists, err := sqlc.New(s.db).TimeshiftChannelExists(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to check the timeshift channel: %w", err)
	}
	if !exists {
		return nil, ErrTimeshiftNotFound
	}
	tl := &timeshiftTimeline{source: s, serviceID: serviceID, since: from}
	segments, err := tl.load(ctx)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, ErrNoSegment
	}

	return &Recording{streamer: s.streamer, timeline: tl, InProgress: true, segments: segments, skip: max(from.Sub(segments[0].startedAt), 0)}, nil
}

// timeshiftTimeline は、サービスのバッファの、since より後まで録ったチャンクの並び。
type timeshiftTimeline struct {
	source    *TimeshiftSource
	serviceID int64
	since     time.Time
}

var _ timeline = new(timeshiftTimeline)

// load は、チャンクを始まりの順に読む。前のチャンクの終わりと同じ時刻に始まったチャンクは、mikan が同じストリームを区切って
// 書き続けたもの (設計書 5.7) なので、続きとして扱う。
func (t *timeshiftTimeline) load(ctx context.Context) ([]*segment, error) {
	chunks, err := sqlc.New(t.source.db).ListTimeshiftChunksSince(ctx, sqlc.ListTimeshiftChunksSinceParams{ServiceID: t.serviceID, Since: t.since})
	if err != nil {
		return nil, fmt.Errorf("failed to list the timeshift chunks: %w", err)
	}
	segments := make([]*segment, 0, len(chunks))
	for i := range chunks {
		chunk := &chunks[i]
		seg := &segment{
			id: chunk.ID, poolID: kago.TimeshiftPoolID, path: chunk.Path, startedAt: chunk.StartedAt, endedAt: chunk.EndedAt,
			open: chunk.EndedAt == nil, continues: false, index: []ts.IndexEntry{},
		}
		if i > 0 {
			prev := segments[i-1]
			seg.continues = prev.endedAt != nil && prev.endedAt.Equal(seg.startedAt)
		}
		if seg.index, err = t.source.streamer.readIndex(kago.IndexLocation(chunk.ID)); err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// inProgress は、サービスをまだタイムシフトで録っているかを返す。録っている間は、チューナーを予約に渡したり、
// 録り直すのを待ったりしてチャンクが途切れても、次のチャンクができるのを待つ。
func (t *timeshiftTimeline) inProgress(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	exists, err := sqlc.New(t.source.db).TimeshiftChannelExists(ctx, t.serviceID)
	if err != nil {
		return false, fmt.Errorf("failed to check the timeshift channel: %w", err)
	}

	return exists, nil
}

// segmentClosed は、チャンクを書き終えたかどうかを返す。チャンクの行が消えたら、破棄したので、流すのをやめる。
func (t *timeshiftTimeline) segmentClosed(ctx context.Context, segmentID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	endedAt, err := sqlc.New(t.source.db).GetTimeshiftChunkEnd(ctx, segmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get the timeshift chunk: %w", err)
	}

	return endedAt != nil, nil
}
