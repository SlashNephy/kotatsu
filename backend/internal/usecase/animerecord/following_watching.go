package animerecord

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// maxFollowingWatchingPages と maxFollowingWatchingEntries は、フォローしている人が見ている作品を、記録先ごとに引くページの数と
	// 項目 (人と作品の組) の数の上限 (#1968)。フォローしている人の数の上限 (maxFollowingViewings、1,000 人) に、1 人あたり数作品を見込む。
	maxFollowingWatchingPages   = 100
	maxFollowingWatchingEntries = 5000
	// annictFollowingWatchingUsers と annictFollowingWatchingWorks は、Annict で 1 回に引く人の数と、1 人あたりの作品の数。
	// 1 人あたりの作品は、この数までにする。「見てる」の作品がこれより多い人は、残りを引かない。
	annictFollowingWatchingUsers = 25
	annictFollowingWatchingWorks = 50
	// aniListFollowingWatchingPage は、AniList で 1 回に引く件数 (Page の perPage の上限)。
	aniListFollowingWatchingPage = 50
	// aniListPhaseUsers と aniListPhaseList は、AniList のフォローしている人が見ている作品を引く段。
	aniListPhaseUsers = "users"
	aniListPhaseList  = "list"
	// aniListVariablePage は、AniList の Page のページの番号の変数。
	aniListVariablePage = "page"
	// aniListVariablePerPage は、AniList の Page の 1 ページの件数の変数。
	aniListVariablePerPage = "perPage"
)

// FollowingWatchingEntry は、フォローしている人が「見てる」にした 1 作品 (#1968)。
type FollowingWatchingEntry struct {
	// Username は、記録先でのユーザー名。Name は、画面に出す名前。AvatarURL は、記録先が示したアバターの URL (なければ空)。
	Username  string
	Name      string
	AvatarURL string
	// WorkID は、記録先の作品 ID。Title は、記録先の作品名。
	WorkID int32
	Title  string
}

// FollowingWatchingPage は、フォローしている人が見ている作品の 1 ページ。
type FollowingWatchingPage struct {
	Entries []*FollowingWatchingEntry
	// Next は、続きを引くときに渡す cursor。続きがなければ空。
	Next string
}

