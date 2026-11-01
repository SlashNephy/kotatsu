package animedata_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/animedata"
)

const (
	sayaPath = "/saya.json"
	armPath  = "/arm.json"
)

// agents は、サーバーが受け取った User-Agent。
type agents struct {
	mu     sync.Mutex
	values []string
}

func (a *agents) add(value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.values = append(a.values, value)
}

func (a *agents) list() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.values)
}

// newServer は、ETag が "v1" の内容を返し、If-None-Match が一致すれば 304 を返すサーバーを用意する。受け取った User-Agent を seen に足す。
func newServer(t *testing.T, bodies map[string]string, seen *agents) *animedata.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.add(r.Header.Get("User-Agent"))
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)

			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return animedata.NewClient(&animedata.Options{
		HTTPClient: server.Client(), SayaDefinitionsURL: server.URL + sayaPath, ArmURL: server.URL + armPath,
	})
}

// TestClient は、取り込み元の JSON を読み、ETag を返し、変わっていなければ ErrNotModified を返すことを確かめる。
// 問い合わせには、Kotatsu からの問い合わせだと分かる User-Agent を付ける。
func TestClient(t *testing.T) {
	t.Parallel()

	seen := &agents{mu: sync.Mutex{}, values: []string{}}
	client := newServer(t, map[string]string{
		sayaPath: `{"channels":[{"name":"NHK 総合","type":"GR","serviceIds":[1024,1025],"networkId":15,"syobocalId":1,"annictId":1},` +
			`{"name":"無い","type":"BS","serviceIds":[999],"networkId":4}]}`,
		armPath: `[{"mal_id":5114,"anilist_id":5114,"annict_id":1745,"syobocal_tid":1575},{"annict_id":2,"anidb_id":1}]`,
	}, seen)

	channels, etag, err := client.SayaDefinitions(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, `"v1"`, etag)
	assert.Equal(t, []*animedata.Channel{
		{Name: "NHK 総合", Type: "GR", ServiceIDs: []int64{1024, 1025}, NetworkID: 15, SyobocalID: new(int32(1)), AnnictID: new(int32(1))},
		{Name: "無い", Type: "BS", ServiceIDs: []int64{999}, NetworkID: 4, SyobocalID: nil, AnnictID: nil},
	}, channels)

	works, _, err := client.Arm(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, []*animedata.Work{
		{SyobocalTID: new(int32(1575)), AnnictID: new(int32(1745)), AniListID: new(int32(5114)), MALID: new(int32(5114))},
		{SyobocalTID: nil, AnnictID: new(int32(2)), AniListID: nil, MALID: nil},
	}, works)

	_, _, err = client.SayaDefinitions(t.Context(), `"v1"`)
	require.ErrorIs(t, err, animedata.ErrNotModified)
	_, _, err = client.Arm(t.Context(), `"v1"`)
	require.ErrorIs(t, err, animedata.ErrNotModified)

	require.Len(t, seen.list(), 4)
	for _, agent := range seen.list() {
		assert.True(t, strings.HasPrefix(agent, "Kotatsu/"), agent)
		assert.Contains(t, agent, "https://github.com/SlashNephy/kotatsu")
	}
}

// TestClientErrors は、取り込めない応答をエラーにすることを確かめる。
func TestClientErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body map[string]string
	}{
		{name: "not found", body: map[string]string{}},
		{name: "broken JSON", body: map[string]string{sayaPath: `{"channels":`}},
		{name: "no channels", body: map[string]string{sayaPath: `{"boards":[]}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			seen := &agents{mu: sync.Mutex{}, values: []string{}}
			_, _, err := newServer(t, tt.body, seen).SayaDefinitions(t.Context(), "")
			require.Error(t, err)
			assert.NotErrorIs(t, err, animedata.ErrNotModified)
		})
	}
}
