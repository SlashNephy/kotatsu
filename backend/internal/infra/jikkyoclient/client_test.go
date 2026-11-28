package jikkyoclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
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
	// hold は、閉じるまで /api/v1/channels の応答を止める。nil なら止めない。
	hold atomic.Pointer[chan struct{}]
}

func newFakeJikkyo(t *testing.T, channels, kakolog string) (*fakeJikkyo, *httptest.Server) {
	t.Helper()

	f := &fakeJikkyo{kakolog: kakolog}
	f.channels.Store(&channels)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/channels", func(w http.ResponseWriter, r *http.Request) {
		f.channelRequests.Add(1)
		if hold := f.hold.Load(); hold != nil {
			select {
			case <-*hold:
			case <-r.Context().Done():
				return
			}
		}
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
	// jkNHK と jkBS は、テストで使う実況のチャンネル。nameNHK は、jkNHK の名前。
	jkNHK   = "jk1"
	jkBS    = "jk101"
	nameNHK = "NHK総合"
)

const channelsJSON = `[{"id":"jk1","name":"NHK総合","threads":[]},{"id":"jk101","name":"NHK BS","threads":[]}]`

func TestClientChannel(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, channelsJSON, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: clk})

	got, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, &jikkyoclient.Channel{ID: jkNHK, Name: nameNHK}, got)
	// NX-Jikkyo が扱っていないチャンネルは nil。一覧は使い回すので、問い合わせない。
	got, err = client.Channel(t.Context(), "jk2")
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, int32(1), fake.channelRequests.Load())

	// 使い回す時間を過ぎたら再取得する。再取得する間と、再取得できなかったときは、前に取った一覧を使う。
	empty := ""
	fake.channels.Store(&empty)
	clk.Advance(time.Hour)
	got, err = client.Channel(t.Context(), jkBS)
	require.NoError(t, err)
	assert.Equal(t, "NHK BS", got.Name)
	assert.Eventually(t, func() bool { return fake.channelRequests.Load() == 2 }, time.Second, time.Millisecond)
	// 再取得できなかった後は、しばらく問い合わせない。再取得している最中も、重ねて問い合わせない。
	_, err = client.Channel(t.Context(), jkBS)
	require.NoError(t, err)
	assert.Equal(t, int32(2), fake.channelRequests.Load())
	clk.Advance(time.Minute)
	assert.Eventually(t, func() bool {
		_, err := client.Channel(t.Context(), jkBS)

		return err == nil && fake.channelRequests.Load() == 3
	}, time.Second, time.Millisecond)
}

// TestClientChannelActivity は、放送中のスレッドの勢いと視聴者数を返し、1 分で再取得することを確認する (#490)。
func TestClientChannelActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		threads string
		want    *jikkyoclient.Activity
	}{
		{
			name:    "放送中のスレッドの値を返す",
			threads: `[{"status":"PAST","jikkyo_force":3,"viewers":5},{"status":"ACTIVE","jikkyo_force":112,"viewers":4567}]`,
			want:    &jikkyoclient.Activity{Force: 112, Viewers: 4567},
		},
		{
			name:    "放送中のスレッドがなければ nil",
			threads: `[{"status":"PAST","jikkyo_force":3,"viewers":5}]`,
			want:    nil,
		},
		{
			name:    "見ている人とコメントがあれば、勢いが 0 でも返す",
			threads: `[{"status":"ACTIVE","jikkyo_force":0,"viewers":3,"comments":12}]`,
			want:    &jikkyoclient.Activity{Force: 0, Viewers: 3},
		},
		{
			name:    "勢いも見ている人も 0 なら nil",
			threads: `[{"status":"ACTIVE","jikkyo_force":0,"viewers":0,"comments":12}]`,
			want:    nil,
		},
		{
			name:    "勢いが 0 で、見ている人がいてもコメントがなければ nil",
			threads: `[{"status":"ACTIVE","jikkyo_force":0,"viewers":3,"comments":0}]`,
			want:    nil,
		},
		{
			name:    "勢いがあれば、コメントの数によらず返す",
			threads: `[{"status":"ACTIVE","jikkyo_force":2,"viewers":0,"comments":0}]`,
			want:    &jikkyoclient.Activity{Force: 2, Viewers: 0},
		},
		{
			name:    "勢いが null なら、0 と扱わず nil",
			threads: `[{"status":"ACTIVE","jikkyo_force":null,"viewers":3,"comments":12}]`,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, server := newFakeJikkyo(t, `[{"id":"jk1","name":"NHK総合","threads":`+tt.threads+`}]`, "")
			client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: nil})

			got, err := client.Channel(t.Context(), jkNHK)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Activity)
		})
	}
}

