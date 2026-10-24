package recording_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const poolID = "main"

// newRecorder は、プール main を一時ディレクトリに置いた Recorder を返す。
func (f *fixture) newRecorder(t *testing.T, tuner recording.StreamOpener, freeSpace kago.FreeSpaceFunc) (*recording.Recorder, string) {
	t.Helper()

	return f.newRecorderWithPeers(t, tuner, freeSpace, nil)
}

// newRecorderWithPeers は、ほかに動いている mikan の数を peers で数える Recorder を作る。nil なら、ほかの mikan はいない。
func (f *fixture) newRecorderWithPeers(t *testing.T, tuner recording.StreamOpener, freeSpace kago.FreeSpaceFunc, peers recording.PeersFunc) (*recording.Recorder, string) {
	t.Helper()

	return f.newRecorderWithLogger(t, tuner, freeSpace, peers, slog.New(slog.DiscardHandler))
}

// newRecorderWithLogger は、logger にログを書く Recorder を作る。
func (f *fixture) newRecorderWithLogger(t *testing.T, tuner recording.StreamOpener, freeSpace kago.FreeSpaceFunc, peers recording.PeersFunc,
	logger *slog.Logger,
) (*recording.Recorder, string) {
	t.Helper()

	dir := t.TempDir()
	// 同じテストの Recorder は、データディレクトリを共有する (dataRootOf)。
	require.NoError(t, os.MkdirAll(dataRootOf(dir), 0o755))
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	if freeSpace == nil {
		freeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	}
	recorder := recording.NewRecorder(f.pool, f.clock, tuner, kago.New(map[string]string{poolID: dir}, freeSpace).WithDataDir(dataRootOf(dir)), nil, tokyo,
		logger, &recording.RecorderOptions{
			TuningTime: 0, Tick: 10 * time.Millisecond, RetryInterval: 20 * time.Millisecond, ReleaseAfter: 0, Peers: peers,
		})

	return recorder, dir
}

// dataRootOf は、プールのディレクトリ dir の Recorder が使う、データディレクトリを返す。t.TempDir の隣に置く。
func dataRootOf(dir string) string {
	return filepath.Join(filepath.Dir(dir), "data")
}

// messageRecorder は、ログのメッセージを順に記録する slog.Handler。録画は別の goroutine からログを書くので、ロックで守る。
type messageRecorder struct {
	mu       sync.Mutex
	messages []string
}

func (m *messageRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (m *messageRecorder) Handle(_ context.Context, record slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, record.Message)

	return nil
}

func (m *messageRecorder) WithAttrs([]slog.Attr) slog.Handler { return m }

func (m *messageRecorder) WithGroup(string) slog.Handler { return m }

// list は、これまでに記録したメッセージを返す。
func (m *messageRecorder) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.messages)
}

var _ slog.Handler = new(messageRecorder)

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
	id     uuid.UUID
	kind   string
	path   string
	size   int64
	closed bool
}

func (f *fixture) files(t *testing.T, reservationID uuid.UUID) []recordedFile {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT id, kind, path, size, ended_at IS NOT NULL FROM recording_files
		WHERE recording_id = (SELECT id FROM recordings WHERE reservation_id = $1) ORDER BY kind`, reservationID)
	require.NoError(t, err)
	defer rows.Close()
	files := []recordedFile{}
	for rows.Next() {
		var file recordedFile
		require.NoError(t, rows.Scan(&file.id, &file.kind, &file.path, &file.size, &file.closed))
		files = append(files, file)
	}
	require.NoError(t, rows.Err())

	return files
}

// session は、Recorder の Session を、割り振りを取ったものとして、別の goroutine で動かす。
func (f *fixture) session(t *testing.T, recorder *recording.Recorder, reservationID uuid.UUID) (context.CancelCauseFunc, <-chan error) {
	t.Helper()

	return f.sessionWith(t, recorder, &recording.Assignment{
		ID: id.New(), ReservationID: reservationID, Priority: recording.Priority, Generation: 1, HandedOver: false, PreviousLeaseExpiresAt: nil,
	})
}

// sessionWith は、Recorder の Session を、割り振り assignment を取ったものとして、別の goroutine で動かす。
func (f *fixture) sessionWith(t *testing.T, recorder *recording.Recorder, assignment *recording.Assignment) (context.CancelCauseFunc, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithCancelCause(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- recorder.Session()(ctx, assignment)
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
	return programStart().Add(-domain.DefaultMarginSeconds * time.Second)
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

		return len(files) == 1
	}, waitTimeout, 10*time.Millisecond)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state)
	// 書いている間も、最終的な保存先のファイルが伸びていく (追っかけ再生のため)。
	segment := f.files(t, reservationID)[0]
	require.Eventually(t, func() bool {
		info, err := os.Stat(filepath.Join(dir, segment.path))

		return err == nil && info.Size() > 4<<20
	}, waitTimeout, 10*time.Millisecond)

	// 予定の終わり (番組の終わりにマージンを足した時刻) が来たら止める。
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	// 偽チューナーサーバーは、同じ TS を繰り返し流すので、つなぎ目で continuity_counter が飛び、ドロップとして数える。
	got, _ = f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompletedWithErrors), got.state)
	assert.Nil(t, got.failureReason)
	assert.Positive(t, got.drops)
	// 索引は DB に記録せず、データディレクトリの、セグメントのファイルの ID のディレクトリに置く。
	files := f.files(t, reservationID)
	require.Len(t, files, 1)
	segment = files[0]
	assert.Equal(t, "segment", segment.kind)
	indexPath := filepath.Join(dataRootOf(dir), kago.IndexLocation(segment.id).Path)
	// 既定のファイル名は、表示のタイムゾーンの放送の開始、サービス名、番組名を並べる。
	var serviceName, programName string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT s.name, r.name FROM reservations r JOIN services s ON s.id = r.service_id WHERE r.id = $1",
		reservationID).Scan(&serviceName, &programName))
	assert.Equal(t, "2026-10-10 2230 "+serviceName+" "+programName+".m2ts", segment.path)
	for _, file := range files {
		assert.True(t, file.closed, file.path)
		info, err := os.Stat(filepath.Join(dir, file.path))
		require.NoError(t, err)
		assert.Equal(t, file.size, info.Size(), "the size in the DB matches the file")
	}
	// セグメントには、PID ごとの内訳を残す。全 PID のドロップの合計は、録画のドロップと同じになる (#473)。
	var pidDrops, videoPackets int64
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT coalesce(sum((e->>'drops')::bigint), 0),
		coalesce(sum((e->>'packets')::bigint) FILTER (WHERE e->>'kind' = 'video'), 0)
		FROM recording_files rf JOIN recordings r ON r.id = rf.recording_id, jsonb_array_elements(rf.pid_stats) e
		WHERE r.reservation_id = $1`, reservationID).Scan(&pidDrops, &videoPackets))
	assert.Equal(t, got.drops, pidDrops)
	assert.Positive(t, videoPackets)
	// 索引には、書きながら見つけた I フレームが入る。
	indexFile, err := os.Open(indexPath)
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

		return len(files) == 1 && files[0].kind == "segment"
	}, waitTimeout, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	cancel(recording.ErrLeaseLost)
	require.ErrorIs(t, wait(t, done), recording.ErrLeaseLost)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state, "the successor continues the recording")
	for _, file := range f.files(t, reservationID) {
		assert.True(t, file.closed, file.path)
	}
	assert.Positive(t, f.files(t, reservationID)[0].size)
}

