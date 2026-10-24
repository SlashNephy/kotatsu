package stream_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/stream"
)

// otherFilePattern は、録画のその他のファイルのパス。
const otherFilePattern = "GET /streams/recordings/{id}/files/{file}"

// otherFile は、録画に、TS 以外のファイルを置いて記録する。
func (f *recordingFixture) otherFile(t *testing.T, recordingID uuid.UUID, kind, name, body string) uuid.UUID {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o600))
	fileID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)`, fileID, recordingID, kind, poolID, name, len(body), f.startedAt)
	require.NoError(t, err)

	return fileID
}

// TestOtherFileHandler は、取り込んだ録画の TS 以外のファイルを、Range に答えながらそのまま流し、
// ほかの種類のファイルや、ほかの録画のファイルは流さないことを確かめる (設計書 6.3、#308)。
func TestOtherFileHandler(t *testing.T) {
	t.Parallel()

	f := newRecordingFixture(t, nil)
	recordingID := f.recording(t, "completed")
	mp4 := f.otherFile(t, recordingID, "other", "encoded.MP4", "0123456789")
	segment := f.otherFile(t, recordingID, "segment", "recorded.m2ts", "ts")
	another := f.otherFile(t, f.recording(t, "completed"), "other", "another.mp4", "another")
	html := f.otherFile(t, recordingID, "other", "番組.html", "<script>alert(1)</script>")
	svg := f.otherFile(t, recordingID, "other", "logo.SVG", "<svg></svg>")

	tests := []struct {
		name       string
		pools      map[string]string
		fileID     uuid.UUID
		rangeValue string
		wantStatus int
		wantType   string
		wantBody   string
		// wantDisposition は、Content-Disposition。空なら付けないことを確かめる。
		wantDisposition string
	}{
		{name: "全体", pools: nil, fileID: mp4, rangeValue: "", wantStatus: http.StatusOK, wantType: "video/mp4", wantBody: "0123456789"},
		{name: "Range", pools: nil, fileID: mp4, rangeValue: "bytes=2-5", wantStatus: http.StatusPartialContent, wantType: "video/mp4", wantBody: "2345"},
		{name: "セグメントは流さない", pools: nil, fileID: segment, rangeValue: "", wantStatus: http.StatusNotFound, wantType: "", wantBody: ""},
		{name: "ほかの録画のファイル", pools: nil, fileID: another, rangeValue: "", wantStatus: http.StatusNotFound, wantType: "", wantBody: ""},
		{name: "ないファイル", pools: nil, fileID: id.New(), rangeValue: "", wantStatus: http.StatusNotFound, wantType: "", wantBody: ""},
		{
			name: "HTML はダウンロードさせる", pools: nil, fileID: html, rangeValue: "", wantStatus: http.StatusOK,
			wantType: "application/octet-stream", wantBody: "<script>alert(1)</script>",
			wantDisposition: "attachment; filename*=utf-8''%E7%95%AA%E7%B5%84.html",
		},
		{
			name: "SVG はダウンロードさせる", pools: nil, fileID: svg, rangeValue: "", wantStatus: http.StatusOK,
			wantType: "application/octet-stream", wantBody: "<svg></svg>", wantDisposition: `attachment; filename=logo.SVG`,
		},
		{
			name: "プールをマウントしていない", pools: map[string]string{}, fileID: mp4, rangeValue: "",
			wantStatus: http.StatusServiceUnavailable, wantType: "", wantBody: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pools := tt.pools
			if pools == nil {
				pools = map[string]string{poolID: f.dir}
			}
			mux := http.NewServeMux()
			mux.Handle(otherFilePattern, stream.NewOtherFileHandler(f.pool, kago.New(pools, nil), slog.New(slog.DiscardHandler)))
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+stream.OtherFileURL(recordingID, tt.fileID), nil)
			require.NoError(t, err)
			if tt.rangeValue != "" {
				req.Header.Set("Range", tt.rangeValue)
			}
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, res.StatusCode)
			if tt.wantType == "" {
				return
			}
			assert.Equal(t, tt.wantType, res.Header.Get("Content-Type"))
			assert.Equal(t, tt.wantBody, string(body))
			assert.Equal(t, tt.wantDisposition, res.Header.Get("Content-Disposition"))
			assert.Equal(t, "sandbox", res.Header.Get("Content-Security-Policy"))
			assert.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
		})
	}
}