// TestClientChannelRefreshesActivityEveryMinute は、勢いが古くならないよう、1 分で一覧を再取得し、再取得したものを返すことを確認する。
func TestClientChannelRefreshesActivityEveryMinute(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, `[{"id":"jk1","name":"NHK総合","threads":[{"status":"ACTIVE","jikkyo_force":1,"viewers":1}]}]`, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: clk})
	_, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)

	next := `[{"id":"jk1","name":"NHK総合","threads":[{"status":"ACTIVE","jikkyo_force":50,"viewers":9}]}]`
	fake.channels.Store(&next)
	clk.Advance(59 * time.Second)
	_, err = client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, int32(1), fake.channelRequests.Load(), "the list is reused within a minute")

	// 1 分を過ぎたら、再取得したものを待って返す。前に取った古い勢いは返さない。
	clk.Advance(time.Second)
	got, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, &jikkyoclient.Activity{Force: 50, Viewers: 9}, got.Activity)
}

// TestClientChannelDoesNotWaitForUnresponsiveServer は、NX-Jikkyo が応答しなくても、一覧がまだなければ少し待つだけで諦め、
// 前に取った一覧があればそれを返して待たせず、同時の要求を 1 つの問い合わせにまとめることを確認する。
func TestClientChannelDoesNotWaitForUnresponsiveServer(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, channelsJSON, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: clk})
	hold := make(chan struct{})
	fake.hold.Store(&hold)

	// 一覧がまだなければ、同時の要求は 1 つの問い合わせを少しだけ待ち、間に合わなければ ErrChannelsPending を返す。
	const callers = 5
	results := make(chan error, callers)
	for range callers {
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err := client.Channel(ctx, jkNHK)
			results <- err
		}()
	}
	for range callers {
		require.ErrorIs(t, <-results, jikkyoclient.ErrChannelsPending)
	}
	require.Eventually(t, func() bool { return fake.channelRequests.Load() == 1 }, time.Second, time.Millisecond)
	// 取得の最中に続けて来た要求は、最初の要求が待った分を重ねて待たない。
	started := time.Now()
	_, err := client.Channel(t.Context(), jkNHK)
	require.ErrorIs(t, err, jikkyoclient.ErrChannelsPending)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	assert.Equal(t, int32(1), fake.channelRequests.Load())
	// 応答が届けば、その一覧を使う。問い合わせ直さない。
	close(hold)
	assert.Eventually(t, func() bool {
		got, err := client.Channel(t.Context(), jkNHK)

		return err == nil && got != nil && got.Name == nameNHK
	}, time.Second, time.Millisecond)
	assert.Equal(t, int32(1), fake.channelRequests.Load())

	// 使い回す時間を過ぎた後、NX-Jikkyo が応答しなくても、少し待つだけで、前に取った一覧を返す。
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	fake.hold.Store(&stuck)
	clk.Advance(time.Hour)
	started = time.Now()
	for range callers {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		got, err := client.Channel(ctx, jkNHK)
		cancel()
		require.NoError(t, err)
		assert.Equal(t, nameNHK, got.Name)
	}
	// 続けて呼んでも、待つのは 1 回の再取得につき staleWait までにとどまる。
	assert.Less(t, time.Since(started), time.Second)
	// 再取得は、裏で 1 つだけ問い合わせる。
	assert.Eventually(t, func() bool { return fake.channelRequests.Load() == 2 }, time.Second, time.Millisecond)
	assert.Never(t, func() bool { return fake.channelRequests.Load() > 2 }, 50*time.Millisecond, time.Millisecond)
}