// TestRecorderSkipsDisabledReservations は、録画を始める前に予約を無効にしたら、録画しないことを確かめる。
func TestRecorderSkipsDisabledReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// cancel は、録画を始める前に予約を取り消す。
		cancel func(t *testing.T, f *fixture, reservationID uuid.UUID)
	}{
		{name: "無効にする", cancel: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
			require.NoError(t, err)
		}},
		{name: "消す (#304)", cancel: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			require.NoError(t, f.reservations.Delete(t.Context(), f.pool, audit.System("test"), reservationID))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, _ := f.newRecorder(t, f.tuner, nil)
			_, done := f.session(t, recorder, reservationID)
			tt.cancel(t, f, reservationID)

			require.NoError(t, wait(t, done))
			var recordings int
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recordings").Scan(&recordings))
			assert.Zero(t, recordings)
		})
	}
}

// failingStream は、ストリームを開くと err を返す。
type failingStream struct{ err error }

func (s failingStream) OpenServiceStream(context.Context, int64, *mirakurun.StreamOptions) (io.ReadCloser, error) {
	return nil, s.err
}

// attemptedStream は、ほかのストリームを包み、録画が開こうとしたら attempted に送る。
// 開こうとする前に時計を予定の終わりより後に進めると、録画は予定が過ぎたとみなして何もしないので、その前にこれを待つ (#329)。
type attemptedStream struct {
	recording.StreamOpener

	attempted chan struct{}
}

func newAttemptedStream(stream recording.StreamOpener) *attemptedStream {
	return &attemptedStream{StreamOpener: stream, attempted: make(chan struct{}, 1)}
}

func (s *attemptedStream) OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	select {
	case s.attempted <- struct{}{}:
	default:
	}

	return s.StreamOpener.OpenServiceStream(ctx, serviceID, opts)
}

// wait は、録画がストリームを開こうとするまで待つ。
func (s *attemptedStream) wait(t *testing.T) {
	t.Helper()

	select {
	case <-s.attempted:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not open the stream")
	}
}

var _ recording.StreamOpener = new(attemptedStream)

// TestRecorderRecordsFailures は、録画できなかった理由を記録することを確かめる (設計書 5.5)。
func TestRecorderRecordsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tuner     func(f *fixture) recording.StreamOpener
		freeSpace kago.FreeSpaceFunc
		want      recording.FailureReason
		// retries は、予定の終わりまで開き直すかどうか。開き直すなら、開こうとしてから予定の終わりまで進める。
		// 保存先がなければ、その場で失敗にする。時計を進めると、残りの時間の推定サイズが 0 になって入ってしまう。
		retries bool
	}{
		{
			name: "チューナーを確保できない",
			tuner: func(*fixture) recording.StreamOpener {
				return failingStream{err: &mirakurun.APIError{StatusCode: http.StatusServiceUnavailable}}
			},
			freeSpace: nil,
			want:      recording.FailureTunerUnavailable,
			retries:   true,
		},
		{
			// チューナーの空きを待って応答しないまま、開く時間の上限が過ぎた (#412)。
			name: "チューナーサーバーが応答を返さない",
			tuner: func(*fixture) recording.StreamOpener {
				return failingStream{err: fmt.Errorf("failed to request: %w", mirakurun.ErrStreamOpenTimeout)}
			},
			freeSpace: nil,
			want:      recording.FailureTunerUnavailable,
			retries:   true,
		},
		{
			name:      "チューナーサーバーに接続できない",
			tuner:     func(*fixture) recording.StreamOpener { return failingStream{err: errors.New("connection refused")} },
			freeSpace: nil,
			want:      recording.FailureTunerServerUnreachable,
			retries:   true,
		},
		{
			name:      "保存先の容量が足りない",
			tuner:     func(f *fixture) recording.StreamOpener { return f.tuner },
			freeSpace: func(string) (uint64, error) { return 1 << 20, nil },
			want:      recording.FailureStorageFull,
			retries:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream := newAttemptedStream(tt.tuner(f))
			recorder, _ := f.newRecorder(t, stream, tt.freeSpace)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			// ストリームを開けないときは、予定の終わりまで開き直すので、録画が開こうとしてから、予定の終わりまで進める。
			stream.wait(t)
			if tt.retries {
				f.clock.Advance(time.Hour)
			}

			require.NoError(t, wait(t, done))
			got, ok := f.recording(t, reservationID)
			require.True(t, ok)
			assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(tt.want)), drops: 0}, got)
		})
	}
}

// onceStream は、TS を 1 回だけ流し、その後は閉じるまで何も流さないストリーム。繰り返しのつなぎ目のドロップを避ける。
type onceStream struct{ data []byte }

// drainedStream は、onceStream と同じく data を流してから閉じられるまで待つ。開くたびに、録画が data を読み終えたら drained に送る。
// 書き始める前に止めたり終えたりすると、何も書けていないセグメントになるので、その前にこれを待つ。
type drainedStream struct {
	data    []byte
	drained chan struct{}
}

func newDrainedStream(data []byte) *drainedStream {
	return &drainedStream{data: data, drained: make(chan struct{}, 4)}
}

func (s *drainedStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		// io.Pipe の Write は、読む側がすべて読むまで戻らない。
		if _, err := writer.Write(s.data); err == nil {
			s.drained <- struct{}{}
		}
		<-ctx.Done()
		_ = writer.CloseWithError(ctx.Err())
	}()

	return reader, nil
}

// wait は、ストリームを 1 回読み終えるまで待つ。
func (s *drainedStream) wait(t *testing.T) {
	t.Helper()

	select {
	case <-s.drained:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not read the stream")
	}
}

var _ recording.StreamOpener = new(drainedStream)

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

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// earlierDrops は、引き継ぐ前のセグメントで数えたドロップ。
		earlierDrops int64
		want         recording.State
		wantHistory  bool
	}{
		{name: "どのセグメントにもドロップがない", earlierDrops: 0, want: recording.StateCompleted, wantHistory: true},
		// 録画履歴には、ドロップが 1 件以下の録画だけを記録する (付録 A)。
		{name: "前のセグメントのドロップが閾値以下", earlierDrops: 1, want: recording.StateCompletedWithErrors, wantHistory: true},
		{name: "前のセグメントのドロップが閾値を超える", earlierDrops: 3, want: recording.StateCompletedWithErrors, wantHistory: false},
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

				return len(files) == 1
			}, waitTimeout, 10*time.Millisecond)
			time.Sleep(100 * time.Millisecond)
			f.clock.Advance(time.Hour)

			require.NoError(t, wait(t, done))
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, recorded{state: string(tt.want), failureReason: nil, drops: tt.earlierDrops}, got)
			assert.Equal(t, tt.wantHistory, f.historyCount(t) == 1)
		})
	}
}

