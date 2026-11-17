// Package dramalibrary は、ユーザーが見ているドラマの作品と見た話を持ち、ドラマの視聴管理の画面 (/drama) に並べる
// (#1652、設計書 3.9)。作品と話は、番組と録画に結びついた TMDb の作品と話 (#1651) を使う。
package dramalibrary

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// StatusWatching と StatusWannaWatch は、見ている作品の状態 (見ている、見たい)。
	StatusWatching   = "watching"
	StatusWannaWatch = "wanna_watch"

	// SourcePlayback は、プレイヤーで見た割合から付けた見た話。SourceManual は、ユーザーが手で付けた見た話。
	SourcePlayback = "playback"
	SourceManual   = "manual"
)

var (
	// ErrUnavailable は、管理者が TMDb との連携を無効にしていることを表す。
	ErrUnavailable = errors.New("drama linking is disabled")
	// ErrNotLinked は、作品が、どの番組にも録画にも結びついていないことを表す。
	ErrNotLinked = errors.New("the TMDb series is not linked to any program or recording")
	// ErrInvalidEpisode は、作品、シーズン、話の番号が正しくないことを表す。
	ErrInvalidEpisode = errors.New("invalid TMDb series, season or episode number")
	// ErrInvalidStatus は、見ている作品の状態が正しくないことを表す。
	ErrInvalidStatus = errors.New("invalid drama watch status")
)

// Database は、トランザクションを張れる DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// available は、システムの設定を返す。管理者が TMDb との連携を有効にしていなければ ErrUnavailable を返す。
func available(ctx context.Context, db sqlc.DBTX) (*settings.System, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	if !system.DramaLinkingEnabled {
		return nil, ErrUnavailable
	}

	return system, nil
}

// thresholdPercent は、ユーザー userID の見た話にする割合を返す。アニメの記録の設定 (設計書 3.8) を使うが、その設定はアニメの連携の
// 画面にしか出さないので、管理者がアニメの連携を無効にしている間は、変えられない設定に従わず、既定の割合を使う。
func thresholdPercent(ctx context.Context, db sqlc.DBTX, system *settings.System, userID uuid.UUID) (int, error) {
	if !system.AnimeLinkingEnabled {
		return animerecord.DefaultThresholdPercent, nil
	}
	userSettings, err := animerecord.LoadSettings(ctx, db, userID)
	if err != nil {
		return 0, err
	}

	return userSettings.ThresholdPercent, nil
}

// SetStatus は、ユーザー userID の見ている作品に、TMDb の作品 seriesID を、状態 status で足すか、状態を変える。status が空なら外す。
// 足せるのは、番組か録画に結びついた作品だけにする (番組と録画の詳細から足す。ID を手で入れて、名前の分からない作品を増やさない)。
// 管理者が連携を無効にしていれば ErrUnavailable を、結びつきのない作品なら ErrNotLinked を返す。外すのは、結びつきがなくてもできる。
func SetStatus(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, seriesID int32, status string, now time.Time) error {
	if seriesID <= 0 {
		return ErrInvalidEpisode
	}
	if _, err := available(ctx, db); err != nil {
		return err
	}
	q := sqlc.New(db)
	if status == "" {
		if _, err := q.DeleteDramaWatchlistEntry(ctx, sqlc.DeleteDramaWatchlistEntryParams{UserID: userID, TmdbSeriesID: seriesID}); err != nil {
			return fmt.Errorf("failed to remove the drama from the watchlist: %w", err)
		}

		return nil
	}
	if status != StatusWatching && status != StatusWannaWatch {
		return ErrInvalidStatus
	}
	linked, err := q.DramaSeriesLinked(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("failed to check the drama links: %w", err)
	}
	if !linked {
		return ErrNotLinked
	}
	if err := q.UpsertDramaWatchlistEntry(ctx, sqlc.UpsertDramaWatchlistEntryParams{
		UserID: userID, TmdbSeriesID: seriesID, Status: status, Now: now,
	}); err != nil {
		return fmt.Errorf("failed to save the drama watchlist: %w", err)
	}

	return nil
}

