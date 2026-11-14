package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestTenbanServesStreams は、tenban が、映像の配信 (/internal/streams/*) を echo のルーターで受け (#1606)、
// mux のときと同じく、セッションの Cookie で認可し、パスの名前付きの部分をハンドラーに渡すことを確かめる。
// この tenban には、チューナーサーバーも kyusu もないので、届いた配信は、流せずに終わる。
func TestTenbanServesStreams(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "streams", DisplayName: "Streams", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)
	const (
		live      = "/internal/streams/services/3273601024"
		recording = "/internal/streams/recordings/0199d000-0000-7000-8000-000000000999"
		timeshift = "/internal/streams/timeshift/3273601024-1763000000"
	)

	tests := []struct {
		name       string
		method     string
		path       string
		session    string
		wantStatus int
		wantAllow  string
	}{
		{name: "live without session", method: http.MethodGet, path: live, session: "", wantStatus: http.StatusUnauthorized, wantAllow: ""},
		{name: "live", method: http.MethodGet, path: live, session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "live HEAD", method: http.MethodHead, path: live, session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "live with invalid ID", method: http.MethodGet, path: "/internal/streams/services/abc", session: session, wantStatus: http.StatusBadRequest, wantAllow: ""},
		{name: "data broadcast", method: http.MethodGet, path: live + "/data-broadcast", session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "live HLS", method: http.MethodGet, path: live + "/hls/720p/0/index.m3u8", session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "recording", method: http.MethodGet, path: recording, session: session, wantStatus: http.StatusNotFound, wantAllow: ""},
		{name: "recording HLS", method: http.MethodGet, path: recording + "/hls/720p/0/index.m3u8", session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		// :file と :id は、包んだハンドラーが r.PathValue で読む。読めない形なら 400 になる。
		{name: "recording file with invalid file ID", method: http.MethodGet, path: recording + "/files/a.txt", session: session, wantStatus: http.StatusBadRequest, wantAllow: ""},
		{name: "timeshift with invalid ID", method: http.MethodGet, path: "/internal/streams/timeshift/3273601024-x", session: session, wantStatus: http.StatusBadRequest, wantAllow: ""},
		{name: "timeshift HLS", method: http.MethodGet, path: timeshift + "/hls/720p/0/index.m3u8", session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "POST to live", method: http.MethodPost, path: live, session: session, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD"},
		{name: "unknown stream path", method: http.MethodGet, path: "/internal/streams/nope", session: session, wantStatus: http.StatusNotFound, wantAllow: ""},
	}
	client := httpclienttest.New(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, allow, body := doAuthRequest(t, client, tt.method, base+tt.path, tt.session, "")
			assert.Equal(t, tt.wantStatus, status, body)
			assert.Equal(t, tt.wantAllow, allow)
		})
	}
}

// TestTenbanStreamsLive は、echo のルーターで受けたライブ視聴の配信が、チューナーサーバーの TS をそのまま流すことを確かめる (#1606)。
func TestTenbanStreamsLive(t *testing.T) {
	t.Parallel()

	ts := bytes.Repeat(append([]byte{0x47, 0x1F, 0xFF, 0x10}, make([]byte, 184)...), 64)
	tuner := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(time.Now(), ts)))
	t.Cleanup(tuner.Close)
	base, pool := startTenbanWithEnv(t, map[string]string{"KOTATSU_TUNER_URL": tuner.URL})
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "live", DisplayName: "Live", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/internal/streams/services/3273601024", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Cookie", auth.CookieName+"="+session)
	resp, err := httpclienttest.New(t).Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "video/mp2t", resp.Header.Get("Content-Type"))
	// 流れ続ける配信なので、TS のパケットを 8 つ読んだら切る。
	packet := make([]byte, 188*8)
	_, err = io.ReadFull(resp.Body, packet)
	require.NoError(t, err)
	assert.Equal(t, ts[:len(packet)], packet)
}
