package update_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/update"
)

// runningVersion は、テストで動いている版。
const runningVersion = "1.2.3"

const releaseURL = "https://github.com/SlashNephy/kotatsu/releases/tag/v1.3.0"

// TestCheck は、動いている版より新しいリリースだけを覚え、確かめられないときはエラーを返すことを確かめる。
func TestCheck(t *testing.T) {
	t.Parallel()

	published := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		current string
		status  int
		body    string
		want    *update.Release
		wantErr bool
	}{
		{
			name: "新しいリリースがある", current: runningVersion, status: http.StatusOK,
			body: `{"tag_name": "v1.3.0", "html_url": "` + releaseURL + `", "published_at": "2026-10-17T00:00:00Z"}`,
			want: &update.Release{Version: "1.3.0", URL: releaseURL, PublishedAt: published},
		},
		{
			name: "同じ版", current: "1.3.0", status: http.StatusOK,
			body: `{"tag_name": "v1.3.0", "html_url": "` + releaseURL + `", "published_at": "2026-10-17T00:00:00Z"}`,
			want: nil,
		},
		{
			name: "動いている版のほうが新しい", current: "v1.4.0", status: http.StatusOK,
			body: `{"tag_name": "v1.3.0", "html_url": "` + releaseURL + `", "published_at": "2026-10-17T00:00:00Z"}`,
			want: nil,
		},
		{
			name: "GitHub のページでない URL は使わない", current: runningVersion, status: http.StatusOK,
			body: `{"tag_name": "v1.3.0", "html_url": "https://example.com/", "published_at": "2026-10-17T00:00:00Z"}`,
			want: nil,
		},
		{name: "版でないタグ", current: runningVersion, status: http.StatusOK, body: `{"tag_name": "nightly"}`, want: nil, wantErr: true},
		{name: "リリースがなければ、新しい版もない", current: runningVersion, status: http.StatusNotFound, body: `{}`, want: nil, wantErr: false},
		{name: "API が失敗した", current: runningVersion, status: http.StatusInternalServerError, body: `{}`, want: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// GitHub の API は、User-Agent のない問い合わせを断る。
				if !strings.HasPrefix(r.Header.Get("User-Agent"), "Kotatsu") {
					w.WriteHeader(http.StatusForbidden)

					return
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			checker := update.NewChecker(&update.Options{
				Current: tt.current, URL: server.URL, Interval: 0, HTTPClient: server.Client(), Logger: slog.New(slog.DiscardHandler),
			})

			err := checker.Check(t.Context())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, checker.Latest())
		})
	}
}

// TestComparable は、リリースの版だけを比べ、main のビルド (edge) と版を埋め込まないビルド (dev) は比べないことを確かめる。
func TestComparable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    bool
	}{
		{version: runningVersion, want: true},
		{version: "v1.2.3", want: true},
		{version: "edge", want: false},
		{version: "dev", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, update.Comparable(tt.version))
		})
	}
}
