package faketuner_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

func TestLog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)

	get := func(path string) string {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return string(body)
	}
	get("/api/version")
	get("/api/log")
	get("/api/tuners")

	lines := strings.Split(strings.TrimSuffix(get("/api/log"), "\n"), "\n")
	// /api/log 自体は残さない。Mirakurun と同じく「時刻 重さ: 本文」の形で、古い順に返す。
	require.Len(t, lines, 2)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z info: 127\.0\.0\.1:\d+ - GET /api/version$`, lines[0])
	assert.Contains(t, lines[1], "GET /api/tuners")
}
