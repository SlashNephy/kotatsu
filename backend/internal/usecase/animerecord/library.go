package animerecord

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// Library は、ユーザーの見たいリストの作品を、作品ごとに、次の話と次の放送と録画と並べたもの (視聴管理の画面。#1231)。
type Library struct {
	// Imports は、連携した記録先ごとの、見たいリストを取り込んだ様子。
	Imports []*WishlistImport
	// Works は、作品。次の放送の早い順に並べ、放送の予定のない作品は、作品名の順に後ろに回す。
	Works []*LibraryWork
}

// LibraryWork は、視聴管理の画面の 1 作品。TID に対応する作品は TID ごとに 1 つにまとめ、Annict と AniList の両方に入れた作品も 1 つにする。
// TID に対応しない作品は、記録先の作品ごとに 1 つにする (番組と録画は引けない)。
type LibraryWork struct {
	// TID は、しょぼいカレンダーの TID。対応しなければ nil。
	TID *int32
	// Title は、作品名。Annict の作品名を先に選ぶ。
	Title string
	// Sources は、作品を見たいリストに入れた記録先。記録先の名前の順に並べる。
	Sources []*LibrarySource
	// Progress は、見終えた話数。記録先ごとの見終えた話数のうち、大きいほう。どの記録先も分からなければ nil。
	Progress *int32
	// EpisodesCount は、作品の全話数。記録先が示したもののうち、大きいほう。分からなければ nil。
	EpisodesCount *int32
	// SeasonYear と SeasonName は、作品の放送の時期。Annict の値を先に選ぶ。
	SeasonYear *int32
	SeasonName string
	// NextEpisode は、次に見る話。Annict が示した次の話があればそれを、なければ見終えた話数の次の話数だけを持つ。分からなければ nil。
	NextEpisode *WishlistEpisode
	// Broadcasts は、これからの放送 (放送中を含む)。放送の早い順に並べる。
	Broadcasts []*LibraryBroadcast
	// NextBroadcast は、次の放送。見終えた話数より後の話の放送のうち最も早いもの。なければ、最も早い放送。放送がなければ nil。
	NextBroadcast *LibraryBroadcast
	// RecordedEpisodes は、録画のある話。話数の順に並べる。
	RecordedEpisodes []*LibraryRecordedEpisode
	// RecentBroadcasts は、この 1 週間に始まった放送のうち、本放送と再放送それぞれの最も新しいもの (#1233)。新しい順に並べる。
	RecentBroadcasts []*LibraryPastBroadcast
}

// LibraryPastBroadcast は、作品の始まった放送。番組はアーカイブに移っていることがあるので、開始時刻と印だけを持つ。
type LibraryPastBroadcast struct {
	StartAt time.Time
	// Rerun は再放送かどうか、Last は最終話 ([終]) かどうか。
	Rerun bool
	Last  bool
}

// Watching は、どれかの記録先で「見てる」にしているかどうか。
func (w *LibraryWork) Watching() bool {
	return slices.ContainsFunc(w.Sources, func(s *LibrarySource) bool { return s.Status == WishlistWatching })
}

// LibrarySource は、作品を見たいリストに入れた記録先と、記録先での様子。
type LibrarySource struct {
	Destination string
	WorkID      int32
	// Status は、WishlistWannaWatch か WishlistWatching。
	Status string
	Title  string
	// Progress は、記録先の見終えた話数。分からなければ nil。
	Progress *int32
}

// LibraryBroadcast は、作品の放送。
type LibraryBroadcast struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	// CountStart と CountEnd は、放送する話数の範囲。話数のない放送 (特番など) では nil。
	CountStart *float64
	CountEnd   *float64
	// Rerun は再放送かどうか、Last は最終話 ([終]) かどうか (#1233)。
	Rerun bool
	Last  bool
}

// LibraryRecordedEpisode は、録画のある話。
type LibraryRecordedEpisode struct {
	// CountStart と CountEnd は、話数の範囲。1 話だけなら同じ値。
	CountStart float64
	CountEnd   float64
	Subtitle   *string
	// RecordingIDs は、その話の録画。新しく録ったものを先に並べる。
	RecordingIDs []uuid.UUID
	// Watched は、見終えた話数までの話かどうか。見終えた話数が分からなければ nil。
	Watched *bool
}

