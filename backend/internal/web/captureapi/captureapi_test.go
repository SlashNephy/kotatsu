package captureapi_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
	"github.com/SlashNephy/kotatsu/internal/web/captureapi"
)

const (
	// maxImageBytes は、テストで受け付ける画像の大きさの上限。
	maxImageBytes = 64
	// pngImage は、PNG の先頭の 8 バイトを持つ、テストの画像。
	pngImage = "\x89PNG\r\n\x1a\nimage"
	alice    = "alice"
	bob      = "bob"
)

type fixture struct {
	server *httptest.Server
	// tokens は、名前ごとの、ログインしたセッションのトークン。
	tokens map[string]string
}

// newFixture は、alice と bob のセッションと、キャプチャを受け取って返す API を用意する。dataDir が空なら、データディレクトリを設定しない。
func newFixture(t *testing.T, dataDir string) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	sessions := auth.NewSessions(&auth.SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: clk})
	tokens := map[string]string{}
	for _, name := range []string{alice, bob} {
		// 視聴者も、自分のキャプチャを残せる。
		user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
			Username: name, DisplayName: name, Role: auth.RoleViewer, PasswordHash: nil,
		})
		require.NoError(t, err)
		token, _, err := sessions.Create(t.Context(), pool, user.ID, nil)
		require.NoError(t, err)
		tokens[name] = token
	}
	logger := slog.New(slog.DiscardHandler)
	store := kago.New(nil, nil).WithDataDir(dataDir)
	mux := http.NewServeMux()
	mux.Handle("POST /media/captures", captureapi.NewUploadHandler(pool, capture.New(store, clk, logger, maxImageBytes), logger))
	mux.Handle("GET /media/captures/{id}", captureapi.NewImageHandler(pool, store, logger))
	server := httptest.NewServer(sessions.Middleware(pool)(mux))
	t.Cleanup(server.Close)

	return &fixture{server: server, tokens: tokens}
}

// part は、multipart の 1 つの部分。
type part struct {
	name string
	data []byte
}

func metadataPart(t *testing.T, metadata map[string]any) part {
	t.Helper()

	data, err := json.Marshal(metadata)
	require.NoError(t, err)

	return part{name: captureapi.MetadataPart, data: data}
}

func validMetadata() map[string]any {
	return map[string]any{
		"serviceName": "こたつテスト1", "programName": "ニュース", "caption": nil, "recordingId": nil, "position": nil,
		"broadcastAt": "2026-10-10T11:59:00Z", "takenAt": "2026-10-10T12:00:00Z",
	}
}

// upload は、user として parts を送り、応答の状態と本文を返す。user が空なら、ログインせずに送る。
func (f *fixture) upload(t *testing.T, user string, parts ...part) (int, []byte) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, p := range parts {
		w, err := writer.CreateFormFile(p.name, p.name)
		require.NoError(t, err)
		_, err = w.Write(p.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/media/captures", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return f.do(t, req, user)
}

// image は、user としてキャプチャ captureID の画像を取り、応答の状態と本文を返す。
func (f *fixture) image(t *testing.T, user, captureID string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/media/captures/"+captureID, http.NoBody)
	require.NoError(t, err)

	return f.do(t, req, user)
}

func (f *fixture) do(t *testing.T, req *http.Request, user string) (int, []byte) {
	t.Helper()

	if user != "" {
		req.Header.Set("Cookie", auth.CookieName+"="+f.tokens[user])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, data
}

// rawCaptureID は、GraphQL のキャプチャの ID から、画像の URL に使う ID を取り出す。
func rawCaptureID(t *testing.T, globalID string) string {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(globalID)
	require.NoError(t, err)
	typeName, raw, ok := bytes.Cut(decoded, []byte(":"))
	require.True(t, ok)
	require.Equal(t, "Capture", string(typeName))

	return string(raw)
}

// TestUploadAndServe は、受け取ったキャプチャの画像を、撮ったユーザーにだけ返すことを確かめる。
func TestUploadAndServe(t *testing.T) {
	t.Parallel()

	f := newFixture(t, t.TempDir())
	status, body := f.upload(t, alice, metadataPart(t, validMetadata()), part{name: captureapi.ImagePart, data: []byte(pngImage)})
	require.Equal(t, http.StatusCreated, status, string(body))
	var created captureapi.UploadResponse
	require.NoError(t, json.Unmarshal(body, &created))
	captureID := rawCaptureID(t, created.ID)

	tests := []struct {
		name       string
		user       string
		wantStatus int
	}{
		{name: "撮ったユーザー", user: alice, wantStatus: http.StatusOK},
		{name: "ほかのユーザーには、ないものとして答える", user: bob, wantStatus: http.StatusNotFound},
		{name: "ログインしていない", user: "", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		status, data := f.image(t, tt.user, captureID)
		assert.Equal(t, tt.wantStatus, status, tt.name)
		if tt.wantStatus == http.StatusOK {
			assert.Equal(t, []byte(pngImage), data, tt.name)
		}
	}
}

// TestUploadRejects は、足りない、または正しくないリクエストを断ることを確かめる。
func TestUploadRejects(t *testing.T) {
	t.Parallel()

	f := newFixture(t, t.TempDir())
	metadata := metadataPart(t, validMetadata())
	withRecording := validMetadata()
	withRecording["recordingId"] = "not-a-recording-id"
	image := part{name: captureapi.ImagePart, data: []byte(pngImage)}
	tests := []struct {
		name       string
		user       string
		parts      []part
		wantStatus int
	}{
		{name: "ログインしていない", user: "", parts: []part{metadata, image}, wantStatus: http.StatusUnauthorized},
		{name: "撮った場面の情報がない", user: alice, parts: []part{image}, wantStatus: http.StatusBadRequest},
		{name: "画像がない", user: alice, parts: []part{metadata}, wantStatus: http.StatusBadRequest},
		{
			name: "PNG でない画像", user: alice,
			parts:      []part{metadata, {name: captureapi.ImagePart, data: []byte("GIF89a....")}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "大きすぎる画像", user: alice,
			parts:      []part{metadata, {name: captureapi.ImagePart, data: append([]byte(pngImage), make([]byte, maxImageBytes)...)}},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "JSON でない情報", user: alice,
			parts:      []part{{name: captureapi.MetadataPart, data: []byte("{")}, image},
			wantStatus: http.StatusBadRequest,
		},
		{name: "録画の ID でない recordingId", user: alice, parts: []part{metadataPart(t, withRecording), image}, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := f.upload(t, tt.user, tt.parts...)
			assert.Equal(t, tt.wantStatus, status, string(body))
		})
	}
}

// TestUploadWithoutDataDir は、データディレクトリを設定していなければ、503 で断ることを確かめる。
func TestUploadWithoutDataDir(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "")
	status, body := f.upload(t, alice, metadataPart(t, validMetadata()), part{name: captureapi.ImagePart, data: []byte(pngImage)})
	assert.Equal(t, http.StatusServiceUnavailable, status, string(body))
}
