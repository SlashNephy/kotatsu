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

const armPath = "/arm.json"

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
		HTTPClient: server.Client(), ArmURL: server.URL + armPath,
	})
}

// TestArm は、arm の JSON を読み、ETag を返し、変わっていなければ ErrNotModified を返すことを確かめる。
// 問い合わせには、Kotatsu からの問い合わせだと分かる User-Agent を付ける。
func TestArm(t *testing.T) {
	t.Parallel()

	seen := &agents{mu: sync.Mutex{}, values: []string{}}
	client := newServer(t, map[string]string{
		armPath: `[{"mal_id":5114,"anilist_id":5114,"annict_id":1745,"syobocal_tid":1575},{"annict_id":2,"anidb_id":1}]`,
	}, seen)

	works, etag, err := client.Arm(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, `"v1"`, etag)
	assert.Equal(t, []*animedata.Work{
		{SyobocalTID: new(int32(1575)), AnnictID: new(int32(1745)), AniListID: new(int32(5114)), MALID: new(int32(5114))},
		{SyobocalTID: nil, AnnictID: new(int32(2)), AniListID: nil, MALID: nil},
	}, works)
	_, _, err = client.Arm(t.Context(), `"v1"`)
	require.ErrorIs(t, err, animedata.ErrNotModified)

	require.Len(t, seen.list(), 2)
	for _, agent := range seen.list() {
		assert.True(t, strings.HasPrefix(agent, "Kotatsu/"), agent)
		assert.Contains(t, agent, "https://github.com/SlashNephy/kotatsu")
	}
}

// TestChannelDefinitions は、埋め込んだ定義を読み、中身が変わらなければ ErrNotModified を返すことを確かめる。
// 地上波の系列局は、キー局ではなく、その局のしょぼいカレンダーの ChID と Annict のチャンネルを持つ (#1589)。
func TestChannelDefinitions(t *testing.T) {
	t.Parallel()

	client := animedata.NewClient(nil)
	channels, etag, err := client.ChannelDefinitions(t.Context(), "")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(etag, "sha256:"), etag)
	_, _, err = client.ChannelDefinitions(t.Context(), etag)
	require.ErrorIs(t, err, animedata.ErrNotModified)

	type ids struct {
		name     string
		syobocal *int32
		annict   *int32
	}
	terrestrial := map[int64]ids{}
	for _, c := range channels {
		if c.Type != "GR" {
			continue
		}
		for _, sid := range c.ServiceIDs {
			_, dup := terrestrial[sid]
			require.False(t, dup, "service %d is defined twice", sid)
			terrestrial[sid] = ids{name: c.Name, syobocal: c.SyobocalID, annict: c.AnnictID}
		}
	}
	tests := []struct {
		name string
		sid  int64
		want ids
	}{
		{name: "key station", sid: 1048, want: ids{name: "TBS", syobocal: new(int32(5)), annict: new(int32(5))}},
		{name: "Kansai affiliate", sid: 2064, want: ids{name: "MBS毎日放送", syobocal: new(int32(48)), annict: new(int32(48))}},
		{name: "Kansai TXN affiliate", sid: 41008, want: ids{name: "テレビ大阪", syobocal: new(int32(28)), annict: new(int32(28))}},
		{name: "Annict ID differs from ChID", sid: 58408, want: ids{name: "長崎国際テレビ", syobocal: new(int32(171)), annict: new(int32(173))}},
		{name: "not on Syobocal", sid: 18448, want: ids{name: "秋田放送", syobocal: nil, annict: new(int32(295))}},
		{name: "Hokkaido sub-area", sid: 11280, want: ids{name: "北海道放送", syobocal: new(int32(89)), annict: new(int32(89))}},
		{name: "NHK in Osaka", sid: 40960, want: ids{name: "NHK 総合", syobocal: new(int32(1)), annict: new(int32(1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, terrestrial[tt.sid])
		})
	}
}

// TestParseChannelDefinitionsErrors は、読めない定義をエラーにすることを確かめる。
func TestParseChannelDefinitionsErrors(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"channels":`, `{"boards":[]}`} {
		_, _, err := animedata.ParseChannelDefinitions([]byte(body), "")
		require.Error(t, err)
		assert.NotErrorIs(t, err, animedata.ErrNotModified)
	}
}

// TestClientErrorsHideURL は、取得できなかったときのエラーに、資格情報を含みうる URL を書かないことを確かめる。
// エラーは、取り込めなかった理由として DB に残り、管理者の画面に出る。
func TestClientErrorsHideURL(t *testing.T) {
	t.Parallel()

	const token = "secret-token"
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "unexpected status", baseURL: server.URL, want: "failed to get arm: unexpected status 404 Not Found"},
		{name: "connection refused", baseURL: closed.URL, want: "failed to get arm: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := animedata.NewClient(&animedata.Options{
				HTTPClient: server.Client(), ArmURL: tt.baseURL + "/arm.json?token=" + token,
			})
			_, _, err := client.Arm(t.Context(), "")
			require.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), token)
		})
	}
}
