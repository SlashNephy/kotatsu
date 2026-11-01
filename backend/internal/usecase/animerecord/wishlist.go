package animerecord

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// WishlistRefreshInterval は、取り込めた見たいリストを取り込み直す間隔 (付録 A)。
	// Annict と AniList への問い合わせを控えめにするため、記録 (見たらすぐ送る) と違い、数時間ごとにする。
	WishlistRefreshInterval = 6 * time.Hour
	// WishlistRetryInterval は、取り込めなかった見たいリストを、問い合わせ直すまでの間隔 (付録 A)。
	// 429 で Retry-After が示されたら、その長さだけ待つが、この長さを上限にする。
	WishlistRetryInterval = time.Hour
	// WishlistHorizon は、候補にする放送の範囲 (付録 A)。今から、この期間に始まる番組を探す。
	WishlistHorizon = 7 * 24 * time.Hour
	// DefaultWishlistRequestInterval は、見たいリストを引く問い合わせの間を空ける既定の長さ (付録 A)。
	DefaultWishlistRequestInterval = time.Second
	// maxWishlistPages と maxWishlistEntries は、記録先ごとに引くページの数と作品の数の上限。
	// Annict は 1 ページ 100 件、AniList は 500 件なので、どちらも 2,000 件までは引く。
	maxWishlistPages   = 20
	maxWishlistEntries = 2000
	// maxWishlistErrorRunes は、取り込めなかった理由を残す長さの上限。
	maxWishlistErrorRunes = 200
)

// Wishlist は、ユーザーの見たいリストの作品のうち、これから放送されるものの候補 (#928)。
type Wishlist struct {
	// Imports は、連携した記録先ごとの、見たいリストを取り込んだ様子。記録先の名前の順に並べる。
	Imports []*WishlistImport
	// Works は、候補の作品。最初の放送の早い順に並べる。
	Works []*WishlistWork
}

// WishlistImport は、記録先ごとの、見たいリストを取り込んだ様子。
type WishlistImport struct {
	Destination string
	// CheckedAt は、最後に問い合わせた日時。ImportedAt は、最後に取り込めた日時。どちらも、まだなら nil。
	CheckedAt  *time.Time
	ImportedAt *time.Time
	// Error は、最後の問い合わせが失敗した理由。取り込めていれば nil。
	Error *string
	// Entries は、取り込んだ作品の数。
	Entries int
}

// WishlistSource は、候補の作品を見たいリストに入れた記録先と、その作品。
type WishlistSource struct {
	Destination string
	WorkID      int32
	// Status は、WishlistWannaWatch か WishlistWatching。
	Status string
	Title  string
}

// WishlistBroadcast は、候補の作品の放送。
type WishlistBroadcast struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	Name      string
	// Preferred は、録るのに選ぶサービスの放送かどうか。作品の最初の放送のサービスを選ぶ。
	// 予約のある作品 (WishlistWork.Covered) では、どの放送も選ばない。録る放送は、もう決まっているため。
	// 重複の回避 (設計書 4.4) を付けたルールで録ると、最初に録った放送のほかは、同じ番組名を録画済みとして見送るため。
	Preferred bool
}

// WishlistWork は、候補の作品。TID ごとに 1 つにまとめ、Annict と AniList の両方に入れた作品も 1 つにする。
type WishlistWork struct {
	TID int32
	// Title は、見出しと、作るルールの名前に使う作品名。Annict の作品名を先に選ぶ。
	Title string
	// Sources は、作品を見たいリストに入れた記録先。記録先の名前の順に並べる。
	Sources []*WishlistSource
	// Broadcasts は、予約していない放送。放送の早い順に並べ、同じイベントを共有する放送は 1 つにする。
	Broadcasts []*WishlistBroadcast
	// ReservedBroadcasts は、予約のある放送の数。候補からは外す。
	ReservedBroadcasts int
	// Covered は、これからの放送のどれかに予約 (ルール予約か手動の予約) があるか、放送のどれかから作るルールと同じ
	// キーワードとサービスのルール (無効なものも含む) がもうあるかどうか。予約済みの作品からは、ルールを作らせない。
	// 同じ作品のルールを二重に作らないため。ルールの照合が予約を作る前や、重複の回避で予約しなかった放送でも、ルールで見分ける。
	Covered bool
}

