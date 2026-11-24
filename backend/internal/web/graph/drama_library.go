package graph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// dramaLibraryWorkType は、ドラマの視聴管理の画面の作品の ID の種類。
const dramaLibraryWorkType = "DramaLibraryWork"

// dramaLibrary は、ログイン中のユーザーが見ているドラマの作品を、次の話と次の放送と録画と並べて返す (#1652)。
// 管理者が TMDB との連携を無効にしていれば、何も引かずに、無効であることだけを返す。
func (r *Resolver) dramaLibrary(ctx context.Context) (*model.DramaLibrary, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	if !system.DramaLinkingEnabled {
		return &model.DramaLibrary{Enabled: false, Works: []*model.DramaLibraryWork{}}, nil
	}
	works, err := dramalibrary.Library(ctx, r.db, principal.UserID, r.clock.Now())
	if err != nil {
		return nil, err
	}
	serviceIDs, eventIDs, recordingIDs := []int64{}, []int64{}, []uuid.UUID{}
	for _, work := range works {
		for _, broadcast := range work.Broadcasts {
			serviceIDs, eventIDs = append(serviceIDs, broadcast.ServiceID), append(eventIDs, broadcast.EventID)
		}
		for _, episode := range work.RecordedEpisodes {
			recordingIDs = append(recordingIDs, episode.RecordingIDs...)
		}
	}
	programs, err := r.programsByKeys(ctx, serviceIDs, eventIDs)
	if err != nil {
		return nil, err
	}
	recordings, err := r.recordingsByIDs(ctx, recordingIDs)
	if err != nil {
		return nil, err
	}
	result := &model.DramaLibrary{Enabled: true, Works: make([]*model.DramaLibraryWork, 0, len(works))}
	for _, work := range works {
		result.Works = append(result.Works, dramaLibraryWorkModel(work, programs, recordings))
	}

	return result, nil
}

// recordingsByIDs は、録画を、録画の一覧と同じ形で取得する。
func (r *Resolver) recordingsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*model.Recording, error) {
	result := map[uuid.UUID]*model.Recording{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.queries.ListRecordingsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings: %w", err)
	}
	recordings, err := r.recordingModels(ctx, rows)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		result[rows[i].ID] = recordings[i]
	}

	return result, nil
}

// dramaLibraryWorkModel は、ドラマの視聴管理の画面の作品を、GraphQL の形にする。引いた後に消えた番組と録画は飛ばす。
func dramaLibraryWorkModel(
	work *dramalibrary.Work, programs map[string]*model.Program, recordings map[uuid.UUID]*model.Recording,
) *model.DramaLibraryWork {
	seriesURL := fmt.Sprintf("%s/tv/%d", tmdbWebURL, work.SeriesID)
	result := &model.DramaLibraryWork{
		ID: GlobalID(dramaLibraryWorkType, strconv.Itoa(int(work.SeriesID))),
		Series: &model.DramaSeries{
			TmdbID: int(work.SeriesID), Name: work.Name, OriginalName: work.OriginalName, URL: seriesURL, WatchStatus: nil,
		},
		Status: dramaWatchStatusModel(work.Status), AddedAt: work.AddedAt,
		LastWatched: dramaLibraryEpisodeModel(seriesURL, work.LastWatched), NextEpisode: dramaLibraryEpisodeModel(seriesURL, work.NextEpisode),
		NextBroadcast: nil, Broadcasts: []*model.DramaLibraryBroadcast{}, RecentBroadcast: nil,
		RecordedEpisodes: []*model.DramaLibraryRecordedEpisode{},
	}
	if recent := work.RecentBroadcast; recent != nil {
		result.RecentBroadcast = &model.DramaLibraryPastBroadcast{StartAt: recent.StartAt, Last: recent.Last}
	}
	for _, broadcast := range work.Broadcasts {
		program, ok := programs[programKeyOf(broadcast.ServiceID, broadcast.EventID)]
		if !ok {
			continue
		}
		converted := &model.DramaLibraryBroadcast{
			Program: program, SeasonNumber: intOf(broadcast.Season), EpisodeNumber: intOf(broadcast.Number), Rerun: broadcast.Rerun, Last: broadcast.Last,
		}
		result.Broadcasts = append(result.Broadcasts, converted)
		if broadcast == work.NextBroadcast {
			result.NextBroadcast = converted
		}
	}
	for _, episode := range work.RecordedEpisodes {
		var latest *model.Recording
		found := 0
		for _, id := range episode.RecordingIDs {
			if recording, ok := recordings[id]; ok {
				if latest == nil {
					latest = recording
				}
				found++
			}
		}
		if latest == nil {
			continue
		}
		result.RecordedEpisodes = append(result.RecordedEpisodes, &model.DramaLibraryRecordedEpisode{
			SeasonNumber: int(episode.Season), EpisodeNumber: int(episode.Number), Name: episode.Name, Recording: latest,
			RecordingCount: found, Watched: episode.Watched,
		})
	}

	return result
}

// dramaLibraryEpisodeModel は、作品の 1 話を GraphQL の形にする。nil なら nil を返す。
func dramaLibraryEpisodeModel(seriesURL string, episode *dramalibrary.LibraryEpisode) *model.DramaLibraryEpisode {
	if episode == nil {
		return nil
	}

	return &model.DramaLibraryEpisode{
		SeasonNumber: int(episode.Season), EpisodeNumber: int(episode.Number), Name: episode.Name,
		URL: fmt.Sprintf("%s/season/%d/episode/%d", seriesURL, episode.Season, episode.Number),
	}
}

