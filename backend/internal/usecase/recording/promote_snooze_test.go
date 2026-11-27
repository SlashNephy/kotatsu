package recording_test

import (
	"context"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// insertPromotion は、昇格を待つ録画と、その昇格の行を作り、録画の ID を返す。
func (f *fixture) insertPromotion(t *testing.T) uuid.UUID {
	t.Helper()

	from := programStart().Add(-time.Minute)
	recordingID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, event_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 6, 'program', '', $3, $4, 'recording', $5, $6, $6)`,
		recordingID, grService(0), programStart(), programStart().Add(9*time.Minute), from, f.clock.Now())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO timeshift_promotions (recording_id, service_id, from_at, to_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, NULL, $5)`, recordingID, grService(0), from, programStart().Add(10*time.Minute), f.clock.Now())
	require.NoError(t, err)

	return recordingID
}

// TestPromoteWorkerSnoozesRecoverableFailures は、昇格のジョブが、待てば通る失敗 (この nekonote にタイムシフトのディレクトリがない、
// 止めるとき) では、試す回数を使わずに再試行し、ジョブを入れてから上限を過ぎて試し尽くしたら録画を失敗にすることを確認する (#2198)。
func TestPromoteWorkerSnoozesRecoverableFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// cancel は、ジョブを止めるときのように、ctx を取り消してから動かすかどうか。
		cancel bool
		// age は、ジョブを入れてからの時間。attempt は、何回目に試すか。
		age     time.Duration
		attempt int
		// wantSnooze は、再試行するまで待つ時間。nil なら再試行しない。
		wantSnooze *time.Duration
		wantState  domain.RecordingState
	}{
		{name: "保存先がなければ、回数を使わずに再試行する", age: time.Minute, attempt: 5, wantSnooze: new(time.Minute), wantState: domain.RecordingStateRecording},
		{name: "止めるときは、すぐに再試行する", cancel: true, age: time.Minute, attempt: 5, wantSnooze: new(time.Duration(0)), wantState: domain.RecordingStateRecording},
		{name: "上限を過ぎて試し尽くしたら、失敗にする", age: 7 * time.Hour, attempt: 5, wantSnooze: nil, wantState: domain.RecordingStateFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.insertPromotion(t)
			// この nekonote には、タイムシフトのディレクトリがない。
			store := kago.New(map[string]string{poolID: t.TempDir()}, func(string) (uint64, error) { return 1 << 40, nil }).WithDataDir(t.TempDir())
			worker := recording.NewPromoteWorker(f.pool, f.clock, store, nil, time.UTC, slog.New(slog.DiscardHandler))
			ctx := t.Context()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			job := &river.Job[recording.PromoteArgs]{
				JobRow: &rivertype.JobRow{Attempt: tt.attempt, MaxAttempts: 5, CreatedAt: f.clock.Now().Add(-tt.age)},
				Args:   recording.PromoteArgs{RecordingID: recordingID},
			}
			err := worker.Work(ctx, job)

			var snooze *rivertype.JobSnoozeError
			if tt.wantSnooze != nil {
				require.ErrorAs(t, err, &snooze)
				assert.Equal(t, *tt.wantSnooze, snooze.Duration)
			} else {
				require.Error(t, err)
				assert.NotErrorAs(t, err, &snooze)
			}
			var state string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state FROM recordings WHERE id = $1", recordingID).Scan(&state))
			assert.Equal(t, string(tt.wantState), state)
		})
	}
}

// TestPromoteWorkerSnoozesOnDatabaseError は、昇格のジョブが、DB のエラー (ローリング更新で、新しいスキーマが古いバイナリの読むカラムを
// 消したときなど) では、試す回数を使わずに再試行することを確認する (#2198)。
func TestPromoteWorkerSnoozesOnDatabaseError(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.insertPromotion(t)
	_, store := f.newRecorderWithTimeshift(t, nil)
	f.renameColumn(t, "timeshift_promotions", "created_by", "created_by_renamed")
	worker := recording.NewPromoteWorker(f.pool, f.clock, store.store, nil, time.UTC, slog.New(slog.DiscardHandler))
	job := &river.Job[recording.PromoteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5, CreatedAt: f.clock.Now()},
		Args:   recording.PromoteArgs{RecordingID: recordingID},
	}

	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, worker.Work(t.Context(), job), &snooze)
	assert.Equal(t, time.Minute, snooze.Duration)
}

// TestPromoteWorkerSnoozesOnUnmountedPool は、録画設定のプールがこの nekonote にマウントされていなければ、昇格のジョブを、
// 試す回数を使わずに再試行することを確認する (#2198)。マウントしている nekonote が書き込める。
func TestPromoteWorkerSnoozesOnUnmountedPool(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.insertPromotion(t)
	_, store := f.newRecorderWithTimeshift(t, nil)
	settings := domain.DefaultRecordingSettings()
	settings.PoolID = "elsewhere"
	worker := recording.NewPromoteWorker(f.pool, f.clock, store.store, assign.FixedDefaults(settings), time.UTC, slog.New(slog.DiscardHandler))
	job := &river.Job[recording.PromoteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5, CreatedAt: f.clock.Now()},
		Args:   recording.PromoteArgs{RecordingID: recordingID},
	}

	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, worker.Work(t.Context(), job), &snooze)
	assert.Equal(t, time.Minute, snooze.Duration)
}