// Library は、ユーザー userID の見たいリストの作品を、作品ごとに、次の話と次の放送と録画と並べて返す (#1231)。
// 放送は、今から WishlistHorizon の間に始まるもの (放送中を含む) を、番組の結びつき (設計書 3.8) で引く。
// 録画は、録画に写した結びつきで引き、録れなかった録画と、話数のない録画は外す。
func (s *Service) Library(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (*Library, error) {
	q := sqlc.New(db)
	rows, err := q.ListAnimeWishlistEntries(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the wishlist entries: %w", err)
	}
	imports, err := wishlistImports(ctx, q, userID, rows)
	if err != nil {
		return nil, err
	}
	works, err := libraryWorks(ctx, q, rows)
	if err != nil {
		return nil, err
	}
	tids := make([]int32, 0, len(works))
	for _, work := range works {
		if work.TID != nil {
			tids = append(tids, *work.TID)
		}
	}
	now := s.clock.Now()
	programs, err := q.ListAnimeLibraryPrograms(ctx, sqlc.ListAnimeLibraryProgramsParams{SyobocalTids: tids, Now: now, Until: now.Add(WishlistHorizon)})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs of the library: %w", err)
	}
	recordings, err := q.ListAnimeLibraryRecordings(ctx, tids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings of the library: %w", err)
	}
	recent, err := q.ListAnimeLibraryRecentBroadcasts(ctx, sqlc.ListAnimeLibraryRecentBroadcastsParams{
		SyobocalTids: tids, Since: now.Add(-WishlistHorizon), Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the recent broadcasts of the library: %w", err)
	}

	return &Library{Imports: imports, Works: arrangeLibrary(works, programs, recordings, recent)}, nil
}

// wishlistImports は、連携した記録先ごとの、見たいリストを取り込んだ様子を返す。
func wishlistImports(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, rows []sqlc.AnimeWishlistEntry) ([]*WishlistImport, error) {
	connections, err := q.ListAnimeRecordConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime record connections: %w", err)
	}
	counts := map[string]int{}
	for i := range rows {
		counts[rows[i].Destination]++
	}
	imports := make([]*WishlistImport, 0, len(connections))
	for i := range connections {
		c := &connections[i]
		imports = append(imports, &WishlistImport{
			Destination: c.Destination, CheckedAt: c.WishlistCheckedAt, ImportedAt: c.WishlistImportedAt, Error: c.WishlistError, Entries: counts[c.Destination],
		})
	}

	return imports, nil
}

// libraryWorks は、見たいリストの作品を、TID ごとにまとめる。TID に対応しない作品は、記録先の作品ごとに 1 つにする。
// 分割して登録された作品のように、1 つの記録先の作品が複数の TID に対応すれば、TID ごとのどの作品にも入れる。
func libraryWorks(ctx context.Context, q *sqlc.Queries, rows []sqlc.AnimeWishlistEntry) ([]*LibraryWork, error) {
	if len(rows) == 0 {
		return []*LibraryWork{}, nil
	}
	annict, anilist := []int32{}, []int32{}
	for i := range rows {
		switch rows[i].Destination {
		case DestinationAnnict:
			annict = append(annict, rows[i].WorkID)
		case DestinationAniList:
			anilist = append(anilist, rows[i].WorkID)
		}
	}
	mappings, err := q.ListAnimeWorksByExternalIDs(ctx, sqlc.ListAnimeWorksByExternalIDsParams{AnnictWorkIds: annict, AnilistIds: anilist})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime works of the wishlist: %w", err)
	}
	tidsOf := map[string][]int32{}
	addTID := func(destination string, id *int32, tid int32) {
		if id == nil {
			return
		}
		key := wishlistKey(destination, *id)
		if !slices.Contains(tidsOf[key], tid) {
			tidsOf[key] = append(tidsOf[key], tid)
		}
	}
	for i := range mappings {
		addTID(DestinationAnnict, mappings[i].AnnictWorkID, mappings[i].SyobocalTid)
		addTID(DestinationAniList, mappings[i].AnilistID, mappings[i].SyobocalTid)
	}
	byTID := map[int32]*LibraryWork{}
	works := []*LibraryWork{}
	for i := range rows {
		row := &rows[i]
		tids := tidsOf[wishlistKey(row.Destination, row.WorkID)]
		if len(tids) == 0 {
			work := newLibraryWork(nil)
			addLibrarySource(work, row)
			works = append(works, work)

			continue
		}
		for _, tid := range tids {
			work, ok := byTID[tid]
			if !ok {
				work = newLibraryWork(new(tid))
				byTID[tid] = work
				works = append(works, work)
			}
			addLibrarySource(work, row)
		}
	}
	for _, work := range works {
		finishLibraryWork(work)
	}

	return works, nil
}

