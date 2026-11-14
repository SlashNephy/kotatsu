package iptvapi_test

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
	"github.com/SlashNephy/kotatsu/internal/web/iptvapi"
)

const (
	validFeed       = "kotatsu_iptv_valid"
	unwatchableFeed = "kotatsu_iptv_cannot_watch"
	base            = "https://kotatsu.test/api/iptv/" + validFeed
)

func testNow() time.Time {
	return time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)
}

func services() []domain.Service {
	return []domain.Service{
		{
			ID: 3273601024, NetworkID: 32736, Sid: 1024, Name: "こたつテスト1", ServiceType: 1, ChannelType: "GR", Channel: "27",
			RemoteControlKeyID: new(int64(1)), LogoID: new(int64(0)), Hidden: false, SortOrder: nil,
		},
		{
			ID: 400141, NetworkID: 4, Sid: 141, Name: "こたつ \"BS\"\n2", ServiceType: 1, ChannelType: "BS", Channel: "BS13_0",
			RemoteControlKeyID: nil, LogoID: nil, Hidden: false, SortOrder: nil,
		},
	}
}

// fakeGate は、管理者が IPTV のクライアント用の URL を有効にしているかを返す。
type fakeGate struct {
	enabled bool
	err     error
}

func (g fakeGate) IPTVFeedsEnabled(context.Context) (bool, error) {
	return g.enabled, g.err
}

type fakeAuth struct{}

