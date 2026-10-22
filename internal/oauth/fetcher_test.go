package oauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/oauth"
	"github.com/SlashNephy/kotatsu/internal/publicnet"
)

const (
	metadataPath      = "/client.json"
	exampleCallback   = "https://example.com/cb"
	claudeCodeName    = "Claude Code"
	loopbackCallback  = "http://localhost/callback"
	privateKeyJWTAuth = "private_key_jwt"

	paramClientID          = "client_id"
	paramCode              = "code"
	paramRedirectURI       = "redirect_uri"
	paramResource          = "resource"
	grantAuthorizationCode = "authorization_code"
	grantRefreshToken      = "refresh_token"
	authNone               = "none"
)

// metadataServer は、path でクライアントのメタデータを返す https のテストのサーバーと、受けたリクエストの数を返す。
// document は、client_id を受け取って、返す文書を作る。
func metadataServer(t *testing.T, cacheControl string, document func(clientID string) any) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("GET "+metadataPath, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if cacheControl != "" {
			w.Header().Set("Cache-Control", cacheControl)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document(srv.URL + metadataPath))
	})
	mux.HandleFunc("GET /redirect/{n}", func(w http.ResponseWriter, r *http.Request) {
		switch n := r.PathValue("n"); n {
		case "0":
			http.Redirect(w, r, metadataPath, http.StatusFound)
		default:
			http.Redirect(w, r, "/redirect/"+string(rune(n[0]-1)), http.StatusFound)
		}
	})
	mux.HandleFunc("GET /http", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+srv.Listener.Addr().String()+metadataPath, http.StatusFound)
	})
	mux.HandleFunc("GET /large", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"client_name":"` + strings.Repeat("a", 6<<10) + `"}`))
	})
	mux.HandleFunc("GET /missing", http.NotFound)
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	return srv, &requests
}

// claudeCodeDocument は、Claude Code が公開しているものと同じ形のメタデータを返す。
func claudeCodeDocument(clientID string) any {
	return map[string]any{
		paramClientID:                clientID,
		"client_name":                claudeCodeName,
		"client_uri":                 "https://claude.ai",
		"redirect_uris":              []string{loopbackCallback, "http://127.0.0.1/callback"},
		"grant_types":                []string{grantAuthorizationCode, grantRefreshToken},
		"response_types":             []string{paramCode},
		"token_endpoint_auth_method": authNone,
	}
}

// TestFetcherFetch は、クライアントのメタデータを確かめ、Kotatsu で使えないものを断ることを確かめる。
func TestFetcherFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document func(clientID string) any
		wantErr  error
	}{
		{name: "Claude Code", document: claudeCodeDocument, wantErr: nil},
		{
			// ChatGPT が公開しているものと同じ形。
			name: "ChatGPT prefers private_key_jwt but supports none",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc["redirect_uris"] = []string{exampleCallback}
				doc["token_endpoint_auth_methods_supported"] = []string{authNone, privateKeyJWTAuth}
				doc["token_endpoint_auth_method"] = privateKeyJWTAuth

				return doc
			},
			wantErr: nil,
		},
		{
			name: "no auth method means a public client",
			document: func(clientID string) any {
				return map[string]any{paramClientID: clientID, "client_name": "App", "redirect_uris": []string{exampleCallback}}
			},
			wantErr: nil,
		},
		{
			name: "client_id mismatch",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc[paramClientID] = clientID + "?"

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "client_secret",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc["client_secret"] = "secret"

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "private_key_jwt only",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc["token_endpoint_auth_method"] = privateKeyJWTAuth

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "no client_name",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				delete(doc, "client_name")

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "no redirect_uris",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				delete(doc, "redirect_uris")

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "non-loopback http redirect URI",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc["redirect_uris"] = []string{"http://example.com/callback"}

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
		{
			name: "implicit grant only",
			document: func(clientID string) any {
				doc, _ := claudeCodeDocument(clientID).(map[string]any)
				doc["grant_types"] = []string{"implicit"}

				return doc
			},
			wantErr: oauth.ErrInvalidClientMetadata,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, _ := metadataServer(t, "", tt.document)
			fetcher := oauth.NewFetcherWithTransport(clocktest.NewFake(time.Now()), srv.Client().Transport)

			metadata, err := fetcher.Fetch(t.Context(), srv.URL+metadataPath)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, srv.URL+metadataPath, metadata.ClientID)
			assert.NotEmpty(t, metadata.RedirectURIs)
		})
	}
}

// TestFetcherFetchFailures は、取ってこられないときと、文書が大きすぎるときに断ることを確かめる。
func TestFetcherFetchFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{name: "not found", path: "/missing", wantErr: oauth.ErrClientMetadataUnavailable},
		{name: "too large", path: "/large", wantErr: oauth.ErrInvalidClientMetadata},
		{name: "redirect to http", path: "/http", wantErr: oauth.ErrClientMetadataUnavailable},
		{name: "three redirects", path: "/redirect/2", wantErr: nil},
		{name: "four redirects", path: "/redirect/3", wantErr: oauth.ErrClientMetadataUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// リダイレクトの先の文書は、リダイレクトの元の URL を client_id にする。
			var srvURL string
			srv, _ := metadataServer(t, "", func(string) any { return claudeCodeDocument(srvURL + tt.path) })
			srvURL = srv.URL
			fetcher := oauth.NewFetcherWithTransport(clocktest.NewFake(time.Now()), srv.Client().Transport)

			_, err := fetcher.Fetch(t.Context(), srv.URL+tt.path)
			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestFetcherCache は、取ってきたメタデータを、Cache-Control に従って使い回すことを確かめる。
func TestFetcherCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cacheControl string
		advance      time.Duration
		wantRequests int32
	}{
		{name: "default freshness", cacheControl: "", advance: 14 * time.Minute, wantRequests: 1},
		{name: "default freshness expired", cacheControl: "", advance: 15 * time.Minute, wantRequests: 2},
		{name: "max-age", cacheControl: "public, max-age=3600", advance: 59 * time.Minute, wantRequests: 1},
		{name: "max-age expired", cacheControl: "public, max-age=3600", advance: time.Hour, wantRequests: 2},
		{name: "no-store still reused briefly", cacheControl: "no-store", advance: 4 * time.Minute, wantRequests: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, requests := metadataServer(t, tt.cacheControl, claudeCodeDocument)
			clk := clocktest.NewFake(time.Now())
			fetcher := oauth.NewFetcherWithTransport(clk, srv.Client().Transport)

			_, err := fetcher.Fetch(t.Context(), srv.URL+metadataPath)
			require.NoError(t, err)
			clk.Advance(tt.advance)
			_, err = fetcher.Fetch(t.Context(), srv.URL+metadataPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRequests, requests.Load())
		})
	}
}

// TestFetcherDoesNotCacheErrors は、正しくない文書を覚えず、次の認可で取り直すことを確かめる。
func TestFetcherDoesNotCacheErrors(t *testing.T) {
	t.Parallel()

	var valid atomic.Bool
	srv, requests := metadataServer(t, "max-age=3600", func(clientID string) any {
		doc, _ := claudeCodeDocument(clientID).(map[string]any)
		if !valid.Load() {
			delete(doc, "redirect_uris")
		}

		return doc
	})
	fetcher := oauth.NewFetcherWithTransport(clocktest.NewFake(time.Now()), srv.Client().Transport)

	_, err := fetcher.Fetch(t.Context(), srv.URL+metadataPath)
	require.ErrorIs(t, err, oauth.ErrInvalidClientMetadata)
	valid.Store(true)
	_, err = fetcher.Fetch(t.Context(), srv.URL+metadataPath)
	require.NoError(t, err)
	assert.Equal(t, int32(2), requests.Load())
}

// TestFetcherFetchesOnceConcurrently は、同じクライアントの認可が重なっても、取りに行くのは 1 回であることを確かめる。
func TestFetcherFetchesOnceConcurrently(t *testing.T) {
	t.Parallel()

	const callers = 8
	release := make(chan struct{})
	var srvURL string
	var requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		<-release
		_ = json.NewEncoder(w).Encode(claudeCodeDocument(srvURL + metadataPath))
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	fetcher := oauth.NewFetcherWithTransport(clocktest.NewFake(time.Now()), srv.Client().Transport)

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			_, err := fetcher.Fetch(t.Context(), srv.URL+metadataPath)
			errs <- err
		})
	}
	// 最初のリクエストが届いてから、ほかの呼び出しが待ちに入るまで少し待ち、まとめて返す。
	require.Eventually(t, func() bool { return requests.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), requests.Load())
}

// TestFetcherRejectsNonPublicAddress は、LAN の中のサーバー (ここではループバック) には接続しないことを確かめる。
func TestFetcherRejectsNonPublicAddress(t *testing.T) {
	t.Parallel()

	srv, requests := metadataServer(t, "", claudeCodeDocument)
	fetcher := oauth.NewFetcher(clocktest.NewFake(time.Now()))

	_, err := fetcher.Fetch(t.Context(), srv.URL+metadataPath)
	require.ErrorIs(t, err, oauth.ErrClientMetadataUnavailable)
	require.ErrorIs(t, err, publicnet.ErrNonPublicAddress)
	assert.Zero(t, requests.Load())
}

// TestFreshness は、Cache-Control から、使い回す長さを 5 分から 24 時間の範囲で決めることを確かめる。
func TestFreshness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cacheControl string
		want         time.Duration
	}{
		{cacheControl: "", want: 15 * time.Minute},
		{cacheControl: "max-age=1800", want: 30 * time.Minute},
		{cacheControl: "public, MAX-AGE=7200", want: 2 * time.Hour},
		{cacheControl: "max-age=10", want: 5 * time.Minute},
		{cacheControl: "max-age=604800", want: 24 * time.Hour},
		{cacheControl: "max-age=invalid", want: 15 * time.Minute},
		{cacheControl: "no-cache", want: 5 * time.Minute},
		{cacheControl: "max-age = 3600", want: time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.cacheControl, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, oauth.Freshness(tt.cacheControl))
		})
	}
}