// newLibraryWork は、記録先をまだ持たない作品を返す。
func newLibraryWork(tid *int32) *LibraryWork {
	return &LibraryWork{
		TID: tid, Title: "", Sources: []*LibrarySource{}, Progress: nil, EpisodesCount: nil, SeasonYear: nil, SeasonName: "",
		NextEpisode: nil, Broadcasts: []*LibraryBroadcast{}, NextBroadcast: nil, RecordedEpisodes: []*LibraryRecordedEpisode{},
		RecentBroadcasts: []*LibraryPastBroadcast{},
	}
}

// addLibrarySource は、見たいリストの作品 row を、作品 work の記録先に足し、記録先が示した作品の情報を、作品に写す。
func addLibrarySource(work *LibraryWork, row *sqlc.AnimeWishlistEntry) {
	work.Sources = append(work.Sources, &LibrarySource{
		Destination: row.Destination, WorkID: row.WorkID, Status: row.Status, Title: row.Title, Progress: row.Progress,
	})
	work.Progress = maxOf(work.Progress, row.Progress)
	work.EpisodesCount = maxOf(work.EpisodesCount, row.EpisodesCount)
	// 時期と次の話は、Annict の値を先に選ぶ。記録先は Annict を先に取り込むとは限らないので、後から来た Annict の値で上書きする。
	annict := row.Destination == DestinationAnnict
	if row.SeasonYear != nil && (work.SeasonYear == nil || annict) {
		work.SeasonYear = row.SeasonYear
		work.SeasonName = ""
		if row.SeasonName != nil {
			work.SeasonName = *row.SeasonName
		}
	}
	if row.NextEpisodeID != nil && (work.NextEpisode == nil || annict) {
		work.NextEpisode = &WishlistEpisode{
			ID: *row.NextEpisodeID, Number: row.NextEpisodeNumber, NumberText: valueOf(row.NextEpisodeNumberText), Title: valueOf(row.NextEpisodeTitle),
		}
	}
}

// finishLibraryWork は、記録先を並べ、作品名と次の話を決める。
func finishLibraryWork(work *LibraryWork) {
	slices.SortFunc(work.Sources, func(a, b *LibrarySource) int {
		return cmp.Or(cmp.Compare(destinationOrder(a.Destination), destinationOrder(b.Destination)), cmp.Compare(a.WorkID, b.WorkID))
	})
	for _, source := range work.Sources {
		if strings.TrimSpace(source.Title) != "" {
			work.Title = source.Title

			break
		}
	}
	// 記録先が次の話を示さなければ、見終えた話数の次の話数だけを持つ。示した次の話が、ほかの記録先で見終えた話なら、使わない。
	if work.NextEpisode != nil && work.NextEpisode.Number != nil && work.Progress != nil && *work.NextEpisode.Number <= *work.Progress {
		work.NextEpisode = nil
	}
	if work.NextEpisode == nil && work.Progress != nil && (work.EpisodesCount == nil || *work.Progress < *work.EpisodesCount) {
		work.NextEpisode = &WishlistEpisode{ID: "", Number: new(*work.Progress + 1), NumberText: "", Title: ""}
	}
}

