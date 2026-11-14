package tvtplay_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/tvtplay"
)

// gate は、管理者が録画のダウンロードを有効にしているかを返す。
type gate struct {
	enabled bool
	err     error
}

func (g gate) RecordingDownloadsEnabled(context.Context) (bool, error) {
	return g.enabled, g.err
}

type chapterFixture struct {
	pool      *pgxpool.Pool
	startedAt time.Time
}

func newChapterFixture(t *testing.T) *chapterFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	startedAt := time.Date(2026, 11, 15, 21, 0, 0, 0, time.UTC)
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, startedAt)
	require.NoError(t, err)

	return &chapterFixture{pool: pool, startedAt: startedAt}
}

// recording は、録画の行を作る。recordedUntil が 0 なら録画中にする。gaps は、途切れた区間の JSON。
func (f *chapterFixture) recording(t *testing.T, recordedUntil time.Duration, gaps string) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	var until *time.Time
	if recordedUntil > 0 {
		until = new(f.startedAt.Add(recordedUntil))
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, recorded_until, gaps, created_at, updated_at)
		VALUES ($1, NULL, 1, NULL, '録画', '', $2, $2, 'completed', false, $2, $3, $4, $2, $2)`, recordingID, f.startedAt, until, gaps)
	require.NoError(t, err)

	return recordingID
}

// segment は、録画のセグメントの行を作る。
func (f *chapterFixture) segment(t *testing.T, recordingID uuid.UUID, path string) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', 'main', $3, 188, $4, $4, $4)`, id.New(), recordingID, path, f.startedAt)
	require.NoError(t, err)
}

// section は、CM の区間の行を作る。
func (f *chapterFixture) section(t *testing.T, recordingID uuid.UUID, start, end time.Duration) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_sections (recording_id, detector, start_offset_ms, end_offset_ms)
		VALUES ($1, 'ffmpeg', $2, $3)`, recordingID, start.Milliseconds(), end.Milliseconds())
	require.NoError(t, err)
}

type chapterResponse struct {
	status      int
	contentType string
	disposition string
	body        string
}

func (f *chapterFixture) get(t *testing.T, g tvtplay.DownloadGate, recordingID string) *chapterResponse {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /internal/media/recordings/{id}/"+tvtplay.ChapterFileName, tvtplay.NewChapterHandler(f.pool, g, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/internal/media/recordings/"+recordingID+"/"+tvtplay.ChapterFileName, nil)
	require.NoError(t, err)
	resp, err := httpclienttest.New(t).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &chapterResponse{
		status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), disposition: resp.Header.Get("Content-Disposition"), body: string(body),
	}
}

// TestChapterHandler は、録画の CM の区間を、ダウンロードした TS の名前の .chapter として返し、返せないときは断ることを確かめる。
func TestChapterHandler(t *testing.T) {
	t.Parallel()

	f := newChapterFixture(t)
	finished := f.recording(t, 30*time.Minute, `[{"from":"2026-11-15T21:01:00Z","to":"2026-11-15T21:01:30Z"}]`)
	f.segment(t, finished, "番組/こたつ特集.m2ts")
	f.detected(t, finished, [2]time.Duration{10 * time.Second, 40 * time.Second}, [2]time.Duration{10 * time.Minute, 11 * time.Minute})
	recording := f.recording(t, 0, `[]`)
	f.segment(t, recording, "録画中.m2ts")
	noSegment := f.recording(t, 30*time.Minute, `[]`)
	enabled := gate{enabled: true, err: nil}

	tests := []struct {
		name            string
		gate            tvtplay.DownloadGate
		recordingID     string
		wantStatus      int
		wantBody        string
		wantDisposition string
	}{
		{
			name: "CM の区間を、途切れた区間を詰めた位置にし、TS の名前の .chapter で返す", gate: enabled, recordingID: finished.String(),
			wantStatus: http.StatusOK, wantBody: "\xef\xbb\xbfc-10000cix-40000cox-570000cix-630000cox-c",
			wantDisposition: "attachment; filename*=utf-8''%E3%81%93%E3%81%9F%E3%81%A4%E7%89%B9%E9%9B%86.chapter",
		},
		{name: "録画中の録画は断る", gate: enabled, recordingID: recording.String(), wantStatus: http.StatusConflict},
		{name: "セグメントのない録画は見つからない", gate: enabled, recordingID: noSegment.String(), wantStatus: http.StatusNotFound},
		{name: "ない録画は見つからない", gate: enabled, recordingID: id.New().String(), wantStatus: http.StatusNotFound},
		{name: "ID が読めなければ断る", gate: enabled, recordingID: "kotatsu", wantStatus: http.StatusBadRequest},
		{name: "ダウンロードを有効にしていなければ断る", gate: gate{enabled: false, err: nil}, recordingID: finished.String(), wantStatus: http.StatusForbidden},
		{
			name: "設定を読めず、覚えた設定もなければ 503", gate: gate{enabled: false, err: settings.ErrDatabaseUnavailable},
			recordingID: finished.String(), wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "設定を読めなければ 500", gate: gate{enabled: false, err: errors.New("broken")},
			recordingID: finished.String(), wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := f.get(t, tt.gate, tt.recordingID)
			assert.Equal(t, tt.wantStatus, got.status)
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantBody, got.body)
			assert.Equal(t, "text/plain; charset=utf-8", got.contentType)
			assert.Equal(t, tt.wantDisposition, got.disposition)
		})
	}
}
