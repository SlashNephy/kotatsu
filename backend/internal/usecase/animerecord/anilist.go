package animerecord

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// aniListWebURL は、AniList の画面のオリジン。利用者を認可の画面へ送り出し、トークンを引き換える。
	aniListWebURL = "https://anilist.co"
	// aniListGraphQLURL は、AniList の GraphQL の API。
	aniListGraphQLURL = "https://graphql.anilist.co"
	// aniListCurrent は、AniList のリストの状態の「見てる」。
	aniListCurrent = "CURRENT"
)

// AniList は、AniList への記録先。エピソードごとではなく、何話まで見たか (progress) を持つ。
type AniList struct {
	client    *http.Client
	endpoints *Endpoints
	webURL    string
}

var _ Destination = new(AniList)

// NewAniList は AniList を返す。baseURL が空でなければ、すべての問い合わせをそのオリジンへ向ける (検証用)。
func NewAniList(client *http.Client, baseURL string) *AniList {
	endpoints := &Endpoints{
		AuthURL: aniListWebURL + "/api/v2/oauth/authorize", TokenURL: aniListWebURL + "/api/v2/oauth/token",
		RevokeURL: "", GraphQLURL: aniListGraphQLURL,
	}
	webURL := aniListWebURL
	if baseURL != "" {
		base := strings.TrimRight(baseURL, "/")
		endpoints = &Endpoints{
			AuthURL: base + "/api/v2/oauth/authorize", TokenURL: base + "/api/v2/oauth/token", RevokeURL: "", GraphQLURL: base + "/graphql",
		}
		webURL = base
	}

	return &AniList{client: client, endpoints: endpoints, webURL: webURL}
}

func (a *AniList) Name() string { return DestinationAniList }

func (a *AniList) AuthCodeURL(app *App, redirectURI, state, verifier string) string {
	return oauth2Config(app, a.endpoints, redirectURI, nil).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (a *AniList) Exchange(ctx context.Context, app *App, redirectURI, code, verifier string) (string, error) {
	return exchange(ctx, a.client, oauth2Config(app, a.endpoints, redirectURI, nil), code, verifier)
}

func (a *AniList) AccountName(ctx context.Context, token string) (string, error) {
	var data struct {
		Viewer *struct {
			Name string `json:"name"`
		} `json:"Viewer"`
	}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, `query { Viewer { name } }`, map[string]any{}, &data); err != nil {
		return "", err
	}
	if data.Viewer == nil {
		return "", fmt.Errorf("%w: AniList returned no viewer", ErrPermanent)
	}

	return data.Viewer.Name, nil
}

// Revoke は何もしない。AniList は、トークンを取り消す API を持たない。利用者は、AniList の設定の画面から連携を外す。
func (a *AniList) Revoke(context.Context, *App, string) error {
	return nil
}

func (a *AniList) WorkIDs(works []*anime.Work) []int32 {
	ids := []int32{}
	for _, work := range works {
		if work.AniListID != nil {
			ids = append(ids, *work.AniListID)
		}
	}

	return ids
}

// aniListMedia は、AniList の作品と、トークンの持ち主のリストの項目。
type aniListMedia struct {
	ID             int32   `json:"id"`
	SiteURL        *string `json:"siteUrl"`
	Episodes       *int    `json:"episodes"`
	MediaListEntry *struct {
		Progress *int    `json:"progress"`
		Status   *string `json:"status"`
	} `json:"mediaListEntry"`
}

