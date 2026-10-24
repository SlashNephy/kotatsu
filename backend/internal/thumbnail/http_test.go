package thumbnail_test

import (
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

const (
	// imagePattern は、録画の画像のパス。
	imagePattern = "GET /media/recordings/{id}/{file}"
)

// addSegment は、録画に、書いたもののあるセグメントの行を足し、そのファイルの ID を返す。画像の配信は、セグメントを読まない。
func (f *jobFixture) addSegment(t *testing.T, recordingID uuid.UUID) uuid.UUID {
	t.Helper()

	segmentID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 1, $5, $5, $5)`, segmentID, recordingID, poolID, segmentID.String()+".m2ts", f.clock.Now())
	require.NoError(t, err)

	return segmentID
}

// addImages は、録画にセグメントを足し、そのデータディレクトリのディレクトリに、代表画像とプレビュー画像とその説明を置く。
func (f *jobFixture) addImages(t *testing.T, recordingID uuid.UUID) {
	t.Helper()

	segmentID := f.addSegment(t, recordingID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dataRoot, segmentID.String()), 0o700))
	for name, body := range map[string]string{
		thumbnail.ThumbnailFileName: "thumbnail-jpeg", thumbnail.PreviewFileName: "preview-jpeg", thumbnail.ManifestFileName: `{"columns":10}`,
		kago.IndexFileName: "index",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(f.dataRoot, kago.DataLocation(segmentID, name).Path), []byte(body), 0o600))
	}
}

// response は、画像の要求に対する応答の、確かめる部分。
type response struct {
	status int
	header http.Header
	body   string
}

func serveImage(t *testing.T, store *kago.Store, f *jobFixture, path string, header http.Header) response {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(imagePattern, thumbnail.NewImageHandler(f.pool, store, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
	require.NoError(t, err)
	maps.Copy(req.Header, header)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	return response{status: res.StatusCode, header: res.Header, body: string(body)}
}

// bareRecording は、ファイルのない録画を作る。
func (f *jobFixture) bareRecording(t *testing.T) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, 'completed', false, $3, $3, $3)`, recordingID, serviceID, f.clock.Now())
	require.NoError(t, err)

	return recordingID
}

func TestImageHandler(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	withImages := f.bareRecording(t)
	f.addImages(t, withImages)
	withoutImages := f.bareRecording(t)
	// missingFile は、セグメントはあるが、画像をまだ作っていない録画。
	missingFile := f.bareRecording(t)
	f.addSegment(t, missingFile)

	tests := []struct {
		name            string
		store           *kago.Store
		path            string
		wantStatus      int
		wantBody        string
		wantContentType string
	}{
		{
			name: "thumbnail", store: f.store, path: "/media/recordings/" + withImages.String() + "/thumbnail.jpg",
			wantStatus: http.StatusOK, wantBody: "thumbnail-jpeg", wantContentType: "image/jpeg",
		},
		{
			name: "preview image", store: f.store, path: "/media/recordings/" + withImages.String() + "/preview.jpg",
			wantStatus: http.StatusOK, wantBody: "preview-jpeg", wantContentType: "image/jpeg",
		},
		{
			name: "preview manifest", store: f.store,
			path:       "/media/recordings/" + withImages.String() + "/preview.json",
			wantStatus: http.StatusOK, wantBody: `{"columns":10}`, wantContentType: "application/json",
		},
		{
			name: "other file names in the segment directory are not served", store: f.store,
			path: "/media/recordings/" + withImages.String() + "/index", wantStatus: http.StatusNotFound,
		},
		{
			name: "recording without segments", store: f.store,
			path: "/media/recordings/" + withoutImages.String() + "/thumbnail.jpg", wantStatus: http.StatusNotFound,
		},
		{
			name: "images not generated yet", store: f.store,
			path: "/media/recordings/" + missingFile.String() + "/thumbnail.jpg", wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown recording", store: f.store,
			path: "/media/recordings/" + id.New().String() + "/thumbnail.jpg", wantStatus: http.StatusNotFound,
		},
		{
			name: "invalid recording ID", store: f.store,
			path: "/media/recordings/not-a-uuid/thumbnail.jpg", wantStatus: http.StatusBadRequest,
		},
		{
			name: "data directory not mounted on this instance", store: kago.New(map[string]string{}, nil),
			path:       "/media/recordings/" + withImages.String() + "/thumbnail.jpg",
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := serveImage(t, tt.store, f, tt.path, nil)
			assert.Equal(t, tt.wantStatus, res.status)
			if tt.wantStatus != http.StatusOK {
				// 「ない」という応答を、ブラウザにも前段のプロキシにも覚えさせない (#426)。ファイルを戻したら、すぐに出るようにする。
				assert.Equal(t, "no-store", res.header.Get("Cache-Control"))

				return
			}
			assert.Equal(t, tt.wantBody, res.body)
			assert.Equal(t, tt.wantContentType, res.header.Get("Content-Type"))
			// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
			assert.Equal(t, "private, max-age=3600", res.header.Get("Cache-Control"))
		})
	}
}

// TestImageHandlerNotModified は、ブラウザが持っている画像が変わっていなければ、本文を返さないことを確かめる。
func TestImageHandlerNotModified(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	recordingID := f.bareRecording(t)
	f.addImages(t, recordingID)
	path := "/media/recordings/" + recordingID.String() + "/thumbnail.jpg"

	first := serveImage(t, f.store, f, path, nil)
	require.Equal(t, http.StatusOK, first.status)
	lastModified := first.header.Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	again := serveImage(t, f.store, f, path, http.Header{"If-Modified-Since": {lastModified}})
	assert.Equal(t, http.StatusNotModified, again.status)
}