// TestRecorderCancelsWhenDisabled は、録画中に予約を無効にしたら、録画を取り消しとして失敗にすることを確かめる。
func TestRecorderCancelsWhenDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// cancel は、録画中の予約を取り消す。
		cancel func(t *testing.T, f *fixture, reservationID uuid.UUID)
	}{
		{name: "無効にする", cancel: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
			require.NoError(t, err)
		}},
		{name: "消す (#304)", cancel: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			require.NoError(t, f.reservations.Delete(t.Context(), f.pool, audit.System("test"), reservationID))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, _ := f.newRecorder(t, f.tuner, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			require.Eventually(t, func() bool {
				return len(f.files(t, reservationID)) == 1
			}, waitTimeout, 10*time.Millisecond)
			// 予約を消すと、録画の reservation_id は NULL になるので、録画の ID で追う。
			var recordingID uuid.UUID
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM recordings WHERE reservation_id = $1", reservationID).Scan(&recordingID))

			tt.cancel(t, f, reservationID)
			require.NoError(t, wait(t, done))
			var state string
			var failureReason *string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state, failure_reason FROM recordings WHERE id = $1", recordingID).
				Scan(&state, &failureReason))
			assert.Equal(t, string(recording.StateFailed), state)
			assert.Equal(t, new(string(recording.FailureCancelled)), failureReason)
			var open int
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_files WHERE recording_id = $1 AND ended_at IS NULL",
				recordingID).Scan(&open))
			assert.Zero(t, open, "all files are closed")
		})
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
// 開き直すときは、reopen を呼ぶ (時計を進めて、開き直すまでにかかった時間を表す)。
type flakyStream struct {
	head   []byte
	data   []byte
	opened *atomic.Int32
	reopen func()
}

func (s flakyStream) OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.opened.Add(1) == 1 {
		return io.NopCloser(bytes.NewReader(s.head)), nil
	}
	s.reopen()

	return onceStream{data: s.data}.OpenServiceStream(ctx, serviceID, opts)
}

// shortStream は、開くたびに data を流して、すぐに切れる。開いた回数を opened に数える。
type shortStream struct {
	data   []byte
	opened *atomic.Int32
}

var _ recording.StreamOpener = new(shortStream)

func (s *shortStream) OpenServiceStream(context.Context, int64, *mirakurun.StreamOptions) (io.ReadCloser, error) {
	s.opened.Add(1)

	return io.NopCloser(bytes.NewReader(s.data)), nil
}

// endingStream は、data を流した後、close を閉じるまで待ってから、ストリームを終える。開いた回数を opened に数える。
type endingStream struct {
	data   []byte
	close  chan struct{}
	opened *atomic.Int32
}

var _ recording.StreamOpener = new(endingStream)

func (s *endingStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	s.opened.Add(1)
	reader, writer := io.Pipe()
	go func() {
		if _, err := writer.Write(s.data); err != nil {
			return
		}
		select {
		case <-s.close:
			_ = writer.Close()
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
		}
	}()

	return reader, nil
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

// TestRecorderDoesNotReopenAfterPlannedEnd は、予定の終わりにチューナーサーバーがストリームを閉じたとき、
// 開き直さずに録画を終えることを確かめる (#305)。
func TestRecorderDoesNotReopenAfterPlannedEnd(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	opened := new(atomic.Int32)
	stream := &endingStream{data: data, close: make(chan struct{}), opened: opened}
	recorder, _ := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)

	// 予定の終わりを過ぎてから、ストリームが閉じる。
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	close(stream.close)
	require.NoError(t, wait(t, done))
	assert.Equal(t, int32(1), opened.Load())
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompleted), got.state)
}

// TestRecorderWaitsBeforeReopeningShortStreams は、開いてすぐに切れ続けるストリームを、RetryInterval に 1 回までしか開き直さないことを確かめる (#305)。
func TestRecorderWaitsBeforeReopeningShortStreams(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	opened := new(atomic.Int32)
	recorder, _ := f.newRecorder(t, &shortStream{data: data[:188*100], opened: opened}, nil)
	cancel, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return opened.Load() > 0
	}, waitTimeout, 10*time.Millisecond)

	// 開き直すのは、RetryInterval (20 ミリ秒) に 1 回までである。実際に待った時間から、開く回数の上限を求める。
	// 開き直しはセグメントを作り直すので、混んだ CI では 1 回に時間がかかる。決まった時間だけ眠らず、開き直すまで待つ (#712)。
	started := time.Now()
	require.Eventually(t, func() bool {
		return opened.Load() >= 2
	}, waitTimeout, 10*time.Millisecond, "it keeps reopening")
	time.Sleep(200 * time.Millisecond)
	cancel(nil)
	_ = wait(t, done)
	limit := int64(time.Since(started)/(20*time.Millisecond)) + 2
	assert.LessOrEqual(t, int64(opened.Load()), limit)
	assert.LessOrEqual(t, int64(f.segments(t, reservationID)), limit)
}

// TestRecorderSkipsUntilPlannedStart は、チューニングのために早く開いたストリームを予定の開始まで読み捨て、
// TS パケットの区切りから書き始めることを確かめる (#847)。開始前のマージンが、チューニングにかかる時間のぶん長くならない。
// 読み捨てる長さは最初に決め、経過時間で測るので、読み捨てている間に壁時計が戻っても、読み捨て続けない。
func TestRecorderSkipsUntilPlannedStart(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// jump は、2 回目に読んだ後に壁時計を動かす幅。経過時間は動かさない。
		jump time.Duration
	}{
		{name: "壁時計が飛ばない", jump: 0},
		// 戻った壁時計で予定の開始を待つと、データを流し終えるまで (1000 秒余り) 読み捨て続け、何も書けない。
		{name: "読み捨てている間に壁時計が 1 時間戻る", jump: -time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			// 1 回の読み込みで 1000 バイト (パケットの区切りではない) を返し、そのたびに 1 秒進める。
			// チューニングにかかる時間 (5 秒) の前に開くので、5 回読むと予定の開始になる。
			reads := 0
			stream := &tickingStream{data: data, chunk: 1000, tick: func() {
				f.clock.Advance(time.Second)
				reads++
				if reads == 2 && tt.jump != 0 {
					f.clock.Set(f.clock.Now().Add(tt.jump))
				}
			}}
			recorder, dir := f.newRecorder(t, stream, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Add(-recording.DefaultTuningTime).Sub(f.clock.Now()))

			// 5000 バイトを読み捨て、次のパケットの区切り (27 パケット目の終わり) まで捨てる。
			skipped := 27 * 188
			require.Eventually(t, func() bool {
				files := f.files(t, reservationID)
				if len(files) != 1 {
					return false
				}
				info, err := os.Stat(filepath.Join(dir, files[0].path))

				return err == nil && info.Size() == int64(len(data)-skipped)
			}, waitTimeout, 10*time.Millisecond)
			var recordedFrom time.Time
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT recorded_from FROM recordings WHERE reservation_id = $1", reservationID).
				Scan(&recordedFrom))
			assert.True(t, plannedStart().Equal(recordedFrom), "recorded_from %s", recordedFrom)
			f.clock.Advance(time.Hour - tt.jump)
			require.NoError(t, wait(t, done))

			written, err := os.ReadFile(filepath.Join(dir, f.files(t, reservationID)[0].path))
			require.NoError(t, err)
			assert.True(t, bytes.Equal(data[skipped:], written), "the segment starts at the packet boundary after the planned start")
		})
	}
}

