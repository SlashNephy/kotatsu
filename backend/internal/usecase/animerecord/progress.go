package animerecord

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// MaxReportedIntervals は、1 回の報告で受け付ける区間の数の上限。
	MaxReportedIntervals = 100
	// maxIntervalLength は、1 つの区間の長さの上限。それより長い区間は、壊れた報告として捨てる。
	maxIntervalLength = 24 * time.Hour
	// maxFutureSkew は、区間の終わりが、今より先にあってよい長さ。端末の時計のずれを見込む。これより先の分は数えない。
	maxFutureSkew = 5 * time.Minute
	// maxEpisodesPerRange は、話数の範囲 (#1～#2 など) から記録する話の数の上限。壊れた範囲で記録を作りすぎないため。
	maxEpisodesPerRange = 13
	// maxStoredSpans は、番組ごとに持つ、見た区間の数の上限。細かくばらけた区間をたくさん送られても、行が大きくなりすぎないようにする。
	maxStoredSpans = 1000
	// spanUnitMs は、区間の端をそろえる単位 (ミリ秒)。これより短い区間は数えない。
	spanUnitMs = 1000
)

var (
	// ErrInvalidReport は、プレイヤーの報告が正しくないことを表す。
	ErrInvalidReport = errors.New("invalid playback report")
	// errNoInserter は、記録のジョブを入れる River のクライアントがないことを表す。
	errNoInserter = errors.New("no job inserter for anime records")
)

// Interval は、プレイヤーが実際に再生した、放送の時刻の区間 [From, To)。
type Interval struct {
	From time.Time
	To   time.Time
}

// span は、DB に置く区間。UNIX ミリ秒の [始まり, 終わり)。
type span [2]int64

// mergeSpans は、区間を重ねずにまとめ、始まりの順に並べる。重なる区間と、接する区間は 1 つにする。
// 同じ報告を何度受け取っても、順番が入れ替わっても、同じ結果になる。複数のタブで同じ番組を見ても、重なった分は 1 回だけ数える。
func mergeSpans(spans []span) []span {
	sorted := slices.Clone(spans)
	slices.SortFunc(sorted, func(a, b span) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) })
	merged := []span{}
	for _, s := range sorted {
		if s[1] <= s[0] {
			continue
		}
		if last := len(merged) - 1; last >= 0 && s[0] <= merged[last][1] {
			merged[last][1] = max(merged[last][1], s[1])

			continue
		}
		merged = append(merged, s)
	}

	return merged
}

// normalizeSpans は、区間の始まりを秒に切り上げ、終わりを秒に切り捨てる。1 秒に満たなくなった区間は捨てる。
// 1 秒より細かい区間で、DB に置く区間の数を増やさせないためである。
func normalizeSpans(spans []span) []span {
	normalized := []span{}
	for _, s := range spans {
		from := (s[0] + spanUnitMs - 1) / spanUnitMs * spanUnitMs
		to := s[1] / spanUnitMs * spanUnitMs
		if to-from >= spanUnitMs {
			normalized = append(normalized, span{from, to})
		}
	}

	return normalized
}

// capSpans は、重ならずに並んだ区間 spans の数を limit までにする。多ければ、間の最も短い隣どうしを 1 つにまとめていく。
// まとめると間も見たことになるが、上限を超えるのは、細かくばらけた区間をわざと送られたときだけなので、行の大きさを抑えるほうを採る。
func capSpans(spans []span, limit int) []span {
	capped := slices.Clone(spans)
	for len(capped) > limit && len(capped) > 1 {
		nearest := 0
		for i := 1; i < len(capped)-1; i++ {
			if capped[i+1][0]-capped[i][1] < capped[nearest+1][0]-capped[nearest][1] {
				nearest = i
			}
		}
		capped[nearest][1] = capped[nearest+1][1]
		capped = slices.Delete(capped, nearest+1, nearest+2)
	}

	return capped
}

// clipSpans は、区間を [start, end) に切り詰める。外にある区間は捨てる。
func clipSpans(spans []span, start, end int64) []span {
	clipped := []span{}
	for _, s := range spans {
		from, to := max(s[0], start), min(s[1], end)
		if from < to {
			clipped = append(clipped, span{from, to})
		}
	}

	return clipped
}

// totalMillis は、重ならない区間の長さの合計 (ミリ秒) を返す。
func totalMillis(spans []span) int64 {
	var total int64
	for _, s := range spans {
		total += s[1] - s[0]
	}

	return total
}