// WishlistArgs は、見たいリストを取り込むジョブの引数。トークンは載せず、ジョブが DB から引く。
type WishlistArgs struct {
	// UserID があれば、そのユーザーの連携をすべて、今すぐ取り込み直す (連携したときと、利用者が取り込み直しを押したとき)。
	// なければ、取り込み直す時期が来た連携を取り込む (定期ジョブ)。
	UserID *uuid.UUID `json:"userId,omitempty"`
}

// ImportWishlist は、ユーザー userID の、記録先 name の見たいリストを取り込み、作品を入れ替える。
// 管理者が連携を無効にしているか、連携していなければ、何もしない。取り込めなければ、前に取り込んだ作品を残し、理由を残す。
// 問い合わせは、このプロセスの中では 1 つずつ、間を空けて送る。
func (s *Service) ImportWishlist(ctx context.Context, db Database, userID uuid.UUID, name string) error {
	destination, err := s.destination(name)
	if err != nil {
		return err
	}
	system, err := settings.Load(ctx, db)
	if err != nil {
		return err
	}
	if !system.AnimeLinkingEnabled {
		return nil
	}
	token, err := s.token(ctx, db, userID, name)
	if errors.Is(err, ErrNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}
	entries, err := s.fetchWishlist(ctx, destination, token)
	if err != nil {
		reason := shortenError(err)
		now := s.clock.Now()
		// 失敗を残す書き込みは、ジョブの期限で取り消さない。
		if _, finishErr := sqlc.New(db).FinishAnimeWishlistImport(context.WithoutCancel(ctx), sqlc.FinishAnimeWishlistImportParams{
			Now: now, Error: &reason, RetryAt: new(now.Add(retryDelay(err))), UserID: userID, Destination: name,
		}); finishErr != nil {
			return errors.Join(err, fmt.Errorf("failed to save the wishlist import failure: %w", finishErr))
		}

		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		// 連携を外した後なら、作品を書かない (行は連携と一緒に消えている)。
		updated, err := q.FinishAnimeWishlistImport(ctx, sqlc.FinishAnimeWishlistImportParams{Now: now, Error: nil, RetryAt: nil, UserID: userID, Destination: name})
		if err != nil {
			return fmt.Errorf("failed to save the wishlist import: %w", err)
		}
		if updated == 0 {
			return nil
		}
		if err := q.DeleteAnimeWishlistEntries(ctx, sqlc.DeleteAnimeWishlistEntriesParams{UserID: userID, Destination: name}); err != nil {
			return fmt.Errorf("failed to delete the wishlist entries: %w", err)
		}
		if err := q.InsertAnimeWishlistEntries(ctx, wishlistEntriesParams(userID, name, now, entries)); err != nil {
			return fmt.Errorf("failed to save the wishlist entries: %w", err)
		}

		return nil
	})
}

// wishlistEntriesParams は、見たいリストの作品を、まとめて書く引数にする。配列は NULL を持てないので、分からない数は -1 に、
// 分からない文字列は空にする (クエリが NULL に戻す)。
func wishlistEntriesParams(userID uuid.UUID, name string, now time.Time, entries []*WishlistEntry) sqlc.InsertAnimeWishlistEntriesParams {
	params := sqlc.InsertAnimeWishlistEntriesParams{
		UserID: userID, Destination: name, Now: now,
		WorkIds: make([]int32, 0, len(entries)), Statuses: make([]string, 0, len(entries)), Titles: make([]string, 0, len(entries)),
		NextEpisodeIds: make([]string, 0, len(entries)), NextEpisodeNumbers: make([]int32, 0, len(entries)),
		NextEpisodeNumberTexts: make([]string, 0, len(entries)), NextEpisodeTitles: make([]string, 0, len(entries)),
		Progresses: make([]int32, 0, len(entries)), EpisodesCounts: make([]int32, 0, len(entries)),
		SeasonYears: make([]int32, 0, len(entries)), SeasonNames: make([]string, 0, len(entries)), ImageURLs: make([]string, 0, len(entries)),
	}
	orNone := func(value *int32) int32 {
		if value == nil {
			return -1
		}

		return *value
	}
	for _, entry := range entries {
		params.WorkIds = append(params.WorkIds, entry.WorkID)
		params.Statuses = append(params.Statuses, entry.Status)
		params.Titles = append(params.Titles, entry.Title)
		next := entry.NextEpisode
		if next == nil {
			next = &WishlistEpisode{ID: "", Number: nil, NumberText: "", Title: ""}
		}
		params.NextEpisodeIds = append(params.NextEpisodeIds, next.ID)
		params.NextEpisodeNumbers = append(params.NextEpisodeNumbers, orNone(next.Number))
		params.NextEpisodeNumberTexts = append(params.NextEpisodeNumberTexts, next.NumberText)
		params.NextEpisodeTitles = append(params.NextEpisodeTitles, next.Title)
		params.Progresses = append(params.Progresses, orNone(entry.Progress))
		params.EpisodesCounts = append(params.EpisodesCounts, orNone(entry.EpisodesCount))
		params.SeasonYears = append(params.SeasonYears, orNone(entry.SeasonYear))
		params.SeasonNames = append(params.SeasonNames, entry.SeasonName)
		params.ImageURLs = append(params.ImageURLs, entry.ImageURL)
	}

	return params
}