// TestRecorderReopensWhenInterruptedBeforePlannedStart は、予定の開始まで読み捨てている間にストリームが切れたら、
// 開き直して、改めて予定の開始まで読み捨ててから書き始めることを確かめる (#847)。
func TestRecorderReopensWhenInterruptedBeforePlannedStart(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	// 最初のストリームは、2 回読ませて (2 秒進めて) 切れる。開き直したストリームは、残りの 3 秒を読み捨てさせる。
	stream := &interruptedStream{first: data[:2000], data: data, chunk: 1000, tick: func() { f.clock.Advance(time.Second) }, opened: atomic.Int32{}}
	recorder, dir := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Add(-recording.DefaultTuningTime).Sub(f.clock.Now()))

	// 開き直したストリームで 3000 バイトを読み捨て、次のパケットの区切り (16 パケット目の終わり) まで捨てる。
	skipped := 16 * 188
	require.Eventually(t, func() bool {
		files := f.files(t, reservationID)
		if len(files) != 1 {
			return false
		}
		info, err := os.Stat(filepath.Join(dir, files[0].path))

		return err == nil && info.Size() == int64(len(data)-skipped)
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	assert.Equal(t, int32(2), stream.opened.Load())
	// 書き始める前に切れたので、セグメントは 1 つだけ作る。
	assert.Equal(t, 1, f.segments(t, reservationID))
	written, err := os.ReadFile(filepath.Join(dir, f.files(t, reservationID)[0].path))
	require.NoError(t, err)
	assert.True(t, bytes.Equal(data[skipped:], written), "the segment starts at the packet boundary after the planned start")
}

// interruptedStream は、最初に開いたときは first を流して切れ、開き直したら data を流す。1 回の読み込みで chunk バイトまで流し、tick を呼ぶ。
type interruptedStream struct {
	first  []byte
	data   []byte
	chunk  int
	tick   func()
	opened atomic.Int32
}

var _ recording.StreamOpener = new(interruptedStream)

func (s *interruptedStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.opened.Add(1) == 1 {
		return &tickingReader{done: ctx.Done(), closed: make(chan struct{}), once: sync.Once{}, rest: s.first, chunk: s.chunk, tick: s.tick, cut: true}, nil
	}

	return (&tickingStream{data: s.data, chunk: s.chunk, tick: s.tick}).OpenServiceStream(ctx, 0, nil)
}

// tickingStream は、1 回の読み込みで chunk バイトまで data を流し、tick を呼ぶ。流し終えたら、閉じるか ctx が終わるまで待つ。
type tickingStream struct {
	data  []byte
	chunk int
	tick  func()
}

var _ recording.StreamOpener = new(tickingStream)

func (s *tickingStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	return &tickingReader{done: ctx.Done(), closed: make(chan struct{}), once: sync.Once{}, rest: s.data, chunk: s.chunk, tick: s.tick, cut: false}, nil
}

type tickingReader struct {
	done   <-chan struct{}
	closed chan struct{}
	once   sync.Once
	rest   []byte
	chunk  int
	tick   func()
	// cut なら、流し終えたら待たずに切れる。
	cut bool
}

func (r *tickingReader) Read(p []byte) (int, error) {
	if len(r.rest) == 0 {
		if r.cut {
			return 0, io.ErrUnexpectedEOF
		}
		select {
		case <-r.done:
			return 0, context.Canceled
		case <-r.closed:
			return 0, io.ErrClosedPipe
		}
	}
	n := copy(p[:min(len(p), r.chunk)], r.rest)
	r.rest = r.rest[n:]
	r.tick()

	return n, nil
}

func (r *tickingReader) Close() error {
	r.once.Do(func() { close(r.closed) })

	return nil
}

// TestRecorderReconnects は、予定の終わりの前にストリームが切れたら、開き直して新しいセグメントに書き、
// 途切れた区間を記録して、録画を一部のみにすることを確かめる (設計書 5.2、5.5)。
func TestRecorderReconnects(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	opened := new(atomic.Int32)
	recorder, _ := f.newRecorder(t, flakyStream{head: data[:188*1000], data: data, opened: opened, reopen: func() { f.clock.Advance(time.Second) }}, nil)
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
// 終了時刻が決まったら、決まった終わりと後ろのマージンまで録って止めることを確かめる (設計書 5.1)。
func TestRecorderExtendsWhileEndIsUndecided(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	plannedEnd := programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second)
	tests := []struct {
		name string
		// stop は、延長を始めた後に、録画を止める出来事を起こし、止まる時刻を返す。
		stop func(t *testing.T, f *fixture, reservationID uuid.UUID) time.Time
	}{
		{name: "決まった終わりが過ぎていれば止める", stop: func(t *testing.T, f *fixture, _ uuid.UUID) time.Time {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = false WHERE service_id = $1 AND event_id = 6", grService(0))
			require.NoError(t, err)

			return f.clock.Now()
		}},
		{name: "決まった終わりが先なら、そこまで録る", stop: func(t *testing.T, f *fixture, reservationID uuid.UUID) time.Time {
			t.Helper()

			// 延長を始めた後に、終了時刻が 25 分後に決まった (#512)。
			decided := f.clock.Now().Add(25 * time.Minute)
			_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = false, end_at = $2 WHERE service_id = $1 AND event_id = 6",
				grService(0), decided)
			require.NoError(t, err)
			stopsAt := decided.Add(domain.DefaultMarginSeconds * time.Second)

			// 決まった終わりとマージンの直前までは、録り続ける。
			f.clock.Advance(stopsAt.Add(-time.Second).Sub(f.clock.Now()))
			require.Never(t, func() bool {
				got, _ := f.recording(t, reservationID)

				return got.state != string(recording.StateRecording)
			}, time.Second, 10*time.Millisecond)

			return stopsAt
		}},
		{name: "未定のままでも、予定の終わりから 3 時間で止める", stop: func(*testing.T, *fixture, uuid.UUID) time.Time {
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
				return len(f.files(t, reservationID)) == 1
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

			stopsAt := tt.stop(t, f, reservationID)
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

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	plannedEnd := programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
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

// TestRecorderCopiesProgramAudios は、録画を始めたときの番組の音声の構成 (#284) と映像の解像度 (#806) を録画に残し、
// 録画中に番組の構成が変わっても書き換えないことを確かめる。
func TestRecorderCopiesProgramAudios(t *testing.T) {
	t.Parallel()

	const bilingual = `[{"componentType":3,"isMain":true,"samplingRate":48000,"langs":["jpn"]},` +
		`{"componentType":3,"isMain":false,"samplingRate":48000,"langs":["eng"]}]`
	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	_, err = f.pool.Exec(t.Context(), "UPDATE programs SET audios = $2, video_resolution = '2160p' WHERE service_id = $1 AND event_id = 6",
		grService(0), bilingual)
	require.NoError(t, err)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)
	_, err = f.pool.Exec(t.Context(), "UPDATE programs SET audios = '[]', video_resolution = NULL WHERE service_id = $1 AND event_id = 6", grService(0))
	require.NoError(t, err)

	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))
	var same bool
	var resolution *string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT audios = $2::jsonb, video_resolution FROM recordings WHERE reservation_id = $1",
		reservationID, bilingual).Scan(&same, &resolution))
	assert.True(t, same)
	assert.Equal(t, new("2160p"), resolution)
}

