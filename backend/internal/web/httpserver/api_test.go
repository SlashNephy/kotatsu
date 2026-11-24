package httpserver_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

const (
	healthzPath = "/healthz"
	healthzName = "healthz"
	livePrefix  = "/internal/kyusu/live/"
	guidePath   = "/guide"
)

// TestAPIRouter は、API の待ち受けの一番外側のルーターが、tenban、kyusu、ヘルスチェックを載せた http.ServeMux と同じく
// 振り分けることを確認する (#2016)。tenban のいるときといないときで、ヘルスチェックへの別のメソッドの行き先が変わる。
func TestAPIRouter(t *testing.T) {
	t.Parallel()

	named := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set(gotHeader, name) })
	}
	health := map[string]http.Handler{healthzPath: named(healthzName)}
	newMux := func(withApp bool) http.Handler {
		mux := http.NewServeMux()
		mux.Handle("GET /healthz", health[healthzPath])
		mux.Handle(livePrefix, named("kyusu"))
		if withApp {
			mux.Handle("/", named("app"))
		}

		return mux
	}
	newEcho := func(withApp bool) http.Handler {
		var app http.Handler
		if withApp {
			app = named("app")
		}

		return httpserver.NewAPIRouter(slog.New(slog.DiscardHandler), health, map[string]http.Handler{livePrefix: named("kyusu")}, app)
	}

	tests := []struct {
		name         string
		withApp      bool
		method       string
		target       string
		wantStatus   int
		wantGot      string
		wantAllow    string
		wantLocation string
	}{
		{name: "ヘルスチェック", withApp: true, method: http.MethodGet, target: healthzPath, wantStatus: http.StatusOK, wantGot: healthzName},
		{name: "HEAD のヘルスチェック", withApp: true, method: http.MethodHead, target: healthzPath, wantStatus: http.StatusOK, wantGot: healthzName},
		{name: "tenban がいれば、ヘルスチェックへの POST は tenban に渡す", withApp: true, method: http.MethodPost, target: healthzPath, wantStatus: http.StatusOK, wantGot: "app"},
		{name: "tenban がいなければ、ヘルスチェックへの POST は 405", withApp: false, method: http.MethodPost, target: healthzPath, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD"},
		{name: "kyusu の下はどのメソッドも kyusu に渡す", withApp: true, method: http.MethodPost, target: "/internal/kyusu/live/1/a", wantStatus: http.StatusOK, wantGot: "kyusu"},
		{name: "kyusu の頭そのもの", withApp: false, method: http.MethodGet, target: livePrefix, wantStatus: http.StatusOK, wantGot: "kyusu"},
		{name: "kyusu の頭のスラッシュのないパスはリダイレクトする", withApp: true, method: http.MethodGet, target: "/internal/kyusu/live", wantStatus: http.StatusTemporaryRedirect, wantLocation: livePrefix},
		{name: "崩れたパスは整えた先へリダイレクトする", withApp: true, method: http.MethodGet, target: "//internal/kyusu/live/1", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/internal/kyusu/live/1"},
		{name: "ほかのパスは tenban に渡す", withApp: true, method: http.MethodGet, target: guidePath, wantStatus: http.StatusOK, wantGot: "app"},
		{name: "tenban がいなければ、ほかのパスは 404", withApp: false, method: http.MethodGet, target: guidePath, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		for name, handler := range map[string]http.Handler{echoName: newEcho(tt.withApp), muxName: newMux(tt.withApp)} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, "http://example.com"+tt.target, nil))
			assert.Equal(t, tt.wantStatus, rec.Code, "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantGot, rec.Header().Get(gotHeader), "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantAllow, rec.Header().Get("Allow"), "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"), "%s: %s", name, tt.name)
		}
	}
}