// Status は、ユーザー userID の、TMDb の作品 seriesID の状態を返す。見ている作品に入れていなければ空を返す。
func Status(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, seriesID int32) (string, error) {
	status, err := sqlc.New(db).GetDramaWatchlistStatus(ctx, sqlc.GetDramaWatchlistStatusParams{UserID: userID, TmdbSeriesID: seriesID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the drama watch status: %w", err)
	}

	return status, nil
}

// SeriesName は、TMDb の作品 seriesID の名前 (日本語) と元の言語の名前を、TMDb のキャッシュから返す。キャッシュになければ nil を返す
// (TMDb には問い合わせない)。
func SeriesName(ctx context.Context, db sqlc.DBTX, seriesID int32) (*string, *string, error) {
	series, err := sqlc.New(db).GetTMDbSeries(ctx, seriesID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the TMDb series: %w", err)
	}

	return nonEmpty(series.Name), nonEmpty(series.OriginalName), nil
}

// Episode は、TMDb の作品の 1 話。
type Episode struct {
	SeriesID int32
	Season   int32
	Number   int32
}

// valid は、作品、シーズン、話の番号が、結びつきと同じ範囲にあるかどうかを返す。
func (e *Episode) valid() bool {
	return e.SeriesID > 0 && e.Season >= 0 && e.Number > 0
}

// SetWatched は、ユーザー userID が、話 episode を見たことを手で付けるか (watched が true)、外す (false)。
// 見たことにした話は、後からプレイヤーで見ても、付け直さない。付けられるのは、番組か録画に結びついた作品の話だけにする
// (配信で見た話も付けられるよう、話そのものの結びつきは求めない)。外すのは、結びつきがなくてもできる。
// 管理者が連携を無効にしていれば ErrUnavailable を、番号が正しくなければ ErrInvalidEpisode を、結びつきのない作品なら ErrNotLinked を返す。
func SetWatched(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, episode *Episode, watched bool, now time.Time) error {
	if !episode.valid() {
		return ErrInvalidEpisode
	}
	if _, err := available(ctx, db); err != nil {
		return err
	}
	q := sqlc.New(db)
	if !watched {
		if _, err := q.DeleteDramaWatchedEpisode(ctx, sqlc.DeleteDramaWatchedEpisodeParams{
			UserID: userID, TmdbSeriesID: episode.SeriesID, SeasonNumber: episode.Season, EpisodeNumber: episode.Number,
		}); err != nil {
			return fmt.Errorf("failed to delete the watched drama episode: %w", err)
		}

		return nil
	}
	linked, err := q.DramaSeriesLinked(ctx, episode.SeriesID)
	if err != nil {
		return fmt.Errorf("failed to check the drama links: %w", err)
	}
	if !linked {
		return ErrNotLinked
	}
	if _, err := q.InsertDramaWatchedEpisode(ctx, sqlc.InsertDramaWatchedEpisodeParams{
		UserID: userID, TmdbSeriesID: episode.SeriesID, SeasonNumber: episode.Season, EpisodeNumber: episode.Number,
		Source: SourceManual, Now: now,
	}); err != nil {
		return fmt.Errorf("failed to insert the watched drama episode: %w", err)
	}

	return nil
}

// Watched は、ユーザー userID が、話 episode を見たかどうかを返す。
func Watched(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, episode *Episode) (bool, error) {
	watched, err := sqlc.New(db).DramaEpisodeWatched(ctx, sqlc.DramaEpisodeWatchedParams{
		UserID: userID, TmdbSeriesID: episode.SeriesID, SeasonNumber: episode.Season, EpisodeNumber: episode.Number,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check the watched drama episode: %w", err)
	}

	return watched, nil
}

// watchedProgram は、見た区間を足す、TMDb の話に結びついた番組。
type watchedProgram struct {
	serviceID int64
	startAt   time.Time
	endAt     time.Time
	episode   *Episode
}

// Report は、ユーザー userID のプレイヤーが、target を実際に再生した区間 intervals を、話に結びついたドラマの番組ごとに足す。
// 番組の長さのうち、ユーザーが決めた割合 (アニメの記録の設定。既定は 20%。アニメの連携が無効の間は既定の割合) を見たら、番組に結びついた話を、見た話にする (設計書 3.9)。
// 見た区間は、アニメの記録と同じ行に持ち、同じ判定 (設計書 3.8) で決める。見ている作品に入れていない作品の話も、見た話にする。
// 管理者が連携を無効にしていれば、何もせずに false を返す。
func Report(
	ctx context.Context, db Database, userID uuid.UUID, target *animerecord.Target, intervals []*animerecord.Interval, now time.Time,
) (bool, error) {
	spans, err := animerecord.NewPlayedSpans(intervals, now)
	if err != nil {
		return false, err
	}
	system, err := available(ctx, db)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return false, nil
		}

		return false, err
	}
	if spans.Empty() {
		return true, nil
	}
	programs, err := watchedPrograms(ctx, db, target, spans)
	if err != nil {
		return false, err
	}
	if len(programs) == 0 {
		return true, nil
	}
	percent, err := thresholdPercent(ctx, db, system, userID)
	if err != nil {
		return false, err
	}
	for _, p := range programs {
		if err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			reached, err := animerecord.AddPlayed(ctx, tx, userID, p.serviceID, p.startAt, p.endAt, spans, percent, now)
			if err != nil || !reached {
				return err
			}
			if _, err := sqlc.New(tx).InsertDramaWatchedEpisode(ctx, sqlc.InsertDramaWatchedEpisodeParams{
				UserID: userID, TmdbSeriesID: p.episode.SeriesID, SeasonNumber: p.episode.Season, EpisodeNumber: p.episode.Number,
				Source: SourcePlayback, Now: now,
			}); err != nil {
				return fmt.Errorf("failed to insert the watched drama episode: %w", err)
			}

			return nil
		}); err != nil {
			return false, err
		}
	}

	return true, nil
}

