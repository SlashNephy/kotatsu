package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// from がバッファの始まりより前か、チャンクの間の穴にあれば、その後の最初のチャンクから流し、位置は from からの時間で表す。
// ライブ視聴の画面は、開いたときのバッファの始まりを起点にし続けるので、古いチャンクを破棄した後も、位置をずらさないためである。
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

	return &Recording{streamer: s.streamer, timeline: tl, InProgress: true, segments: segments, skip: from.Sub(segments[0].startedAt)}, nil
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
			open: chunk.EndedAt == nil, continues: false, continued: false, index: []ts.IndexEntry{},
		}
		if i > 0 {
			prev := segments[i-1]
			seg.continues = prev.endedAt != nil && prev.endedAt.Equal(seg.startedAt)
			prev.continued = seg.continues
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

// TimeshiftRun は、バッファのうち、mikan が同じストリームを区切って書き続けた、続いたチャンクの並び (#826)。
// 録画の開始が遅れたとき、mikan が、予定の開始からの分を、バッファから録画のセグメントに写すのに使う (設計書 5.7)。
type TimeshiftRun struct {
	// From は、写し始める時刻。並びの最初のチャンクの始まりか、OpenRuns に渡した from の遅いほう。
	From time.Time
	// To は、並びの最後のチャンクの終わり。書いている途中なら nil で、開いた時点の大きさまで写す。
	To        *time.Time
	recording *Recording
	// limits は、書いている途中のチャンクの、開いた時点の大きさ。そこまでを写す。
	limits map[uuid.UUID]int64
}

// OpenRuns は、サービスのバッファのうち、from より後まで録ったチャンクを、続いたチャンクの並びごとに、始まりの順に開く。
// 並びの間は、チューナーを予約に渡したり、mikan が録り直したりして途切れた区間なので、録画では別のセグメントにする。
// 書いている途中のチャンクは、開いた時点の大きさまでを写し、伸びるのを待たない。録画の続きは、mikan がライブのストリームで書いているためである。
// タイムシフトのディレクトリがなければ ErrNotMounted、サービスを録っていなければ ErrTimeshiftNotFound を返す。
func (s *TimeshiftSource) OpenRuns(ctx context.Context, serviceID int64, from time.Time) ([]*TimeshiftRun, error) {
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
	segments, err := (&timeshiftTimeline{source: s, serviceID: serviceID, since: from}).load(ctx)
	if err != nil {
		return nil, err
	}
	runs := []*TimeshiftRun{}
	for start := 0; start < len(segments); {
		end := start + 1
		for end < len(segments) && segments[end].continues {
			end++
		}
		run, err := s.openRun(segments[start:end], from)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
		start = end
	}

	return runs, nil
}

// openRun は、続いたチャンクの並び segments を、from から写せるように開く。書いている途中のチャンクは、今の大きさを写す範囲の終わりにする。
func (s *TimeshiftSource) openRun(segments []*segment, from time.Time) (*TimeshiftRun, error) {
	last := segments[len(segments)-1]
	// 書いている途中のチャンクは、測った大きさで切るので、末尾の PES が途中で切れることがある。
	// 録画の再生とセグメントをつなぐ処理は、ライブのセグメントとの重なりで切り詰めるので、そのまま写す。
	limits := map[uuid.UUID]int64{}
	for _, seg := range segments {
		if !seg.open {
			continue
		}
		info, err := s.streamer.store.Stat(seg.poolID, seg.path)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect the timeshift chunk: %w", err)
		}
		limits[seg.id] = info.Size()
	}
	first := segments[0]
	start := first.startedAt
	if from.After(start) {
		start = from
	}
	run := &TimeshiftRun{
		From: start, To: last.endedAt,
		recording: &Recording{
			streamer: s.streamer, timeline: &snapshotTimeline{segments: segments}, InProgress: false,
			segments: segments, skip: max(from.Sub(first.startedAt), 0),
		},
		limits: limits,
	}

	return run, nil
}

// Write は、並びを From の手前の I フレームから w に写す。書いている途中のチャンクは、開いた時点の大きさまで写して終える。
func (r *TimeshiftRun) Write(ctx context.Context, w io.Writer) error {
	pos := r.recording.Seek(0)
	for _, p := range pos.pieces {
		if limit, ok := r.limits[p.segment.id]; ok && (p.to < 0 || p.to > limit) {
			p.to = limit
		}
	}

	return r.recording.Write(ctx, w, pos)
}

// snapshotTimeline は、開いた時点のチャンクの並び。並びは増えず、書いている途中のチャンクも、書き終えたものとして扱い、伸びるのを待たない。
type snapshotTimeline struct {
	segments []*segment
}

var _ timeline = new(snapshotTimeline)

func (t *snapshotTimeline) load(context.Context) ([]*segment, error) {
	return t.segments, nil
}

func (t *snapshotTimeline) inProgress(context.Context) (bool, error) {
	return false, nil
}

func (t *snapshotTimeline) segmentClosed(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}
