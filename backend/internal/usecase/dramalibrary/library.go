package dramalibrary

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
)

// Horizon は、次の放送を探す範囲 (付録 A)。アニメの視聴管理の画面と同じく、今から 1 週間に始まる番組を探す。
const Horizon = 7 * 24 * time.Hour

// Work は、ドラマの視聴管理の画面の 1 作品 (#1652)。
type Work struct {
	SeriesID int32
	// Name と OriginalName は、TMDb のキャッシュにある作品の名前。キャッシュがなければ nil。
	Name         *string
	OriginalName *string
	// Status は、StatusWatching か StatusWannaWatch。
	Status  string
	AddedAt time.Time
	// LastWatched は、見た話のうち、最も後の話 (特別編を除く)。見た話がなければ nil。
	LastWatched *LibraryEpisode
	// NextEpisode は、次に見る話。見た話がないか、最後のシーズンの最後の話まで見ていれば nil。
	NextEpisode *LibraryEpisode
	// Broadcasts は、これからの放送 (放送中を含む)。放送の早い順に並べる。
	Broadcasts []*Broadcast
	// NextBroadcast は、次の放送。最後に見た話より後の話の放送のうち最も早いもの。なければ、最も早い放送。放送がなければ nil。
	NextBroadcast *Broadcast
	// RecentBroadcast は、この 1 週間に始まった最も新しい放送。なければ nil。
	RecentBroadcast *PastBroadcast
	// RecordedEpisodes は、録画のある話。シーズンと話の順に並べる。
	RecordedEpisodes []*RecordedEpisode
}

// title は、並べるときの作品名。
func (w *Work) title() string {
	switch {
	case w.Name != nil:
		return *w.Name
	case w.OriginalName != nil:
		return *w.OriginalName
	default:
		return ""
	}
}

// LibraryEpisode は、作品の 1 話と、TMDb のキャッシュにある話の名前。
type LibraryEpisode struct {
	Season int32
	Number int32
	// Name は、話の名前。キャッシュになければ nil。
	Name *string
}

// Broadcast は、作品の放送。
type Broadcast struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	// Season と Number は、放送する話。話を決められなかった放送では nil。
	Season *int32
	Number *int32
	// Rerun は再放送かどうか、Last は TMDb のシーズンの最後の話かどうか。
	Rerun bool
	Last  bool
}

// PastBroadcast は、作品の始まった放送。番組はアーカイブに移っていることがあるので、開始時刻と印だけを持つ。
type PastBroadcast struct {
	StartAt time.Time
	// Last は、TMDb のシーズンの最後の話かどうか。
	Last bool
}

// RecordedEpisode は、録画のある話。
type RecordedEpisode struct {
	Season int32
	Number int32
	Name   *string
	// RecordingIDs は、その話の録画。新しく録ったものを先に並べる。
	RecordingIDs []uuid.UUID
	// Watched は、見た話かどうか。見た話がひとつもない作品では nil。
	Watched *bool
}

