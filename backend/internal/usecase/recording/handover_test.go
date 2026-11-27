package recording_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// kindSegment は、録画のファイルのうち、セグメントの種類。
const kindSegment = "segment"

// errUndefinedColumn は、マイグレーションで消えたカラムを古いバイナリが読んだときのエラー (#2195)。DB には届いているので、IsUnavailable ではない。
var errUndefinedColumn = &pgconn.PgError{Code: "42703", Message: `column "rule_unmatched_grace_minutes" does not exist`}

// TestRecorderContinuesHandoverWithoutSettings は、引き継いだ mikan が、セグメントを作る前に録画設定を読めなくても、
// 録画を失敗にせず、続きを録ることを確認する (#2195)。古い mikan は書いている途中なので、セグメントの大きさはまだ 0 である。
// 録画設定を読めなければ、Go の既定値に予約自体の上書きを重ねて書くので、予約で決めたファイル名で書く。
func TestRecorderContinuesHandoverWithoutSettings(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name     string
		defaults func(context.Context, sqlc.DBTX) (*domain.RecordingSettings, error)
	}{
		{
			name: "消えたカラムを読む",
			defaults: func(context.Context, sqlc.DBTX) (*domain.RecordingSettings, error) {
				return nil, errUndefinedColumn
			},
		},
		{
			name: "DB に届かない",
			defaults: func(context.Context, sqlc.DBTX) (*domain.RecordingSettings, error) {
				return nil, &pgconn.PgError{Code: "57P01", Message: "terminating connection due to administrator command"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			_, err := f.pool.Exec(t.Context(), `UPDATE reservations SET settings = '{"fileNameTemplate": "予約のファイル名"}' WHERE id = $1`, reservationID)
			require.NoError(t, err)
			oldRecorder, _ := f.newRecorder(t, onceStream{data: data}, nil)
			assignmentID := id.New()
			stopOld, oldDone := f.sessionWith(t, oldRecorder, &recording.Assignment{
				ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 1, HandedOver: false, PreviousLeaseExpiresAt: nil,
			})
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			require.Eventually(t, func() bool {
				return f.segments(t, reservationID) == 1
			}, waitTimeout, 10*time.Millisecond)
			// 古い mikan は書いている途中なので、セグメントは閉じておらず、大きさも 0 のままである。
			require.Equal(t, []bool{false}, closedSegments(f.files(t, reservationID)))

			f.clock.Advance(time.Minute)
			newRecorder, _ := f.newRecorderWithDefaults(t, f.pool, onceStream{data: data}, nil, nil, slog.New(slog.DiscardHandler), tt.defaults)
			_, newDone := f.sessionWith(t, newRecorder, &recording.Assignment{
				ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 2, HandedOver: true, PreviousLeaseExpiresAt: nil,
			})
			require.Eventually(t, func() bool {
				return f.segments(t, reservationID) == 2
			}, waitTimeout, 10*time.Millisecond, "the new mikan starts a segment of its own")
			got, _ := f.recording(t, reservationID)
			require.Equal(t, string(domain.RecordingStateRecording), got.state)
			stopOld(recording.ErrHandedOver)
			require.ErrorIs(t, wait(t, oldDone), recording.ErrHandedOver)

			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, newDone))
			got, _ = f.recording(t, reservationID)
			assert.Equal(t, string(domain.RecordingStateCompleted), got.state)
			assert.Equal(t, gapsAndLost{gaps: 0, programLost: false}, f.gapsAndLost(t, reservationID))
			for _, file := range f.files(t, reservationID) {
				if file.kind == kindSegment {
					assert.True(t, strings.HasPrefix(file.path, "予約のファイル名"), file.path)
				}
			}
		})
	}
}

// TestRecorderRetriesFirstSegment は、まだ何も録っていない録画でも、セグメントを作れなければ、その場では失敗にせず、
// 作れるようになるまでやり直すことを確認する (#2195)。作れるようになったら、録画を続ける。
func TestRecorderRetriesFirstSegment(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	free := new(atomic.Uint64)
	logs := newMessageRecorder()
	recorder, _ := f.newRecorderWithLogger(t, f.tuner, func(string) (uint64, error) { return free.Load(), nil }, nil, slog.New(logs))
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	logs.waitForMessage(t, "failed to create a recording file"+retrying)
	got, _ := f.recording(t, reservationID)
	require.Equal(t, string(domain.RecordingStateRecording), got.state)

	free.Store(1 << 40)
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 1
	}, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))
	got, _ = f.recording(t, reservationID)
	assert.NotEqual(t, string(domain.RecordingStateFailed), got.state)
}

// closedSegments は、セグメントのそれぞれを閉じたかどうかを返す。
func closedSegments(files []recordedFile) []bool {
	closed := []bool{}
	for _, file := range files {
		if file.kind == kindSegment {
			closed = append(closed, file.closed)
		}
	}

	return closed
}