// FollowingWatching は、トークンの持ち主がフォローしている人が「見てる」にした作品の 1 ページを返す (#1968)。
// フォローしている人を annictFollowingWatchingUsers 人ずつ引き、1 人あたり annictFollowingWatchingWorks 作品まで引く。
func (a *Annict) FollowingWatching(ctx context.Context, token, cursor string) (*FollowingWatchingPage, error) {
	const query = `query ($cursor: String, $users: Int!, $works: Int!) {
  viewer { followingWatching: following(first: $users, after: $cursor) {
    nodes { name username avatarUrl watchingWorks: works(state: WATCHING, first: $works) { nodes { annictId title } } }
    pageInfo { hasNextPage endCursor }
  } }
}`
	var data struct {
		Viewer *struct {
			Following *struct {
				Nodes []*struct {
					Name      *string `json:"name"`
					Username  string  `json:"username"`
					AvatarURL *string `json:"avatarUrl"`
					Works     *struct {
						Nodes []*struct {
							AnnictID int32  `json:"annictId"`
							Title    string `json:"title"`
						} `json:"nodes"`
					} `json:"watchingWorks"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"followingWatching"`
		} `json:"viewer"`
	}
	var after any
	if cursor != "" {
		after = cursor
	}
	variables := map[string]any{"cursor": after, "users": annictFollowingWatchingUsers, "works": annictFollowingWatchingWorks}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &FollowingWatchingPage{Entries: []*FollowingWatchingEntry{}, Next: ""}
	if data.Viewer == nil || data.Viewer.Following == nil {
		return page, nil
	}
	for _, user := range data.Viewer.Following.Nodes {
		if user == nil || user.Works == nil {
			continue
		}
		name := user.Username
		if user.Name != nil && strings.TrimSpace(*user.Name) != "" {
			name = strings.TrimSpace(*user.Name)
		}
		avatar := ""
		if user.AvatarURL != nil {
			avatar = strings.TrimSpace(*user.AvatarURL)
		}
		for _, work := range user.Works.Nodes {
			if work == nil || work.AnnictID <= 0 {
				continue
			}
			page.Entries = append(page.Entries, &FollowingWatchingEntry{
				Username: user.Username, Name: name, AvatarURL: avatar, WorkID: work.AnnictID, Title: strings.TrimSpace(work.Title),
			})
		}
	}
	info := data.Viewer.Following.PageInfo
	if info.HasNextPage && info.EndCursor != nil {
		page.Next = *info.EndCursor
	}

	return page, nil
}

// aniListFollowingWatchingCursor は、AniList のフォローしている人が見ている作品を引く、どこまで進んだか。
// 自分の ID を引き、フォローしている人を Page.following で引いてから、その人たちの「視聴中」を Page.mediaList でまとめて引く。
// 引いた人の ID は、続きを引くのに要るので、cursor に入れて持ち回る (保存はしない)。
type aniListFollowingWatchingCursor struct {
	// phase は、aniListPhaseUsers (フォローしている人を引く) か aniListPhaseList (視聴中を引く)。
	phase   string
	viewer  int64
	page    int
	userIDs []int64
}

func (c *aniListFollowingWatchingCursor) String() string {
	ids := make([]string, 0, len(c.userIDs))
	for _, id := range c.userIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}

	return c.phase + ":" + strconv.FormatInt(c.viewer, 10) + ":" + strconv.Itoa(c.page) + ":" + strings.Join(ids, ",")
}

func parseAniListFollowingWatchingCursor(cursor string) (*aniListFollowingWatchingCursor, error) {
	parts := strings.SplitN(cursor, ":", 4)
	invalid := fmt.Errorf("invalid AniList following cursor %q", cursor)
	if len(parts) != 4 || (parts[0] != aniListPhaseUsers && parts[0] != aniListPhaseList) {
		return nil, invalid
	}
	viewer, viewerErr := strconv.ParseInt(parts[1], 10, 64)
	page, pageErr := strconv.Atoi(parts[2])
	if viewerErr != nil || pageErr != nil || page < 1 {
		return nil, invalid
	}
	c := &aniListFollowingWatchingCursor{phase: parts[0], viewer: viewer, page: page, userIDs: []int64{}}
	if parts[3] != "" {
		for raw := range strings.SplitSeq(parts[3], ",") {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, invalid
			}
			c.userIDs = append(c.userIDs, id)
		}
	}

	return c, nil
}

// FollowingWatching は、トークンの持ち主がフォローしている人の「視聴中」(CURRENT) の作品の 1 ページを返す (#1968)。
// 1 回目は自分の ID を、続けてフォローしている人を 1 ページずつ引き (人のページには作品を入れない)、最後に視聴中を 1 ページずつ引く。
func (a *AniList) FollowingWatching(ctx context.Context, token, cursor string) (*FollowingWatchingPage, error) {
	page := &FollowingWatchingPage{Entries: []*FollowingWatchingEntry{}, Next: ""}
	if cursor == "" {
		viewer, _, err := a.wishlistCursor(ctx, token, "")
		if err != nil {
			return nil, err
		}
		page.Next = (&aniListFollowingWatchingCursor{phase: aniListPhaseUsers, viewer: viewer, page: 1, userIDs: []int64{}}).String()

		return page, nil
	}
	c, err := parseAniListFollowingWatchingCursor(cursor)
	if err != nil {
		return nil, err
	}
	if c.phase == aniListPhaseUsers {
		return a.followingUsersPage(ctx, token, c)
	}

	return a.followingListPage(ctx, token, c)
}

