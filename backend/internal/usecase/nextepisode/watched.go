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
	// stem は、シリーズ名で探すシリーズの見た回の番組名が「作品名 副題 #N」の形のときの、副題を除いた作品名 (episode.Stem)。
	stem string
	// family なら、シリーズ名が key.Series で始まり、番組名の作品名 (episode.Stem) が key.Series の番組も、同じシリーズの回とする (#2023)。
	family bool
}

// id は、シリーズを見分ける文字列。TID で探すシリーズと、シリーズ名で探すシリーズと、TMDB の作品で探すシリーズは、別のものとする。
// TMDB の作品は、シーズンごとに別のシリーズとする。
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

// rank は、鍵の強さの順。TID、シリーズ名、TMDB の作品の順に強い。
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

// dramaID は、TMDB の作品のシーズンで探すシリーズの id。
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
	if series, err = mergeFamilies(ctx, q, series); err != nil {
		return nil, err
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
	// 同じ番組が、違う鍵で探したシリーズに出たら、同じ作品なので、強い鍵 (TID、シリーズ名、TMDB の作品の順) で探した方の 1 行にする。
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
// 返す順は、強い鍵 (TID、シリーズ名、TMDB の作品の順) で探すシリーズを先に、それぞれシリーズの回を最初に読んだ順 (最後に見た新しい順) である。
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
		stem := ""
		if key.rank() == rankSeries {
			stem = episode.Stem(rows[i].Name)
		}
		j, ok := index[key.id()]
		if !ok {
			index[key.id()] = len(series)
			series = append(series, &watchedSeries{key: key, recordingID: rows[i].ID, stem: stem, family: false})

			continue
		}
		if key.number() > series[j].key.number() {
			series[j] = &watchedSeries{key: key, recordingID: rows[i].ID, stem: stem, family: false}
		}
	}
	// 強い鍵で探すシリーズを先に置き、同じ番組に当たった弱い鍵で探すシリーズより優先する。
	slices.SortStableFunc(series, func(a, b *watchedSeries) int {
		return cmp.Compare(a.key.rank(), b.key.rank())
	})

	return series
}

// mergeFamilies は、番組名に副題が入ってシリーズ名が回ごとに変わった見た回を、副題を除いた作品名のシリーズにまとめる (#2023)。
//
// 「作品名 副題 #N」の番組名は、副題までをシリーズ名に読むので、回ごとに別のシリーズになる。番組名だけでは、最後の語が副題か
// 作品名の一部かを決められないので、副題のない回 (「作品名 #N」) が、見た回、番組表、録画履歴のどれかにあるときだけ、まとめる。
// まとめたシリーズは、見た回の最大の話数を持ち、作品名のシリーズ名の番組に加えて、番組名の作品名が同じ番組も続きの候補にする。
// 見た回の再放送 (副題のない番組名のことがある) を、見た回より前の回として外し、次の回 (副題のある番組名) を出すためである。
func mergeFamilies(ctx context.Context, q *sqlc.Queries, series []*watchedSeries) ([]*watchedSeries, error) {
	stems := []string{}
	for _, s := range series {
		if s.stem != "" && !slices.Contains(stems, s.stem) {
			stems = append(stems, s.stem)
		}
	}
	if len(stems) == 0 {
		return series, nil
	}
	known, err := q.ListKnownSeries(ctx, stems)
	if err != nil {
		return nil, fmt.Errorf("failed to list the known series: %w", err)
	}
	for _, s := range series {
		if s.key.rank() == rankSeries {
			known = append(known, s.key.Series)
		}
	}
	merged := make([]*watchedSeries, 0, len(series))
	// bases は、まとめたシリーズを、作品名ごとに引く。
	bases := map[string]*watchedSeries{}
	base := func(stem string) *watchedSeries {
		if b, ok := bases[stem]; ok {
			return b
		}
		b := &watchedSeries{key: &Key{TID: 0, Count: 0, Series: stem, Number: 0, Drama: nil}, recordingID: uuid.Nil(), stem: "", family: true}
		bases[stem] = b
		merged = append(merged, b)

		return b
	}
	for _, s := range series {
		var b *watchedSeries
		switch {
		case s.key.rank() != rankSeries:
		case slices.Contains(stems, s.key.Series):
			// 副題のない回 (シリーズ名が作品名) のシリーズ。見た回に副題のある回がなければ、まとめるものがないので、そのまま残す。
			b = base(s.key.Series)
		case s.stem != "" && slices.Contains(known, s.stem):
			b = base(s.stem)
		}
		if b == nil {
			merged = append(merged, s)

			continue
		}
		if b.recordingID == uuid.Nil() || s.key.Number > b.key.Number {
			b.key = &Key{TID: 0, Count: 0, Series: b.key.Series, Number: s.key.Number, Drama: nil}
			b.recordingID = s.recordingID
		}
	}

	return merged, nil
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
	stems, stemNumbers := []string{}, []int64{}
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
			if s.family {
				stems, stemNumbers = append(stems, s.key.Series), append(stemNumbers, int64(s.key.Number))
			}
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
		if err := listSeriesCandidatesOf(ctx, q, names, numbers, now, candidates, recorded); err != nil {
			return nil, nil, err
		}
	}

	if len(stems) > 0 {
		if err := listFamilyCandidatesOf(ctx, q, stems, stemNumbers, now, candidates, recorded); err != nil {
			return nil, nil, err
		}
	}

	if len(dramas) > 0 {
		if err := listDramaCandidatesOf(ctx, q, dramas, now, candidates, recorded); err != nil {
			return nil, nil, err
		}
	}

	return candidates, recorded, nil
}