// RecordEpisode は、何話まで見たか (progress) を、今の値より大きいときだけ更新する。見直しで巻き戻さないため。
// 話数のない放送は、1 話だけの作品 (劇場版など) なら 1 話を見たことにし、ほかは記録できないものとする。
// 作品が複数あれば、最初の作品に記録する。
func (a *AniList) RecordEpisode(ctx context.Context, token string, workIDs []int32, episode *Episode, rules *StatusRules) (*Recorded, error) {
	if len(workIDs) == 0 {
		return nil, fmt.Errorf("%w: the work is not on AniList", ErrPermanent)
	}
	var data struct {
		Media *aniListMedia `json:"Media"`
	}
	const query = `query ($id: Int) { Media(id: $id, type: ANIME) { id siteUrl episodes mediaListEntry { progress status } } }`
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, map[string]any{"id": workIDs[0]}, &data); err != nil {
		return nil, err
	}
	media := data.Media
	if media == nil {
		return nil, fmt.Errorf("%w: the work is not on AniList", ErrPermanent)
	}
	watched, err := aniListEpisode(media, episode)
	if err != nil {
		return nil, err
	}
	current, status := 0, WorkStatusNone
	if media.MediaListEntry != nil {
		if media.MediaListEntry.Progress != nil {
			current = *media.MediaListEntry.Progress
		}
		status = aniListStatus(media.MediaListEntry.Status)
	}
	variables := map[string]any{"mediaId": media.ID}
	if watched > current {
		variables["progress"] = watched
	}
	if next, ok := NextStatus(rules, episode.First, episode.Last, status); ok {
		variables["status"] = aniListCurrent
		if next == WorkStatusWatched {
			variables["status"] = "COMPLETED"
		}
	}
	// 進めた話数と状態は 1 回の SaveMediaListEntry で送る。話数は今より大きいときだけ進めるので、送り直しても二重にならない。
	recorded := &Recorded{URL: fmt.Sprintf("%s/anime/%d", a.webURL, media.ID), FollowUp: nil}
	if media.SiteURL != nil && strings.HasPrefix(*media.SiteURL, aniListWebURL+"/") {
		recorded.URL = *media.SiteURL
	}
	if len(variables) == 1 {
		// 見直しなどで、進めるものも変える状態もなければ、送らずに記録したことにする。
		return recorded, nil
	}
	if err := a.save(ctx, token, variables); err != nil {
		return nil, err
	}

	return recorded, nil
}

// aniListWishlistChunk は、見たいリストを 1 回に引く数。AniList の MediaListCollection の perChunk の上限。
const aniListWishlistChunk = 500

// Wishlist は、AniList で PLANNING か CURRENT にした作品の 1 ページを返す (#928)。
// MediaListCollection はユーザーを指して引くので、最初のページでは、先にトークンの持ち主の ID を引く。
// cursor は「ユーザー ID:チャンクの番号」の形にする。
func (a *AniList) Wishlist(ctx context.Context, token, cursor string) (*WishlistPage, error) {
	userID, chunk, err := a.wishlistCursor(ctx, token, cursor)
	if err != nil {
		return nil, err
	}
	const query = `query ($userId: Int, $chunk: Int, $perChunk: Int) {
  MediaListCollection(userId: $userId, type: ANIME, status_in: [PLANNING, CURRENT], chunk: $chunk, perChunk: $perChunk) {
    hasNextChunk lists { entries {
      status progress
      media { id title { native romaji } episodes season seasonYear coverImage { large } }
    } }
  } }`
	var data struct {
		MediaListCollection *struct {
			HasNextChunk bool `json:"hasNextChunk"`
			Lists        []*struct {
				Entries []*struct {
					Status   string                `json:"status"`
					Progress *int32                `json:"progress"`
					Media    *aniListWishlistMedia `json:"media"`
				} `json:"entries"`
			} `json:"lists"`
		} `json:"MediaListCollection"`
	}
	variables := map[string]any{"userId": userID, "chunk": chunk, "perChunk": aniListWishlistChunk}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &WishlistPage{Entries: []*WishlistEntry{}, Next: ""}
	if data.MediaListCollection == nil {
		return page, nil
	}
	for _, list := range data.MediaListCollection.Lists {
		if list == nil {
			continue
		}
		for _, entry := range list.Entries {
			if entry == nil || entry.Media == nil {
				continue
			}
			var status string
			switch entry.Status {
			case "PLANNING":
				status = WishlistWannaWatch
			case aniListCurrent:
				status = WishlistWatching
			default:
				continue
			}
			page.Entries = append(page.Entries, aniListWishlistEntry(entry.Media, status, entry.Progress))
		}
	}
	if data.MediaListCollection.HasNextChunk {
		page.Next = strconv.FormatInt(userID, 10) + ":" + strconv.Itoa(chunk+1)
	}

	return page, nil
}

// aniListWishlistMedia は、見たいリストの作品。
type aniListWishlistMedia struct {
	ID         int32              `json:"id"`
	Title      *aniListMediaTitle `json:"title"`
	Episodes   *int32             `json:"episodes"`
	Season     *string            `json:"season"`
	SeasonYear *int32             `json:"seasonYear"`
	CoverImage *struct {
		Large *string `json:"large"`
	} `json:"coverImage"`
}