// followingUsersPage は、フォローしている人の 1 ページを引き、続きの cursor を返す。人が尽きたら、視聴中を引く段に移る。
func (a *AniList) followingUsersPage(ctx context.Context, token string, c *aniListFollowingWatchingCursor) (*FollowingWatchingPage, error) {
	const query = `query ($userId: Int!, $page: Int!, $perPage: Int!) {
  Page(page: $page, perPage: $perPage) { followingUsers: following(userId: $userId) { id } pageInfo { hasNextPage } }
}`
	var data struct {
		Page *struct {
			Following []*struct {
				ID int64 `json:"id"`
			} `json:"followingUsers"`
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		} `json:"Page"`
	}
	variables := map[string]any{"userId": c.viewer, aniListVariablePage: c.page, aniListVariablePerPage: aniListFollowingWatchingPage}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &FollowingWatchingPage{Entries: []*FollowingWatchingEntry{}, Next: ""}
	ids := slices.Clone(c.userIDs)
	hasNext := false
	if data.Page != nil {
		for _, user := range data.Page.Following {
			if user != nil && user.ID > 0 {
				ids = append(ids, user.ID)
			}
		}
		hasNext = data.Page.PageInfo.HasNextPage
	}
	if len(ids) > maxFollowingViewings {
		ids, hasNext = ids[:maxFollowingViewings], false
	}
	switch {
	case hasNext:
		page.Next = (&aniListFollowingWatchingCursor{phase: aniListPhaseUsers, viewer: c.viewer, page: c.page + 1, userIDs: ids}).String()
	case len(ids) > 0:
		page.Next = (&aniListFollowingWatchingCursor{phase: aniListPhaseList, viewer: c.viewer, page: 1, userIDs: ids}).String()
	}

	return page, nil
}

// followingListPage は、フォローしている人の視聴中の作品の 1 ページを引く。
func (a *AniList) followingListPage(ctx context.Context, token string, c *aniListFollowingWatchingCursor) (*FollowingWatchingPage, error) {
	const query = `query ($userIds: [Int], $page: Int!, $perPage: Int!) {
  Page(page: $page, perPage: $perPage) {
    watchingList: mediaList(userId_in: $userIds, type: ANIME, status: CURRENT) {
      user { name avatar { large } }
      media { id title { native romaji } }
    }
    pageInfo { hasNextPage }
  }
}`
	var data struct {
		Page *struct {
			MediaList []*struct {
				User *struct {
					Name   string `json:"name"`
					Avatar *struct {
						Large *string `json:"large"`
					} `json:"avatar"`
				} `json:"user"`
				Media *struct {
					ID    int32              `json:"id"`
					Title *aniListMediaTitle `json:"title"`
				} `json:"media"`
			} `json:"watchingList"`
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		} `json:"Page"`
	}
	variables := map[string]any{"userIds": c.userIDs, aniListVariablePage: c.page, aniListVariablePerPage: aniListFollowingWatchingPage}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &FollowingWatchingPage{Entries: []*FollowingWatchingEntry{}, Next: ""}
	if data.Page == nil {
		return page, nil
	}
	for _, entry := range data.Page.MediaList {
		if entry == nil || entry.User == nil || entry.Media == nil || entry.Media.ID <= 0 || entry.User.Name == "" {
			continue
		}
		avatar := ""
		if entry.User.Avatar != nil && entry.User.Avatar.Large != nil {
			avatar = strings.TrimSpace(*entry.User.Avatar.Large)
		}
		page.Entries = append(page.Entries, &FollowingWatchingEntry{
			Username: entry.User.Name, Name: entry.User.Name, AvatarURL: avatar, WorkID: entry.Media.ID, Title: aniListTitle(entry.Media.Title),
		})
	}
	if data.Page.PageInfo.HasNextPage {
		page.Next = (&aniListFollowingWatchingCursor{phase: aniListPhaseList, viewer: c.viewer, page: c.page + 1, userIDs: c.userIDs}).String()
	}

	return page, nil
}

