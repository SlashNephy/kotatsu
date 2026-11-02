package anime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// LinkedEpisode は、番組や録画に結びついた話と、その作品 (設計書 3.8)。
type LinkedEpisode struct {
	*Episode

	// Works は、TID に対応する作品。対応がなければ空。
	Works []*Work
}

// ProgramEpisodes は、番組 (serviceID, startAt) に結びついた話を、放送の順に返す。
func ProgramEpisodes(ctx context.Context, db sqlc.DBTX, serviceID int64, startAt time.Time) ([]*LinkedEpisode, error) {
	rows, err := sqlc.New(db).ListProgramAnimeEpisodes(ctx, sqlc.ListProgramAnimeEpisodesParams{
		ServiceIds: []int64{serviceID}, StartAts: []time.Time{startAt},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime episodes of the program: %w", err)
	}
	episodes := make([]*Episode, 0, len(rows))
	for _, row := range rows {
		episodes = append(episodes, &Episode{
			Source: row.Source, TID: row.SyobocalTid, PID: row.SyobocalPid, CountStart: row.CountStart, CountEnd: row.CountEnd,
			Subtitle: row.Subtitle, First: row.IsFirst, Last: row.IsLast, Rerun: row.IsRerun,
		})
	}

	return linked(ctx, db, episodes)
}

// RecordingEpisodes は、録画 recordingID に結びついた話を、放送の順に返す。
func RecordingEpisodes(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) ([]*LinkedEpisode, error) {
	rows, err := sqlc.New(db).ListRecordingAnimeEpisodes(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime episodes of the recording: %w", err)
	}
	episodes := make([]*Episode, 0, len(rows))
	for _, row := range rows {
		episodes = append(episodes, &Episode{
			Source: row.Source, TID: row.SyobocalTid, PID: row.SyobocalPid, CountStart: row.CountStart, CountEnd: row.CountEnd,
			Subtitle: row.Subtitle, First: row.IsFirst, Last: row.IsLast, Rerun: row.IsRerun,
		})
	}

	return linked(ctx, db, episodes)
}

// linked は、話に、TID に対応する作品を添える。作品は、使うときに作品 ID の対応から引く。
func linked(ctx context.Context, db sqlc.DBTX, episodes []*Episode) ([]*LinkedEpisode, error) {
	tids := []int32{}
	for _, episode := range episodes {
		if episode.TID != nil {
			tids = append(tids, *episode.TID)
		}
	}
	works, err := ListWorks(ctx, db, tids)
	if err != nil {
		return nil, err
	}
	result := make([]*LinkedEpisode, 0, len(episodes))
	for _, episode := range episodes {
		found := []*Work{}
		if episode.TID != nil {
			found = append(found, works[*episode.TID]...)
		}
		result = append(result, &LinkedEpisode{Episode: episode, Works: found})
	}

	return result, nil
}
