package animerecord

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// annictWebURL は、Annict の画面のオリジン。利用者を認可の画面へ送り出し、記録したページを示すのに使う。
	annictWebURL = "https://annict.com"
	// annictAPIURL は、Annict の API のオリジン。トークンの引き換え、取り消し、GraphQL に使う。
	annictAPIURL = "https://api.annict.com"
	// annictEpisodesPage は、作品のエピソードを 1 回に引く数。
	annictEpisodesPage = 100
	// annictMaxEpisodePages は、エピソードを引くページの数の上限。長く続く作品でも、1,000 話までは引く。
	annictMaxEpisodePages = 10
	// annictWatching は、Annict の作品の状態の「見てる」。
	annictWatching = "WATCHING"
)

// annictScopes は、Annict に求めるスコープを返す。記録を付けるので、write も求める。
func annictScopes() []string {
	return []string{"read", "write"}
}

// Annict は、Annict への記録先。エピソードごとに記録する。
type Annict struct {
	client    *http.Client
	endpoints *Endpoints
	// webURL は、記録したページの URL のオリジン。
	webURL string
}

var _ Destination = new(Annict)

// NewAnnict は Annict を返す。baseURL が空でなければ、すべての問い合わせをそのオリジンへ向ける (検証用)。
func NewAnnict(client *http.Client, baseURL string) *Annict {
	endpoints := &Endpoints{
		AuthURL: annictWebURL + "/oauth/authorize", TokenURL: annictAPIURL + "/oauth/token",
		RevokeURL: annictAPIURL + "/oauth/revoke", GraphQLURL: annictAPIURL + "/graphql",
	}
	webURL := annictWebURL
	if baseURL != "" {
		base := strings.TrimRight(baseURL, "/")
		endpoints = &Endpoints{
			AuthURL: base + "/oauth/authorize", TokenURL: base + "/oauth/token", RevokeURL: base + "/oauth/revoke", GraphQLURL: base + "/graphql",
		}
		webURL = base
	}

	return &Annict{client: client, endpoints: endpoints, webURL: webURL}
}

func (a *Annict) Name() string { return DestinationAnnict }

func (a *Annict) AuthCodeURL(app *App, redirectURI, state, verifier string) string {
	return oauth2Config(app, a.endpoints, redirectURI, annictScopes()).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (a *Annict) Exchange(ctx context.Context, app *App, redirectURI, code, verifier string) (string, error) {
	return exchange(ctx, a.client, oauth2Config(app, a.endpoints, redirectURI, annictScopes()), code, verifier)
}

func (a *Annict) AccountName(ctx context.Context, token string) (string, error) {
	var data struct {
		Viewer *struct {
			Username string `json:"username"`
		} `json:"viewer"`
	}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, `query { viewer { username } }`, map[string]any{}, &data); err != nil {
		return "", err
	}
	if data.Viewer == nil {
		return "", fmt.Errorf("%w: Annict returned no viewer", ErrPermanent)
	}

	return data.Viewer.Username, nil
}

// Revoke は、Annict のトークンを取り消す (POST /oauth/revoke)。
func (a *Annict) Revoke(ctx context.Context, app *App, token string) error {
	form := url.Values{"client_id": {app.ClientID}, "client_secret": {app.ClientSecret}, "token": {token}}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoints.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create the revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke the Annict token: %w", err)
	}
	_ = res.Body.Close()
	if res.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("the Annict revoke endpoint returned %d", res.StatusCode)
	}

	return nil
}

func (a *Annict) WorkIDs(works []*anime.Work) []int32 {
	ids := []int32{}
	for _, work := range works {
		if work.AnnictWorkID != nil {
			ids = append(ids, *work.AnnictWorkID)
		}
	}

	return ids
}

// annictWork は、Annict の作品。
type annictWork struct {
	ID                string  `json:"id"`
	AnnictID          int32   `json:"annictId"`
	NoEpisodes        bool    `json:"noEpisodes"`
	ViewerStatusState *string `json:"viewerStatusState"`
}

