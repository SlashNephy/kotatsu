package cmdetect_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// recordingsVersion は、録画の変更を知らせるチャンネルの版数を読む SQL。
const recordingsVersion = "SELECT coalesce((SELECT version FROM change_versions WHERE channel = 'recordings'), 0)"

// job は、録画 recordingID の見分けのジョブを入れ、状態を state にして、ジョブの ID を返す。
// attemptedAt は、試し始めた時刻。ゼロなら試していないとする。
func (f *fixture) job(t *testing.T, recordingID uuid.UUID, state string, attempt int, attemptedAt time.Time) int64 {
	t.Helper()

	tx, err := f.pool.Begin(t.Context())
	require.NoError(t, err)
	results, err := f.inserter.InsertManyTx(t.Context(), tx, []river.InsertManyParams{{Args: cmdetect.DetectArgs{RecordingID: recordingID}, InsertOpts: nil}})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(t.Context()))
	jobID := results[0].Job.ID
	var attempted *time.Time
	if !attemptedAt.IsZero() {
		attempted = &attemptedAt
	}
	// 終えた状態にするときは、River の制約に合わせて終えた時刻も書く。
	_, err = f.pool.Exec(t.Context(), `UPDATE river_job SET state = $2::river_job_state, attempt = $3, attempted_at = $4,
		finalized_at = CASE WHEN $2 IN ('completed', 'cancelled', 'discarded') THEN now() END WHERE id = $1`,
		jobID, state, attempt, attempted)
	require.NoError(t, err)

	return jobID
}

