package nextepisode

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

const (
	// WatchedWindow は、ホームの「続きの放送」で、見た回に数える期間 (付録 A)。この間に再生した録画だけを見た回とする。
	// 昔に 1 話だけ見たシリーズを、薦め続けないためである。
	WatchedWindow = 30 * 24 * time.Hour
	// WatchedRatio は、録画の長さのうち、ここまで再生したら見た回とみなす割合 (付録 A)。
	// アニメの記録の「見たとみなす割合」(既定 20%) は、記録先に送るための値で、連携を無効にすると変えられないので使わない。
	WatchedRatio = 0.5
	// MaxSeries は、ホームの「続きの放送」に出すシリーズの最大の数 (付録 A)。
	MaxSeries = 10
	// maxWatched は、見た回として読む録画の最大の数。最後に見た新しい順に読む。
	maxWatched = 200
)

// Suggestion は、ホームの「続きの放送」の 1 行。ユーザーが見たシリーズの、次の放送。
type Suggestion struct {
	// RecordingID は、シリーズの見た回のうち、話数の最も大きい録画。
	RecordingID uuid.UUID
	// Broadcast は、シリーズの続きの放送のうち、最も早い放送。
	Broadcast *Broadcast
}

// watchedSeries は、見た回をシリーズでまとめたもの。key は、シリーズの見た回のうち、話数の最も大きい回の鍵。
type watchedSeries struct {
	key         *Key
	recordingID uuid.UUID
}

// id は、シリーズを見分ける文字列。TID で探すシリーズと、シリーズ名で探すシリーズと、TMDb の作品で探すシリーズは、別のものとする。
// TMDb の作品は、シーズンごとに別のシリーズとする。
func (k *Key) id() string {
	if k.TID > 0 {
		return animeID(k.TID)
	}
	if k.Drama != nil {
		return dramaID(k.Drama.SeriesID, k.Drama.Season)
	}

	return seriesID(k.Series)
}

// 鍵の強さの順。小さいほど強い。
const (
	rankAnime = iota
	rankSeries
	rankDrama
)

// rank は、鍵の強さの順。TID、シリーズ名、TMDb の作品の順に強い。
func (k *Key) rank() int {
	switch {
	case k.TID > 0:
		return rankAnime
	case k.Drama != nil:
		return rankDrama
	default:
		return rankSeries
	}
}

// animeID は、TID で探すシリーズの id。
func animeID(tid int32) string {
	return "t:" + strconv.FormatInt(int64(tid), 10)
}

// seriesID は、シリーズ名で探すシリーズの id。
func seriesID(series string) string {
	return "s:" + series
}

// dramaID は、TMDb の作品のシーズンで探すシリーズの id。
func dramaID(seriesID, season int32) string {
	return "d:" + strconv.FormatInt(int64(seriesID), 10) + ":" + strconv.FormatInt(int64(season), 10)
}

// number は、比べるための話数。
func (k *Key) number() float64 {
	if k.TID > 0 {
		return k.Count
	}
	if k.Drama != nil {
		return float64(k.Drama.Episode)
	}

	return float64(k.Number)
}

