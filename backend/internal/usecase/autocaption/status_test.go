package autocaption_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

// job は、録画 recordingID の生成のジョブを入れ、状態を state にして、ジョブの ID を返す。attemptedAt は、試し始めた時刻。ゼロなら試していない。
// lastError は、最後の試行のエラー。空なら書かない。
func (f *fixture) job(t *testing.T, recordingID uuid.UUID, state string, attemptedAt time.Time, lastError string) int64 {
	t.Helper()

	tx, err := f.pool.Begin(t.Context())
	require.NoError(t, err)
	results, err := f.inserter.InsertManyTx(t.Context(), tx, []river.InsertManyParams{{Args: autocaption.Args{RecordingID: recordingID}, InsertOpts: nil}})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(t.Context()))
	jobID := results[0].Job.ID
	var attempted *time.Time
	if !attemptedAt.IsZero() {
		attempted = &attemptedAt
	}
	// 終えた状態にするときは、River の制約に合わせて終えた時刻も書く。エラーは、River と同じく {at, attempt, error} の形で足す。
	_, err = f.pool.Exec(t.Context(), `UPDATE river_job SET state = $2::river_job_state, attempt = 1, attempted_at = $3,
		finalized_at = CASE WHEN $2 IN ('completed', 'cancelled', 'discarded') THEN now() END,
		errors = CASE WHEN $4 = '' THEN errors ELSE ARRAY[jsonb_build_object('at', now(), 'attempt', 1, 'error', $4::text)] END
		WHERE id = $1`, jobID, state, attempted, lastError)
	require.NoError(t, err)

	return jobID
}

// generated は、録画 recordingID の字幕を、at に生成し終えたとする。
func (f *fixture) generated(t *testing.T, recordingID uuid.UUID, at time.Time) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_auto_captions (recording_id, model, generator_version, generated_at, cues)
		VALUES ($1, 'ggml-model.bin', 1, $2, '[{"startMs":0,"endMs":1000,"text":"あ"},{"startMs":1000,"endMs":2000,"text":"い"}]')`, recordingID, at)
	require.NoError(t, err)
}

func TestRecordingStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time)
		want  func(now time.Time) *autocaption.Status
	}{
		{
			name:  "生成していず、ジョブもなければ、生成していない",
			setup: func(*testing.T, *fixture, uuid.UUID, time.Time) {},
			want: func(time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateNotGenerated, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
			},
		},
		{
			name: "River がジョブを消した後も、字幕があれば、生成し終えている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.generated(t, recordingID, now.Add(-48*time.Hour))
			},
			want: func(now time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateGenerated, GeneratedAt: new(now.Add(-48 * time.Hour)), Model: "ggml-model.bin", Cues: 2, Progress: nil}
			},
		},
		{
			name: "ジョブが動くのを待っていれば、前に生成した日時と一緒に、待っている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.generated(t, recordingID, now.Add(-time.Hour))
				f.job(t, recordingID, "available", time.Time{}, "")
			},
			want: func(now time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateQueued, GeneratedAt: new(now.Add(-time.Hour)), Model: "ggml-model.bin", Cues: 2, Progress: nil}
			},
		},
		{
			name: "動いているジョブは、進み具合と一緒に、生成している",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				jobID := f.job(t, recordingID, "running", now.Add(-time.Minute), "")
				_, err := f.pool.Exec(t.Context(), `INSERT INTO job_progress (job_id, attempt, ratio, remaining_ms, updated_at) VALUES ($1, 1, 0.25, 60000, $2)`, jobID, now)
				require.NoError(t, err)
			},
			want: func(now time.Time) *autocaption.Status {
				return &autocaption.Status{
					State: autocaption.StateRunning, GeneratedAt: nil, Model: "", Cues: 0,
					Progress: &autocaption.Progress{Ratio: 0.25, Remaining: new(time.Minute), UpdatedAt: now},
				}
			},
		},
		{
			name: "今の試行で字幕を書いた後は、River がジョブを完了にする前でも、生成し終えている",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "running", now.Add(-time.Minute), "")
				f.generated(t, recordingID, now)
			},
			want: func(now time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateGenerated, GeneratedAt: new(now), Model: "ggml-model.bin", Cues: 2, Progress: nil}
			},
		},
		{
			name: "試す回数を使い切ったら、失敗",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "discarded", now.Add(-time.Minute), "failed to transcribe with whisper-cli: exit status 1")
			},
			want: func(time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateFailed, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
			},
		},
		{
			name: "whisper-cli かモデルがなくて取りやめたら、設定されていない",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "cancelled", now.Add(-time.Minute),
					"JobCancelError: whisper.cpp is not configured: the model is not found at /data/whisper/ggml-model.bin")
			},
			want: func(time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateNotConfigured, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
			},
		},
		{
			name: "ほかの理由で取りやめたら、失敗",
			setup: func(t *testing.T, f *fixture, recordingID uuid.UUID, now time.Time) {
				t.Helper()
				f.job(t, recordingID, "cancelled", now.Add(-time.Minute), "JobCancelError: the recording is still being recorded")
			},
			want: func(time.Time) *autocaption.Status {
				return &autocaption.Status{State: autocaption.StateFailed, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, "completed", segmentAt{name: "a.m2ts", start: 0})
			now := f.clock.Now()
			tt.setup(t, f, recordingID, now)

			got, err := autocaption.RecordingStatus(t.Context(), f.pool, recordingID)
			require.NoError(t, err)
			want := tt.want(now)
			if got.GeneratedAt != nil {
				got.GeneratedAt = new(got.GeneratedAt.UTC())
			}
			if got.Progress != nil {
				got.Progress.UpdatedAt = got.Progress.UpdatedAt.UTC()
			}
			assert.Equal(t, want, got)
		})
	}
}
