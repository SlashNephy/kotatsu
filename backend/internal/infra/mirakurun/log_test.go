package mirakurun_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

func TestClientLog(t *testing.T) {
	t.Parallel()

	// 1 行 100 バイトで 3 万行 (約 3MB)。上限 (2MB) を超えるので、先頭が捨てられる。
	var long strings.Builder
	for i := range 30000 {
		fmt.Fprintf(&long, "2026-11-12T00:00:00.000+09:00 info: line %05d %s\n", i, strings.Repeat("x", 59))
	}
	tests := []struct {
		name       string
		status     int
		body       string
		wantFirst  string
		wantLast   string
		wantStatus int
	}{
		{
			name: "本文をそのまま返す", status: http.StatusOK, body: "a\nb\n",
			wantFirst: "a", wantLast: "b", wantStatus: 0,
		},
		// 半端に切った行は捨てるので、最初の行も頭から始まる。
		{
			name: "大きい本文は最後の部分だけを、行の頭から返す", status: http.StatusOK, body: long.String(),
			wantFirst: "2026-11-12T00:00:00.000+09:00 info: line ", wantLast: "line 29999 ", wantStatus: 0,
		},
		{
			name: "持たないサーバーは状態を返す", status: http.StatusNotFound, body: "not found",
			wantFirst: "", wantLast: "", wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/log", r.URL.Path)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			body, err := client.Log(t.Context())
			if tt.wantStatus != 0 {
				require.Error(t, err)
				assert.Equal(t, tt.wantStatus, mirakurun.HTTPStatus(err))

				return
			}
			require.NoError(t, err)
			assert.LessOrEqual(t, len(body), 2<<20)
			lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
			assert.True(t, strings.HasPrefix(lines[0], tt.wantFirst), lines[0])
			assert.Contains(t, lines[len(lines)-1], tt.wantLast)
		})
	}
}

func TestClientStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"4.1.5","process":{"arch":"x64","platform":"linux","pid":1}}`))
	}))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	status, err := client.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, &mirakurun.Status{Version: "4.1.5", Process: mirakurun.StatusProcess{Arch: "x64", Platform: "linux"}}, status)
}