// retryDelay は、取り込めなかった見たいリストを問い合わせ直すまでの長さを返す。
// 記録先が 429 の Retry-After で長さを示せば、それに従う (WishlistRetryInterval を上限にする)。ほかは WishlistRetryInterval。
func retryDelay(err error) time.Duration {
	var limited *RateLimitError
	if errors.As(err, &limited) && limited.RetryAfter > 0 {
		return min(limited.RetryAfter, WishlistRetryInterval)
	}

	return WishlistRetryInterval
}

// fetchWishlist は、記録先の見たいリストを、ページをたどって引く。問い合わせの間は、wishlistInterval だけ空ける。
func (s *Service) fetchWishlist(ctx context.Context, destination Destination, token string) ([]*WishlistEntry, error) {
	s.wishlistMu.Lock()
	defer s.wishlistMu.Unlock()
	entries := []*WishlistEntry{}
	cursor := ""
	for range maxWishlistPages {
		if err := s.waitWishlistTurn(ctx, destination.Name()); err != nil {
			return nil, err
		}
		page, err := destination.Wishlist(ctx, token, cursor)
		s.wishlistLast[destination.Name()] = time.Now()
		if err != nil {
			return nil, err
		}
		entries = append(entries, page.Entries...)
		if page.Next == "" || len(entries) >= maxWishlistEntries {
			break
		}
		cursor = page.Next
	}
	if len(entries) > maxWishlistEntries {
		entries = entries[:maxWishlistEntries]
	}

	return entries, nil
}

