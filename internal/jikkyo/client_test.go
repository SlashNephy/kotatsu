package jikkyo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/jikkyo"
)

// fakeJikkyo は、NX-Jikkyo と過去ログ API の代わり。
type fakeJikkyo struct {
	// channels は、/api/v1/channels が返す本文。空なら 503 を返す。
	channels atomic.Pointer[string]
	// kakolog は、/api/kakolog/ が返す本文。
	kakolog string
	// channelRequests は、チャンネルの一覧を要求された回数。query は、最後の過去ログの要求のクエリ。
	channelRequests atomic.Int32
	query           atomic.Pointer[string]
}

func newFakeJikkyo(t *testing.T, channels, kakolog string) (*fakeJikkyo, *httptest.Server) {
	t.Helper()

	f := &fakeJikkyo{kakolog: kakolog}
	f.channels.Store(&channels)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/channels", func(w http.ResponseWriter, _ *http.Request) {
		f.channelRequests.Add(1)
		body := *f.channels.Load()
		if body == "" {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("GET /api/kakolog/{id}", func(w http.ResponseWriter, r *http.Request) {
		query := r.PathValue("id") + "?" + r.URL.RawQuery
		f.query.Store(&query)
		_, _ = w.Write([]byte(f.kakolog))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return f, server
}

const (
	// jkNHK と jkBS は、テストで使う実況のチャンネル。
	jkNHK = "jk1"
	jkBS  = "jk101"
)

const channelsJSON = `[{"id":"jk1","name":"NHK総合","threads":[]},{"id":"jk101","name":"NHK BS","threads":[]}]`

func TestClientChannel(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, channelsJSON, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: nil, Clock: clk})

	got, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, &jikkyo.Channel{ID: jkNHK, Name: "NHK総合"}, got)
	// NX-Jikkyo が扱っていないチャンネルは nil。一覧は使い回すので、問い合わせない。
	got, err = client.Channel(t.Context(), "jk2")
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, int32(1), fake.channelRequests.Load())

	// 使い回す時間を過ぎたら取り直す。取り直せなくても、前に取った一覧を使う。
	empty := ""
	fake.channels.Store(&empty)
	clk.Advance(time.Hour)
	got, err = client.Channel(t.Context(), jkBS)
	require.NoError(t, err)
	assert.Equal(t, "NHK BS", got.Name)
	assert.Equal(t, int32(2), fake.channelRequests.Load())
	// 取り直せなかった後は、しばらく問い合わせない。
	_, err = client.Channel(t.Context(), jkBS)
	require.NoError(t, err)
	assert.Equal(t, int32(2), fake.channelRequests.Load())
	clk.Advance(time.Minute)
	_, err = client.Channel(t.Context(), jkBS)
	require.NoError(t, err)
	assert.Equal(t, int32(3), fake.channelRequests.Load())
}

func TestClientChannelRetriesAfterCanceledRequest(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, channelsJSON, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: nil, Clock: clk})

	// 取りやめた要求で取れなかったことは、NX-Jikkyo が落ちているとはみなさない。次の要求では、すぐに取り直す。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Channel(ctx, jkNHK)
	require.Error(t, err)
	got, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, "NHK総合", got.Name)
	assert.Equal(t, int32(1), fake.channelRequests.Load())
}

func TestClientChannelWithoutAnyList(t *testing.T) {
	t.Parallel()

	_, server := newFakeJikkyo(t, "", "")
	client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: nil, Clock: nil})

	// 一度も一覧を取れていなければ、エラーにする。
	_, err := client.Channel(t.Context(), jkNHK)
	assert.Error(t, err)
}

func TestClientComments(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		kakolog string
		want    []*jikkyo.Comment
		wantErr bool
	}{
		{
			name: "投稿した時刻の順に並べ、マイクロ秒まで読む",
			kakolog: `{"packet":[
				{"chat":{"thread":"27325","no":"2","date":"1791633602","date_usec":"500000","user_id":"a:x","mail":"184 shita","content":"二つ目"}},
				{"chat":{"thread":"lv1","no":"1","date":"1791633601","user_id":"a:y","content":"一つ目"}}]}`,
			want: []*jikkyo.Comment{
				{At: time.Unix(1791633601, 0), Mail: "", Content: "一つ目"},
				{At: time.Unix(1791633602, 500000000), Mail: "184 shita", Content: "二つ目"},
			},
		},
		{name: "コメントがない", kakolog: `{"packet":[]}`, want: []*jikkyo.Comment{}},
		{name: "過去ログ API が理由を返す", kakolog: `{"error":"指定された実況 ID は存在しません。"}`, wantErr: true},
		{name: "JSON でない", kakolog: `<html>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeJikkyo(t, channelsJSON, tt.kakolog)
			client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: nil, Clock: nil})
			got, err := client.Comments(t.Context(), jkNHK, from, from.Add(30*time.Minute))
			if tt.wantErr {
				assert.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, "jk1?endtime=1791635400&format=json&starttime=1791633600", *fake.query.Load())
		})
	}
}
