package recording_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
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
		slog.New(slog.DiscardHandler), &recording.RecorderOptions{TuningTime: 0, Tick: 10 * time.Millisecond, RetryInterval: 20 * time.Millisecond})

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
			// ストリームを開けないときは、予定の終わりまで開き直すので、予定の終わりまで進める。
			time.Sleep(100 * time.Millisecond)
			f.clock.Advance(time.Hour)

			require.NoError(t, wait(t, done))
			got, ok := f.recording(t, reservationID)
			require.True(t, ok)
			assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(tt.want)), drops: 0}, got)
		})
	}
}

// onceStream は、TS を 1 回だけ流し、その後は閉じるまで何も流さないストリーム。繰り返しのつなぎ目のドロップを避ける。
type onceStream struct{ data []byte }

func (s onceStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		_, _ = writer.Write(s.data)
		<-ctx.Done()
		_ = writer.CloseWithError(ctx.Err())
	}()

	return reader, nil
}

// TestRecorderDecidesStateFromWholeRecording は、録画を終えたときの状態を、前のセグメントも含めた録画全体のドロップで決めることを確かめる。
func TestRecorderDecidesStateFromWholeRecording(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// earlierDrops は、引き継ぐ前のセグメントで数えたドロップ。
		earlierDrops int64
		want         recording.State
	}{
		{name: "どのセグメントにもドロップがない", earlierDrops: 0, want: recording.StateCompleted},
		{name: "前のセグメントにドロップがある", earlierDrops: 3, want: recording.StateCompletedWithErrors},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			if tt.earlierDrops > 0 {
				_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
					state, protected, drops, recorded_from, created_at, updated_at)
					SELECT $1, id, service_id, event_id, name, description, start_at, end_at, 'recording', false, $2, $4, $4, $4
					FROM reservations WHERE id = $3`, id.New(), tt.earlierDrops, reservationID, f.clock.Now())
				require.NoError(t, err)
			}
			recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			require.Eventually(t, func() bool {
				files := f.files(t, reservationID)

				return len(files) == 2
			}, waitTimeout, 10*time.Millisecond)
			time.Sleep(100 * time.Millisecond)
			f.clock.Advance(time.Hour)

			require.NoError(t, wait(t, done))
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, recorded{state: string(tt.want), failureReason: nil, drops: tt.earlierDrops}, got)
		})
	}
}

// TestRecorderCancelsWhenDisabled は、録画中に予約を無効にしたら、録画を取り消しとして失敗にすることを確かめる。
func TestRecorderCancelsWhenDisabled(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, f.tuner, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 2
	}, waitTimeout, 10*time.Millisecond)

	_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
	require.NoError(t, err)
	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateFailed), got.state)
	assert.Equal(t, new(string(recording.FailureCancelled)), got.failureReason)
	for _, file := range f.files(t, reservationID) {
		assert.True(t, file.closed, file.path)
	}
}

// TestRecorderFailsOnUnmountedPool は、録画設定のプールがこのインスタンスにマウントされていなければ、保存先がないとして失敗にすることを確かめる。
func TestRecorderFailsOnUnmountedPool(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	_, err := f.pool.Exec(t.Context(), `UPDATE reservations SET settings = '{"poolId": "elsewhere"}' WHERE id = $1`, reservationID)
	require.NoError(t, err)
	recorder, _ := f.newRecorder(t, f.tuner, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))

	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(recording.FailureStorageUnavailable)), drops: 0}, got)
}

// flakyStream は、最初に開いたストリームを head だけ流して切り、2 回目からは TS を 1 回だけ流す。
type flakyStream struct {
	head   []byte
	data   []byte
	opened *atomic.Int32
}

func (s flakyStream) OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.opened.Add(1) == 1 {
		return io.NopCloser(bytes.NewReader(s.head)), nil
	}

	return onceStream{data: s.data}.OpenServiceStream(ctx, serviceID, opts)
}

type gapsAndLost struct {
	gaps        int
	programLost bool
}

func (f *fixture) gapsAndLost(t *testing.T, reservationID uuid.UUID) gapsAndLost {
	t.Helper()

	var got gapsAndLost
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT jsonb_array_length(gaps), program_lost FROM recordings WHERE reservation_id = $1", reservationID).
		Scan(&got.gaps, &got.programLost))

	return got
}

func (f *fixture) segments(t *testing.T, reservationID uuid.UUID) int {
	t.Helper()

	n := 0
	for _, file := range f.files(t, reservationID) {
		if file.kind == "segment" {
			n++
		}
	}

	return n
}

// TestRecorderReconnects は、予定の終わりの前にストリームが切れたら、開き直して新しいセグメントに書き、
// 途切れた区間を記録して、録画を一部のみにすることを確かめる (設計書 5.2、5.5)。
func TestRecorderReconnects(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	opened := new(atomic.Int32)
	recorder, _ := f.newRecorder(t, flakyStream{head: data[:188*1000], data: data, opened: opened}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)

	require.NoError(t, wait(t, done))
	assert.Equal(t, int32(2), opened.Load())
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
	assert.Equal(t, gapsAndLost{gaps: 1, programLost: false}, f.gapsAndLost(t, reservationID))
	for _, file := range f.files(t, reservationID) {
		assert.True(t, file.closed, file.path)
	}
}

// TestRecorderExtendsWhileEndIsUndecided は、番組の終了時刻が未定 (延長中) の間は、終わりが来るたびに録画を 5 分ずつ延ばし、
// 終了時刻が決まったら止めることを確かめる (設計書 5.1)。
func TestRecorderExtendsWhileEndIsUndecided(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	plannedEnd := programStart().Add(30*time.Minute + reserve.DefaultMarginSeconds*time.Second)
	tests := []struct {
		name string
		// stop は、延長を始めた後に、録画を止める出来事を起こし、止まる時刻を返す。
		stop func(t *testing.T, f *fixture) time.Time
	}{
		{name: "終了時刻が決まったら止める", stop: func(t *testing.T, f *fixture) time.Time {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = false WHERE service_id = $1 AND event_id = 6", grService(0))
			require.NoError(t, err)

			return f.clock.Now()
		}},
		{name: "未定のままでも、予定の終わりから 3 時間で止める", stop: func(*testing.T, *fixture) time.Time {
			return plannedEnd.Add(recording.MaxExtension)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			require.Eventually(t, func() bool {
				return len(f.files(t, reservationID)) == 2
			}, waitTimeout, 10*time.Millisecond)
			_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = true WHERE service_id = $1 AND event_id = 6", grService(0))
			require.NoError(t, err)

			// 予定の終わりと、延ばした 5 分後の終わりが来ても、録り続ける。
			for _, at := range []time.Time{plannedEnd, plannedEnd.Add(recording.ExtensionStep)} {
				f.clock.Advance(at.Sub(f.clock.Now()))
				time.Sleep(100 * time.Millisecond)
				got, _ := f.recording(t, reservationID)
				require.Equal(t, string(recording.StateRecording), got.state, at)
			}

			stopsAt := tt.stop(t, f)
			f.clock.Advance(max(stopsAt.Sub(f.clock.Now()), 0))
			require.NoError(t, wait(t, done))
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(recording.StateCompleted), got.state)
			assert.Equal(t, gapsAndLost{gaps: 0, programLost: false}, f.gapsAndLost(t, reservationID))
		})
	}
}

// TestRecorderKeepsRecordingWhenProgramIsLost は、番組が EPG から消えたら、予定の終わりまで録って、見失ったと記録することを確かめる (設計書 5.1)。
func TestRecorderKeepsRecordingWhenProgramIsLost(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	plannedEnd := programStart().Add(30*time.Minute + reserve.DefaultMarginSeconds*time.Second)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 2
	}, waitTimeout, 10*time.Millisecond)
	_, err = f.pool.Exec(t.Context(), "DELETE FROM programs WHERE service_id = $1 AND event_id = 6", grService(0))
	require.NoError(t, err)

	f.clock.Advance(plannedEnd.Add(-time.Second).Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return f.gapsAndLost(t, reservationID).programLost
	}, waitTimeout, 10*time.Millisecond)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state)

	f.clock.Advance(time.Second)
	require.NoError(t, wait(t, done))
	got, _ = f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompleted), got.state)
}

// TestRecorderRecordsGapOnResume は、引き継ぎや再開で続きを録るとき、前のセグメントを書き終えてからの間を、途切れた区間として記録することを確かめる。
func TestRecorderRecordsGapOnResume(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	stop, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 2
	}, waitTimeout, 10*time.Millisecond)
	stop(recording.ErrLeaseLost)
	require.ErrorIs(t, wait(t, done), recording.ErrLeaseLost)

	// 1 分後に、別の mikan が続きを録る。
	f.clock.Advance(time.Minute)
	_, done = f.session(t, recorder, reservationID)
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
	var gap struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	}
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT gaps->0 FROM recordings WHERE reservation_id = $1", reservationID).Scan(&gap))
	assert.Equal(t, time.Minute, gap.To.Sub(gap.From))
}

// TestRecorderIgnoresEmptyEarlierSegments は、前の mikan が何も書けなかったセグメントを、録れたものに数えないことを確かめる。
func TestRecorderIgnoresEmptyEarlierSegments(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recordingID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		SELECT $1, id, service_id, event_id, name, description, start_at, end_at, 'recording', false, $3, $3, $3
		FROM reservations WHERE id = $2`, recordingID, reservationID, f.clock.Now())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', 'main', 'empty.m2ts', 0, $3, $3, $3)`, id.New(), recordingID, f.clock.Now())
	require.NoError(t, err)

	recorder, _ := f.newRecorder(t, failingStream{err: errors.New("connection refused")}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	time.Sleep(100 * time.Millisecond)
	f.clock.Advance(time.Hour)

	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(recording.FailureTunerServerUnreachable)), drops: 0}, got)
}
