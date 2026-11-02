package graph

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/animeimage"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// animeLibraryWorkType は、視聴管理の画面の作品の ID の種類。
const animeLibraryWorkType = "AnimeLibraryWork"

// animeLibrary は、ログイン中のユーザーの見たいリストの作品を、次の話と次の放送と録画と並べて返す (#1231)。
// 管理者が連携を無効にしていれば、何も引かずに、無効であることだけを返す。
func (r *Resolver) animeLibrary(ctx context.Context) (*model.AnimeLibrary, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	if !system.AnimeLinkingEnabled {
		return &model.AnimeLibrary{Enabled: false, Imports: []*model.AnimeWishlistImport{}, Works: []*model.AnimeLibraryWork{}}, nil
	}
	library, err := service.Library(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	programs, err := r.libraryPrograms(ctx, library.Works)
	if err != nil {
		return nil, err
	}
	recordings, err := r.libraryRecordings(ctx, library.Works)
	if err != nil {
		return nil, err
	}
	result := &model.AnimeLibrary{Enabled: true, Imports: make([]*model.AnimeWishlistImport, 0, len(library.Imports)), Works: []*model.AnimeLibraryWork{}}
	for _, imported := range library.Imports {
		result.Imports = append(result.Imports, &model.AnimeWishlistImport{
			Destination: destinationOf(imported.Destination), CheckedAt: imported.CheckedAt, ImportedAt: imported.ImportedAt,
			Error: imported.Error, EntryCount: imported.Entries,
		})
	}
	for _, work := range library.Works {
		result.Works = append(result.Works, libraryWorkModel(work, programs, recordings))
	}

	return result, nil
}

// libraryWorkModel は、視聴管理の画面の作品を、GraphQL の形にする。引いた後に消えた番組と録画は飛ばす。
func libraryWorkModel(work *animerecord.LibraryWork, programs map[string]*model.Program, recordings map[uuid.UUID]*model.Recording) *model.AnimeLibraryWork {
	status := model.AnimeWishlistStatusWannaWatch
	if work.Watching() {
		status = model.AnimeWishlistStatusWatching
	}
	sources := make([]*model.AnimeLibrarySource, 0, len(work.Sources))
	for _, source := range work.Sources {
		sources = append(sources, &model.AnimeLibrarySource{
			Destination: destinationOf(source.Destination), WorkID: int(source.WorkID), Status: wishlistStatusOf(source.Status),
			URL: wishlistWorkURL(source.Destination, source.WorkID), Progress: intOf(source.Progress),
		})
	}
	result := &model.AnimeLibraryWork{
		ID: GlobalID(animeLibraryWorkType, libraryWorkKey(work)), Tid: intOf(work.TID), Title: work.Title, ImageURL: nil, Status: status, Sources: sources,
		Progress: intOf(work.Progress), EpisodesCount: intOf(work.EpisodesCount), SeasonYear: intOf(work.SeasonYear),
		Season: seasonOf(work.SeasonName), NextEpisode: nil, NextBroadcast: nil, Broadcasts: []*model.AnimeLibraryBroadcast{},
		RecordedEpisodes: []*model.AnimeLibraryRecordedEpisode{}, RecentBroadcasts: make([]*model.AnimeLibraryPastBroadcast, 0, len(work.RecentBroadcasts)),
	}
	for _, past := range work.RecentBroadcasts {
		result.RecentBroadcasts = append(result.RecentBroadcasts, &model.AnimeLibraryPastBroadcast{StartAt: past.StartAt, Rerun: past.Rerun, Last: past.Last})
	}
	if work.Image != nil {
		result.ImageURL = new(animeimage.Path(work.Image.Destination, work.Image.WorkID, work.Image.URL))
	}
	if next := work.NextEpisode; next != nil {
		result.NextEpisode = &model.AnimeLibraryEpisode{Number: intOf(next.Number), NumberText: nonEmpty(next.NumberText), Title: nonEmpty(next.Title)}
	}
	for _, broadcast := range work.Broadcasts {
		program, ok := programs[programKeyOf(broadcast.ServiceID, broadcast.EventID)]
		if !ok {
			continue
		}
		converted := &model.AnimeLibraryBroadcast{
			Program: program, CountStart: broadcast.CountStart, CountEnd: broadcast.CountEnd, Rerun: broadcast.Rerun, Last: broadcast.Last,
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
		result.RecordedEpisodes = append(result.RecordedEpisodes, &model.AnimeLibraryRecordedEpisode{
			CountStart: episode.CountStart, CountEnd: episode.CountEnd, Subtitle: episode.Subtitle, Recording: latest,
			RecordingCount: found, Watched: episode.Watched,
		})
	}

	return result
}

// libraryWorkKey は、作品を見分ける ID の中身。TID があれば TID を、なければ記録先の作品を使う。
func libraryWorkKey(work *animerecord.LibraryWork) string {
	if work.TID != nil {
		return "tid:" + strconv.Itoa(int(*work.TID))
	}
	source := work.Sources[0]

	return source.Destination + ":" + strconv.Itoa(int(source.WorkID))
}

// libraryPrograms は、作品の放送の番組を、番組表と同じ形で引く。
func (r *Resolver) libraryPrograms(ctx context.Context, works []*animerecord.LibraryWork) (map[string]*model.Program, error) {
	serviceIDs, eventIDs := []int64{}, []int64{}
	for _, work := range works {
		for _, broadcast := range work.Broadcasts {
			serviceIDs, eventIDs = append(serviceIDs, broadcast.ServiceID), append(eventIDs, broadcast.EventID)
		}
	}

	return r.programsByKeys(ctx, serviceIDs, eventIDs)
}

// libraryRecordings は、作品の録画を、録画の一覧と同じ形で引く。
func (r *Resolver) libraryRecordings(ctx context.Context, works []*animerecord.LibraryWork) (map[uuid.UUID]*model.Recording, error) {
	ids := []uuid.UUID{}
	for _, work := range works {
		for _, episode := range work.RecordedEpisodes {
			ids = append(ids, episode.RecordingIDs...)
		}
	}
	result := map[uuid.UUID]*model.Recording{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.queries.ListRecordingsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings of the library: %w", err)
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

// programsByKeys は、番組 (サービス, eventId) を、番組表と同じ形で引く。
func (r *Resolver) programsByKeys(ctx context.Context, serviceIDs, eventIDs []int64) (map[string]*model.Program, error) {
	programs := map[string]*model.Program{}
	if len(serviceIDs) == 0 {
		return programs, nil
	}
	rows, err := r.queries.ListGuideProgramsByKeys(ctx, query.ListGuideProgramsByKeysParams{ServiceIDs: serviceIDs, EventIDs: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs: %w", err)
	}
	for i := range rows {
		program, err := toProgram(&rows[i])
		if err != nil {
			return nil, err
		}
		programs[programKeyOf(rows[i].ServiceID, rows[i].EventID)] = program
	}

	return programs, nil
}

// seasonOf は、作品の放送のシーズンを、GraphQL の形にする。分からなければ nil を返す。
func seasonOf(name string) *model.AnimeSeason {
	switch name {
	case animerecord.SeasonWinter:
		return new(model.AnimeSeasonWinter)
	case animerecord.SeasonSpring:
		return new(model.AnimeSeasonSpring)
	case animerecord.SeasonSummer:
		return new(model.AnimeSeasonSummer)
	case animerecord.SeasonAutumn:
		return new(model.AnimeSeasonAutumn)
	default:
		return nil
	}
}