// TestRecorderCopiesRuleAndGenres は、録画を始めたときに、予約のジャンルと、一致したルールのうち最も古いものとその名前を、
// 録画に残すことを確かめる (#468)。予約やルールを消しても、録画をルールとジャンルで絞れるようにするため。
func TestRecorderCopiesRuleAndGenres(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	_, err = f.pool.Exec(t.Context(), `UPDATE reservations SET genres = '[{"lv1":7,"lv2":0}]' WHERE id = $1`, reservationID)
	require.NoError(t, err)
	older, newer := id.New(), id.New()
	for _, r := range []struct {
		id        uuid.UUID
		name      string
		createdAt time.Time
	}{{newer, "新しいルール", testNow()}, {older, "古いルール", testNow().Add(-time.Hour)}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay, created_at, updated_at)
			VALUES ($1, $2, '', true, '{"version":1,"conditions":{}}', '{}', false, true, $3, $3)`, r.id, r.name, r.createdAt)
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), "INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)", reservationID, r.id)
		require.NoError(t, err)
	}
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	var ruleID *uuid.UUID
	var ruleName *string
	var genres string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT rule_id, rule_name, genres::text FROM recordings WHERE reservation_id = $1",
		reservationID).Scan(&ruleID, &ruleName, &genres))
	assert.Equal(t, &older, ruleID)
	assert.Equal(t, new("古いルール"), ruleName)
	assert.JSONEq(t, `[{"lv1":7,"lv2":0}]`, genres)
}

// TestRecorderRecordsGapOnResume は、引き継ぎや再開で続きを録るとき、前のセグメントを書き終えてからの間を、途切れた区間として記録することを確かめる。
func TestRecorderRecordsGapOnResume(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := newDrainedStream(data)
	recorder, _ := f.newRecorder(t, stream, nil)
	stop, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	// 最初のセグメントに書き終えてから止める。何も書けていないセグメントは、録れた範囲に数えない。
	stream.wait(t)
	stop(recording.ErrLeaseLost)
	require.ErrorIs(t, wait(t, done), recording.ErrLeaseLost)

	// 1 分後に、別の mikan が続きを録る。
	f.clock.Advance(time.Minute)
	_, done = f.session(t, recorder, reservationID)
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	// 続きのセグメントにも書き終えてから、予定の終わりを過ぎさせる。
	stream.wait(t)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
	// 続きを録るときは、録画の開始を通知し直さない。一部のみの録画は、録画履歴に記録しない。
	events := f.events(t)
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	assert.Equal(t, []string{string(domain.NotificationKindRecordingStarted), string(domain.NotificationKindRecordingCompleted)}, kinds)
	assert.Zero(t, f.historyCount(t))
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

	stream := newAttemptedStream(failingStream{err: errors.New("connection refused")})
	recorder, _ := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.wait(t)
	f.clock.Advance(time.Hour)

	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, recorded{state: string(recording.StateFailed), failureReason: new(string(recording.FailureTunerServerUnreachable)), drops: 0}, got)
}

func (f *fixture) historyCount(t *testing.T) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_history").Scan(&n))

	return n
}

type event struct {
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

func (f *fixture) events(t *testing.T) []event {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT kind, payload FROM notification_events ORDER BY occurred_at, id")
	require.NoError(t, err)
	defer rows.Close()
	events := []event{}
	for rows.Next() {
		var e event
		require.NoError(t, rows.Scan(&e.Kind, &e.Payload))
		events = append(events, e)
	}
	require.NoError(t, rows.Err())

	return events
}

// TestRecorderEmitsEvents は、録画を始めたとき、終えたとき、失敗したときに、通知するイベントを記録し、
// 正常に終えた録画だけを録画履歴に記録することを確かめる (設計書 4.4、6.2)。
func TestRecorderEmitsEvents(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// tuner は、ストリームと、録画がそのストリームを使い始めるまで待つ関数を返す。
		tuner       func() (recording.StreamOpener, func(t *testing.T))
		wantKinds   []string
		wantHistory int
	}{
		{
			name: "録画を終えた",
			tuner: func() (recording.StreamOpener, func(t *testing.T)) {
				stream := newDrainedStream(data)

				return stream, stream.wait
			},
			wantKinds: []string{string(domain.NotificationKindRecordingStarted), string(domain.NotificationKindRecordingCompleted)}, wantHistory: 1,
		},
		{
			name: "チューナーサーバーに接続できず失敗した",
			tuner: func() (recording.StreamOpener, func(t *testing.T)) {
				stream := newAttemptedStream(failingStream{err: errors.New("connection refused")})

				return stream, stream.wait
			},
			wantKinds: []string{string(domain.NotificationKindRecordingStarted), string(domain.NotificationKindRecordingFailed)}, wantHistory: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream, started := tt.tuner()
			recorder, _ := f.newRecorder(t, stream, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			// 録画がストリームを使い始めてから、予定の終わりの後まで進める (#329)。
			started(t)
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))

			events := f.events(t)
			kinds := make([]string, 0, len(events))
			for _, e := range events {
				kinds = append(kinds, e.Kind)
				assert.Equal(t, reservationName(t, f, reservationID), e.Payload["name"], e.Kind)
				assert.NotEmpty(t, e.Payload["recordingId"], e.Kind)
				assert.NotEmpty(t, e.Payload["serviceName"], e.Kind)
			}
			assert.Equal(t, tt.wantKinds, kinds)
			last := events[len(events)-1].Payload
			switch last["state"] {
			case string(recording.StateFailed):
				assert.Equal(t, string(recording.FailureTunerServerUnreachable), last["reason"])
			default:
				// Webhook で録画のファイルを示せるよう、完了の通知にはセグメントのプール内のパスを載せる。
				files, ok := last["files"].([]any)
				require.True(t, ok)
				require.Len(t, files, 1)
				file, ok := files[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, poolID, file["poolId"])
			}
			assert.Equal(t, tt.wantHistory, f.historyCount(t))

			// 録画を消しても、録画履歴は残す (設計書 4.4)。
			_, err := f.pool.Exec(t.Context(), "DELETE FROM recordings WHERE reservation_id = $1", reservationID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHistory, f.historyCount(t))
		})
	}
}

func reservationName(t *testing.T, f *fixture, reservationID uuid.UUID) string {
	t.Helper()

	var name string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT name FROM reservations WHERE id = $1", reservationID).Scan(&name))

	return name
}

// TestRecorderUsesPoolSettings は、録画を書くプールを、設定した優先順位と容量の閾値で選ぶことを確かめる (設計書 5.6)。
func TestRecorderUsesPoolSettings(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// settings は、プール ID ごとの優先順位と容量の閾値。
		settings map[string][2]int64
		want     string
	}{
		{name: "設定がなければ ID の順", settings: map[string][2]int64{}, want: "a"},
		{name: "優先順位の高いプール", settings: map[string][2]int64{"b": {10, 0}}, want: "b"},
		{name: "書くと閾値を下回るプールは選ばない", settings: map[string][2]int64{"b": {10, 1 << 40}}, want: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			for id, setting := range tt.settings {
				_, err := f.pool.Exec(t.Context(), "INSERT INTO storage_pools (id, priority, min_free_bytes, updated_at) VALUES ($1, $2, $3, $4)",
					id, setting[0], setting[1], f.clock.Now())
				require.NoError(t, err)
			}
			tokyo, err := time.LoadLocation("Asia/Tokyo")
			require.NoError(t, err)
			store := kago.New(map[string]string{"a": t.TempDir(), "b": t.TempDir()}, func(string) (uint64, error) { return 1 << 40, nil })
			recorder := recording.NewRecorder(f.pool, f.clock, onceStream{data: data}, store, nil, tokyo, slog.New(slog.DiscardHandler),
				&recording.RecorderOptions{TuningTime: 0, Tick: 10 * time.Millisecond, RetryInterval: 20 * time.Millisecond})
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			require.Eventually(t, func() bool {
				return len(f.files(t, reservationID)) == 1
			}, waitTimeout, 10*time.Millisecond)
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))

			var got string
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT pool_id FROM recording_files WHERE kind = 'segment' AND recording_id = (SELECT id FROM recordings WHERE reservation_id = $1)",
				reservationID).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRecorderContinuesAfterHandover は、止まる mikan から引き継いだときは、古い mikan と重なって録るので、
// 途切れた区間を記録しないことを確かめる (設計書 5.4)。
func TestRecorderContinuesAfterHandover(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	assignmentID := id.New()
	stopOld, oldDone := f.sessionWith(t, recorder, &recording.Assignment{
		ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 1, HandedOver: false, PreviousLeaseExpiresAt: nil,
	})
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)

	// 古い mikan が書いている間に、新しい mikan が引き継いで、2 つ目のセグメントを書き始める。
	f.clock.Advance(time.Minute)
	_, newDone := f.sessionWith(t, recorder, &recording.Assignment{
		ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 2, HandedOver: true, PreviousLeaseExpiresAt: nil,
	})
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	stopOld(recording.ErrHandedOver)
	require.ErrorIs(t, wait(t, oldDone), recording.ErrHandedOver)

	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, newDone))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompleted), got.state)
	assert.Equal(t, gapsAndLost{gaps: 0, programLost: false}, f.gapsAndLost(t, reservationID))
}

// TestRecorderTakesOverFromCrashedMikan は、落ちた mikan の代わりに取ったときは、書きかけのセグメントを、
// ファイルの大きさと、最後に書き込んだ時刻で閉じ、そこからを途切れた区間とすることを確かめる (設計書 5.4)。
func TestRecorderTakesOverFromCrashedMikan(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, dir := f.newRecorder(t, onceStream{data: data}, nil)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))

	// 落ちた mikan が、1000 バイトを書いたところで止まり、セグメントを書きかけのまま残したことにする。
	recordingID := id.New()
	startedAt := f.clock.Now()
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		SELECT $1, id, service_id, event_id, name, description, start_at, end_at, 'recording', false, $3, $3, $3
		FROM reservations WHERE id = $2`, recordingID, reservationID, startedAt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crashed.m2ts"), make([]byte, 1000), 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, 'crashed.m2ts', 0, $4, NULL, $4)`, id.New(), recordingID, poolID, startedAt)
	require.NoError(t, err)

	// 落ちた mikan は、録画を始めてから 1 分後に最後に書き込み、その後に落ちた。
	lastRenewal := startedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "crashed.m2ts"), lastRenewal, lastRenewal))
	f.clock.Advance(5 * time.Minute)
	_, done := f.sessionWith(t, recorder, &recording.Assignment{
		ID: id.New(), ReservationID: reservationID, Priority: recording.Priority, Generation: 2,
		HandedOver: false, PreviousLeaseExpiresAt: new(lastRenewal.Add(recording.DefaultLeaseTTL)),
	})
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	var crashed struct {
		size    int64
		endedAt time.Time
	}
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT size, ended_at FROM recording_files WHERE path = 'crashed.m2ts'").Scan(&crashed.size, &crashed.endedAt))
	assert.Equal(t, int64(1000), crashed.size)
	assert.True(t, lastRenewal.Equal(crashed.endedAt), crashed.endedAt)
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
	var gapFrom time.Time
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT (gaps->0->>'from')::timestamptz FROM recordings WHERE id = $1", recordingID).Scan(&gapFrom))
	assert.True(t, lastRenewal.Equal(gapFrom), gapFrom)
}