// annictEpisode は、Annict のエピソード。
type annictEpisode struct {
	ID         string  `json:"id"`
	AnnictID   int32   `json:"annictId"`
	Number     *int    `json:"number"`
	NumberText *string `json:"numberText"`
	Title      *string `json:"title"`
}

// RecordEpisode は、話を Annict に記録する。エピソードのない作品 (劇場版など) は、作品を「見た」にする。
// エピソードは、ユーザーのトークンで引き、話数、話数の文字列 (第1話、#1)、サブタイトルの順で照らし合わせる。
// 作品が複数あれば、エピソードが見つかった最初の作品に記録する。
func (a *Annict) RecordEpisode(ctx context.Context, token string, workIDs []int32, episode *Episode, rules *StatusRules) (*Recorded, error) {
	if len(workIDs) == 0 {
		return nil, fmt.Errorf("%w: the work is not on Annict", ErrPermanent)
	}
	works, err := a.works(ctx, token, workIDs)
	if err != nil {
		return nil, err
	}
	for _, work := range works {
		if work.NoEpisodes {
			if err := a.updateStatus(ctx, token, work.ID, "WATCHED"); err != nil {
				return nil, err
			}

			// エピソードのない作品は、「見た」にすることが記録なので、何度送っても同じになる。
			return &Recorded{URL: fmt.Sprintf("%s/works/%d", a.webURL, work.AnnictID), FollowUp: nil}, nil
		}
		episodes, err := a.episodes(ctx, token, work.AnnictID)
		if err != nil {
			return nil, err
		}
		found := findAnnictEpisode(episodes, episode)
		if found == nil {
			continue
		}
		if err := a.createRecord(ctx, token, found.ID); err != nil {
			return nil, err
		}
		recorded := &Recorded{URL: fmt.Sprintf("%s/works/%d/episodes/%d", a.webURL, work.AnnictID, found.AnnictID), FollowUp: nil}
		// 作品の状態は、記録を付けたことを残してから変える。ここで失敗を返すと、送り直しで記録が二重になる。
		if next, ok := NextStatus(rules, episode.First, episode.Last, annictStatus(work.ViewerStatusState)); ok {
			state := annictWatching
			if next == WorkStatusWatched {
				state = "WATCHED"
			}
			workID := work.ID
			recorded.FollowUp = func(ctx context.Context) error { return a.updateStatus(ctx, token, workID, state) }
		}

		return recorded, nil
	}

	return nil, fmt.Errorf("%w: the episode is not on Annict", ErrPermanent)
}

// annictWishlistPage は、見たいリストを 1 回に引く数。Annict の 1 ページの上限。
const annictWishlistPage = 100

