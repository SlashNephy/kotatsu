package recording_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/ts"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const poolID = "main"

// newRecorder は、プール main を一時ディレクトリに置いた Recorder を返す。
func (f *fixture) newRecorder(t *testing.T, tuner recording.StreamOpener, freeSpace kago.FreeSpaceFunc) (*recording.Recorder, string) {
	t.Helper()

	dir := t.TempDir()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	if freeSpace == nil {
		freeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	}
	recorder := recording.NewRecorder(f.pool, f.clock, tuner, kago.New(map[string]string{poolID: dir}, freeSpace), nil, tokyo,
		slog.New(slog.DiscardHandler), &recording.RecorderOptions{TuningTime: 0, Tick: 10 * time.Millisecond})

	return recorder, dir
}

type recorded struct {
	state         string
	failureReason *string
	drops         int64
}

func (f *fixture) recording(t *testing.T, reservationID uuid.UUID) (recorded, bool) {
	t.Helper()

	var r recorded
	err := f.pool.QueryRow(t.Context(), "SELECT state, failure_reason, drops FROM recordings WHERE reservation_id = $1", reservationID).
		Scan(&r.state, &r.failureReason, &r.drops)
	if err != nil {
		return r, false
	}

	return r, true
}

type recordedFile struct {
	kind   string
	path   string
	size   int64
	closed bool
}