// failOnceStream は、最初に開くときだけ失敗し、2 回目からは TS を 1 回だけ流す。
type failOnceStream struct {
	data   []byte
	opened *atomic.Int32
}

func (s failOnceStream) OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.opened.Add(1) == 1 {
		return nil, errors.New("connection refused")
	}

	return onceStream{data: s.data}.OpenServiceStream(ctx, serviceID, opts)
}

// TestRecorderHandoverGap は、引き継いだ mikan がストリームをすぐに開けなくても、古い mikan が書いている間は途切れた区間にせず、
// 古い mikan がもう止まっていれば、そのセグメントの終わりからを途切れた区間にすることを確かめる (設計書 5.4)。
func TestRecorderHandoverGap(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// oldEnded は、古い mikan のセグメントを、止まって閉じたことにするかどうか。
		oldEnded bool
		wantGaps int
	}{
		{name: "古い mikan がまだ書いている", oldEnded: false, wantGaps: 0},
		{name: "古い mikan がもう止まった", oldEnded: true, wantGaps: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, dir := f.newRecorder(t, failOnceStream{data: data, opened: new(atomic.Int32)}, nil)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))

			// 古い mikan が書いているセグメントを用意する。
			recordingID := id.New()
			startedAt := f.clock.Now()
			_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
				state, protected, recorded_from, created_at, updated_at)
				SELECT $1, id, service_id, event_id, name, description, start_at, end_at, 'recording', false, $3, $3, $3
				FROM reservations WHERE id = $2`, recordingID, reservationID, startedAt)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "old.m2ts"), make([]byte, 1000), 0o600))
			var endedAt *time.Time
			// 古い mikan が書いている間は、ファイルに書き込み続ける。
			touch := func() {
				require.NoError(t, os.Chtimes(filepath.Join(dir, "old.m2ts"), f.clock.Now(), f.clock.Now()))
			}
			if tt.oldEnded {
				endedAt = new(startedAt.Add(time.Minute))
			}
			_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
				VALUES ($1, $2, 'segment', $3, 'old.m2ts', 1000, $4, $5, $4)`, id.New(), recordingID, poolID, startedAt, endedAt)
			require.NoError(t, err)

			f.clock.Advance(2 * time.Minute)
			if !tt.oldEnded {
				touch()
			}
			_, done := f.sessionWith(t, recorder, &recording.Assignment{
				ID: id.New(), ReservationID: reservationID, Priority: recording.Priority, Generation: 2, HandedOver: true, PreviousLeaseExpiresAt: nil,
			})
			require.Eventually(t, func() bool {
				return f.segments(t, reservationID) == 2
			}, waitTimeout, 10*time.Millisecond)
			if !tt.oldEnded {
				// 古い mikan は、新しい mikan がストリームを開いたのを見て止まる。
				_, err := f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = $1 WHERE path = 'old.m2ts'", f.clock.Now())
				require.NoError(t, err)
			}
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))
			assert.Equal(t, gapsAndLost{gaps: tt.wantGaps, programLost: false}, f.gapsAndLost(t, reservationID))
		})
	}
}

