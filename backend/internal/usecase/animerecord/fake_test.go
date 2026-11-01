package animerecord_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// 偽の API が答える操作と、変数の名前。
const (
	opCreateRecord = "createRecord"
	opUpdateStatus = "updateStatus"
	opSaveEntry    = "SaveMediaListEntry"
	opEpisodes     = "episodes"
	opSearchWorks  = "searchWorks"
	opMedia        = "Media"
	keyProgress    = "progress"
	keyStatus      = "status"
	keyMediaID     = "mediaId"
	// opAniListViewer は、AniList のトークンの持ち主を引く操作。Annict の viewer と大文字で分ける。
	opAniListViewer = "Viewer"
	opAnnictViewer  = "viewer"
	// opLibrary と opMediaLists は、Annict と AniList の見たいリストを引く操作 (#928)。
	opLibrary    = "libraryEntries"
	opMediaLists = "MediaListCollection"
	// fakeAniListUserID は、偽の AniList のトークンの持ち主の ID。
	fakeAniListUserID = 77
	keyNodes          = "nodes"
)

// fakeListEntry は、偽の見たいリストの作品。Status は、Annict なら WANNA_WATCH などの状態、AniList なら PLANNING などのリストの状態。
type fakeListEntry struct {
	ID     int32
	Title  string
	Status string
	// Next は、Annict の次に見る話。Progress は、AniList の見終えた話数。
	Next     *fakeEpisode
	Progress *int
	// Episodes は全話数、Season と SeasonYear は放送の時期 (記録先の名前のまま)、Image は画像の URL。空なら返さない。
	Episodes   *int
	Season     string
	SeasonYear int
	Image      string
}

// orNull は、空の値を、GraphQL の null として返す。
func orNull[T comparable](value T) any {
	var zero T
	if value == zero {
		return nil
	}

	return value
}

// fakeEpisode は、偽の Annict のエピソード。
type fakeEpisode struct {
	ID         string  `json:"id"`
	AnnictID   int32   `json:"annictId"`
	Number     *int    `json:"number"`
	NumberText *string `json:"numberText"`
	Title      *string `json:"title"`
}

// fakeWork は、偽の Annict の作品。
type fakeWork struct {
	AnnictID   int32
	NoEpisodes bool
	Status     *string
	Episodes   []fakeEpisode
}

// fakeEntry は、偽の AniList のリストの項目。
type fakeEntry struct {
	Progress int
	Status   *string
}

// fakeService は、Annict と AniList の OAuth と GraphQL の API を真似る。本物には問い合わせない。
type fakeService struct {
	t      *testing.T
	server *httptest.Server

	mu sync.Mutex
	// codes は、受け付ける認可コードから、発行するトークンへの表。
	codes map[string]string
	// verifiers は、トークンの引き換えで受け取った code_verifier。
	verifiers []string
	// revoked は、取り消したトークン。
	revoked []string
	// works は、Annict の作品。
	works map[int32]*fakeWork
	// records は、Annict で記録したエピソードの ID。
	records []string
	// statuses は、Annict で変えた作品の状態 (作品の ID: 状態)。
	statuses []string
	// media は、AniList の作品の ID から、話数への表。entries は、リストの項目。
	media   map[int32]*int
	entries map[int32]*fakeEntry
	// saves は、AniList に送った SaveMediaListEntry の変数。
	saves []map[string]any
	// failures は、GraphQL の操作の名前ごとに、あと何回 500 を返すか。
	failures map[string]int
	// tokens は、GraphQL で受け付けるトークン。
	tokens map[string]bool
	// hooks は、GraphQL の操作の名前ごとに、答える前に呼ぶ関数。ジョブの途中で何かを起こすのに使う。
	hooks map[string]func()
	// list は、見たいリストの作品。pageSize ずつ返す (0 なら 1 ページで返す)。
	list     []fakeListEntry
	pageSize int
	// rateLimits は、GraphQL の操作の名前ごとに、あと何回 429 を返すか。retryAfter は、そのときの Retry-After。
	rateLimits map[string]int
	retryAfter string
	// listRequests は、見たいリストを引いた回数。listVariables は、そのときの変数。
	listRequests  int
	listVariables []map[string]any
}