// aniListWishlistEntry は、AniList のリストの作品を、見たいリストの作品にする。AniList は次に見る話を持たない。
func aniListWishlistEntry(media *aniListWishlistMedia, status string, progress *int32) *WishlistEntry {
	entry := &WishlistEntry{
		WorkID: media.ID, Status: status, Title: aniListTitle(media.Title), NextEpisode: nil, Progress: nil,
		EpisodesCount: positive(media.Episodes), SeasonYear: positive(media.SeasonYear), SeasonName: seasonName(media.Season), ImageURL: "",
	}
	if progress != nil && *progress >= 0 {
		entry.Progress = progress
	}
	if media.CoverImage != nil && media.CoverImage.Large != nil {
		entry.ImageURL = strings.TrimSpace(*media.CoverImage.Large)
	}

	return entry
}

// aniListMediaTitle は、AniList の作品名。
type aniListMediaTitle struct {
	Native *string `json:"native"`
	Romaji *string `json:"romaji"`
}

// aniListTitle は、作品名を、日本語の名前 (native) を先に選んで返す。
func aniListTitle(title *aniListMediaTitle) string {
	if title == nil {
		return ""
	}
	if title.Native != nil && strings.TrimSpace(*title.Native) != "" {
		return strings.TrimSpace(*title.Native)
	}
	if title.Romaji != nil {
		return strings.TrimSpace(*title.Romaji)
	}

	return ""
}

// wishlistCursor は、見たいリストの cursor から、ユーザー ID とチャンクの番号を返す。最初のページなら、トークンの持ち主の ID を引く。
func (a *AniList) wishlistCursor(ctx context.Context, token, cursor string) (int64, int, error) {
	if cursor == "" {
		var viewer struct {
			Viewer *struct {
				ID int64 `json:"id"`
			} `json:"Viewer"`
		}
		if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, `query { Viewer { id } }`, map[string]any{}, &viewer); err != nil {
			return 0, 0, err
		}
		if viewer.Viewer == nil {
			return 0, 0, fmt.Errorf("%w: AniList returned no viewer", ErrPermanent)
		}

		return viewer.Viewer.ID, 1, nil
	}
	user, next, ok := strings.Cut(cursor, ":")
	userID, userErr := strconv.ParseInt(user, 10, 64)
	chunk, chunkErr := strconv.Atoi(next)
	if !ok || userErr != nil || chunkErr != nil {
		return 0, 0, fmt.Errorf("invalid AniList wishlist cursor %q", cursor)
	}

	return userID, chunk, nil
}

// save は、リストの項目を書く。variables に入れた項目だけを送り、ほかの項目は変えない。
func (a *AniList) save(ctx context.Context, token string, variables map[string]any) error {
	params, args := []string{"$mediaId: Int"}, []string{"mediaId: $mediaId"}
	if _, ok := variables["progress"]; ok {
		params, args = append(params, "$progress: Int"), append(args, "progress: $progress")
	}
	if _, ok := variables["status"]; ok {
		params, args = append(params, "$status: MediaListStatus"), append(args, "status: $status")
	}
	query := fmt.Sprintf("mutation (%s) { SaveMediaListEntry(%s) { id progress status } }", strings.Join(params, ", "), strings.Join(args, ", "))
	var data struct {
		SaveMediaListEntry *struct {
			ID int64 `json:"id"`
		} `json:"SaveMediaListEntry"`
	}
	if err := postGraphQL(ctx, a.client, a.endpoints.GraphQLURL, token, query, variables, &data); err != nil {
		return err
	}
	if data.SaveMediaListEntry == nil {
		return fmt.Errorf("%w: AniList did not save the entry", ErrPermanent)
	}

	return nil
}

// aniListEpisode は、記録する話数を返す。話数のない放送は、1 話だけの作品でだけ 1 にする。
func aniListEpisode(media *aniListMedia, episode *Episode) (int, error) {
	if episode.Number == nil {
		if media.Episodes != nil && *media.Episodes == 1 {
			return 1, nil
		}

		return 0, fmt.Errorf("%w: the episode number is unknown", ErrPermanent)
	}
	// 総集編の 12.5 話のような話は、その前の話までを見たことにする。
	watched := int(math.Floor(*episode.Number))
	if watched < 1 {
		return 0, fmt.Errorf("%w: the episode number is out of range", ErrPermanent)
	}

	return watched, nil
}

// aniListStatus は、AniList のリストの状態を、記録先によらない状態にする。
func aniListStatus(status *string) WorkStatus {
	if status == nil {
		return WorkStatusNone
	}
	switch *status {
	case aniListCurrent, "REPEATING":
		return WorkStatusWatching
	case "COMPLETED":
		return WorkStatusWatched
	default:
		return WorkStatusNone
	}
}