func (f *fixture) files(t *testing.T, reservationID uuid.UUID) []recordedFile {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT kind, path, size, ended_at IS NOT NULL FROM recording_files
		WHERE recording_id = (SELECT id FROM recordings WHERE reservation_id = $1) ORDER BY kind`, reservationID)
	require.NoError(t, err)
	defer rows.Close()
	files := []recordedFile{}
	for rows.Next() {
		var file recordedFile
		require.NoError(t, rows.Scan(&file.kind, &file.path, &file.size, &file.closed))
		files = append(files, file)
	}
	require.NoError(t, rows.Err())

	return files
}

// session は、Recorder の Session を、割り振りを取ったものとして、別の goroutine で動かす。
func (f *fixture) session(t *testing.T, recorder *recording.Recorder, reservationID uuid.UUID) (context.CancelCauseFunc, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithCancelCause(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- recorder.Session()(ctx, &recording.Assignment{ID: id.New(), ReservationID: reservationID, Priority: recording.Priority, Generation: 1})
	}()
	t.Cleanup(func() { cancel(nil) })

	return cancel, done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(waitTimeout):
		t.Fatal("the recording session did not return")

		return nil
	}
}

// plannedStart は、予約した番組の、マージンを含めた録画の予定の開始。
func plannedStart() time.Time {
	return programStart().Add(-reserve.DefaultMarginSeconds * time.Second)
}

// TestRecorderRecordsUntilPlannedEnd は、予定の開始にストリームを開き、予定の終わりまで、録画ファイルと索引を書くことを確かめる (設計書 5.1、5.2、5.3)。
func TestRecorderRecordsUntilPlannedEnd(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, dir := f.newRecorder(t, f.tuner, nil)
	_, done := f.session(t, recorder, reservationID)

	// 予定の開始までは、録画を始めない。
	time.Sleep(100 * time.Millisecond)
	_, started := f.recording(t, reservationID)
	assert.False(t, started)

	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		files := f.files(t, reservationID)

		return len(files) == 2
	}, waitTimeout, 10*time.Millisecond)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state)
	// 書いている間も、最終的な保存先のファイルが伸びていく (追っかけ再生のため)。
	segment := f.files(t, reservationID)[1]
	require.Eventually(t, func() bool {
		info, err := os.Stat(filepath.Join(dir, segment.path))

		return err == nil && info.Size() > 4<<20
	}, waitTimeout, 10*time.Millisecond)

	// 予定の終わり (番組の終わりにマージンを足した時刻) が来たら止める。
	f.clock.Advance(programStart().Add(30*time.Minute + reserve.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	// 偽チューナーサーバーは、同じ TS を繰り返し流すので、つなぎ目で continuity_counter が飛び、ドロップとして数える。
	got, _ = f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompletedWithErrors), got.state)
	assert.Nil(t, got.failureReason)
	assert.Positive(t, got.drops)
	files := f.files(t, reservationID)
	require.Len(t, files, 2)
	index, segment := files[0], files[1]
	assert.Equal(t, "index", index.kind)
	assert.Equal(t, "segment", segment.kind)
	// 既定のファイル名は、表示のタイムゾーンの放送の開始、サービス名、番組名を並べる。
	var serviceName, programName string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT s.name, r.name FROM reservations r JOIN services s ON s.id = r.service_id WHERE r.id = $1",
		reservationID).Scan(&serviceName, &programName))
	assert.Equal(t, "2026-10-10 2230 "+serviceName+" "+programName+".m2ts", segment.path)
	assert.Equal(t, segment.path+".kotatsu/index", index.path)
	for _, file := range files {
		assert.True(t, file.closed, file.path)
		info, err := os.Stat(filepath.Join(dir, file.path))
		require.NoError(t, err)
		assert.Equal(t, file.size, info.Size(), "the size in the DB matches the file")
	}
	// 索引には、書きながら見つけた I フレームが入る。
	indexFile, err := os.Open(filepath.Join(dir, index.path))
	require.NoError(t, err)
	defer indexFile.Close()
	entries, err := ts.ReadIndex(indexFile)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}

// TestRecorderStopsWhenStopped は、リースを失うか止めるときは、録画ファイルを閉じて、録画を録画中のまま残すことを確かめる (設計書 5.4)。
func TestRecorderStopsWhenStopped(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, f.tuner, nil)
	cancel, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		files := f.files(t, reservationID)

		return len(files) == 2 && files[1].kind == "segment"
	}, waitTimeout, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	cancel(recording.ErrLeaseLost)
	require.ErrorIs(t, wait(t, done), recording.ErrLeaseLost)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state, "the successor continues the recording")
	for _, file := range f.files(t, reservationID) {
		assert.True(t, file.closed, file.path)
	}
	assert.Positive(t, f.files(t, reservationID)[1].size)
}

// TestRecorderSkipsDisabledReservations は、録画を始める前に予約を無効にしたら、録画しないことを確かめる。
func TestRecorderSkipsDisabledReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, f.tuner, nil)
	_, done := f.session(t, recorder, reservationID)
	_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
	require.NoError(t, err)

	require.NoError(t, wait(t, done))
	_, started := f.recording(t, reservationID)
	assert.False(t, started)
}

// failingStream は、ストリームを開くと err を返す。
type failingStream struct{ err error }

func (s failingStream) OpenServiceStream(context.Context, int64, *mirakurun.StreamOptions) (io.ReadCloser, error) {
	return nil, s.err
}

// TestRecorderRecordsFailures は、録画できなかった理由を記録することを確かめる (設計書 5.5)。
func TestRecorderRecordsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tuner     func(f *fixture) recording.StreamOpener
		freeSpace kago.FreeSpaceFunc
		want      recording.FailureReason
	}{
		{
			name: "チューナーを確保できない",
			tuner: func(*fixture) recording.StreamOpener {
				return failingStream{err: &mirakurun.APIError{StatusCode: http.StatusServiceUnavailable}}
			},
			freeSpace: nil,
			want:      recording.FailureTunerUnavailable,
		},
		{
			name:      "チューナーサーバーに接続できない",
			tuner:     func(*fixture) recording.StreamOpener { return failingStream{err: errors.New("connection refused")} },
			freeSpace: nil,
			want:      recording.FailureTunerServerUnreachable,
		},
		{
			name:      "保存先の容量が足りない",
			tuner:     func(f *fixture) recording.StreamOpener { return f.tuner },
			freeSpace: func(string) (uint64, error) { return 1 << 20, nil },
			want:      recording.FailureStorageFull,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, _ := f.newRecorder(t, tt.tuner(f), tt.freeSpace)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))

			require.NoError(t, wait(t, done))
			got, ok := f.recording(t, reservationID)
			require.True(t, ok)
			assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(tt.want)), drops: 0}, got)
		})
	}
}
