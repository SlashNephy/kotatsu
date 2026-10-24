package library_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/library"
)

// segmentA と segmentB は、テストの録画のセグメントのファイル名。
const (
	segmentA = "a.m2ts"
	segmentB = "b.m2ts"
)

// segment は、テストの録画のセグメント。exists なら、ファイルを作る。
type segment struct {
	pool   string
	path   string
	exists bool
}

// segmented は、セグメント segments を持つ録画を作る。各セグメントには、データディレクトリに索引も置く。
func (f *fixture) segmented(t *testing.T, ago time.Duration, protected bool, segments ...segment) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	at := f.clock.Now().Add(-ago)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, 'completed', $4, $3, $3, $3)`, recordingID, serviceID, at, protected)
	require.NoError(t, err)
	for i, s := range segments {
		if s.exists {
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(f.dir, s.path)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(f.dir, s.path), []byte("ts"), 0o600))
		}
		started := at.Add(time.Duration(i) * time.Minute)
		// 生成物は、セグメントのファイルがなくても、データディレクトリに残っている。
		segmentID := id.New()
		require.NoError(t, os.MkdirAll(filepath.Join(f.dataRoot, segmentID.String()), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(f.dataRoot, kago.IndexLocation(segmentID).Path), []byte("index"), 0o600))
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', $3, $4, 2, $5, $5, $5)`, segmentID, recordingID, s.pool, s.path, started)
		require.NoError(t, err)
	}

	return recordingID
}

func (f *fixture) files(t *testing.T, recordingID uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_files WHERE recording_id = $1", recordingID).Scan(&n))

	return n
}

// TestFindMissing は、セグメントのファイルが見つからない録画だけを、新しい順に返し、
// マウントしていないプールのセグメントは見つからないとはみなさないことを確かめる。
func TestFindMissing(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	present := f.segmented(t, 3*time.Hour, false, segment{pool: poolID, path: "present.m2ts", exists: true})
	allGone := f.segmented(t, 2*time.Hour, false, segment{pool: poolID, path: "gone.m2ts", exists: false})
	partial := f.segmented(t, time.Hour, false,
		segment{pool: poolID, path: "partial1.m2ts", exists: true}, segment{pool: poolID, path: "partial2.m2ts", exists: false})
	unmounted := f.segmented(t, 4*time.Hour, false, segment{pool: "remote", path: "remote.m2ts", exists: false})

	got, err := f.library.FindMissing(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Equal(t, []*library.MissingRecording{
		{RecordingID: partial, Segments: 2, MissingSegments: 1},
		{RecordingID: allGone, Segments: 1, MissingSegments: 1},
	}, got)
	assert.NotContains(t, []uuid.UUID{got[0].RecordingID, got[1].RecordingID}, present)
	assert.NotContains(t, []uuid.UUID{got[0].RecordingID, got[1].RecordingID}, unmounted)
}

// TestCleanUpMissing は、セグメントが 1 つも見つからない録画は消し、一部だけなら見つからないセグメントと索引の記録だけを消し、
// 監査ログに残すことを確かめる。保護した録画は消さない。
func TestCleanUpMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		protected   bool
		segments    []segment
		want        *library.CleanUpResult
		wantErr     error
		wantFiles   int
		wantActions []string
	}{
		{
			name:     "すべて見つからなければ録画を消す",
			segments: []segment{{pool: poolID, path: segmentA, exists: false}, {pool: poolID, path: segmentB, exists: false}},
			want:     &library.CleanUpResult{Deleted: true, PrunedSegments: 0}, wantFiles: 0, wantActions: []string{"recording.deleted"},
		},
		{
			name:     "一部だけなら、見つからないセグメントの記録だけを消す",
			segments: []segment{{pool: poolID, path: segmentA, exists: true}, {pool: poolID, path: segmentB, exists: false}},
			want:     &library.CleanUpResult{Deleted: false, PrunedSegments: 1}, wantFiles: 1, wantActions: []string{"recording.missing_segments_removed"},
		},
		{
			name:     "見つかれば何もしない",
			segments: []segment{{pool: poolID, path: segmentA, exists: true}},
			want:     &library.CleanUpResult{Deleted: false, PrunedSegments: 0}, wantFiles: 1, wantActions: []string{},
		},
		{
			name: "保護した録画は消さない", protected: true,
			segments: []segment{{pool: poolID, path: segmentA, exists: false}},
			wantErr:  library.ErrProtected, wantFiles: 1, wantActions: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.segmented(t, time.Hour, tt.protected, tt.segments...)
			actor := audit.User(f.userID)
			got, err := f.library.CleanUpMissing(t.Context(), f.pool, actor, recordingID)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			assert.Equal(t, tt.wantFiles, f.files(t, recordingID))
			assert.Equal(t, tt.wantActions, f.actions(t, recordingID))
			// 記録が残るセグメントの生成物だけが、データディレクトリに残る。
			entries, err := os.ReadDir(f.dataRoot)
			require.NoError(t, err)
			assert.Len(t, entries, tt.wantFiles)
		})
	}
}