func newFakeService(t *testing.T) *fakeService {
	t.Helper()
	f := &fakeService{
		t: t, codes: map[string]string{}, works: map[int32]*fakeWork{}, media: map[int32]*int{}, entries: map[int32]*fakeEntry{},
		failures: map[string]int{}, tokens: map[string]bool{}, hooks: map[string]func(){},
		list: []fakeListEntry{}, pageSize: 0, rateLimits: map[string]int{}, listVariables: []map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", f.token)
	mux.HandleFunc("POST /api/v2/oauth/token", f.token)
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.revoked = append(f.revoked, r.FormValue("token"))
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("POST /graphql", f.graphql)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeService) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	token, ok := f.codes[r.FormValue("code")]
	if !ok || r.FormValue("code_verifier") == "" || r.FormValue("client_secret") != "secret" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))

		return
	}
	f.verifiers = append(f.verifiers, r.FormValue("code_verifier"))
	f.tokens[token] = true
	w.Header().Set("Content-Type", "application/json")
	f.write(w, map[string]any{"access_token": token, "token_type": "bearer"})
}

func (f *fakeService) graphql(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`))

		return
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}
	operation := operationOf(req.Query)
	if f.rateLimits[operation] > 0 {
		f.rateLimits[operation]--
		if f.retryAfter != "" {
			w.Header().Set("Retry-After", f.retryAfter)
		}
		w.WriteHeader(http.StatusTooManyRequests)

		return
	}
	if f.failures[operation] > 0 {
		f.failures[operation]--
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
	data := f.resolve(operation, req.Variables)
	if hook, ok := f.hooks[operation]; ok {
		hook()
	}
	w.Header().Set("Content-Type", "application/json")
	f.write(w, map[string]any{"data": data})
}

// write は、v を JSON で書く。
func (f *fakeService) write(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("failed to write the response: %v", err)
	}
}

// number は、JSON の数を int32 にする。数でなければ 0 にする。
func number(value any) int32 {
	n, _ := value.(float64)

	return int32(n)
}

// text は、JSON の文字列を返す。文字列でなければ空にする。
func text(value any) string {
	s, _ := value.(string)

	return s
}

// operationOf は、query から、偽の API が答える操作を決める。
func operationOf(query string) string {
	for _, operation := range []string{
		opLibrary, opMediaLists, opCreateRecord, opUpdateStatus, opSaveEntry, opEpisodes + "(", opSearchWorks, opMedia + "(", opAniListViewer, opAnnictViewer,
	} {
		if strings.Contains(query, operation) {
			return strings.TrimSuffix(operation, "(")
		}
	}

	return ""
}

func (f *fakeService) resolve(operation string, variables map[string]any) any {
	switch operation {
	case opAnnictViewer:
		return map[string]any{opAnnictViewer: map[string]any{"username": "annict-user"}}
	case opAniListViewer:
		return map[string]any{opAniListViewer: map[string]any{"id": fakeAniListUserID, "name": "anilist-user"}}
	case opLibrary, opMediaLists:
		return f.listPage(operation, variables)
	case opSearchWorks, opEpisodes:
		nodes := []any{}
		ids, _ := variables["ids"].([]any)
		for _, raw := range ids {
			work, ok := f.works[number(raw)]
			if !ok {
				continue
			}
			node := map[string]any{"id": "W" + jsonID(work.AnnictID), "annictId": work.AnnictID, "noEpisodes": work.NoEpisodes, "viewerStatusState": work.Status}
			if operation == opEpisodes {
				node = map[string]any{opEpisodes: map[string]any{"nodes": work.Episodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil}}}
			}
			nodes = append(nodes, node)
		}

		return map[string]any{"searchWorks": map[string]any{"nodes": nodes}}
	case opCreateRecord:
		f.records = append(f.records, text(variables["episodeId"]))

		return map[string]any{"createRecord": map[string]any{"record": map[string]any{"id": "R1"}}}
	case opUpdateStatus:
		f.statuses = append(f.statuses, text(variables["workId"])+":"+text(variables["state"]))

		return map[string]any{"updateStatus": map[string]any{"work": map[string]any{"id": variables["workId"]}}}
	case opMedia:
		id := number(variables["id"])
		episodes, ok := f.media[id]
		if !ok {
			return map[string]any{opMedia: nil}
		}
		var entry any
		if e, ok := f.entries[id]; ok {
			entry = map[string]any{keyProgress: e.Progress, keyStatus: e.Status}
		}

		return map[string]any{opMedia: map[string]any{"id": id, "siteUrl": nil, opEpisodes: episodes, "mediaListEntry": entry}}
	case opSaveEntry:
		f.saves = append(f.saves, variables)
		id := number(variables[keyMediaID])
		entry, ok := f.entries[id]
		if !ok {
			entry = &fakeEntry{Progress: 0, Status: nil}
			f.entries[id] = entry
		}
		if progress, ok := variables[keyProgress].(float64); ok {
			entry.Progress = int(progress)
		}
		if status, ok := variables[keyStatus].(string); ok {
			entry.Status = &status
		}

		return map[string]any{opSaveEntry: map[string]any{"id": 1, keyProgress: entry.Progress, keyStatus: entry.Status}}
	default:
		f.t.Errorf("unexpected GraphQL operation %q", operation)

		return nil
	}
}

func jsonID(id int32) string {
	return strconv.FormatInt(int64(id), 10)
}

// snapshot は、記録した内容 (Annict の記録、作品の状態、AniList に送った変数) を、ロックを取ってから写す。
func (f *fakeService) snapshot() ([]string, []string, []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string{}, f.records...), append([]string{}, f.statuses...), append([]map[string]any{}, f.saves...)
}

// listPage は、見たいリストの 1 ページを返す。Annict は after の cursor (返した数) で、AniList は chunk の番号で続きを引く。
func (f *fakeService) listPage(operation string, variables map[string]any) any {
	f.listRequests++
	f.listVariables = append(f.listVariables, variables)
	size := f.pageSize
	if size <= 0 {
		size = len(f.list)
	}
	start := 0
	if operation == opLibrary {
		if after, ok := variables["after"].(string); ok {
			start, _ = strconv.Atoi(after)
		}
	} else {
		if n := int(number(variables["chunk"])); n > 1 {
			start = (n - 1) * size
		}
		if number(variables["userId"]) != fakeAniListUserID {
			f.t.Errorf("unexpected AniList user %v", variables["userId"])
		}
	}
	end := min(start+size, len(f.list))
	start = min(start, end)
	hasNext := end < len(f.list)
	if operation == opLibrary {
		nodes := make([]any, 0, end-start)
		for _, entry := range f.list[start:end] {
			var image any
			if entry.Image != "" {
				image = map[string]any{"recommendedImageUrl": entry.Image}
			}
			nodes = append(nodes, map[string]any{
				"status": map[string]any{"state": entry.Status}, "nextEpisode": entry.Next,
				"work": map[string]any{
					"annictId": entry.ID, "title": entry.Title, "seasonYear": orNull(entry.SeasonYear), "seasonName": orNull(entry.Season),
					"episodesCount": entry.Episodes, "image": image,
				},
			})
		}

		return map[string]any{opAnnictViewer: map[string]any{opLibrary: map[string]any{
			keyNodes: nodes, "pageInfo": map[string]any{"hasNextPage": hasNext, "endCursor": strconv.Itoa(end)},
		}}}
	}
	entries := []any{}
	for _, entry := range f.list[start:end] {
		var cover any
		if entry.Image != "" {
			cover = map[string]any{"large": entry.Image}
		}
		entries = append(entries, map[string]any{"status": entry.Status, "progress": entry.Progress, "media": map[string]any{
			"id": entry.ID, "title": map[string]any{"native": entry.Title, "romaji": "romaji"},
			"episodes": entry.Episodes, "season": orNull(entry.Season), "seasonYear": orNull(entry.SeasonYear), "coverImage": cover,
		}})
	}

	return map[string]any{opMediaLists: map[string]any{"hasNextChunk": hasNext, "lists": []any{map[string]any{"entries": entries}}}}
}

// setList は、見たいリストの作品と、1 ページの数を、ロックを取ってから変える。
func (f *fakeService) setList(list []fakeListEntry, pageSize int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list, f.pageSize = list, pageSize
}

// failNext は、操作 operation に、次の n 回、状態コード status を返させる。
func (f *fakeService) failNext(operation string, status, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status == http.StatusTooManyRequests {
		f.rateLimits[operation] = n

		return
	}
	f.failures[operation] = n
}

// requests は、見たいリストを引いた回数を返す。
func (f *fakeService) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.listRequests
}

// rateLimitWith は、操作 operation に、次の 1 回、Retry-After に value を付けて 429 を返させる。
func (f *fakeService) rateLimitWith(operation, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rateLimits[operation] = 1
	f.retryAfter = value
}
