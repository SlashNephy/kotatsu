package jikkyoapi_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
	mux.HandleFunc("GET /api/v1/channels", func(w http.ResponseWriter, _ *http.Request) {
		f.channelRequests.Add(1)
		if hold := f.hold.Load(); hold != nil {
			<-*hold
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

// jkNHK は、テストで使う実況のチャンネル。
const jkNHK = "jk1"

const channelsJSON = `[{"id":"jk1","name":"NHK総合","threads":[]},{"id":"jk101","name":"NHK BS","threads":[]}]`