func (fakeAuth) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	switch token {
	case validFeed:
		return &auth.Principal{UserID: uuid.New(), Role: domain.RoleMember, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
	case unwatchableFeed:
		return &auth.Principal{UserID: uuid.New(), Role: domain.Role("none"), SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
	case "broken":
		return nil, errors.New("database is down")
	default:
		return nil, auth.ErrUnauthenticated
	}
}

type fakeQueries struct {
	from, to time.Time
}

func (f *fakeQueries) ListGuideServices(context.Context, query.ListGuideServicesParams) ([]domain.Service, error) {
	return services(), nil
}

func (f *fakeQueries) ListProgramsInRange(_ context.Context, arg query.ListProgramsInRangeParams) ([]domain.Program, error) {
	f.from, f.to = arg.FromAt, arg.ToAt

	return []domain.Program{program()}, nil
}

func program() domain.Program {
	return domain.Program{
		ServiceID: 400141, EventID: 1, StartAt: time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		EndAt: time.Date(2026, 11, 7, 12, 30, 0, 0, time.FixedZone("JST", 9*60*60)), EndUndecided: false, IsFree: true,
		Name: "ニュース & 天気 <速報>", Description: "説明", Extended: nil,
		Genres: []byte(`[{"lv1":0,"lv2":0,"un1":15,"un2":15},{"lv1":0,"lv2":1,"un1":15,"un2":15},{"lv1":7,"lv2":0,"un1":15,"un2":15},{"lv1":15,"lv2":15,"un1":15,"un2":15}]`),
		Audios: nil, RelatedItems: nil,
	}
}

// TestPlaylist は、サービスごとに、無変換の TS のストリームとロゴと、XMLTV のチャンネルの ID を書いた M3U を作ることを確かめる (#1324)。
func TestPlaylist(t *testing.T) {
	t.Parallel()

	want := strings.Join([]string{
		`#EXTM3U url-tvg="` + base + `/guide.xml" x-tvg-url="` + base + `/guide.xml"`,
		`#EXTINF:-1 tvg-id="3273601024" tvg-name="こたつテスト1" tvg-logo="` + base + `/services/3273601024/logo" tvg-chno="1" group-title="GR",こたつテスト1`,
		base + "/services/3273601024/stream.m2ts",
		// 属性の二重引用符と、名前の改行は、M3U を壊さないよう置き換える。ロゴのないサービスには tvg-logo を書かない。
		`#EXTINF:-1 tvg-id="400141" tvg-name="こたつ 'BS' 2" tvg-chno="141" group-title="BS",こたつ "BS" 2`,
		base + "/services/400141/stream.m2ts",
		"",
	}, "\n")
	assert.Equal(t, want, iptvapi.Playlist(base, services()))
}

// TestGuide は、M3U と同じチャンネルの ID で、番組を UTC の時刻で書いた XMLTV を作ることを確かめる (#1324)。
func TestGuide(t *testing.T) {
	t.Parallel()

	undecided := program()
	undecided.EventID, undecided.EndUndecided, undecided.Description, undecided.Genres = 2, true, "", nil
	body, err := iptvapi.Guide(base, services(), []domain.Program{program(), undecided})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), xml.Header+`<!DOCTYPE tv SYSTEM "xmltv.dtd">`), string(body))

	var doc struct {
		Channels []struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"display-name"`
			Icon *struct {
				Src string `xml:"src,attr"`
			} `xml:"icon"`
		} `xml:"channel"`
		Programmes []struct {
			Start      string   `xml:"start,attr"`
			Stop       *string  `xml:"stop,attr"`
			Channel    string   `xml:"channel,attr"`
			Title      string   `xml:"title"`
			Desc       *string  `xml:"desc"`
			Categories []string `xml:"category"`
		} `xml:"programme"`
	}
	require.NoError(t, xml.Unmarshal(body, &doc))
	require.Len(t, doc.Channels, 2)
	assert.Equal(t, "3273601024", doc.Channels[0].ID)
	require.NotNil(t, doc.Channels[0].Icon)
	assert.Equal(t, base+"/services/3273601024/logo", doc.Channels[0].Icon.Src)
	assert.Nil(t, doc.Channels[1].Icon)
	require.Len(t, doc.Programmes, 2)
	first := doc.Programmes[0]
	assert.Equal(t, "20261107030000 +0000", first.Start)
	require.NotNil(t, first.Stop)
	assert.Equal(t, "20261107033000 +0000", *first.Stop)
	assert.Equal(t, "400141", first.Channel)
	assert.Equal(t, "ニュース & 天気 <速報>", first.Title)
	require.NotNil(t, first.Desc)
	// 大分類だけを、重ねずに書く。分からない大分類は書かない。
	assert.Equal(t, []string{"ニュース／報道", "アニメ／特撮"}, first.Categories)
	// 終了時刻が未定の番組は、終わりを書かない。
	assert.Nil(t, doc.Programmes[1].Stop)
	assert.Nil(t, doc.Programmes[1].Desc)
}

// TestHandler は、パスの秘密の値で認証し、違えば 404 を返し、URL を途中のキャッシュに残させないことを確かめる (#1324)。
func TestHandler(t *testing.T) {
	t.Parallel()

	// enabledGate は、管理者が IPTV のクライアント用の URL を有効にしている状態。
	enabledGate := fakeGate{enabled: true, err: nil}
	tests := []struct {
		name        string
		gate        fakeGate
		path        string
		wantStatus  int
		contentType string
		contains    string
	}{
		{name: "playlist", gate: enabledGate, path: "/api/iptv/" + validFeed + "/channels.m3u", wantStatus: http.StatusOK, contentType: "audio/x-mpegurl; charset=utf-8", contains: base + "/services/400141/stream.m2ts"},
		{name: "guide", gate: enabledGate, path: "/api/iptv/" + validFeed + "/guide.xml", wantStatus: http.StatusOK, contentType: "application/xml; charset=utf-8", contains: `channel="400141"`},
		{name: "wrong value", gate: enabledGate, path: "/api/iptv/kotatsu_iptv_wrong/channels.m3u", wantStatus: http.StatusNotFound},
		{name: "user who cannot watch", gate: enabledGate, path: "/api/iptv/" + unwatchableFeed + "/guide.xml", wantStatus: http.StatusNotFound},
		{name: "authentication failure", gate: enabledGate, path: "/api/iptv/broken/guide.xml", wantStatus: http.StatusInternalServerError},
		// 管理者が無効にしている間は、正しい秘密の値でも、URL があることを知らせないよう 404 を返す (#1755)。
		{name: "disabled by the administrator", gate: fakeGate{enabled: false, err: nil}, path: "/api/iptv/" + validFeed + "/channels.m3u", wantStatus: http.StatusNotFound},
		{name: "setting unreadable", gate: fakeGate{enabled: false, err: errors.New("database is down")}, path: "/api/iptv/" + validFeed + "/channels.m3u", wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queries := new(fakeQueries)
			h := iptvapi.NewHandler(fakeAuth{}, tt.gate, queries, clocktest.NewFake(testNow()), func(*http.Request) string { return "https://kotatsu.test" })
			router := httpserver.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler())
			router.GET(iptvapi.PlaylistRoute, h.Playlist, h.Authenticate)
			router.GET(iptvapi.GuideRoute, h.Guide, h.Authenticate)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+tt.path, nil)
			require.NoError(t, err)
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Equal(t, tt.contentType, resp.Header.Get("Content-Type"))
			assert.Equal(t, "private, no-store", resp.Header.Get("Cache-Control"))
			assert.Contains(t, string(body), tt.contains)
			if tt.name == "guide" {
				// 放送中の番組と、これから 7 日の番組を引く。
				assert.Equal(t, testNow().Add(-time.Hour), queries.from)
				assert.Equal(t, testNow().Add(7*24*time.Hour), queries.to)
			}
		})
	}
}

// switchGate は、管理者が IPTV のクライアント用の URL を有効にしているかを、テストの途中で変えられる。
type switchGate struct {
	enabled atomic.Bool
}

func (g *switchGate) IPTVFeedsEnabled(context.Context) (bool, error) {
	return g.enabled.Load(), nil
}

// TestStopWhenRevoked は、IPTV のストリームを流している間に、管理者が無効にしたら、流しているストリームも止め、
// 使えるうちは止めないことを確かめる (#1755)。
func TestStopWhenRevoked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		disable  bool
		wantStop bool
	}{
		{name: "管理者が無効にした", disable: true, wantStop: true},
		{name: "使えるうちは流し続ける", disable: false, wantStop: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gate := new(switchGate)
			gate.enabled.Store(true)
			h := iptvapi.NewHandler(fakeAuth{}, gate, new(fakeQueries), clocktest.NewFake(testNow()), func(*http.Request) string { return "https://kotatsu.test" })
			h.SetRecheckInterval(10 * time.Millisecond)
			streaming := make(chan struct{})
			stopped := make(chan struct{})
			flow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				flusher, _ := w.(http.Flusher)
				once := false
				for {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(5 * time.Millisecond):
					}
					if _, err := io.WriteString(w, "ts"); err != nil {
						return
					}
					flusher.Flush()
					if !once {
						once = true
						close(streaming)
					}
				}
			})
			mux := http.NewServeMux()
			mux.Handle("GET /api/iptv/{token}/services/{id}/stream.m2ts", h.StopWhenRevoked(flow))
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/iptv/"+validFeed+"/services/400141/stream.m2ts", nil)
			require.NoError(t, err)
			// 流し続けるので、要求して読み捨てるのは別の goroutine にする。
			client := server.Client()
			go func() {
				resp, err := client.Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()
				_, _ = io.Copy(io.Discard, resp.Body)
			}()

			<-streaming
			if tt.disable {
				gate.enabled.Store(false)
			}
			select {
			case <-stopped:
				assert.True(t, tt.wantStop, "the stream stopped while it was still allowed")
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantStop, "the stream kept flowing after IPTV feeds were disabled")
			}
		})
	}
}