// fetchFollowingWatching は、記録先の、フォローしている人が見ている作品を、ページをたどって引く (#1968)。
// 問い合わせは、見たいリストと同じく、このプロセスの中では 1 つずつ、wishlistInterval だけ空けて送る。
func (s *Service) fetchFollowingWatching(ctx context.Context, destination Destination, token string) ([]*FollowingWatchingEntry, error) {
	s.wishlistMu.Lock()
	defer s.wishlistMu.Unlock()
	entries := []*FollowingWatchingEntry{}
	cursor := ""
	for range maxFollowingWatchingPages {
		if err := s.waitWishlistTurn(ctx, destination.Name()); err != nil {
			return nil, err
		}
		page, err := destination.FollowingWatching(ctx, token, cursor)
		s.wishlistLast[destination.Name()] = time.Now()
		if err != nil {
			return nil, err
		}
		entries = append(entries, page.Entries...)
		if page.Next == "" || len(entries) >= maxFollowingWatchingEntries {
			break
		}
		cursor = page.Next
	}
	if len(entries) > maxFollowingWatchingEntries {
		entries = entries[:maxFollowingWatchingEntries]
	}

	return entries, nil
}

// importFollowingWatching は、ユーザー userID の、記録先 destination のフォローしている人が見ている作品を取り込み、入れ替える (#1968)。
// 見たいリストを取り込めた後に呼ぶ。取り込めなければ、前に取り込んだ作品を残し、ログに残すだけにする (見たいリストの取り込みは失敗にしない)。
func (s *Service) importFollowingWatching(ctx context.Context, db Database, userID uuid.UUID, destination Destination, token string) {
	entries, err := s.fetchFollowingWatching(ctx, destination, token)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to import the works the following users are watching",
			slog.String("destination", destination.Name()), slog.Any("error", err))

		return
	}
	name := destination.Name()
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteAnimeFollowingWatching(ctx, sqlc.DeleteAnimeFollowingWatchingParams{UserID: userID, Destination: name}); err != nil {
			return fmt.Errorf("failed to delete the works the following users are watching: %w", err)
		}
		params := sqlc.InsertAnimeFollowingWatchingParams{
			UserID: userID, Destination: name, Now: s.clock.Now(),
			Usernames: make([]string, 0, len(entries)), Names: make([]string, 0, len(entries)), AvatarUrls: make([]string, 0, len(entries)),
			WorkIds: make([]int32, 0, len(entries)), Titles: make([]string, 0, len(entries)),
		}
		for _, entry := range entries {
			params.Usernames = append(params.Usernames, entry.Username)
			params.Names = append(params.Names, entry.Name)
			params.AvatarUrls = append(params.AvatarUrls, entry.AvatarURL)
			params.WorkIds = append(params.WorkIds, entry.WorkID)
			params.Titles = append(params.Titles, entry.Title)
		}
		// 連携を外した後なら、外部キーで書けないので、何もしない (行は連携と一緒に消えている)。
		if err := q.InsertAnimeFollowingWatching(ctx, params); err != nil {
			return fmt.Errorf("failed to save the works the following users are watching: %w", err)
		}

		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.WarnContext(ctx, "failed to save the works the following users are watching",
			slog.String("destination", name), slog.Any("error", err))
	}
}

// FollowingWatcher は、作品を見ている、フォローしている人。
type FollowingWatcher struct {
	Destination string
	Username    string
	Name        string
	// AvatarURL は、記録先が示したアバターの URL。なければ空。tenban が中継する。
	AvatarURL string
}

// FollowingWatchingWork は、フォローしている人が見ている作品のうち、これから放送されるもの (#1968)。
// 放送、予約済みの判定、ルールの作り方は、見たいリストの候補 (WishlistWork) と同じにする。
type FollowingWatchingWork struct {
	*WishlistWork

	// Watchers は、作品を見ている人。記録先、名前の順に並べ、同じ人が Annict と AniList の両方にいれば、両方を出す。
	Watchers []*FollowingWatcher
}