// waitWishlistTurn は、記録先 name への前の問い合わせから、wishlistInterval が過ぎるまで待つ。wishlistMu を取ってから呼ぶ。
// 間を空けるのは実際の時間なので、テストのための時計ではなく、壁の時計で測る。
func (s *Service) waitWishlistTurn(ctx context.Context, name string) error {
	last, ok := s.wishlistLast[name]
	if !ok || s.wishlistInterval <= 0 {
		return nil
	}
	wait := time.Until(last.Add(s.wishlistInterval))
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("canceled while waiting to fetch the wishlist: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// ImportDueWishlists は、取り込み直す時期が来た連携の見たいリストを、1 つずつ取り込む。
// 記録先が 429 を返したら、その記録先の残りの連携は、次の機会に回す (問い合わせを重ねない)。
// 1 つの連携を取り込めなくても、ほかの連携は取り込む。取り込めなかった連携は、retryDelay の後に問い合わせ直す。
func (s *Service) ImportDueWishlists(ctx context.Context, db Database) error {
	now := s.clock.Now()
	due, err := sqlc.New(db).ListDueAnimeWishlistConnections(ctx, sqlc.ListDueAnimeWishlistConnectionsParams{
		SucceededBefore: now.Add(-WishlistRefreshInterval), Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to list the wishlists to import: %w", err)
	}
	limited := map[string]bool{}
	for _, row := range due {
		if limited[row.Destination] {
			continue
		}
		if err := s.ImportWishlist(ctx, db, row.UserID, row.Destination); err != nil {
			if ctx.Err() != nil {
				return err
			}
			if errors.Is(err, ErrRateLimited) {
				limited[row.Destination] = true
			}
			s.logger.WarnContext(ctx, "failed to import the wishlist", slog.String("destination", row.Destination), slog.Any("error", err))
		}
	}

	return nil
}

// ImportUserWishlists は、ユーザー userID が連携した記録先の見たいリストを、すべて取り込み直す。
func (s *Service) ImportUserWishlists(ctx context.Context, db Database, userID uuid.UUID) error {
	connections, err := s.Connections(ctx, db, userID)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if err := s.ImportWishlist(ctx, db, userID, connection.Destination); err != nil {
			if ctx.Err() != nil {
				return err
			}
			s.logger.WarnContext(ctx, "failed to import the wishlist", slog.String("destination", connection.Destination), slog.Any("error", err))
		}
	}

	return nil
}

// shortenError は、取り込めなかった理由を、画面に出せる長さにする。トークンや応答の本文は、もともとエラーに入れない。
func shortenError(err error) string {
	runes := []rune(err.Error())
	if len(runes) > maxWishlistErrorRunes {
		return string(runes[:maxWishlistErrorRunes]) + "…"
	}

	return string(runes)
}

// Wishlist は、ユーザー userID の見たいリストの作品のうち、これから WishlistHorizon の間に放送されるものを、候補として返す。
// 見たいリストの作品は、作品 ID の対応 (anime_works) で TID に、TID は番組の結びつき (設計書 3.8) で番組に引く。
// 予約のある番組 (ルール予約を含む) と、それと同じイベントを共有する番組は、候補から外す。
func (s *Service) Wishlist(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (*Wishlist, error) {
	q := sqlc.New(db)
	connections, err := q.ListAnimeRecordConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime record connections: %w", err)
	}
	rows, err := q.ListAnimeWishlistEntries(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the wishlist entries: %w", err)
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
	sources, err := wishlistSources(ctx, q, rows)
	if err != nil {
		return nil, err
	}
	tids := make([]int32, 0, len(sources))
	for tid := range sources {
		tids = append(tids, tid)
	}
	slices.Sort(tids)
	now := s.clock.Now()
	programs, err := q.ListAnimeWishlistPrograms(ctx, sqlc.ListAnimeWishlistProgramsParams{SyobocalTids: tids, Since: now, Until: now.Add(WishlistHorizon)})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs of the wishlist: %w", err)
	}
	reserved, err := reservedWishlistPrograms(ctx, q, programs)
	if err != nil {
		return nil, err
	}
	rules, err := existingRuleIdentities(ctx, q)
	if err != nil {
		return nil, err
	}

	return &Wishlist{Imports: imports, Works: pickWishlistWorks(sources, programs, reserved, rules)}, nil
}

// wishlistSources は、見たいリストの作品を、TID ごとにまとめる。対応のない作品は外す。
func wishlistSources(ctx context.Context, q *sqlc.Queries, rows []sqlc.AnimeWishlistEntry) (map[int32][]*WishlistSource, error) {
	annict, anilist := []int32{}, []int32{}
	byKey := map[string]*sqlc.AnimeWishlistEntry{}
	for i := range rows {
		row := &rows[i]
		switch row.Destination {
		case DestinationAnnict:
			annict = append(annict, row.WorkID)
		case DestinationAniList:
			anilist = append(anilist, row.WorkID)
		}
		byKey[wishlistKey(row.Destination, row.WorkID)] = row
	}
	sources := map[int32][]*WishlistSource{}
	if len(rows) == 0 {
		return sources, nil
	}
	works, err := q.ListAnimeWorksByExternalIDs(ctx, sqlc.ListAnimeWorksByExternalIDsParams{AnnictWorkIds: annict, AnilistIds: anilist})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime works of the wishlist: %w", err)
	}
	add := func(tid int32, destination string, id *int32) {
		if id == nil {
			return
		}
		row, ok := byKey[wishlistKey(destination, *id)]
		if !ok {
			return
		}
		// 分割して登録された作品などで、同じ TID の対応が何行あっても、記録先の作品は 1 つにする。
		if slices.ContainsFunc(sources[tid], func(s *WishlistSource) bool { return s.Destination == destination && s.WorkID == *id }) {
			return
		}
		sources[tid] = append(sources[tid], &WishlistSource{Destination: destination, WorkID: *id, Status: row.Status, Title: row.Title})
	}
	for i := range works {
		add(works[i].SyobocalTid, DestinationAnnict, works[i].AnnictWorkID)
		add(works[i].SyobocalTid, DestinationAniList, works[i].AnilistID)
	}
	for _, list := range sources {
		slices.SortFunc(list, func(a, b *WishlistSource) int {
			return cmp.Or(cmp.Compare(destinationOrder(a.Destination), destinationOrder(b.Destination)), cmp.Compare(a.WorkID, b.WorkID))
		})
	}

	return sources, nil
}

func wishlistKey(destination string, workID int32) string {
	return fmt.Sprintf("%s/%d", destination, workID)
}

// destinationOrder は、記録先を並べる順。Annict を先にする。
func destinationOrder(destination string) int {
	if destination == DestinationAnnict {
		return 0
	}

	return 1
}

// programKey は、番組 (サービス, eventId)。
type programKey struct {
	serviceID int64
	eventID   int64
}

// reservedWishlistPrograms は、候補の番組と、同じイベントを共有する番組のうち、予約のあるものを返す。無効にした予約も含める。
func reservedWishlistPrograms(ctx context.Context, q *sqlc.Queries, programs []sqlc.ListAnimeWishlistProgramsRow) (map[programKey]bool, error) {
	reserved := map[programKey]bool{}
	serviceIDs, eventIDs := []int64{}, []int64{}
	for i := range programs {
		serviceIDs, eventIDs = append(serviceIDs, programs[i].ServiceID), append(eventIDs, programs[i].EventID)
		shared, err := domain.SharedPrograms(programs[i].ServiceID, programs[i].RelatedItems)
		if err != nil {
			continue
		}
		for _, other := range shared {
			serviceIDs, eventIDs = append(serviceIDs, other.ServiceID), append(eventIDs, other.EventID)
		}
	}
	if len(serviceIDs) == 0 {
		return reserved, nil
	}
	rows, err := q.ListReservationsByPrograms(ctx, sqlc.ListReservationsByProgramsParams{ServiceIds: serviceIDs, EventIds: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the reservations of the wishlist programs: %w", err)
	}
	for i := range rows {
		if rows[i].EventID != nil {
			reserved[programKey{serviceID: rows[i].ServiceID, eventID: *rows[i].EventID}] = true
		}
	}

	return reserved, nil
}

// pickWishlistWorks は、TID ごとの番組から、候補の作品を作る。DB は使わない。
//
//   - 予約のある番組と、予約のある番組とイベントを共有する番組は、候補から外して数える。
//   - 同じイベントを共有する番組 (メインチャンネルとサブチャンネルなど) が並んだら、ルールの照合 (設計書 4.4) と同じく、
//     サービス ID の最も小さい 1 つだけを残す。
//   - 放送の早い順 (同じ時刻ならサービス ID の小さい順) に並べ、最初の放送のサービスを、録るのに選ぶサービスにする。
//     重複の回避を付けたルールは、最初に録った放送のほかを、同じ番組名を録画済みとして見送るため。
//   - 予約のある放送が 1 つでもある作品と、放送のどれかから作るルールと同じキーワードとサービスのルールが rules にある作品は、
//     予約済み (Covered) とし、録るサービスを選ばない。録る放送は、もう決まっているため。
//   - 予約していない放送のない作品は、候補にしない。
func pickWishlistWorks(
	sources map[int32][]*WishlistSource, programs []sqlc.ListAnimeWishlistProgramsRow, reserved map[programKey]bool, rules map[string]bool,
) []*WishlistWork {
	type candidate struct {
		row    *sqlc.ListAnimeWishlistProgramsRow
		shared []*domain.SharedProgram
	}
	byTID := map[int32][]*candidate{}
	reservedCount := map[int32]int{}
	for i := range programs {
		row := &programs[i]
		if _, ok := sources[row.SyobocalTid]; !ok {
			continue
		}
		shared, err := domain.SharedPrograms(row.ServiceID, row.RelatedItems)
		if err != nil {
			shared = []*domain.SharedProgram{}
		}
		taken := reserved[programKey{serviceID: row.ServiceID, eventID: row.EventID}] ||
			slices.ContainsFunc(shared, func(p *domain.SharedProgram) bool {
				return reserved[programKey{serviceID: p.ServiceID, eventID: p.EventID}]
			})
		if taken {
			reservedCount[row.SyobocalTid]++

			continue
		}
		byTID[row.SyobocalTid] = append(byTID[row.SyobocalTid], &candidate{row: row, shared: shared})
	}
	works := make([]*WishlistWork, 0, len(byTID))
	for tid, candidates := range byTID {
		present := map[programKey]bool{}
		for _, c := range candidates {
			present[programKey{serviceID: c.row.ServiceID, eventID: c.row.EventID}] = true
		}
		broadcasts := []*WishlistBroadcast{}
		for _, c := range candidates {
			// 同じイベントを、サービス ID のより小さいサービスでも放送していれば、そちらを残す。
			if slices.ContainsFunc(c.shared, func(p *domain.SharedProgram) bool {
				return p.ServiceID < c.row.ServiceID && present[programKey{serviceID: p.ServiceID, eventID: p.EventID}]
			}) {
				continue
			}
			broadcasts = append(broadcasts, &WishlistBroadcast{
				ServiceID: c.row.ServiceID, EventID: c.row.EventID, StartAt: c.row.StartAt, Name: c.row.Name, Preferred: false,
			})
		}
		slices.SortFunc(broadcasts, func(a, b *WishlistBroadcast) int {
			return cmp.Or(a.StartAt.Compare(b.StartAt), cmp.Compare(a.ServiceID, b.ServiceID))
		})
		covered := reservedCount[tid] > 0 || slices.ContainsFunc(broadcasts, func(b *WishlistBroadcast) bool {
			return rules[ruleIdentity(ruleKeyword(b.Name), []int64{b.ServiceID})]
		})
		for _, b := range broadcasts {
			b.Preferred = !covered && b.ServiceID == broadcasts[0].ServiceID
		}
		list := sources[tid]
		works = append(works, &WishlistWork{
			TID: tid, Title: workTitle(list, broadcasts[0]), Sources: list, Broadcasts: broadcasts, ReservedBroadcasts: reservedCount[tid],
			Covered: covered,
		})
	}
	slices.SortFunc(works, func(a, b *WishlistWork) int {
		return cmp.Or(a.Broadcasts[0].StartAt.Compare(b.Broadcasts[0].StartAt), cmp.Compare(a.TID, b.TID))
	})

	return works
}

// workTitle は、作品名を決める。記録先の作品名を、記録先の順に選び、どれもなければ、放送から読んだ作品名にする。
func workTitle(sources []*WishlistSource, first *WishlistBroadcast) string {
	for _, source := range sources {
		if title := strings.TrimSpace(source.Title); title != "" {
			return title
		}
	}

	return ruleKeyword(first.Name)
}

// bracketedMarks は、番組名の [新] や [字] のような印。
var bracketedMarks = regexp.MustCompile(`\[[^\[\]]{1,3}\]`)

// ruleKeyword は、ルールのキーワードにする作品名を、番組名から読む。話数の前を作品名にし (設計書 3.8 の番組名の解析)、
// 話数が書かれていなければ、印を除いた番組名をそのまま使う。話数まで入れると、その話にしか一致しないので、話数は入れない。
func ruleKeyword(name string) string {
	if parsed, ok := anime.ParseTitle(name); ok {
		return parsed.Title
	}

	return strings.Join(strings.Fields(bracketedMarks.ReplaceAllString(domain.Normalize(name), " ")), " ")
}

// ErrWishlistWorkNotFound は、ユーザーの候補に、その作品がないことを表す。
var ErrWishlistWorkNotFound = errors.New("the work is not in the wishlist candidates")

// ruleIdentity は、ルールを見分けるキー。見たいリストから作るルールは、キーワード 1 つとサービスの集合で決まるので、
// その 2 つを正規化してつなぐ。キーワードは、照合と同じく NFKC で正規化し、大文字と小文字をそろえる。
func ruleIdentity(keyword string, serviceIDs []int64) string {
	ids := slices.Clone(serviceIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}

	return strings.ToLower(strings.TrimSpace(domain.Normalize(keyword))) + "\x00" + strings.Join(parts, ",")
}

// existingRuleIdentities は、今あるルールの、キーワードごとの ruleIdentity を返す。無効なルールも含める。
// 無効にしたルールは、家族が録らないと決めたものなので、見たいリストから同じルールを作り直させない。
// 条件を読めないルール (新しい版が足した条件など) は、照合と同じく飛ばす。
func existingRuleIdentities(ctx context.Context, q *sqlc.Queries) (map[string]bool, error) {
	rows, err := q.ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}
	identities := map[string]bool{}
	for i := range rows {
		conditions, err := match.Decode(rows[i].Conditions)
		if err != nil {
			continue
		}
		for _, keyword := range conditions.Keywords {
			if keyword != nil && !keyword.Regexp {
				identities[ruleIdentity(keyword.Text, conditions.ServiceIDs)] = true
			}
		}
	}

	return identities, nil
}

// ErrWishlistWorkCovered は、候補の作品の放送に、もう予約があることを表す。ルールを二重に作らないため、ルールを作らせない。
var ErrWishlistWorkCovered = errors.New("the work already has a reservation")

// CreateWishlistRule は、ユーザー userID の候補の作品 tid を録るルールを、rules で作る (#928)。
// 作品の TID ごとにアドバイザリーロックを取ったトランザクションの中で、候補を求め直してから作る。二人が同時に押しても、
// 同じタブで二度押しても、ルールの照合が予約を作る前でも、後から来た方は、先に作ったルールで予約済み (ErrWishlistWorkCovered) になる。
func (s *Service) CreateWishlistRule(
	ctx context.Context, db Database, rules *rule.Rules, actor *audit.Actor, userID uuid.UUID, tid int32,
) (*rule.Rule, error) {
	var created *rule.Rule
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := sqlc.New(tx).LockAnimeWishlistRule(ctx, tid); err != nil {
			return fmt.Errorf("failed to lock the wishlist work: %w", err)
		}
		// ロックを取った後に読むので、先に作ったルールのトランザクションが終わっていれば、そのルールが見える。
		wishlist, err := s.Wishlist(ctx, tx, userID)
		if err != nil {
			return err
		}
		work, err := RuleCandidate(wishlist, tid)
		if err != nil {
			return err
		}
		system, err := settings.Load(ctx, tx)
		if err != nil {
			return err
		}
		created, err = rules.Create(ctx, tx, actor, WishlistRuleInput(work, system.Rule))

		return err
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// RuleCandidate は、候補 wishlist から、ルールを作る TID tid の作品を返す。
// 候補になければ ErrWishlistWorkNotFound を、放送のどれかに予約があれば ErrWishlistWorkCovered を返す。
// 予約済みの作品 (WishlistWork.Covered) には、ルールか手動の予約で録る放送が決まっているので、ルールを重ねて作らせない。
func RuleCandidate(wishlist *Wishlist, tid int32) (*WishlistWork, error) {
	for _, work := range wishlist.Works {
		if work.TID != tid {
			continue
		}
		if work.Covered {
			return nil, fmt.Errorf("%w: %d", ErrWishlistWorkCovered, tid)
		}

		return work, nil
	}

	return nil, fmt.Errorf("%w: %d", ErrWishlistWorkNotFound, tid)
}

// WishlistRuleInput は、候補の作品 work を録るルールの入力を作る (#928)。条件は、最初の放送から決める。
//
// ルールは作品 (TID) では照合できない (ルールの条件は番組の項目だけを見る。設計書 4.2) ので、最初の放送の番組名から読んだ作品名を
// 番組名のキーワードにし、最初の放送のサービスに限る。重複の回避、その期間、イベントリレーへの追従は、ルールを作るときの既定値
// (設計書 4.2) に従う。時間帯は、放送の延期や特番で外れると録り逃すので、条件に入れない。
func WishlistRuleInput(work *WishlistWork, defaults *settings.RuleDefaults) *rule.Input {
	first := work.Broadcasts[0]
	conditions := &match.Conditions{
		Keywords:     []*match.Keyword{{Text: ruleKeyword(first.Name), Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		KeywordMatch: match.KeywordMatchAll, ExcludeKeywords: []*match.Keyword{}, ChannelTypes: []string{}, ServiceIDs: []int64{first.ServiceID},
		ExcludeServiceIDs: []int64{}, Genres: []*match.Genre{}, ExcludeGenres: []*match.Genre{}, TimeRanges: []*match.TimeRange{},
		MinDurationSeconds: nil, MaxDurationSeconds: nil, FreeOnly: false, Marks: []string{}, ExcludeMarks: []string{},
	}

	return &rule.Input{
		Name: work.Title, Memo: "", Enabled: true, Conditions: conditions,
		Overrides: &domain.RecordingOverrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil, JoinSegments: nil, DetectCM: nil},
		ValidFrom: nil, ValidUntil: nil,
		AvoidDuplicates: defaults.AvoidDuplicates, DuplicateWindowDays: defaults.DuplicateWindowDays, FollowEventRelay: defaults.FollowEventRelay,
	}
}
