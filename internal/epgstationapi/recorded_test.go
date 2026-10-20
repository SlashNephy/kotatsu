package epgstationapi_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
)

type recorded struct {
	ID          int64  `json:"id"`
	ChannelID   int64  `json:"channelId"`
	Name        string `json:"name"`
	Extended    string `json:"extended"`
	IsRecording bool   `json:"isRecording"`
	VideoFiles  []struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"videoFiles"`
	Thumbnails []int64 `json:"thumbnails"`
}

type recordedList struct {
	Records []recorded `json:"records"`
	Total   int64      `json:"total"`
}

// addRecording は、録画を足し、その整数の ID を返す。segments が 0 なら、再生できない録画にする。
// thumbnail なら、データディレクトリ dataRoot の、最初のセグメントのディレクトリに代表画像を置く。
func addRecording(
	t *testing.T, pool *pgxpool.Pool, dataRoot, name, state string, recordedFrom time.Time, segments int, thumbnail bool,
) (uuid.UUID, int64) {
	t.Helper()

	id := uuid.New()
	exec(t, pool, `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, $5, $6, $4, $4, $4)`, id, grService, name, recordedFrom, recordedFrom.Add(time.Hour), state)
	for i := range segments {
		segmentID := uuid.New()
		exec(t, pool, `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, created_at)
			VALUES ($1, $2, 'segment', 'default', $3, 1000, $4, $4)`, segmentID, id, uuid.NewString(), recordedFrom.Add(time.Duration(i)*time.Minute))
		if thumbnail && i == 0 {
			image := kago.DataLocation(segmentID, "thumbnail.jpg")
			require.NoError(t, os.MkdirAll(filepath.Join(dataRoot, filepath.Dir(image.Path)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dataRoot, image.Path), []byte("jpeg"), 0o600))
		}
	}
	var apiID int64
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT api_id FROM recordings WHERE id = $1", id).Scan(&apiID))

	return id, apiID
}

// TestRecorded は、再生できる録画を、新しい順に、件数と番組名で区切って返すことを確かめる (#431)。
func TestRecorded(t *testing.T) {
	t.Parallel()

	h, pool, dataRoot := newHandler(t)
	now := testNow()
	_, oldest := addRecording(t, pool, dataRoot, "こたつ特集", "completed", now.Add(-3*time.Hour), 1, false)
	_, middle := addRecording(t, pool, dataRoot, "ねこの番組２", "completed", now.Add(-2*time.Hour), 2, true)
	_, newest := addRecording(t, pool, dataRoot, "こたつとねこ", "recording", now.Add(-time.Hour), 1, false)
	_, failed := addRecording(t, pool, dataRoot, "失敗した録画", "failed", now, 0, false)

	tests := []struct {
		name      string
		query     string
		wantIDs   []int64
		wantTotal int64
	}{
		{name: "すべて", query: "", wantIDs: []int64{newest, middle, oldest}, wantTotal: 3},
		{name: "区切る", query: "?offset=1&limit=1", wantIDs: []int64{middle}, wantTotal: 3},
		{name: "番組名", query: "?keyword=こたつ", wantIDs: []int64{newest, oldest}, wantTotal: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got recordedList
			require.Equal(t, http.StatusOK, get(t, h.Recorded, "/api/recorded"+tt.query, &got))
			ids := make([]int64, 0, len(got.Records))
			for _, r := range got.Records {
				ids = append(ids, r.ID)
			}
			assert.Equal(t, tt.wantIDs, ids)
			assert.Equal(t, tt.wantTotal, got.Total)
		})
	}
	assert.Equal(t, http.StatusBadRequest, get(t, h.Recorded, "/api/recorded?limit=-1", nil))

	var item recorded
	require.Equal(t, http.StatusOK, getItem(t, h.RecordedItem, middle, &item))
	assert.Equal(t, middle, item.ID)
	assert.Equal(t, grService, item.ChannelID)
	assert.False(t, item.IsRecording)
	require.Len(t, item.VideoFiles, 1)
	assert.Equal(t, middle, item.VideoFiles[0].ID)
	assert.Equal(t, int64(2000), item.VideoFiles[0].Size)
	assert.Equal(t, []int64{middle}, item.Thumbnails)
	assert.Equal(t, http.StatusNotFound, getItem(t, h.RecordedItem, 999999, nil))
	// セグメントのない録画は、再生できないので返さない。
	assert.Equal(t, http.StatusNotFound, getItem(t, h.RecordedItem, failed, nil))

	// isHalfWidth なら、番組名の全角の英数字を半角にする。
	var half recordedList
	require.Equal(t, http.StatusOK, get(t, h.Recorded, "/api/recorded?keyword=ねこの&isHalfWidth=true", &half))
	require.Len(t, half.Records, 1)
	assert.Equal(t, "ねこの番組2", half.Records[0].Name)
}

func getItem(t *testing.T, handler http.HandlerFunc, id int64, out any) int {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/recorded/{id}", handler)

	return get(t, mux.ServeHTTP, "/api/recorded/"+strconv.FormatInt(id, 10), out)
}

// TestWithRecording は、動画とサムネイルの ID を、Kotatsu の録画の ID に読み替えてつなぐことを確かめる。
func TestWithRecording(t *testing.T) {
	t.Parallel()

	h, pool, dataRoot := newHandler(t)
	recordingID, apiID := addRecording(t, pool, dataRoot, "こたつ特集", "completed", testNow(), 1, true)
	var gotID, gotFile string
	mux := http.NewServeMux()
	mux.Handle("GET /api/thumbnails/{id}", h.WithRecording(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotFile = r.PathValue("id"), r.PathValue("file")
		w.WriteHeader(http.StatusNoContent)
	}), "thumbnail.jpg"))

	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "ある録画", path: "/api/thumbnails/" + strconv.FormatInt(apiID, 10), want: http.StatusNoContent},
		{name: "ない録画", path: "/api/thumbnails/999999", want: http.StatusNotFound},
		{name: "数でない ID", path: "/api/thumbnails/x", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, http.NoBody))
		assert.Equal(t, tt.want, rec.Code, tt.name)
	}
	assert.Equal(t, recordingID.String(), gotID)
	assert.Equal(t, "thumbnail.jpg", gotFile)
}