// dramaWatchStatusModel は、見ている作品の状態を GraphQL の形にする。
func dramaWatchStatusModel(status string) model.DramaWatchStatus {
	if status == dramalibrary.StatusWannaWatch {
		return model.DramaWatchStatusWannaWatch
	}

	return model.DramaWatchStatusWatching
}

// dramaWatchStatusOf は、GraphQL の状態を、usecase の形にする。nil なら空 (外す) を返す。
func dramaWatchStatusOf(status *model.DramaWatchStatus) string {
	switch {
	case status == nil:
		return ""
	case *status == model.DramaWatchStatusWannaWatch:
		return dramalibrary.StatusWannaWatch
	default:
		return dramalibrary.StatusWatching
	}
}

// dramaNumberOf は、GraphQL の Int を、TMDB のシーズンや話の番号にする。int32 に収まらなければ BAD_USER_INPUT を返す。
func dramaNumberOf(value int) (int32, error) {
	if value < 0 || value > math.MaxInt32 {
		return 0, NewError(CodeBadUserInput, "season and episode numbers must be non-negative")
	}

	return int32(value), nil
}

// dramaLibraryError は、見ている作品と見た話のエラーを、エラーコードを持つエラーにする。
func dramaLibraryError(err error) error {
	switch {
	case errors.Is(err, dramalibrary.ErrUnavailable):
		return NewError(CodeConflict, "drama linking is not available; ask an administrator to enable it")
	case errors.Is(err, dramalibrary.ErrNotLinked):
		return NewError(CodeNotFound, "the TMDB series is not linked to any program or recording")
	case errors.Is(err, dramalibrary.ErrInvalidEpisode), errors.Is(err, dramalibrary.ErrInvalidStatus):
		return NewError(CodeBadUserInput, err.Error())
	default:
		return err
	}
}

// setDramaWatchStatus は、ログイン中のユーザーの見ている作品を変え、変えた後の作品を返す。名前は TMDB のキャッシュから引く。
func (r *Resolver) setDramaWatchStatus(ctx context.Context, seriesID int, status *model.DramaWatchStatus) (*model.DramaSeries, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	id, err := tmdbSeriesIDOf(seriesID)
	if err != nil {
		return nil, err
	}
	if err := dramalibrary.SetStatus(ctx, r.db, principal.UserID, id, dramaWatchStatusOf(status), r.clock.Now()); err != nil {
		return nil, dramaLibraryError(err)
	}
	name, originalName, err := dramalibrary.SeriesName(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	series := &model.DramaSeries{
		TmdbID: int(id), Name: name, OriginalName: originalName, URL: fmt.Sprintf("%s/tv/%d", tmdbWebURL, id), WatchStatus: nil,
	}

	return series, nil
}

// setDramaEpisodeWatched は、ログイン中のユーザーが話を見たことを、手で付けるか外す。
func (r *Resolver) setDramaEpisodeWatched(ctx context.Context, seriesID, seasonNumber, episodeNumber int, watched bool) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	id, err := tmdbSeriesIDOf(seriesID)
	if err != nil {
		return false, err
	}
	season, err := dramaNumberOf(seasonNumber)
	if err != nil {
		return false, err
	}
	number, err := dramaNumberOf(episodeNumber)
	if err != nil {
		return false, err
	}
	episode := &dramalibrary.Episode{SeriesID: id, Season: season, Number: number}
	if err := dramalibrary.SetWatched(ctx, r.db, principal.UserID, episode, watched, r.clock.Now()); err != nil {
		return false, dramaLibraryError(err)
	}

	return watched, nil
}

// dramaSeriesWatchStatus は、ログイン中のユーザーが、作品を見ている作品に入れた状態を返す。入れていなければ nil を返す。
func (r *Resolver) dramaSeriesWatchStatus(ctx context.Context, series *model.DramaSeries) (*model.DramaWatchStatus, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	id, err := tmdbSeriesIDOf(series.TmdbID)
	if err != nil {
		return nil, err
	}
	status, err := dramalibrary.Status(ctx, r.db, principal.UserID, id)
	if err != nil || status == "" {
		return nil, err
	}

	return new(dramaWatchStatusModel(status)), nil
}

// dramaEpisodeWatched は、ログイン中のユーザーが、話を見たかどうかを返す。話を決められなければ false を返す。
func (r *Resolver) dramaEpisodeWatched(ctx context.Context, episode *model.DramaEpisode) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	if episode.SeasonNumber == nil || episode.EpisodeNumber == nil {
		return false, nil
	}
	id, err := tmdbSeriesIDOf(episode.Series.TmdbID)
	if err != nil {
		return false, err
	}
	season, err := dramaNumberOf(*episode.SeasonNumber)
	if err != nil {
		return false, err
	}
	number, err := dramaNumberOf(*episode.EpisodeNumber)
	if err != nil {
		return false, err
	}

	return dramalibrary.Watched(ctx, r.db, principal.UserID, &dramalibrary.Episode{SeriesID: id, Season: season, Number: number})
}