// reachedThreshold は、長さ length の番組で、played ミリ秒を見たら、percent % に届いたかどうかを返す。
func reachedThreshold(played int64, length time.Duration, percent int) bool {
	if length <= 0 {
		return false
	}

	return played*100 >= length.Milliseconds()*int64(percent)
}

// Target は、プレイヤーが見ているもの。ライブ視聴ならサービスを、録画の再生なら録画を持つ。
type Target struct {
	ServiceID   *int64
	RecordingID *uuid.UUID
}

// program は、見た区間を足す番組。
type program struct {
	serviceID int64
	startAt   time.Time
	endAt     time.Time
	name      string
	episodes  []*anime.LinkedEpisode
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Report は、ユーザー userID のプレイヤーが、target を実際に再生した区間 intervals を、番組ごとに足す。
// 番組の長さのうち、ユーザーが決めた割合 (既定は 20%) を見たら、番組に結びついた話を、連携した記録先ごとに 1 回だけ記録するジョブを入れる。
// 区間は重ねずにまとめるので、同じ報告を送り直しても、複数のタブで見ても、多く数えない。
// 連携を無効にしているか、どの記録先とも連携していなければ、何もせずに false を返す。
func (s *Service) Report(ctx context.Context, db Database, inserter Inserter, userID uuid.UUID, target *Target, intervals []*Interval) (bool, error) {
	spans, err := s.validSpans(intervals)
	if err != nil {
		return false, err
	}
	system, err := settings.Load(ctx, db)
	if err != nil {
		return false, err
	}
	connections, err := s.Connections(ctx, db, userID)
	if err != nil {
		return false, err
	}
	if !system.AnimeLinkingEnabled || len(connections) == 0 {
		return false, nil
	}
	if len(spans) == 0 {
		return true, nil
	}
	programs, err := s.programs(ctx, db, target, spans)
	if err != nil {
		return false, err
	}
	userSettings, err := LoadSettings(ctx, db, userID)
	if err != nil {
		return false, err
	}
	for _, p := range programs {
		if err := s.add(ctx, db, inserter, userID, p, spans, connections, userSettings.ThresholdPercent); err != nil {
			return false, err
		}
	}

	return true, nil
}

// validSpans は、報告された区間を確かめ、DB に置く形にする。
func (s *Service) validSpans(intervals []*Interval) ([]span, error) {
	if len(intervals) > MaxReportedIntervals {
		return nil, fmt.Errorf("%w: too many intervals", ErrInvalidReport)
	}
	// 端末の時計が進んでいると、ライブ視聴の区間が先の時刻になる。断ると端末が送り直し続けるので、今より先の分は切り捨てる。
	limit := s.clock.Now().Add(maxFutureSkew).UnixMilli()
	spans := make([]span, 0, len(intervals))
	for _, interval := range intervals {
		if !interval.From.Before(interval.To) || interval.To.Sub(interval.From) > maxIntervalLength {
			return nil, fmt.Errorf("%w: an interval is out of range", ErrInvalidReport)
		}
		spans = append(spans, span{interval.From.UnixMilli(), min(interval.To.UnixMilli(), limit)})
	}

	return mergeSpans(normalizeSpans(spans)), nil
}

// programs は、見た区間 spans に重なる、話に結びついた番組を返す。
// ライブ視聴は、サービスの番組のうち区間に重なるものを、録画の再生は、録画した番組を返す。
func (s *Service) programs(ctx context.Context, db sqlc.DBTX, target *Target, spans []span) ([]*program, error) {
	q := sqlc.New(db)
	candidates := []*program{}
	switch {
	case target.ServiceID != nil:
		rows, err := q.ListLiveProgramsInRange(ctx, sqlc.ListLiveProgramsInRangeParams{
			ServiceID: *target.ServiceID, Since: time.UnixMilli(spans[0][0]), Until: time.UnixMilli(spans[len(spans)-1][1]),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list the programs: %w", err)
		}
		for _, row := range rows {
			episodes, err := anime.ProgramEpisodes(ctx, db, row.ServiceID, row.StartAt)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, &program{serviceID: row.ServiceID, startAt: row.StartAt, endAt: row.EndAt, name: row.Name, episodes: episodes})
		}
	case target.RecordingID != nil:
		row, err := q.GetRecordingProgram(ctx, *target.RecordingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: recording not found", ErrInvalidReport)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the recording: %w", err)
		}
		episodes, err := anime.RecordingEpisodes(ctx, db, row.ID)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, &program{serviceID: row.ServiceID, startAt: row.StartAt, endAt: row.EndAt, name: row.Name, episodes: episodes})
	default:
		return nil, fmt.Errorf("%w: no target", ErrInvalidReport)
	}
	programs := []*program{}
	for _, candidate := range candidates {
		if slices.ContainsFunc(candidate.episodes, func(episode *anime.LinkedEpisode) bool { return episode.TID != nil }) {
			programs = append(programs, candidate)
		}
	}

	return programs, nil
}