// detected は、録画 recordingID を、いま使っている見分け方で at に見分け終えたとする。
func (f *fixture) detected(t *testing.T, recordingID uuid.UUID, at time.Time) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
		VALUES ($1, 'ffmpeg', 1, $2)`, recordingID, at)
	require.NoError(t, err)
}

// progress は、ジョブ jobID の試行 attempt の進み具合を書く。
func (f *fixture) progress(t *testing.T, jobID int64, attempt int, ratio float64, remainingMs *int64) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO job_progress (job_id, attempt, ratio, remaining_ms, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		jobID, attempt, ratio, remainingMs, f.clock.Now())
	require.NoError(t, err)
}

func TestRecordingStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup は、録画 recordingID の見分けの記録とジョブを作る。now は、テストの今の時刻。
		setup func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time)
		want  func(now time.Time) *cmdetect.Status
	}{
		{
			name:  "見分けていず、ジョブもなければ、見分けていない",
			setup: func(*testing.T, *fixture, uuid.UUID, time.Time) {},
			want: func(time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateNotDetected, DetectedAt: nil, Progress: nil}
			},
		},
		{
			name: "River がジョブを消した後も、見分けた記録があれば、見分け終えている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.detected(t, recordingID, now.Add(-48*time.Hour))
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateDetected, DetectedAt: new(now.Add(-48 * time.Hour)), Progress: nil}
			},
		},
		{
			name: "ジョブが動くのを待っていれば、前に見分けた日時と一緒に、待っている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.detected(t, recordingID, now.Add(-time.Hour))
				f.job(t, recordingID, "available", 0, time.Time{})
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateQueued, DetectedAt: new(now.Add(-time.Hour)), Progress: nil}
			},
		},
		{
			name: "試し直しを待っているジョブも、待っている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "retryable", 1, now.Add(-time.Minute))
			},
			want: func(time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateQueued, DetectedAt: nil, Progress: nil}
			},
		},
		{
			name: "動いているジョブは、今の試行の進み具合と一緒に、見分けている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				jobID := f.job(t, recordingID, "running", 2, now.Add(-time.Minute))
				f.progress(t, jobID, 2, 0.4, new(int64(90_000)))
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateRunning, DetectedAt: nil, Progress: &cmdetect.Progress{
					Ratio: 0.4, Remaining: new(90 * time.Second), UpdatedAt: now,
				}}
			},
		},
		{
			name: "試し直す前の試行の進み具合は出さない",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				jobID := f.job(t, recordingID, "running", 2, now.Add(-time.Minute))
				f.progress(t, jobID, 1, 0.8, nil)
			},
			want: func(time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateRunning, DetectedAt: nil, Progress: nil}
			},
		},
		{
			name: "見分け直しているジョブは、前に見分けた日時と一緒に、見分けている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.detected(t, recordingID, now.Add(-time.Hour))
				f.job(t, recordingID, "running", 1, now.Add(-time.Minute))
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateRunning, DetectedAt: new(now.Add(-time.Hour)), Progress: nil}
			},
		},
		{
			name: "区間を書いた後、River がジョブを完了にする前でも、見分け終えている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				jobID := f.job(t, recordingID, "running", 1, now.Add(-time.Minute))
				f.progress(t, jobID, 1, 1, nil)
				f.detected(t, recordingID, now)
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateDetected, DetectedAt: new(now), Progress: nil}
			},
		},
		{
			name: "試す回数を使い切ったら、前に見分けた日時と一緒に、失敗",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.detected(t, recordingID, now.Add(-time.Hour))
				f.job(t, recordingID, "discarded", 3, now.Add(-time.Minute))
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateFailed, DetectedAt: new(now.Add(-time.Hour)), Progress: nil}
			},
		},
		{
			name: "取りやめたジョブ (FFmpeg がないなど) も、失敗",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "cancelled", 1, now.Add(-time.Minute))
			},
			want: func(time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateFailed, DetectedAt: nil, Progress: nil}
			},
		},
		{
			name: "最後に入れたジョブで決める。前に失敗していても、見分け直し終えたら、見分け終えている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "discarded", 3, now.Add(-time.Hour))
				f.job(t, recordingID, "completed", 1, now.Add(-time.Minute))
				f.detected(t, recordingID, now)
			},
			want: func(now time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateDetected, DetectedAt: new(now), Progress: nil}
			},
		},
		{
			name: "ほかの録画のジョブは見ない",
			setup: func(t *testing.T, f *fixture, _ uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, f.recording(t, "completed"), "running", 1, now.Add(-time.Minute))
			},
			want: func(time.Time) *cmdetect.Status {
				return &cmdetect.Status{State: cmdetect.StateNotDetected, DetectedAt: nil, Progress: nil}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			now := f.clock.Now()
			recordingID := f.recording(t, "completed")
			tt.setup(t, f, recordingID, now)

			got, err := cmdetect.RecordingStatus(t.Context(), f.pool, recordingID)
			require.NoError(t, err)
			want := tt.want(now)
			assert.Equal(t, want.State, got.State)
			if want.DetectedAt == nil {
				assert.Nil(t, got.DetectedAt)
			} else if assert.NotNil(t, got.DetectedAt) {
				assert.True(t, want.DetectedAt.Equal(*got.DetectedAt), "detectedAt: want %v, got %v", want.DetectedAt, got.DetectedAt)
			}
			if want.Progress == nil {
				assert.Nil(t, got.Progress)
			} else if assert.NotNil(t, got.Progress) {
				assert.InDelta(t, want.Progress.Ratio, got.Progress.Ratio, 1e-9)
				assert.Equal(t, want.Progress.Remaining, got.Progress.Remaining)
				assert.True(t, want.Progress.UpdatedAt.Equal(got.Progress.UpdatedAt))
			}
		})
	}
}

// TestEnqueueNotifiesRecordings は、見分けのジョブを入れたら、録画の詳細が待っていると出せるよう、録画の変更を知らせることを確かめる (#1571)。
// 同じ録画のジョブを待っていて入れなかったときは、知らせない。
func TestEnqueueNotifiesRecordings(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, "completed", segmentAt{name: segmentA, start: 0})
	version := func() int64 {
		var v int64
		require.NoError(t, f.pool.QueryRow(t.Context(), recordingsVersion).Scan(&v))

		return v
	}
	before := version()

	inserted, err := cmdetect.Enqueue(t.Context(), f.pool, f.inserter, f.clock, recordingID)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Equal(t, before+1, version())

	inserted, err = cmdetect.Enqueue(t.Context(), f.pool, f.inserter, f.clock, recordingID)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.Equal(t, before+1, version())
}

// TestDetectWorkerNotifiesRecordings は、見分け終えたら、録画の詳細とプレイヤーが区間を読み直せるよう、録画の変更を知らせることを確かめる (#1571)。
func TestDetectWorkerNotifiesRecordings(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, "completed", segmentAt{name: segmentA, start: 0})
	var before int64
	require.NoError(t, f.pool.QueryRow(t.Context(), recordingsVersion).Scan(&before))

	require.NoError(t, f.detect(t, &fakeDetector{sections: map[string][]cmdetect.Section{segmentA: {}}}, recordingID))

	var after int64
	require.NoError(t, f.pool.QueryRow(t.Context(), recordingsVersion).Scan(&after))
	assert.Equal(t, before+1, after)
}