// TestClientChannelAfterFailure は、取得に失敗したら、取得を終えてから 1 分は再取得せず、
// 再取得するときも、NX-Jikkyo が落ちているものとして待たないことを確認する。
func TestClientChannelAfterFailure(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, "", "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: clk})

	// 応答を止めている間に 30 秒たってから、取得に失敗する。
	hold := make(chan struct{})
	fake.hold.Store(&hold)
	_, err := client.Channel(t.Context(), jkNHK)
	require.ErrorIs(t, err, jikkyoclient.ErrChannelsPending)
	clk.Advance(30 * time.Second)
	close(hold)
	require.Eventually(t, func() bool {
		_, err := client.Channel(t.Context(), jkNHK)

		return err != nil && !errors.Is(err, jikkyoclient.ErrChannelsPending)
	}, time.Second, time.Millisecond)

	// 取得を始めてから 1 分を過ぎても、取得を終えてから 1 分たつまでは問い合わせない。
	clk.Advance(40 * time.Second)
	_, err = client.Channel(t.Context(), jkNHK)
	require.Error(t, err)
	assert.Never(t, func() bool { return fake.channelRequests.Load() > 1 }, 100*time.Millisecond, time.Millisecond)

	// 失敗してから 1 分たったら再取得する。NX-Jikkyo がまた応答しなくても、再取得を待たずに、前のエラーを返す。
	stuck := make(chan struct{})
	release := sync.OnceFunc(func() { close(stuck) })
	t.Cleanup(release)
	fake.hold.Store(&stuck)
	clk.Advance(20 * time.Second)
	started := time.Now()
	_, err = client.Channel(t.Context(), jkNHK)
	require.Error(t, err)
	require.NotErrorIs(t, err, jikkyoclient.ErrChannelsPending)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	assert.Eventually(t, func() bool { return fake.channelRequests.Load() == 2 }, time.Second, time.Millisecond)

	// NX-Jikkyo が戻れば、裏で進めた再取得の一覧を、次の要求から返す。
	recovered := channelsJSON
	fake.channels.Store(&recovered)
	release()
	assert.Eventually(t, func() bool {
		got, err := client.Channel(t.Context(), jkNHK)

		return err == nil && got != nil && got.Name == nameNHK
	}, time.Second, time.Millisecond)
	assert.Equal(t, int32(2), fake.channelRequests.Load())
}

func TestClientChannelRetriesAfterCanceledRequest(t *testing.T) {
	t.Parallel()

	fake, server := newFakeJikkyo(t, channelsJSON, "")
	clk := clocktest.NewFake(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: clk})

	// 取りやめた要求で取れなかったことは、NX-Jikkyo が落ちているとはみなさない。次の要求では、すぐに再取得する。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Channel(ctx, jkNHK)
	require.Error(t, err)
	got, err := client.Channel(t.Context(), jkNHK)
	require.NoError(t, err)
	assert.Equal(t, nameNHK, got.Name)
	assert.Equal(t, int32(1), fake.channelRequests.Load())
}

func TestClientChannelWithoutAnyList(t *testing.T) {
	t.Parallel()

	_, server := newFakeJikkyo(t, "", "")
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: nil})

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
		want    []*jikkyoclient.Comment
		wantErr bool
	}{
		{
			name: "投稿した時刻の順に並べ、マイクロ秒まで読む",
			kakolog: `{"packet":[
				{"chat":{"thread":"27325","no":"2","date":"1791633602","date_usec":"500000","user_id":"a:x","mail":"184 shita","content":"二つ目"}},
				{"chat":{"thread":"lv1","no":"1","date":"1791633601","user_id":"a:y","content":"一つ目"}}]}`,
			want: []*jikkyoclient.Comment{
				{At: time.Unix(1791633601, 0), Mail: "", Content: "一つ目", UserID: "a:y"},
				{At: time.Unix(1791633602, 500000000), Mail: "184 shita", Content: "二つ目", UserID: "a:x"},
			},
		},
		{name: "コメントがない", kakolog: `{"packet":[]}`, want: []*jikkyoclient.Comment{}},
		{name: "過去ログ API が理由を返す", kakolog: `{"error":"指定された実況 ID は存在しません。"}`, wantErr: true},
		{name: "JSON でない", kakolog: `<html>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeJikkyo(t, channelsJSON, tt.kakolog)
			client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: httpclienttest.New(t), Clock: nil})
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