// add は、番組 p に、見た区間 spans を足す。割合に届いたら、記録先ごとに記録を作り、記録するジョブを入れる。
func (s *Service) add(
	ctx context.Context, db Database, inserter Inserter, userID uuid.UUID, p *program, spans []span, connections []*Connection, percent int,
) error {
	clipped := clipSpans(spans, p.startAt.UnixMilli(), p.endAt.UnixMilli())
	if len(clipped) == 0 {
		return nil
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		raw, err := q.LockAnimeWatchProgress(ctx, sqlc.LockAnimeWatchProgressParams{UserID: userID, ServiceID: p.serviceID, StartAt: p.startAt, Now: now})
		if err != nil {
			return fmt.Errorf("failed to lock the watch progress: %w", err)
		}
		var played []span
		if err := json.Unmarshal(raw, &played); err != nil {
			return fmt.Errorf("failed to decode the watch progress: %w", err)
		}
		played = capSpans(mergeSpans(append(played, clipped...)), maxStoredSpans)
		encoded, err := json.Marshal(played)
		if err != nil {
			return fmt.Errorf("failed to encode the watch progress: %w", err)
		}
		total := totalMillis(played)
		if err := q.UpdateAnimeWatchProgress(ctx, sqlc.UpdateAnimeWatchProgressParams{
			Played: encoded, PlayedMs: total, Now: now, UserID: userID, ServiceID: p.serviceID, StartAt: p.startAt,
		}); err != nil {
			return fmt.Errorf("failed to save the watch progress: %w", err)
		}
		if !reachedThreshold(total, p.endAt.Sub(p.startAt), percent) {
			return nil
		}

		return s.enqueue(ctx, tx, inserter, userID, p, connections)
	})
}

// enqueue は、番組 p の話を、連携した記録先ごとに記録する行を作り、新しく作った記録先のジョブを入れる。
// 同じユーザー、番組、話、記録先の組は 1 回だけ記録するので、見直しても、割合に届いた後の報告でも、行もジョブも増えない。
func (s *Service) enqueue(ctx context.Context, tx pgx.Tx, inserter Inserter, userID uuid.UUID, p *program, connections []*Connection) error {
	q := sqlc.New(tx)
	now := s.clock.Now()
	jobs := []river.InsertManyParams{}
	for _, connection := range connections {
		var inserted int64
		for _, episode := range p.episodes {
			if episode.TID == nil {
				continue
			}
			for _, number := range episodeNumbers(episode.Episode) {
				n, err := q.InsertAnimeRecord(ctx, sqlc.InsertAnimeRecordParams{
					ID: id.New(), UserID: userID, ServiceID: p.serviceID, StartAt: p.startAt, Destination: connection.Destination,
					SyobocalTid: *episode.TID, EpisodeNumber: number, Subtitle: episode.Subtitle, IsFirst: episode.First, IsLast: episode.Last,
					ProgramName: p.name, Now: now,
				})
				if err != nil {
					return fmt.Errorf("failed to insert the anime record: %w", err)
				}
				inserted += n
			}
		}
		if inserted > 0 {
			jobs = append(jobs, river.InsertManyParams{
				Args:       RecordArgs{UserID: userID, ServiceID: p.serviceID, StartAt: p.startAt, Destination: connection.Destination},
				InsertOpts: nil,
			})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	// ジョブを入れられないまま記録の行だけを残すと、送られないまま、次の報告でも作り直されない。行ごと取り消す。
	if inserter == nil {
		return errNoInserter
	}
	if _, err := inserter.InsertManyTx(ctx, tx, jobs); err != nil {
		return fmt.Errorf("failed to enqueue the anime record: %w", err)
	}

	return nil
}

// episodeNumbers は、結びついた話の話数を、1 話ずつに分けて返す。話数のない放送は、話数のない 1 つにする。
// 2 話連続の放送 (#1～#2) は、すべての話を記録する。
func episodeNumbers(episode *anime.Episode) []*float64 {
	if episode.CountStart == nil {
		return []*float64{nil}
	}
	end := *episode.CountStart
	if episode.CountEnd != nil && *episode.CountEnd > end {
		end = *episode.CountEnd
	}
	numbers := []*float64{}
	for n := *episode.CountStart; n <= end && len(numbers) < maxEpisodesPerRange; n++ {
		numbers = append(numbers, new(n))
	}

	return numbers
}
