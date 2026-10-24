package analysis_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

// segmented は、state の状態の、大きさが sizes のセグメントを持つ録画を作り、録画とセグメントのファイルの ID を返す。
// 録画を始めた日時は、作るたびに 1 分ずつ新しくする。ファイルは作らない (Candidates は、データディレクトリだけを見る)。
func (f *fixture) segmented(t *testing.T, state string, sizes ...int) (uuid.UUID, []uuid.UUID) {
	t.Helper()

	f.clock.Advance(time.Minute)
	recordingID := id.New()
	at := f.clock.Now()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $3, $3)`, recordingID, serviceID, at, state)
	require.NoError(t, err)
	segmentIDs := make([]uuid.UUID, 0, len(sizes))
	for i, size := range sizes {
		segmentID := id.New()
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', $3, $4, $5, $6, $6, $6)`, segmentID, recordingID, poolID, segmentID.String()+".m2ts", size, at.Add(time.Duration(i)*time.Second))
		require.NoError(t, err)
		segmentIDs = append(segmentIDs, segmentID)
	}

	return recordingID, segmentIDs
}

// generate は、データディレクトリに、セグメントのファイル segmentID の name のファイルを置く。
func (f *fixture) generate(t *testing.T, segmentID uuid.UUID, name string) {
	t.Helper()

	path := filepath.Join(f.dataRoot, kago.DataLocation(segmentID, name).Path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
}

// TestCandidates は、録画中でない、書いたもののあるセグメントを持つ録画を、新しい順に返し、
// 索引か代表画像がデータディレクトリにない録画を Missing にすることを確かめる (#897)。
func TestCandidates(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 録画を始めた日時の古い順に作る。
	complete, segments := f.segmented(t, "completed", 1, 1)
	f.generate(t, segments[0], kago.IndexFileName)
	f.generate(t, segments[1], kago.IndexFileName)
	f.generate(t, segments[0], thumbnail.ThumbnailFileName)
	noThumbnail, segments := f.segmented(t, "completed", 1, 1)
	f.generate(t, segments[0], kago.IndexFileName)
	f.generate(t, segments[1], kago.IndexFileName)
	secondUnindexed, segments := f.segmented(t, "partial", 1, 1)
	f.generate(t, segments[0], kago.IndexFileName)
	f.generate(t, segments[0], thumbnail.ThumbnailFileName)
	// 書いたもののない最初のセグメントは飛ばし、書いたもののある最初のセグメントに代表画像を置く。
	emptyFirst, segments := f.segmented(t, "completed", 0, 1)
	f.generate(t, segments[1], kago.IndexFileName)
	f.generate(t, segments[1], thumbnail.ThumbnailFileName)
	nothing, _ := f.segmented(t, "completed_with_errors", 1)
	// 録画中の録画と、書いたもののない録画は返さない。
	f.segmented(t, "recording", 1)
	f.segmented(t, "failed", 0)

	got, err := analysis.Candidates(t.Context(), f.pool, f.store)
	require.NoError(t, err)
	assert.Equal(t, []*analysis.Candidate{
		{RecordingID: nothing, Missing: true},
		{RecordingID: emptyFirst, Missing: false},
		{RecordingID: secondUnindexed, Missing: true},
		{RecordingID: noThumbnail, Missing: true},
		{RecordingID: complete, Missing: false},
	}, got)
}

// TestEnqueue は、解析し直すジョブを投入し、待っているジョブの録画には重ねて投入しないことを確かめる (#897)。
func TestEnqueue(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	first, _ := f.segmented(t, "completed", 1)
	second, _ := f.segmented(t, "completed", 1)

	queued, err := analysis.Enqueue(t.Context(), f.pool, f.inserter, []uuid.UUID{first})
	require.NoError(t, err)
	assert.Equal(t, 1, queued)
	queued, err = analysis.Enqueue(t.Context(), f.pool, f.inserter, []uuid.UUID{first, second})
	require.NoError(t, err)
	assert.Equal(t, 1, queued, "the recording already queued is skipped")

	// 終えたジョブには重ねて投入する。River は、終えたジョブを 1 日残す。
	_, err = f.pool.Exec(t.Context(), "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = $1", analysis.AnalyzeArgs{}.Kind())
	require.NoError(t, err)
	queued, err = analysis.Enqueue(t.Context(), f.pool, f.inserter, []uuid.UUID{first, second})
	require.NoError(t, err)
	assert.Equal(t, 2, queued, "finished jobs do not block reanalysis")

	var jobs, priority int
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT count(*), max(priority) FROM river_job WHERE kind = $1 AND state = 'available'", analysis.AnalyzeArgs{}.Kind()).Scan(&jobs, &priority))
	assert.Equal(t, 2, jobs)
	// 新しく録った録画のジョブより後に回す。
	assert.Equal(t, 4, priority)
}
