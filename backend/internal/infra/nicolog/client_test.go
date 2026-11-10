package nicolog_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
)

func newServer(t *testing.T, handler func(srv *httptest.Server, w http.ResponseWriter, r *http.Request)) *nicolog.Client {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler(srv, w, r) }))
	t.Cleanup(srv.Close)

	return nicolog.NewClient(&nicolog.Options{HTTPClient: srv.Client(), URL: srv.URL})
}

func TestList(t *testing.T) {
	t.Parallel()

	client := newServer(t, func(_ *httptest.Server, w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		assert.Equal(t, "/api/fs/list", r.URL.Path)
		assert.Equal(t, "/nico-live-anime", body["path"])
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[{"name":"作品","is_dir":true},{"name":"a.xml","is_dir":false}]}}`))
	})

	entries, err := client.List(t.Context(), nicolog.RootPath)

	require.NoError(t, err)
	assert.Equal(t, []*nicolog.Entry{{Name: "作品", IsDir: true}, {Name: "a.xml", IsDir: false}}, entries)
}

func TestComments(t *testing.T) {
	t.Parallel()

	client := newServer(t, func(srv *httptest.Server, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"` + srv.URL + `/p/a.xml"}}`))

			return
		}
		_, _ = w.Write([]byte(`<?xml version='1.0' encoding='UTF-8'?><packet>
			<chat vpos="1250" date="1" user_id="a:x" mail="184 ue">わこつ</chat>
			<chat vpos="x" date="1">読めない</chat></packet>`))
	})

	comments, err := client.Comments(t.Context(), "/nico-live-anime/作品/a.xml")

	require.NoError(t, err)
	assert.Equal(t, []*nicolog.Comment{{Time: 12.5, Mail: "184 ue", Content: "わこつ", UserID: "a:x"}}, comments)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler func(srv *httptest.Server, w http.ResponseWriter, r *http.Request)
	}{
		{name: "HTTP のエラー", handler: func(_ *httptest.Server, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }},
		{name: "API のエラー", handler: func(_ *httptest.Server, w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"code":500}`))
		}},
		{name: "ほかのホストの URL", handler: func(_ *httptest.Server, w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"http://example.invalid/a.xml"}}`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newServer(t, tt.handler)
			_, err := client.Comments(t.Context(), "/a.xml")
			require.ErrorIs(t, err, nicolog.ErrUnexpectedResponse)
		})
	}
}

// TestRedirectToAnotherHost は、ほかのホストへのリダイレクトに従わないことを確かめる。
func TestRedirectToAnotherHost(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/internal", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	client := nicolog.NewClient(&nicolog.Options{HTTPClient: nicolog.NewHTTPClient(nil), URL: srv.URL})

	_, err := client.List(t.Context(), nicolog.RootPath)

	require.ErrorIs(t, err, nicolog.ErrUnexpectedResponse)
	assert.False(t, reached.Load())
}