// watchedPrograms は、見た区間 spans に重なる、TMDb の話に結びついた番組を返す。
// ライブ視聴は、サービスの番組のうち区間に重なるものを、録画の再生は、録画した番組 (録画に写した結びつき) を返す。
func watchedPrograms(ctx context.Context, db sqlc.DBTX, target *animerecord.Target, spans *animerecord.PlayedSpans) ([]*watchedProgram, error) {
	q := sqlc.New(db)
	programs := []*watchedProgram{}
	switch {
	case target.ServiceID != nil:
		since, until := spans.Range()
		rows, err := q.ListLiveProgramsInRange(ctx, sqlc.ListLiveProgramsInRangeParams{ServiceID: *target.ServiceID, Since: since, Until: until})
		if err != nil {
			return nil, fmt.Errorf("failed to list the programs: %w", err)
		}
		for _, row := range rows {
			linked, found, err := drama.ProgramEpisode(ctx, db, row.ServiceID, row.StartAt)
			if err != nil {
				return nil, err
			}
			if episode := episodeOf(linked, found); episode != nil {
				programs = append(programs, &watchedProgram{serviceID: row.ServiceID, startAt: row.StartAt, endAt: row.EndAt, episode: episode})
			}
		}
	case target.RecordingID != nil:
		row, err := q.GetRecordingProgram(ctx, *target.RecordingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: recording not found", animerecord.ErrInvalidReport)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the recording: %w", err)
		}
		linked, found, err := drama.RecordingEpisode(ctx, db, row.ID)
		if err != nil {
			return nil, err
		}
		if episode := episodeOf(linked, found); episode != nil {
			programs = append(programs, &watchedProgram{serviceID: row.ServiceID, startAt: row.StartAt, endAt: row.EndAt, episode: episode})
		}
	default:
		return nil, fmt.Errorf("%w: no target", animerecord.ErrInvalidReport)
	}

	return programs, nil
}

// episodeOf は、結びついた作品と話を、話にする。話まで決まっていなければ nil を返す。
func episodeOf(linked *drama.LinkedEpisode, found bool) *Episode {
	if !found || linked.SeasonNumber == nil || linked.EpisodeNumber == nil {
		return nil
	}

	return &Episode{SeriesID: linked.SeriesID, Season: *linked.SeasonNumber, Number: *linked.EpisodeNumber}
}