// TestRecorderHandoverEdgeCases は、引き継いだのに録れなかったときや、古い mikan が引き継ぎの最中に落ちたときに、
// 途切れた区間と録画の状態を正しく記録することを確かめる (設計書 5.4)。
func TestRecorderHandoverEdgeCases(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// oldFile は、古い mikan が書いたセグメントを用意するかどうか。
		oldFile bool
		// oldEnded は、古い mikan のセグメントを閉じたことにするかどうか。閉じていなければ、書き込みが 5 分前に止まった (落ちた) ことにする。
		oldEnded bool
		// tuner は、新しい mikan のストリームと、録画がそのストリームを使い始めるまで待つ関数を返す。
		tuner     func() (recording.StreamOpener, func(t *testing.T))
		wantState recording.State
		wantGaps  int
	}{
		{
			name: "古い mikan が止まった後、新しい mikan がストリームを開けないまま終わる", oldFile: true, oldEnded: true,
			tuner: func() (recording.StreamOpener, func(t *testing.T)) {
				stream := newAttemptedStream(failingStream{err: errors.New("connection refused")})

				return stream, stream.wait
			},
			wantState: recording.StatePartial, wantGaps: 1,
		},
		{
			name: "古い mikan がまだ何も書いていないときに引き継ぎ、開けないまま終わる", oldFile: false, oldEnded: false,
			tuner: func() (recording.StreamOpener, func(t *testing.T)) {
				stream := newAttemptedStream(failingStream{err: errors.New("connection refused")})

				return stream, stream.wait
			},
			wantState: recording.StateFailed, wantGaps: 0,
		},
		{
			name: "古い mikan が引き継ぎの最中に落ちた", oldFile: true, oldEnded: false,
			tuner: func() (recording.StreamOpener, func(t *testing.T)) {
				stream := newDrainedStream(data)

				return stream, stream.wait
			},
			wantState: recording.StatePartial, wantGaps: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream, started := tt.tuner()
			recorder, dir := f.newRecorder(t, stream, nil)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			startedAt := f.clock.Now()
			if tt.oldFile {
				recordingID := id.New()
				_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
					state, protected, recorded_from, created_at, updated_at)
					SELECT $1, id, service_id, event_id, name, description, start_at, end_at, 'recording', false, $3, $3, $3
					FROM reservations WHERE id = $2`, recordingID, reservationID, startedAt)
				require.NoError(t, err)
				path := filepath.Join(dir, "old.m2ts")
				require.NoError(t, os.WriteFile(path, make([]byte, 1000), 0o600))
				lastWrite := startedAt.Add(time.Minute)
				require.NoError(t, os.Chtimes(path, lastWrite, lastWrite))
				var endedAt *time.Time
				if tt.oldEnded {
					endedAt = &lastWrite
				}
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
					VALUES ($1, $2, 'segment', $3, 'old.m2ts', 1000, $4, $5, $4)`, id.New(), recordingID, poolID, startedAt, endedAt)
				require.NoError(t, err)
			}

			f.clock.Advance(6 * time.Minute)
			_, done := f.sessionWith(t, recorder, &recording.Assignment{
				ID: id.New(), ReservationID: reservationID, Priority: recording.Priority, Generation: 2, HandedOver: true, PreviousLeaseExpiresAt: nil,
			})
			// 録画がストリームを使い始めてから、予定の終わりの後まで進める。決まった時間だけ待つと、混んだ CI では、
			// 録画が始まる前に時計が進み、録画が予定の過ぎた予約として何もせずに戻ることがある (#329)。
			started(t)
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))

			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(tt.wantState), got.state)
			assert.Equal(t, gapsAndLost{gaps: tt.wantGaps, programLost: false}, f.gapsAndLost(t, reservationID))
			if tt.oldFile {
				var closed bool
				require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT ended_at IS NOT NULL FROM recording_files WHERE path = 'old.m2ts'").Scan(&closed))
				assert.True(t, closed, "the abandoned segment is closed")
			}
		})
	}
}

// TestRecorderLeavesFinishingToCurrentGeneration は、ほかの mikan が割り振りを取った後に予定の終わりが来ても、
// 古い世代の mikan は録画を終えず、今の世代の mikan に任せることを確かめる。
func TestRecorderLeavesFinishingToCurrentGeneration(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.assignOne(t)
	var assignmentID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "UPDATE recording_assignments SET generation = 1 WHERE reservation_id = $1 RETURNING id",
		reservationID).Scan(&assignmentID))
	recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
	_, done := f.sessionWith(t, recorder, &recording.Assignment{
		ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 1, HandedOver: false, PreviousLeaseExpiresAt: nil,
	})
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)

	// ほかの mikan が、世代 2 で取った。
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_assignments SET generation = 2 WHERE id = $1", assignmentID)
	require.NoError(t, err)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateRecording), got.state, "the current generation finishes the recording")
	assert.Empty(t, f.events(t)[1:], "the old generation does not notify the completion")
}

// TestRecorderRecordsTrailingGap は、ストリームが切れたまま開き直せずに予定の終わりが来たら、
// 最後のセグメントの終わりから予定の終わりまでを、途切れた区間にすることを確かめる。
func TestRecorderRecordsTrailingGap(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	opened := new(atomic.Int32)
	// 最初に開いたストリームは少しだけ流して切れ、その後は開けない。
	stream := flakyStream{head: data[:188*1000], data: nil, opened: opened, reopen: func() {}}
	recorder, _ := f.newRecorder(t, cutThenFail{flaky: stream}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return opened.Load() >= 2
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
	assert.True(t, f.clock.Now().Equal(gap.To), "the gap lasts until the end")
}

// cutThenFail は、最初は flaky の最初のストリームを返し、2 回目からは開けない。
type cutThenFail struct{ flaky flakyStream }

func (s cutThenFail) OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.flaky.opened.Load() == 0 {
		return s.flaky.OpenServiceStream(ctx, serviceID, opts)
	}
	s.flaky.opened.Add(1)

	return nil, errors.New("connection refused")
}

// TestRecorderWaitsThroughDatabaseErrors は、録画を始めるのを待つ間に、予約を一時的に読めなくても、録画を諦めないことを確かめる (#516)。
// 諦めると、割り振りが終わったことになり、録画されない。
func TestRecorderWaitsThroughDatabaseErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, f.tuner, nil)
	// 予約を読むクエリが失敗するよう、サービスの表の名前を一時的に変える。
	_, err := f.pool.Exec(t.Context(), "ALTER TABLE services RENAME TO services_unavailable")
	require.NoError(t, err)
	_, done := f.session(t, recorder, reservationID)

	select {
	case err := <-done:
		t.Fatalf("the recording session gave up on a database error: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	_, err = f.pool.Exec(t.Context(), "ALTER TABLE services_unavailable RENAME TO services")
	require.NoError(t, err)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		_, started := f.recording(t, reservationID)

		return started
	}, waitTimeout, 10*time.Millisecond)
}

// TestRecorderRecordsLateStartAsGap は、録画を始めるのが予定の開始より 30 秒を超えて遅れたら、冒頭の欠けを途切れた区間にし、
// 一部のみ (partial) にすることを確かめる (#517)。完了にすると録画履歴に載り、重複の回避で再放送も録らなくなる。
func TestRecorderRecordsLateStartAsGap(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// late は、予定の開始から、mikan が割り振りを取るまでの遅れ。
		late      time.Duration
		wantState recording.State
		wantGaps  int
	}{
		{name: "2 分遅れた", late: 2 * time.Minute, wantState: recording.StatePartial, wantGaps: 1},
		{name: "遅れが 30 秒以内", late: 20 * time.Second, wantState: recording.StateCompleted, wantGaps: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
			f.clock.Advance(plannedStart().Add(tt.late).Sub(f.clock.Now()))
			_, done := f.session(t, recorder, reservationID)
			require.Eventually(t, func() bool {
				return len(f.files(t, reservationID)) == 1
			}, waitTimeout, 10*time.Millisecond)

			f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
			require.NoError(t, wait(t, done))
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(tt.wantState), got.state)
			assert.Equal(t, gapsAndLost{gaps: tt.wantGaps, programLost: false}, f.gapsAndLost(t, reservationID))
		})
	}
}