// Library は、ユーザー userID の見ている作品と見たい作品を、作品ごとに、次の話と次の放送と録画と並べて返す (#1652)。
// 放送は、今から Horizon の間に始まるもの (放送中を含む) を、番組の結びつき (設計書 3.9) で引く。録画は、録画に写した結びつきで引き、
// 録れなかった録画と、話のない録画は外す。TMDb には問い合わせず、キャッシュにある名前とシーズンの話を使う。
func Library(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, now time.Time) ([]*Work, error) {
	q := sqlc.New(db)
	entries, err := q.ListDramaWatchlistEntries(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama watchlist: %w", err)
	}
	if len(entries) == 0 {
		return []*Work{}, nil
	}
	ids := make([]int32, 0, len(entries))
	for i := range entries {
		ids = append(ids, entries[i].TmdbSeriesID)
	}
	cache, err := loadCache(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	watched, err := q.ListDramaWatchedEpisodes(ctx, sqlc.ListDramaWatchedEpisodesParams{UserID: userID, TmdbSeriesIds: ids})
	if err != nil {
		return nil, fmt.Errorf("failed to list the watched drama episodes: %w", err)
	}
	programs, err := q.ListDramaLibraryPrograms(ctx, sqlc.ListDramaLibraryProgramsParams{TmdbSeriesIds: ids, Now: now, Until: now.Add(Horizon)})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs of the drama library: %w", err)
	}
	recent, err := q.ListDramaLibraryRecentBroadcasts(ctx, sqlc.ListDramaLibraryRecentBroadcastsParams{
		TmdbSeriesIds: ids, Since: now.Add(-Horizon), Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the recent broadcasts of the drama library: %w", err)
	}
	recordings, err := q.ListDramaLibraryRecordings(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings of the drama library: %w", err)
	}

	return arrange(&arrangeInput{entries: entries, cache: cache, watched: watched, programs: programs, recent: recent, recordings: recordings}), nil
}

// cache は、TMDb のキャッシュにある、作品とシーズンの話。
type cache struct {
	series map[int32]*sqlc.TmdbSeries
	// seasons は、作品ごとの、キャッシュにある作品のシーズンの一覧。
	seasons map[int32][]*tmdb.Season
	// episodes は、作品とシーズンごとの話。
	episodes map[seasonKey][]*tmdb.Episode
}

type seasonKey struct {
	series int32
	season int32
}

// loadCache は、作品 ids の、TMDb のキャッシュを読む。
func loadCache(ctx context.Context, q *sqlc.Queries, ids []int32) (*cache, error) {
	c := &cache{series: map[int32]*sqlc.TmdbSeries{}, seasons: map[int32][]*tmdb.Season{}, episodes: map[seasonKey][]*tmdb.Episode{}}
	series, err := q.ListTMDbSeries(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the TMDb series: %w", err)
	}
	for i := range series {
		s := &series[i]
		c.series[s.ID] = s
		seasons := []*tmdb.Season{}
		if err := json.Unmarshal(s.Seasons, &seasons); err != nil {
			return nil, fmt.Errorf("failed to decode the TMDb seasons: %w", err)
		}
		c.seasons[s.ID] = seasons
	}
	seasons, err := q.ListTMDbSeasonsOf(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the TMDb seasons: %w", err)
	}
	for i := range seasons {
		episodes := []*tmdb.Episode{}
		if err := json.Unmarshal(seasons[i].Episodes, &episodes); err != nil {
			return nil, fmt.Errorf("failed to decode the TMDb episodes: %w", err)
		}
		c.episodes[seasonKey{series: seasons[i].SeriesID, season: seasons[i].SeasonNumber}] = episodes
	}

	return c, nil
}

// episodeName は、話の名前を返す。キャッシュになければ nil を返す。
func (c *cache) episodeName(series, season, number int32) *string {
	for _, e := range c.episodes[seasonKey{series: series, season: season}] {
		if e.EpisodeNumber == number && e.Name != "" {
			return new(e.Name)
		}
	}

	return nil
}

// lastNumber は、シーズンの最後の話の番号を返す。シーズンの話がキャッシュになければ、作品のシーズンの一覧の話数を使い、
// それもなければ 0 を返す。
func (c *cache) lastNumber(series, season int32) int32 {
	var last int32
	for _, e := range c.episodes[seasonKey{series: series, season: season}] {
		last = max(last, e.EpisodeNumber)
	}
	if last > 0 {
		return last
	}
	for _, s := range c.seasons[series] {
		if s.SeasonNumber == season {
			return s.EpisodeCount
		}
	}

	return 0
}

// isLast は、話が、TMDb のシーズンの最後の話かどうかを返す。特別編と、最後の話が分からないシーズンは false にする。
func (c *cache) isLast(series int32, season, number *int32) bool {
	if season == nil || number == nil || *season == 0 {
		return false
	}
	last := c.lastNumber(series, *season)

	return last > 0 && *number >= last
}

// nextEpisode は、作品 series の話 (season, number) の次に見る話を返す。
// シーズンの中に後の話があれば、その最初の話を選ぶ。シーズンの最後の話なら、次のシーズン (特別編を除く、番号が次に大きいシーズン) の
// 最初の話を選び、次のシーズンがなければ (見終えた作品) nil を返す。シーズンの話がキャッシュになく、最後の話も分からなければ、
// 次の番号の話とする (名前は分からない)。
func (c *cache) nextEpisode(series, season, number int32) *LibraryEpisode {
	episodes := c.episodes[seasonKey{series: series, season: season}]
	var next *tmdb.Episode
	for _, e := range episodes {
		if e.EpisodeNumber > number && (next == nil || e.EpisodeNumber < next.EpisodeNumber) {
			next = e
		}
	}
	if next != nil {
		return &LibraryEpisode{Season: season, Number: next.EpisodeNumber, Name: nonEmpty(next.Name)}
	}
	last := c.lastNumber(series, season)
	if last == 0 {
		return &LibraryEpisode{Season: season, Number: number + 1, Name: nil}
	}
	if number < last {
		return &LibraryEpisode{Season: season, Number: number + 1, Name: c.episodeName(series, season, number+1)}
	}
	var following *int32
	for _, s := range c.seasons[series] {
		if s.SeasonNumber > season && (following == nil || s.SeasonNumber < *following) {
			following = new(s.SeasonNumber)
		}
	}
	if following == nil {
		return nil
	}
	// 次のシーズンの話がキャッシュにあれば、その最初の話を、なければ第 1 話を選ぶ。
	var first int32
	for _, e := range c.episodes[seasonKey{series: series, season: *following}] {
		if e.EpisodeNumber > 0 && (first == 0 || e.EpisodeNumber < first) {
			first = e.EpisodeNumber
		}
	}
	if first == 0 {
		first = 1
	}

	return &LibraryEpisode{Season: *following, Number: first, Name: c.episodeName(series, *following, first)}
}

// arrangeInput は、作品を並べるのに使う、DB から読んだ行。
type arrangeInput struct {
	entries    []sqlc.DramaWatchlistEntry
	cache      *cache
	watched    []sqlc.ListDramaWatchedEpisodesRow
	programs   []sqlc.ListDramaLibraryProgramsRow
	recent     []sqlc.ListDramaLibraryRecentBroadcastsRow
	recordings []sqlc.ListDramaLibraryRecordingsRow
}

// episodeKey は、作品の 1 話を見分ける鍵。
type episodeKey struct {
	series int32
	season int32
	number int32
}

// after は、話 (season, number) が、話 last より後の話かどうかを返す。特別編 (シーズン 0) は、どの話より後でもない。
func after(season, number int32, last *LibraryEpisode) bool {
	if season == 0 {
		return false
	}

	return season > last.Season || (season == last.Season && number > last.Number)
}

// arrange は、作品ごとに、見た話と、これからの放送と、録画のある話をまとめ、次の放送の早い順に並べる。
func arrange(in *arrangeInput) []*Work {
	works := make([]*Work, 0, len(in.entries))
	byID := map[int32]*Work{}
	for i := range in.entries {
		entry := &in.entries[i]
		work := &Work{
			SeriesID: entry.TmdbSeriesID, Name: nil, OriginalName: nil, Status: entry.Status, AddedAt: entry.CreatedAt,
			LastWatched: nil, NextEpisode: nil, Broadcasts: []*Broadcast{}, NextBroadcast: nil, RecentBroadcast: nil,
			RecordedEpisodes: []*RecordedEpisode{},
		}
		if series, ok := in.cache.series[entry.TmdbSeriesID]; ok {
			work.Name, work.OriginalName = nonEmpty(series.Name), nonEmpty(series.OriginalName)
		}
		works = append(works, work)
		byID[work.SeriesID] = work
	}
	watched := addWatched(byID, in.cache, in.watched)
	addBroadcasts(byID, in)
	for i := range in.recordings {
		addRecording(byID, in.cache, watched, &in.recordings[i])
	}
	for _, work := range works {
		work.NextBroadcast = nextBroadcast(work)
	}
	slices.SortStableFunc(works, func(a, b *Work) int {
		switch {
		case a.NextBroadcast != nil && b.NextBroadcast != nil:
			return a.NextBroadcast.StartAt.Compare(b.NextBroadcast.StartAt)
		case a.NextBroadcast != nil:
			return -1
		case b.NextBroadcast != nil:
			return 1
		default:
			return cmp.Or(strings.Compare(a.title(), b.title()), cmp.Compare(a.SeriesID, b.SeriesID))
		}
	})

	return works
}

// addWatched は、見た話から、作品ごとの最後に見た話と次に見る話を決め、見た話の集まりを返す。
func addWatched(byID map[int32]*Work, c *cache, rows []sqlc.ListDramaWatchedEpisodesRow) map[episodeKey]bool {
	watched := map[episodeKey]bool{}
	for _, row := range rows {
		watched[episodeKey{series: row.TmdbSeriesID, season: row.SeasonNumber, number: row.EpisodeNumber}] = true
		work, ok := byID[row.TmdbSeriesID]
		// 特別編 (シーズン 0) は、本編のどこに入るか分からないので、最後に見た話にしない。
		if !ok || row.SeasonNumber == 0 || (work.LastWatched != nil && !after(row.SeasonNumber, row.EpisodeNumber, work.LastWatched)) {
			continue
		}
		work.LastWatched = &LibraryEpisode{
			Season: row.SeasonNumber, Number: row.EpisodeNumber, Name: c.episodeName(row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber),
		}
	}
	for _, work := range byID {
		if work.LastWatched != nil {
			work.NextEpisode = c.nextEpisode(work.SeriesID, work.LastWatched.Season, work.LastWatched.Number)
		}
	}

	return watched
}

// addBroadcasts は、作品に、これからの放送と、この 1 週間に始まった最も新しい放送を足す。
func addBroadcasts(byID map[int32]*Work, in *arrangeInput) {
	for i := range in.programs {
		row := &in.programs[i]
		work, ok := byID[row.TmdbSeriesID]
		if !ok {
			continue
		}
		work.Broadcasts = append(work.Broadcasts, &Broadcast{
			ServiceID: row.ServiceID, EventID: row.EventID, StartAt: row.StartAt, Season: row.SeasonNumber, Number: row.EpisodeNumber,
			Rerun: drama.Rerun(row.Name, row.SeriesRepeat.Valid && row.SeriesRepeat.Bool),
			Last:  in.cache.isLast(row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber),
		})
	}
	for i := range in.recent {
		row := &in.recent[i]
		if work, ok := byID[row.TmdbSeriesID]; ok {
			work.RecentBroadcast = &PastBroadcast{StartAt: row.StartAt, Last: in.cache.isLast(row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber)}
		}
	}
}

// addRecording は、録画 row を、作品の録画のある話に足す。同じ話の録画は続けて返ってくるので、前の話と同じなら、その話に足す。
// 見た話か、最後に見た話より前の話は、見た話とする。見た話がひとつもない作品では、見たかどうかを決めない。
func addRecording(byID map[int32]*Work, c *cache, watched map[episodeKey]bool, row *sqlc.ListDramaLibraryRecordingsRow) {
	work, ok := byID[row.TmdbSeriesID]
	if !ok {
		return
	}
	if n := len(work.RecordedEpisodes); n > 0 && work.RecordedEpisodes[n-1].Season == row.SeasonNumber &&
		work.RecordedEpisodes[n-1].Number == row.EpisodeNumber {
		last := work.RecordedEpisodes[n-1]
		last.RecordingIDs = append(last.RecordingIDs, row.RecordingID)

		return
	}
	var seen *bool
	if work.LastWatched != nil {
		seen = new(watched[episodeKey{series: row.TmdbSeriesID, season: row.SeasonNumber, number: row.EpisodeNumber}] ||
			(row.SeasonNumber != 0 && !after(row.SeasonNumber, row.EpisodeNumber, work.LastWatched)))
	}
	work.RecordedEpisodes = append(work.RecordedEpisodes, &RecordedEpisode{
		Season: row.SeasonNumber, Number: row.EpisodeNumber, Name: c.episodeName(row.TmdbSeriesID, row.SeasonNumber, row.EpisodeNumber),
		RecordingIDs: []uuid.UUID{row.RecordingID}, Watched: seen,
	})
}

// nextBroadcast は、作品の次の放送を返す。最後に見た話より後の話の放送のうち最も早いものを選び、なければ最も早い放送を選ぶ。
// 再放送や、見た話の放送が先にあっても、次に見る話の放送を示すため。
func nextBroadcast(work *Work) *Broadcast {
	if len(work.Broadcasts) == 0 {
		return nil
	}
	if work.LastWatched != nil {
		for _, broadcast := range work.Broadcasts {
			if broadcast.Season != nil && broadcast.Number != nil && after(*broadcast.Season, *broadcast.Number, work.LastWatched) {
				return broadcast
			}
		}
	}

	return work.Broadcasts[0]
}

// nonEmpty は、空でない文字列だけを返す。
func nonEmpty(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