// Wishlist は、Annict で「見たい」「見てる」にした作品の 1 ページを返す (#928)。ライブラリーを、状態で絞って引く。
// cursor は、Annict のページの endCursor。
func (a *Annict) Wishlist(ctx context.Context, token, cursor string) (*WishlistPage, error) {
	const query = `query ($first: Int, $after: String) { viewer {
  libraryEntries(states: [WANNA_WATCH, WATCHING], first: $first, after: $after) {
    nodes {
      status { state }
      nextEpisode { id number numberText title }
      work { annictId title seasonYear seasonName episodesCount image { recommendedImageUrl } }
    }
    pageInfo { hasNextPage endCursor }
  } } }`
	var data struct {
		Viewer *struct {
			LibraryEntries *struct {
				Nodes []*struct {
					Status *struct {
						State string `json:"state"`
					} `json:"status"`
					NextEpisode *annictNextEpisode  `json:"nextEpisode"`
					Work        *annictWishlistWork `json:"work"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"libraryEntries"`
		} `json:"viewer"`
	}
	variables := map[string]any{"first": annictWishlistPage, "after": nil}
	if cursor != "" {
		variables["after"] = cursor
	}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &WishlistPage{Entries: []*WishlistEntry{}, Next: ""}
	if data.Viewer == nil || data.Viewer.LibraryEntries == nil {
		return page, nil
	}
	entries := data.Viewer.LibraryEntries
	for _, node := range entries.Nodes {
		if node == nil || node.Work == nil || node.Status == nil {
			continue
		}
		var status string
		switch node.Status.State {
		case "WANNA_WATCH":
			status = WishlistWannaWatch
		case annictWatching:
			status = WishlistWatching
		default:
			// 絞って引いているので来ないが、ほかの状態の作品は候補にしない。
			continue
		}
		page.Entries = append(page.Entries, annictWishlistEntry(node.Work, status, node.NextEpisode))
	}
	if entries.PageInfo.HasNextPage && entries.PageInfo.EndCursor != nil {
		page.Next = *entries.PageInfo.EndCursor
	}

	return page, nil
}

// annictWishlistWork は、見たいリストの作品。
type annictWishlistWork struct {
	AnnictID      int32   `json:"annictId"`
	Title         string  `json:"title"`
	SeasonYear    *int32  `json:"seasonYear"`
	SeasonName    *string `json:"seasonName"`
	EpisodesCount *int32  `json:"episodesCount"`
	Image         *struct {
		RecommendedImageURL *string `json:"recommendedImageUrl"`
	} `json:"image"`
}

// annictNextEpisode は、ライブラリーの作品の、次に見る話。
type annictNextEpisode struct {
	ID         string  `json:"id"`
	Number     *int32  `json:"number"`
	NumberText *string `json:"numberText"`
	Title      *string `json:"title"`
}

// annictWishlistEntry は、Annict のライブラリーの作品を、見たいリストの作品にする。
// Annict は見終えた話数を持たないので、次に見る話の話数の 1 つ前を見終えた話数とする。
func annictWishlistEntry(work *annictWishlistWork, status string, next *annictNextEpisode) *WishlistEntry {
	entry := &WishlistEntry{
		WorkID: work.AnnictID, Status: status, Title: work.Title, NextEpisode: nil, Progress: nil,
		EpisodesCount: positive(work.EpisodesCount), SeasonYear: positive(work.SeasonYear), SeasonName: seasonName(work.SeasonName), ImageURL: "",
	}
	if work.Image != nil && work.Image.RecommendedImageURL != nil {
		entry.ImageURL = strings.TrimSpace(*work.Image.RecommendedImageURL)
	}
	if next == nil || next.ID == "" {
		return entry
	}
	entry.NextEpisode = &WishlistEpisode{ID: next.ID, Number: next.Number, NumberText: trimmed(next.NumberText), Title: trimmed(next.Title)}
	if next.Number != nil && *next.Number >= 1 {
		entry.Progress = new(*next.Number - 1)
	}

	return entry
}

// trimmed は、文字列の前後の空白を除く。nil なら空を返す。
func trimmed(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

// positive は、0 より大きい数だけを返す。記録先が分からない数を 0 で返すことがあるため。
func positive(value *int32) *int32 {
	if value == nil || *value <= 0 {
		return nil
	}

	return value
}

// works は、Annict の作品を、ids の順に引く。Annict にない作品は返さない。
func (a *Annict) works(ctx context.Context, token string, ids []int32) ([]*annictWork, error) {
	var data struct {
		SearchWorks *struct {
			Nodes []*annictWork `json:"nodes"`
		} `json:"searchWorks"`
	}
	const query = `query ($ids: [Int!]) { searchWorks(annictIds: $ids) { nodes { id annictId noEpisodes viewerStatusState } } }`
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, map[string]any{"ids": ids}, &data); err != nil {
		return nil, err
	}
	byID := map[int32]*annictWork{}
	if data.SearchWorks != nil {
		for _, work := range data.SearchWorks.Nodes {
			if work != nil {
				byID[work.AnnictID] = work
			}
		}
	}
	works := []*annictWork{}
	for _, id := range ids {
		if work, ok := byID[id]; ok {
			works = append(works, work)
		}
	}

	return works, nil
}

// episodes は、Annict の作品 workID のエピソードを、並びの順にすべて引く。
func (a *Annict) episodes(ctx context.Context, token string, workID int32) ([]*annictEpisode, error) {
	const query = `query ($ids: [Int!], $first: Int, $after: String) { searchWorks(annictIds: $ids) { nodes {
  episodes(first: $first, after: $after, orderBy: {field: SORT_NUMBER, direction: ASC}) {
    nodes { id annictId number numberText title } pageInfo { hasNextPage endCursor }
  } } } }`
	episodes := []*annictEpisode{}
	var after *string
	for range annictMaxEpisodePages {
		var data struct {
			SearchWorks *struct {
				Nodes []*struct {
					Episodes *struct {
						Nodes    []*annictEpisode `json:"nodes"`
						PageInfo struct {
							HasNextPage bool    `json:"hasNextPage"`
							EndCursor   *string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"episodes"`
				} `json:"nodes"`
			} `json:"searchWorks"`
		}
		variables := map[string]any{"ids": []int32{workID}, "first": annictEpisodesPage, "after": after}
		if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
			return nil, err
		}
		if data.SearchWorks == nil || len(data.SearchWorks.Nodes) == 0 || data.SearchWorks.Nodes[0] == nil || data.SearchWorks.Nodes[0].Episodes == nil {
			break
		}
		page := data.SearchWorks.Nodes[0].Episodes
		for _, episode := range page.Nodes {
			if episode != nil {
				episodes = append(episodes, episode)
			}
		}
		if !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == nil {
			break
		}
		after = page.PageInfo.EndCursor
	}

	return episodes, nil
}

// findAnnictEpisode は、Annict のエピソードから、話 episode に当たるものを探す。なければ nil を返す。
// 話数、話数の文字列 (第1話、#1。Annict には、まれに number だけがないデータがある)、サブタイトルの順に照らし合わせる。
func findAnnictEpisode(episodes []*annictEpisode, episode *Episode) *annictEpisode {
	if episode.Number != nil {
		number := *episode.Number
		for _, candidate := range episodes {
			if candidate.Number != nil && float64(*candidate.Number) == number {
				return candidate
			}
		}
		text := strconv.FormatFloat(number, 'f', -1, 64)
		for _, candidate := range episodes {
			if candidate.NumberText == nil {
				continue
			}
			numberText := domain.Normalize(strings.TrimSpace(*candidate.NumberText))
			if numberText == "第"+text+"話" || numberText == "#"+text {
				return candidate
			}
		}
	}
	if episode.Subtitle != nil && strings.TrimSpace(*episode.Subtitle) != "" {
		subtitle := domain.Normalize(strings.TrimSpace(*episode.Subtitle))
		for _, candidate := range episodes {
			if candidate.Title != nil && domain.Normalize(strings.TrimSpace(*candidate.Title)) == subtitle {
				return candidate
			}
		}
	}

	return nil
}

// annictStatus は、Annict の作品の状態を、記録先によらない状態にする。
func annictStatus(state *string) WorkStatus {
	if state == nil {
		return WorkStatusNone
	}
	switch *state {
	case annictWatching:
		return WorkStatusWatching
	case "WATCHED":
		return WorkStatusWatched
	default:
		return WorkStatusNone
	}
}

func (a *Annict) createRecord(ctx context.Context, token, episodeID string) error {
	var data struct {
		CreateRecord *struct {
			Record *struct {
				ID string `json:"id"`
			} `json:"record"`
		} `json:"createRecord"`
	}
	const query = `mutation ($episodeId: ID!) { createRecord(input: {episodeId: $episodeId}) { record { id } } }`
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, map[string]any{"episodeId": episodeID}, &data); err != nil {
		return err
	}
	if data.CreateRecord == nil || data.CreateRecord.Record == nil {
		return fmt.Errorf("%w: Annict did not create the record", ErrPermanent)
	}

	return nil
}

func (a *Annict) updateStatus(ctx context.Context, token, workID, state string) error {
	var data struct {
		UpdateStatus *struct {
			Work *struct {
				ID string `json:"id"`
			} `json:"work"`
		} `json:"updateStatus"`
	}
	const query = `mutation ($workId: ID!, $state: StatusState!) { updateStatus(input: {workId: $workId, state: $state}) { work { id } } }`

	return postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, map[string]any{"workId": workID, "state": state}, &data)
}