// TestRecorderReselectsPoolForRemainingTime は、録画の途中で開き直すときに、保存先を、番組の全長ではなく残りの時間の
// 推定サイズで選び直すことを確かめる (#519)。全長で選ぶと、終盤ほど選べるプールがなくなり、それまで録れていても失敗していた。
func TestRecorderReselectsPoolForRemainingTime(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	// 始めは十分に空いているが、終わりの 5 分前に切れて開き直すときには、残りの 5 分の分 (約 0.9 GB) しか空いていない。
	free := new(atomic.Uint64)
	free.Store(1 << 40)
	opened := new(atomic.Int32)
	stream := flakyStream{head: data[:188*1000], data: data, opened: opened, reopen: func() {
		free.Store(2 << 30)
		f.clock.Advance(programStart().Add(25 * time.Minute).Sub(f.clock.Now()))
	}}
	recorder, _ := f.newRecorder(t, stream, func(string) (uint64, error) { return free.Load(), nil })
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)

	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
}

// TestRecorderRetriesSegmentAfterStorageError は、録画の途中で新しいセグメントを作れなくても、録画を失敗にせず、
// 作れるようになるまでやり直して続きを録り、途切れた区間を記録することを確かめる (#519)。
func TestRecorderRetriesSegmentAfterStorageError(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	// 開き直すときに、一時的に空きがなくなる。
	free := new(atomic.Uint64)
	free.Store(1 << 40)
	opened := new(atomic.Int32)
	stream := flakyStream{head: data[:188*1000], data: data, opened: opened, reopen: func() {
		if opened.Load() == 2 {
			free.Store(0)
		}
		f.clock.Advance(time.Second)
	}}
	recorder, _ := f.newRecorder(t, stream, func(string) (uint64, error) { return free.Load(), nil })
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return opened.Load() >= 2
	}, waitTimeout, 10*time.Millisecond)

	// 録画を失敗にせず、やり直している間に空きができる。
	time.Sleep(100 * time.Millisecond)
	got, _ := f.recording(t, reservationID)
	require.Equal(t, string(recording.StateRecording), got.state)
	free.Store(1 << 40)
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)

	require.NoError(t, wait(t, done))
	got, _ = f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
}

// TestRecorderRetriesSegmentOnResume は、続きを録る mikan が、最初のセグメントを作れなくても、前の mikan が録った分があれば、
// 録画を失敗にせず、作れるようになるまでやり直すことを確かめる (#519)。
func TestRecorderRetriesSegmentOnResume(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	free := new(atomic.Uint64)
	free.Store(1 << 40)
	stream := newDrainedStream(data)
	recorder, _ := f.newRecorder(t, stream, func(string) (uint64, error) { return free.Load(), nil })
	stop, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.wait(t)
	stop(recording.ErrLeaseLost)
	require.ErrorIs(t, wait(t, done), recording.ErrLeaseLost)

	// 続きを録る mikan が始めるときに、保存先に空きがない。
	free.Store(0)
	f.clock.Advance(time.Minute)
	_, done = f.session(t, recorder, reservationID)
	time.Sleep(200 * time.Millisecond)
	got, _ := f.recording(t, reservationID)
	require.Equal(t, string(recording.StateRecording), got.state)

	free.Store(1 << 40)
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 2
	}, waitTimeout, 10*time.Millisecond)
	stream.wait(t)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))
	got, _ = f.recording(t, reservationID)
	assert.Equal(t, string(recording.StatePartial), got.state)
}

// TestRecorderReleasesWhenUnableToRecord は、録画できない理由がこの mikan にあり、ほかに mikan が動いていれば、
// 録画を失敗にせず、ErrRecorderUnavailable を返して割り振りを手放させることを確かめる (#524)。
// 理由は、ほかの mikan も録れなかったときのために、録画の行に残す。ほかに mikan がいなければ、今までどおり自分で扱う。
func TestRecorderReleasesWhenUnableToRecord(t *testing.T) {
	t.Parallel()

	onePeer := func(context.Context) (int, error) { return 1, nil }
	tests := []struct {
		name string
		// prepare は、録画できない状態にし、Recorder と、録画が開こうとするのを待つストリームを作る。
		prepare func(t *testing.T, f *fixture, reservationID uuid.UUID) (*recording.Recorder, *attemptedStream)
		reason  recording.FailureReason
		// advance は、録画が開こうとしてから進める時間。
		advance time.Duration
	}{
		{name: "プールがマウントされていない", prepare: func(t *testing.T, f *fixture, reservationID uuid.UUID) (*recording.Recorder, *attemptedStream) {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), `UPDATE reservations SET settings = '{"poolId": "elsewhere"}' WHERE id = $1`, reservationID)
			require.NoError(t, err)
			stream := newAttemptedStream(f.tuner)
			recorder, _ := f.newRecorderWithPeers(t, stream, nil, onePeer)

			return recorder, stream
		}, reason: recording.FailureStorageUnavailable, advance: 0},
		{name: "チューナーサーバーにつながらないまま 30 秒が過ぎた", prepare: func(t *testing.T, f *fixture, _ uuid.UUID) (*recording.Recorder, *attemptedStream) {
			t.Helper()

			stream := newAttemptedStream(failingStream{err: errors.New("connection refused")})
			recorder, _ := f.newRecorderWithPeers(t, stream, nil, onePeer)

			return recorder, stream
		}, reason: recording.FailureTunerServerUnreachable, advance: 31 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, stream := tt.prepare(t, f, reservationID)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			if tt.advance > 0 {
				// つながらなくなった時刻を覚えてから、時計を進める。開こうとした合図は、つながらなかったと分かる前に出るので、
				// 2 回目に開こうとするまで待つ。
				stream.wait(t)
				stream.wait(t)
				f.clock.Advance(tt.advance)
			}

			require.ErrorIs(t, wait(t, done), recording.ErrRecorderUnavailable)
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, recorded{state: string(recording.StateRecording), failureReason: new(string(tt.reason)), drops: 0}, got)
		})
	}
}

// TestRecorderFlushesLowBitrateStreams は、ビットレートが低くても、書き込みのバッファを一定の時間ごとに書き出し、
// 録画ファイルの更新時刻を遅らせないことを確かめる (#525)。遅れると、落ちた mikan のものとみなして早すぎる時刻で閉じ、
// 実際にはない途切れを記録していた。
func TestRecorderFlushesLowBitrateStreams(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := &endingStream{data: data[:188*50], close: make(chan struct{}), opened: new(atomic.Int32)}
	recorder, dir := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return len(f.files(t, reservationID)) == 1
	}, waitTimeout, 10*time.Millisecond)

	// ストリームを閉じる前に、書いた分がファイルに出る。
	segmentPath := f.files(t, reservationID)[0].path
	require.Eventually(t, func() bool {
		info, err := os.Stat(filepath.Join(dir, segmentPath))

		return err == nil && info.Size() == int64(188*50)
	}, waitTimeout, 10*time.Millisecond)

	close(stream.close)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))
}