// FindForUser は、ユーザー userID が見たシリーズごとに、now から Horizon までに始まる続きの放送のうち最も早い 1 件を、
// 放送の早い順に MaxSeries 件まで返す (#1564、ホームの「続きの放送」)。
//
// 見た回は、WatchedWindow の間に、録画の長さの WatchedRatio 以上まで再生した録画である。シリーズごとに、見た回の最大の話数より
// 後の放送を、録画の続きの放送 (Find) と同じ決まりで選ぶ。シリーズの数によらず、決まった数のクエリで引く。
func FindForUser(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, now time.Time) ([]*Suggestion, error) {
	q := sqlc.New(db)
	rows, err := q.ListWatchedNextEpisodeSources(ctx, sqlc.ListWatchedNextEpisodeSourcesParams{
		UserID: userID, Since: now.Add(-WatchedWindow), MinRatio: WatchedRatio, MaxRows: maxWatched,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the watched recordings: %w", err)
	}
	series := groupWatched(rows)
	if len(series) == 0 {
		return []*Suggestion{}, nil
	}
	candidates, recorded, err := listCandidatesOf(ctx, q, series, now)
	if err != nil {
		return nil, err
	}
	all := []*Candidate{}
	for _, cs := range candidates {
		all = append(all, cs...)
	}
	reserved, err := reservedPrograms(ctx, q, all)
	if err != nil {
		return nil, err
	}

	suggestions := []*Suggestion{}
	// 同じ番組が、違う鍵で探したシリーズに出たら、同じ作品なので、強い鍵 (TID、シリーズ名、TMDb の作品の順) で探した方の 1 行にする。
	// 後の方を、その次の放送に繰り下げて出すと、同じ作品の行が 2 つ並ぶ。
	seen := map[ProgramKey]bool{}
	for _, s := range series {
		id := s.key.id()
		broadcasts := Pick(candidates[id], reserved, recorded[id])
		if len(broadcasts) == 0 {
			continue
		}
		b := broadcasts[0]
		key := ProgramKey{ServiceID: b.ServiceID, EventID: b.EventID}
		if seen[key] {
			continue
		}
		seen[key] = true
		suggestions = append(suggestions, &Suggestion{RecordingID: s.recordingID, Broadcast: b})
	}
	slices.SortStableFunc(suggestions, func(a, b *Suggestion) int {
		return cmp.Or(
			a.Broadcast.StartAt.Compare(b.Broadcast.StartAt),
			cmp.Compare(a.Broadcast.ServiceID, b.Broadcast.ServiceID),
			cmp.Compare(a.Broadcast.EventID, b.Broadcast.EventID),
		)
	})
	if len(suggestions) > MaxSeries {
		suggestions = suggestions[:MaxSeries]
	}

	return suggestions, nil
}

// groupWatched は、見た回をシリーズでまとめ、シリーズごとに話数の最も大きい回を残す。シリーズも話数も読めない回は外す。
// 返す順は、強い鍵 (TID、シリーズ名、TMDb の作品の順) で探すシリーズを先に、それぞれシリーズの回を最初に読んだ順 (最後に見た新しい順) である。
func groupWatched(rows []sqlc.ListWatchedNextEpisodeSourcesRow) []*watchedSeries {
	series := []*watchedSeries{}
	index := map[string]int{}
	for i := range rows {
		row := sqlc.GetRecordingNextEpisodeSourceRow{
			Name: rows[i].Name, Description: rows[i].Description, Extended: rows[i].Extended,
			ProgramSeriesEpisode: rows[i].ProgramSeriesEpisode, ArchivedSeriesEpisode: rows[i].ArchivedSeriesEpisode,
			SyobocalTid: rows[i].SyobocalTid, CountStart: rows[i].CountStart,
			TmdbSeriesID: rows[i].TmdbSeriesID, TmdbSeasonNumber: rows[i].TmdbSeasonNumber, TmdbEpisodeNumber: rows[i].TmdbEpisodeNumber,
		}
		key := KeyOf(sourceOf(&row), dramaOf(&row))
		if key == nil {
			continue
		}
		j, ok := index[key.id()]
		if !ok {
			index[key.id()] = len(series)
			series = append(series, &watchedSeries{key: key, recordingID: rows[i].ID})

			continue
		}
		if key.number() > series[j].key.number() {
			series[j] = &watchedSeries{key: key, recordingID: rows[i].ID}
		}
	}
	// 強い鍵で探すシリーズを先に置き、同じ番組に当たった弱い鍵で探すシリーズより優先する。
	slices.SortStableFunc(series, func(a, b *watchedSeries) int {
		return cmp.Compare(a.key.rank(), b.key.rank())
	})

	return series
}

// listCandidatesOf は、シリーズごとの続きの放送の候補と、録画履歴にある録った回の話数を、シリーズの id ごとに返す。
// TID で探すシリーズと、シリーズ名で探すシリーズを、それぞれ 1 つのクエリでまとめて引く。
func listCandidatesOf(
	ctx context.Context, q *sqlc.Queries, series []*watchedSeries, now time.Time,
) (map[string][]*Candidate, map[string]map[float64]bool, error) {
	candidates := map[string][]*Candidate{}
	recorded := map[string]map[float64]bool{}
	tids, counts := []int32{}, []float64{}
	names, numbers := []string{}, []int64{}
	dramas := []*Drama{}
	for _, s := range series {
		recorded[s.key.id()] = map[float64]bool{}
		switch {
		case s.key.TID > 0:
			tids, counts = append(tids, s.key.TID), append(counts, s.key.Count)
		case s.key.Drama != nil:
			dramas = append(dramas, s.key.Drama)
		default:
			names, numbers = append(names, s.key.Series), append(numbers, int64(s.key.Number))
		}
	}

	if len(tids) > 0 {
		rows, err := q.ListAnimeNextProgramsBatch(ctx, sqlc.ListAnimeNextProgramsBatchParams{
			Tids: tids, Counts: counts, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the next programs of the anime: %w", err)
		}
		for i := range rows {
			id := animeID(rows[i].KeyTid)
			candidates[id] = append(candidates[id], &Candidate{
				ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
				RelatedItems: rows[i].RelatedItems,
			})
		}
		history, err := q.ListRecordedAnimeCountsBatch(ctx, tids)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the recorded episodes of the anime: %w", err)
		}
		for _, h := range history {
			recorded[animeID(h.KeyTid)][h.AnimeCount] = true
		}
	}

	if len(names) > 0 {
		rows, err := q.ListSeriesNextProgramsBatch(ctx, sqlc.ListSeriesNextProgramsBatchParams{
			Series: names, Numbers: numbers, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the next programs of the series: %w", err)
		}
		for i := range rows {
			id := seriesID(rows[i].KeySeries)
			candidates[id] = append(candidates[id], &Candidate{
				ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
				RelatedItems: rows[i].RelatedItems,
			})
		}
		history, err := q.ListRecordedSeriesEpisodesBatch(ctx, names)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the recorded episodes of the series: %w", err)
		}
		for i := range history {
			ep := episode.Parse(&episode.Source{
				Name: history[i].Name, Description: history[i].Description, Extended: "", SeriesEpisode: history[i].SeriesEpisode, Anime: nil,
			})
			if ep.Series == history[i].KeySeries && ep.Number > 0 {
				recorded[seriesID(history[i].KeySeries)][float64(ep.Number)] = true
			}
		}
	}

	if len(dramas) > 0 {
		if err := listDramaCandidatesOf(ctx, q, dramas, now, candidates, recorded); err != nil {
			return nil, nil, err
		}
	}

	return candidates, recorded, nil
}

// listDramaCandidatesOf は、TMDb の作品のシーズン dramas ごとの続きの放送の候補と、録画履歴にある録った話の番号を、
// シリーズの id ごとに candidates と recorded に足す。候補の番組と録画履歴は、それぞれまとめて引く。
// 録画の続きの放送 (listDramaCandidates) と同じく、今のシーズンの後の話がこれから放送されないシリーズは、次のシーズンの第 1 話を候補にする。
func listDramaCandidatesOf(
	ctx context.Context, q *sqlc.Queries, dramas []*Drama, now time.Time,
	candidates map[string][]*Candidate, recorded map[string]map[float64]bool,
) error {
	current, err := listDramaProgramsOf(ctx, q, dramas, now)
	if err != nil {
		return err
	}
	// seasonOf は、シリーズの id ごとの、候補を出すシーズン。録った話を、このシーズンの録画履歴で引く。
	seasonOf := map[string]*Drama{}
	nexts := []*Drama{}
	for _, d := range dramas {
		id := dramaID(d.SeriesID, d.Season)
		if cs := current[id]; len(cs) > 0 {
			candidates[id] = cs
			seasonOf[id] = d

			continue
		}
		next := &Drama{SeriesID: d.SeriesID, Season: nextSeason(d.Season), Episode: 0}
		nexts = append(nexts, next)
		seasonOf[id] = next
	}
	if len(nexts) > 0 {
		next, err := listDramaProgramsOf(ctx, q, nexts, now)
		if err != nil {
			return err
		}
		for _, d := range nexts {
			// 次のシーズンの候補は、元のシーズンのシリーズの id に足す。
			if cs := firstEpisodes(next[dramaID(d.SeriesID, d.Season)]); len(cs) > 0 {
				candidates[dramaID(d.SeriesID, d.Season-1)] = cs
			}
		}
	}

	seriesIDs, seasons := make([]int32, 0, len(seasonOf)), make([]int32, 0, len(seasonOf))
	// ids は、作品とシーズンごとの、そのシーズンの録画履歴を使うシリーズの id。
	ids := map[string][]string{}
	for id, d := range seasonOf {
		key := dramaID(d.SeriesID, d.Season)
		if _, ok := ids[key]; !ok {
			seriesIDs, seasons = append(seriesIDs, d.SeriesID), append(seasons, d.Season)
		}
		ids[key] = append(ids[key], id)
	}
	history, err := q.ListRecordedDramaEpisodesBatch(ctx, sqlc.ListRecordedDramaEpisodesBatchParams{SeriesIds: seriesIDs, Seasons: seasons})
	if err != nil {
		return fmt.Errorf("failed to list the recorded episodes of the drama: %w", err)
	}
	for _, h := range history {
		for _, id := range ids[dramaID(h.KeySeriesID, h.KeySeason)] {
			recorded[id][h.EpisodeNumber] = true
		}
	}

	return nil
}

// listDramaProgramsOf は、TMDb の作品のシーズン dramas ごとに、話の番号が Episode より大きい話の、now から Horizon までに始まる放送を、
// 作品とシーズンの id ごとに返す。1 つのクエリでまとめて引く。
func listDramaProgramsOf(ctx context.Context, q *sqlc.Queries, dramas []*Drama, now time.Time) (map[string][]*Candidate, error) {
	seriesIDs, seasons, episodes := make([]int32, 0, len(dramas)), make([]int32, 0, len(dramas)), make([]int32, 0, len(dramas))
	for _, d := range dramas {
		seriesIDs, seasons, episodes = append(seriesIDs, d.SeriesID), append(seasons, d.Season), append(episodes, d.Episode)
	}
	rows, err := q.ListDramaNextProgramsBatch(ctx, sqlc.ListDramaNextProgramsBatchParams{
		SeriesIds: seriesIDs, Seasons: seasons, Episodes: episodes, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the next programs of the drama: %w", err)
	}
	candidates := map[string][]*Candidate{}
	for i := range rows {
		id := dramaID(rows[i].KeySeriesID, rows[i].KeySeason)
		candidates[id] = append(candidates[id], &Candidate{
			ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
			RelatedItems: rows[i].RelatedItems,
		})
	}

	return candidates, nil
}
