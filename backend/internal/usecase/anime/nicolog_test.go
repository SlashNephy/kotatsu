package anime_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

type fakeNicolog struct {
	entries map[string][]*nicolog.Entry
	calls   []string
	fail    bool
}

var _ anime.NicologClient = new(fakeNicolog)

func (f *fakeNicolog) List(_ context.Context, path string) ([]*nicolog.Entry, error) {
	f.calls = append(f.calls, path)
	if f.fail {
		return nil, errors.New("nicolog is down")
	}

	return f.entries[path], nil
}

const nicologFolder = "テスト作品"

func nicologFixture(t *testing.T) (*pgxpool.Pool, *fakeNicolog, *anime.NicologMatcher, uuid.UUID) {
	t.Helper()

	db, clk, _, _ := newMatchFixture(t)
	recordingID := insertRecording(t, db, grService, jst(1, 0, 0), jst(1, 0, 30), false)
	linkRecording(t, db, recordingID, jst(1, 0, 30))
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := db.Exec(t.Context(), sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO syobocal_titles (tid, title, title_key) VALUES ($1, 'テスト 作品', 'テスト作品') ON CONFLICT (tid) DO NOTHING`, broadcastTID)
	exec(`INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at) VALUES ($1, 'ffmpeg', 1, now())`, recordingID)
	exec(`INSERT INTO recording_cm_sections (recording_id, detector, start_offset_ms, end_offset_ms) VALUES ($1, 'ffmpeg', 1000, 2000)`, recordingID)
	exec(`UPDATE system_settings SET comment_sources = ARRAY['nicolog']`)
	client := &fakeNicolog{entries: map[string][]*nicolog.Entry{
		nicolog.RootPath:                       {{Name: nicologFolder, IsDir: true}},
		nicolog.RootPath + "/" + nicologFolder: {{Name: nicologFolder + " 6話上映会.xml", IsDir: false}},
	}, calls: nil, fail: false}

	return db, client, anime.NewNicologMatcher(db, clk, client, 0, slog.New(slog.DiscardHandler)), recordingID
}

func nicologPath(t *testing.T, db *pgxpool.Pool, recordingID uuid.UUID) *string {
	t.Helper()

	var path *string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT path FROM recording_nicolog_files WHERE recording_id = $1`, recordingID).Scan(&path))

	return path
}

func TestNicologMatch(t *testing.T) {
	t.Parallel()

	db, client, matcher, recordingID := nicologFixture(t)

	result, err := matcher.Match(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 1, result.Linked)
	require.NotNil(t, nicologPath(t, db, recordingID))
	assert.Equal(t, nicolog.RootPath+"/"+nicologFolder+"/"+nicologFolder+" 6話上映会.xml", *nicologPath(t, db, recordingID))

	// 見つけた録画は、もう問い合わせない。
	client.calls = nil
	_, err = matcher.Match(t.Context())
	require.NoError(t, err)
	assert.Empty(t, client.calls)
}

func TestNicologMatchNotFound(t *testing.T) {
	t.Parallel()

	db, client, matcher, recordingID := nicologFixture(t)
	client.entries[nicolog.RootPath+"/"+nicologFolder] = []*nicolog.Entry{{Name: nicologFolder + " 5話上映会.xml", IsDir: false}}

	result, err := matcher.Match(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 0, result.Linked)
	assert.Nil(t, nicologPath(t, db, recordingID))
}

func TestNicologMatchDisabled(t *testing.T) {
	t.Parallel()

	db, client, matcher, _ := nicologFixture(t)
	_, err := db.Exec(t.Context(), `UPDATE system_settings SET comment_sources = '{}'`)
	require.NoError(t, err)

	result, err := matcher.Match(t.Context())

	require.NoError(t, err)
	assert.True(t, result.Disabled)
	assert.Empty(t, client.calls)
}

func TestNicologMatchFailure(t *testing.T) {
	t.Parallel()

	_, client, matcher, _ := nicologFixture(t)
	client.fail = true

	_, err := matcher.Match(t.Context())

	require.Error(t, err)
}