// listSeriesCandidatesOf は、シリーズ名 names[i] ごとに、話数が numbers[i] より大きい続きの放送の候補と、録画履歴にある録った回の話数を、
// candidates と recorded に足す。候補の番組と録画履歴は、それぞれまとめて引く。
func listSeriesCandidatesOf(
	ctx context.Context, q *sqlc.Queries, names []string, numbers []int64, now time.Time,
	candidates map[string][]*Candidate, recorded map[string]map[float64]bool,
) error {
	rows, err := q.ListSeriesNextProgramsBatch(ctx, sqlc.ListSeriesNextProgramsBatchParams{
		Series: names, Numbers: numbers, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
	})
	if err != nil {
		return fmt.Errorf("failed to list the next programs of the series: %w", err)
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
		return fmt.Errorf("failed to list the recorded episodes of the series: %w", err)
	}
	for i := range history {
		ep := episode.Parse(&episode.Source{
			Name: history[i].Name, Description: history[i].Description, Extended: "", SeriesEpisode: history[i].SeriesEpisode, Anime: nil,
		})
		if ep.Series == history[i].KeySeries && ep.Number > 0 {
			recorded[seriesID(history[i].KeySeries)][float64(ep.Number)] = true
		}
	}

	return nil
}

// listFamilyCandidatesOf は、まとめたシリーズ (mergeFamilies) の作品名 stems[i] ごとに、番組名に副題が入った回の、話数が numbers[i] より
// 大きい続きの放送の候補と、録画履歴にある録った回の話数を、candidates と recorded に足す (#2023)。
// シリーズ名が作品名で始まるだけの別の番組を拾わないよう、番組名から読み直した作品名 (episode.Stem) が同じものだけを使う。
func listFamilyCandidatesOf(
	ctx context.Context, q *sqlc.Queries, stems []string, numbers []int64, now time.Time,
	candidates map[string][]*Candidate, recorded map[string]map[float64]bool,
) error {
	rows, err := q.ListSeriesFamilyNextProgramsBatch(ctx, sqlc.ListSeriesFamilyNextProgramsBatchParams{
		Stems: stems, Numbers: numbers, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
	})
	if err != nil {
		return fmt.Errorf("failed to list the next programs of the series with subtitles: %w", err)
	}
	for i := range rows {
		if episode.Stem(rows[i].Name) != rows[i].KeyStem {
			continue
		}
		id := seriesID(rows[i].KeyStem)
		candidates[id] = append(candidates[id], &Candidate{
			ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
			RelatedItems: rows[i].RelatedItems,
		})
	}
	// 作品名のシリーズ名の候補と合わせたので、放送の早い順に並べ直す。
	for _, stem := range stems {
		slices.SortStableFunc(candidates[seriesID(stem)], func(a, b *Candidate) int {
			return cmp.Or(a.StartAt.Compare(b.StartAt), cmp.Compare(a.ServiceID, b.ServiceID), cmp.Compare(a.EventID, b.EventID))
		})
	}
	history, err := q.ListRecordedSeriesFamilyEpisodesBatch(ctx, stems)
	if err != nil {
		return fmt.Errorf("failed to list the recorded episodes of the series with subtitles: %w", err)
	}
	for i := range history {
		if episode.Stem(history[i].Name) != history[i].KeyStem {
			continue
		}
		ep := episode.Parse(&episode.Source{
			Name: history[i].Name, Description: history[i].Description, Extended: "", SeriesEpisode: history[i].SeriesEpisode, Anime: nil,
		})
		if ep.Number > 0 {
			recorded[seriesID(history[i].KeyStem)][float64(ep.Number)] = true
		}
	}

	return nil
}

// listDramaCandidatesOf は、TMDB の作品のシーズン dramas ごとの続きの放送の候補と、録画履歴にある録った話の番号を、
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

// listDramaProgramsOf は、TMDB の作品のシーズン dramas ごとに、話の番号が Episode より大きい話の、now から Horizon までに始まる放送を、
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