// arrangeLibrary は、作品に、これからの放送と録画のある話を足し、次の放送の早い順に並べる。
func arrangeLibrary(
	works []*LibraryWork, programs []sqlc.ListAnimeLibraryProgramsRow, recordings []sqlc.ListAnimeLibraryRecordingsRow,
	recent []sqlc.ListAnimeLibraryRecentBroadcastsRow,
) []*LibraryWork {
	byTID := map[int32]*LibraryWork{}
	for _, work := range works {
		if work.TID != nil {
			byTID[*work.TID] = work
		}
	}
	for i := range programs {
		row := &programs[i]
		work, ok := byTID[row.SyobocalTid]
		if !ok {
			continue
		}
		work.Broadcasts = append(work.Broadcasts, &LibraryBroadcast{
			ServiceID: row.ServiceID, EventID: row.EventID, StartAt: row.StartAt, CountStart: count(row.CountStart), CountEnd: count(row.CountEnd),
			Rerun: row.IsRerun, Last: row.IsLast,
		})
	}
	for i := range recordings {
		row := &recordings[i]
		work, ok := byTID[row.SyobocalTid]
		if !ok {
			continue
		}
		// 同じ話の録画は続けて返ってくるので、前の話と同じなら、その話に足す。
		if n := len(work.RecordedEpisodes); n > 0 && work.RecordedEpisodes[n-1].CountStart == row.CountStart &&
			work.RecordedEpisodes[n-1].CountEnd == row.CountEnd {
			last := work.RecordedEpisodes[n-1]
			last.RecordingIDs = append(last.RecordingIDs, row.RecordingID)

			continue
		}
		var watched *bool
		if work.Progress != nil {
			watched = new(row.CountEnd <= float64(*work.Progress))
		}
		work.RecordedEpisodes = append(work.RecordedEpisodes, &LibraryRecordedEpisode{
			CountStart: row.CountStart, CountEnd: row.CountEnd, Subtitle: row.Subtitle, RecordingIDs: []uuid.UUID{row.RecordingID}, Watched: watched,
		})
	}
	for i := range recent {
		row := &recent[i]
		if work, ok := byTID[row.SyobocalTid]; ok {
			work.RecentBroadcasts = append(work.RecentBroadcasts, &LibraryPastBroadcast{StartAt: row.StartAt, Rerun: row.IsRerun, Last: row.IsLast})
		}
	}
	for _, work := range works {
		slices.SortFunc(work.RecentBroadcasts, func(a, b *LibraryPastBroadcast) int { return b.StartAt.Compare(a.StartAt) })
		work.NextBroadcast = nextBroadcast(work)
	}
	slices.SortStableFunc(works, func(a, b *LibraryWork) int {
		switch {
		case a.NextBroadcast != nil && b.NextBroadcast != nil:
			return a.NextBroadcast.StartAt.Compare(b.NextBroadcast.StartAt)
		case a.NextBroadcast != nil:
			return -1
		case b.NextBroadcast != nil:
			return 1
		default:
			return strings.Compare(a.Title, b.Title)
		}
	})

	return works
}

// nextBroadcast は、作品の次の放送を返す。見終えた話数より後の話の放送のうち最も早いものを選び、なければ最も早い放送を選ぶ。
// 再放送や、見終えた話の放送が先にあっても、次に見る話の放送を示すため。
func nextBroadcast(work *LibraryWork) *LibraryBroadcast {
	if len(work.Broadcasts) == 0 {
		return nil
	}
	if work.Progress != nil {
		for _, broadcast := range work.Broadcasts {
			if broadcast.CountEnd != nil && *broadcast.CountEnd > float64(*work.Progress) {
				return broadcast
			}
		}
	}

	return work.Broadcasts[0]
}

// count は、クエリが -1 で返した、話数のない放送の話数を nil にする。
func count(value float64) *float64 {
	if value < 0 {
		return nil
	}

	return &value
}

// maxOf は、2 つの数のうち、大きいほうを返す。片方が nil なら、もう片方を返す。
func maxOf(a, b *int32) *int32 {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return new(max(*a, *b))
	}
}

// valueOf は、文字列を返す。nil なら空を返す。
func valueOf(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