// FollowingWatchingWorks は、ユーザー userID がフォローしている人が「見てる」にした作品のうち、これから WishlistHorizon の間に
// 放送されるものを返す (#1968)。見ている人の多い順に、同じ数なら最初の放送の早い順に並べる。
func (s *Service) FollowingWatchingWorks(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*FollowingWatchingWork, error) {
	wishlist, watchers, err := s.followingWatchingCandidates(ctx, db, userID)
	if err != nil {
		return nil, err
	}
	works := make([]*FollowingWatchingWork, 0, len(wishlist.Works))
	for _, work := range wishlist.Works {
		works = append(works, &FollowingWatchingWork{WishlistWork: work, Watchers: watchers[work.TID]})
	}
	slices.SortStableFunc(works, func(a, b *FollowingWatchingWork) int {
		return cmp.Or(cmp.Compare(len(b.Watchers), len(a.Watchers)), a.Broadcasts[0].StartAt.Compare(b.Broadcasts[0].StartAt), cmp.Compare(a.TID, b.TID))
	})

	return works, nil
}

// followingWatchingCandidates は、フォローしている人が見ている作品を、見たいリストの候補と同じ形の候補にし、作品 (TID) ごとの見ている人と返す。
func (s *Service) followingWatchingCandidates(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (*Wishlist, map[int32][]*FollowingWatcher, error) {
	q := sqlc.New(db)
	rows, err := q.ListAnimeFollowingWatching(ctx, sqlc.ListAnimeFollowingWatchingParams{
		UserID: userID, FetchedAfter: s.clock.Now().Add(-FollowingRetention),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the works the following users are watching: %w", err)
	}
	// 作品ごとに 1 行にして、見たいリストの作品と同じく TID に引く。
	entries := []sqlc.AnimeWishlistEntry{}
	seen := map[string]bool{}
	byWork := map[string][]*FollowingWatcher{}
	for i := range rows {
		row := &rows[i]
		key := wishlistKey(row.Destination, row.WorkID)
		avatar := ""
		if row.AvatarUrl != nil {
			avatar = *row.AvatarUrl
		}
		byWork[key] = append(byWork[key], &FollowingWatcher{Destination: row.Destination, Username: row.Username, Name: row.Name, AvatarURL: avatar})
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, sqlc.AnimeWishlistEntry{
			UserID: userID, Destination: row.Destination, WorkID: row.WorkID, Status: WishlistWatching, Title: row.Title,
			NextEpisodeID: nil, NextEpisodeNumber: nil, NextEpisodeNumberText: nil, NextEpisodeTitle: nil, Progress: nil, EpisodesCount: nil,
			SeasonYear: nil, SeasonName: nil, ImageURL: nil, FetchedAt: time.Time{},
		})
	}
	sources, err := wishlistSources(ctx, q, entries)
	if err != nil {
		return nil, nil, err
	}
	watchers := map[int32][]*FollowingWatcher{}
	for tid, list := range sources {
		for _, source := range list {
			watchers[tid] = append(watchers[tid], byWork[wishlistKey(source.Destination, source.WorkID)]...)
		}
		slices.SortFunc(watchers[tid], func(a, b *FollowingWatcher) int {
			return cmp.Or(cmp.Compare(destinationOrder(a.Destination), destinationOrder(b.Destination)), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Username, b.Username))
		})
	}
	works, err := s.candidateWorks(ctx, q, sources)
	if err != nil {
		return nil, nil, err
	}

	return &Wishlist{Imports: []*WishlistImport{}, Works: works}, watchers, nil
}

// CreateFollowingWatchingRule は、ユーザー userID がフォローしている人が見ている作品 tid を録るルールを、rules で作る (#1968)。
// 見たいリストの候補からルールを作るとき (CreateWishlistRule) と同じく、作品の TID ごとのロックを取ってから候補を求め直す。
func (s *Service) CreateFollowingWatchingRule(
	ctx context.Context, db Database, rules *rule.Rules, actor *audit.Actor, userID uuid.UUID, tid int32,
) (*rule.Rule, error) {
	var created *rule.Rule
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := sqlc.New(tx).LockAnimeWishlistRule(ctx, tid); err != nil {
			return fmt.Errorf("failed to lock the wishlist work: %w", err)
		}
		candidates, _, err := s.followingWatchingCandidates(ctx, tx, userID)
		if err != nil {
			return err
		}
		work, err := RuleCandidate(candidates, tid)
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
