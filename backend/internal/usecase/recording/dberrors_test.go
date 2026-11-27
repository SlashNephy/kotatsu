package recording_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// renameColumn は、テーブル table のカラム from を to に改名する。ローリング更新で、新しいスキーマが古いバイナリの読むカラムを消した間を作る (#2195)。
func (f *fixture) renameColumn(t *testing.T, table, from, to string) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", table, from, to))
	require.NoError(t, err)
}

// TestRecorderWaitsThroughErrorsFromDatabase は、録画の途中や終わりに、DB に届いていても DB のエラーで読み書きできない間は、
// 録画を止めずに、最後に読めた予約で予定の終わりに止め、読み書きできるようになってから録画を終えることを確認する (#2195)。
func TestRecorderWaitsThroughErrorsFromDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// table と column は、録画の終わりの前後に読み書きできなくするカラム。
		table  string
		column string
	}{
		{name: "予約を読めない間に予定の終わりが来る", table: "reservations", column: "description"},
		{name: "録画を終えるときに途切れた区間を書けない", table: "recordings", column: "gaps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("../../../testdata/ts/test.ts")
			require.NoError(t, err)
			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			logs := newMessageRecorder()
			stream := newDrainedStream(data)
			recorder, _ := f.newRecorderWithLogger(t, stream, nil, nil, slog.New(logs))
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.wait(t)

			f.renameColumn(t, tt.table, tt.column, tt.column+"_renamed")
			f.clock.Advance(time.Hour)
			logs.waitForMessage(t, "the database operation failed; retrying")
			got, _ := f.recording(t, reservationID)
			require.Equal(t, string(domain.RecordingStateRecording), got.state)
			f.renameColumn(t, tt.table, tt.column+"_renamed", tt.column)

			require.NoError(t, wait(t, done))
			got, _ = f.recording(t, reservationID)
			assert.Equal(t, string(domain.RecordingStateCompleted), got.state)
		})
	}
}

// TestWorkerReleasesWhenSessionStopsBeforeEnd は、Session が予定の終わりの前に録画をやめたら (ErrResumeLater)、
// 割り振りを終わったことにせず手放し、続きを録らせることを確認する (#2195)。終わったことにすると、neko が録画を失敗にする。
func TestWorkerReleasesWhenSessionStopsBeforeEnd(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	sessions := new(atomic.Int32)
	resumed := make(chan struct{})
	stop := f.startWorker(t, id.New(), func(ctx context.Context, _ *recording.Assignment) error {
		if sessions.Add(1) == 1 {
			return fmt.Errorf("%w: %w", recording.ErrResumeLater, errors.New("failed to finish the recording"))
		}
		close(resumed)
		<-ctx.Done()

		return nil
	})
	defer stop()

	select {
	case <-resumed:
	case <-time.After(waitTimeout):
		t.Fatal("the assignment was not taken again to resume the recording")
	}
	got := f.assignment(t, reservationID)
	assert.Equal(t, string(domain.RecordingAssignmentStateRecording), got.state)
	assert.Equal(t, int64(2), got.generation)
}
