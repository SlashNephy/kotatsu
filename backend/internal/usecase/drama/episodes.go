package drama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
)

// LinkedEpisode は、番組や録画に結びついた TMDb の作品と話 (設計書 3.9)。作品と話の名前は、TMDb から引いたキャッシュから読み、
// キャッシュがなければ (引く前や、6 か月で消した後) nil にする。
type LinkedEpisode struct {
	SeriesID int32
	// SeriesName と SeriesOriginalName は、作品の名前 (日本語) と、元の言語の名前。
	SeriesName         *string
	SeriesOriginalName *string
	// SeasonNumber と EpisodeNumber は、話を決められたときだけ持つ。
	SeasonNumber  *int32
	EpisodeNumber *int32
	// EpisodeName は、話の名前。
	EpisodeName *string
	// SeriesSource と EpisodeSource は、作品と話を決めた出どころ。EpisodeSource は、話を決められなければ空。
	SeriesSource  string
	EpisodeSource string
}

// ProgramEpisode は、番組 (serviceID, startAt) に結びついた作品と話を返す。照合していないか、作品に結びつかなければ、found が false。
func ProgramEpisode(ctx context.Context, db sqlc.DBTX, serviceID int64, startAt time.Time) (*LinkedEpisode, bool, error) {
	row, err := sqlc.New(db).GetProgramDramaMatch(ctx, sqlc.GetProgramDramaMatchParams{ServiceID: serviceID, StartAt: startAt})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the drama match of the program: %w", err)
	}

	return linked(ctx, db, row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber, row.SeriesSource, row.EpisodeSource)
}

// RecordingEpisode は、録画 recordingID に結びついた作品と話を返す。写していないか、作品に結びつかなければ、found が false。
func RecordingEpisode(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) (*LinkedEpisode, bool, error) {
	row, err := sqlc.New(db).GetRecordingDramaMatch(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the drama match of the recording: %w", err)
	}

	return linked(ctx, db, row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber, row.SeriesSource, row.EpisodeSource)
}

// linked は、結びつきの列から LinkedEpisode を作る。作品に結びつかなければ、found が false。
func linked(
	ctx context.Context, db sqlc.DBTX, seriesID, seasonNumber, episodeNumber *int32, seriesSource, episodeSource *string,
) (*LinkedEpisode, bool, error) {
	if seriesID == nil {
		return nil, false, nil
	}
	q := sqlc.New(db)
	episode := &LinkedEpisode{
		SeriesID: *seriesID, SeriesName: nil, SeriesOriginalName: nil, SeasonNumber: seasonNumber, EpisodeNumber: episodeNumber,
		EpisodeName: nil, SeriesSource: valueOf(seriesSource), EpisodeSource: valueOf(episodeSource),
	}
	series, err := q.GetTMDbSeries(ctx, *seriesID)
	switch {
	case err == nil:
		if series.Name != "" {
			episode.SeriesName = new(series.Name)
		}
		if series.OriginalName != "" {
			episode.SeriesOriginalName = new(series.OriginalName)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, false, fmt.Errorf("failed to read the TMDb series: %w", err)
	}
	if seasonNumber == nil || episodeNumber == nil {
		return episode, true, nil
	}
	season, err := q.GetTMDbSeason(ctx, sqlc.GetTMDbSeasonParams{SeriesID: *seriesID, SeasonNumber: *seasonNumber})
	if errors.Is(err, pgx.ErrNoRows) {
		return episode, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the TMDb season: %w", err)
	}
	episodes := []*tmdb.Episode{}
	if err := json.Unmarshal(season.Episodes, &episodes); err != nil {
		return nil, false, fmt.Errorf("failed to decode the TMDb season: %w", err)
	}
	for _, e := range episodes {
		if e.EpisodeNumber == *episodeNumber && e.Name != "" {
			episode.EpisodeName = new(e.Name)
		}
	}

	return episode, true, nil
}

func valueOf(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
