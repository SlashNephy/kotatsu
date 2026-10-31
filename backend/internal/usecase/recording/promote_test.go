package recording_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// promoted は、昇格した録画の、書き出した後の様子。
type promoted struct {
	state         string
	failureReason *string
	gaps          int
	// starts と ends は、書けたセグメントの始まりと終わり。
	starts, ends []time.Time
	recordedFrom time.Time
	history      int
	promotion    bool
	// notified は、通知したイベントの種類と、届けるユーザー。
	notified []string
	audience [][]uuid.UUID
}

// promotedOf は、昇格した録画 recordingID の様子を読む。
func (f *fixture) promotedOf(t *testing.T, recordingID uuid.UUID) *promoted {
	t.Helper()

	ctx := t.Context()
	got := &promoted{starts: []time.Time{}, ends: []time.Time{}, notified: []string{}, audience: [][]uuid.UUID{}}
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT state, failure_reason, jsonb_array_length(gaps), recorded_from FROM recordings WHERE id = $1`, recordingID).
		Scan(&got.state, &got.failureReason, &got.gaps, &got.recordedFrom))
	got.recordedFrom = got.recordedFrom.UTC()
	rows, err := f.pool.Query(ctx, `SELECT started_at, ended_at FROM recording_files WHERE recording_id = $1 AND kind = 'segment' AND size > 0 ORDER BY started_at`, recordingID)
	require.NoError(t, err)
	for rows.Next() {
		var start time.Time
		var end *time.Time
		require.NoError(t, rows.Scan(&start, &end))
		require.NotNil(t, end)
		got.starts, got.ends = append(got.starts, start.UTC()), append(got.ends, end.UTC())
	}
	require.NoError(t, rows.Err())
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM recording_history`).Scan(&got.history))
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM timeshift_promotions WHERE recording_id = $1)`, recordingID).Scan(&got.promotion))
	rows, err = f.pool.Query(ctx, `SELECT kind, coalesce(audience, '{}') FROM notification_events WHERE payload->>'recordingId' = $1 ORDER BY occurred_at`, recordingID.String())
	require.NoError(t, err)
	for rows.Next() {
		var kind string
		var audience []uuid.UUID
		require.NoError(t, rows.Scan(&kind, &audience))
		got.notified, got.audience = append(got.notified, kind), append(got.audience, audience)
	}
	require.NoError(t, rows.Err())

	return got
}

// TestPromoteWorkerWritesTimeshiftBuffer は、放送の終わった番組を昇格すると、nekonote のジョブが、バッファの、書き出す範囲の
// 分を、続いたチャンクの並びごとに録画のセグメントに書き出し、録画を完了にすることを確かめる (#1057、設計書 5.7)。
// バッファの穴と、もうバッファにない番組の頭は、途切れた区間にする。
func TestPromoteWorkerWritesTimeshiftBuffer(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	from := programStart().Add(-time.Minute)
	to := programStart().Add(10 * time.Minute)
	tests := []struct {
		name   string
		chunks []chunk
		want   func(userID uuid.UUID) *promoted
	}{
		{
			name: "書き出す範囲を通して録れていれば、1 つのセグメントに書き出して完了にし、録画履歴に記録する",
			chunks: []chunk{
				{start: from.Add(-5 * time.Minute), end: new(from.Add(5 * time.Minute))},
				{start: from.Add(5 * time.Minute), end: new(to.Add(5 * time.Minute))},
			},
			want: func(userID uuid.UUID) *promoted {
				return &promoted{
					state: string(recording.StateCompleted), gaps: 0, starts: []time.Time{from}, ends: []time.Time{to}, recordedFrom: from,
					history: 1, promotion: false, notified: []string{"recording_completed"}, audience: [][]uuid.UUID{{userID}},
				}
			},
		},
		{
			name:   "番組の頭がもうバッファになければ、残っている分を書き出し、冒頭の欠けを途切れた区間にする",
			chunks: []chunk{{start: from.Add(2 * time.Minute), end: new(to.Add(time.Minute))}},
			want: func(userID uuid.UUID) *promoted {
				return &promoted{
					state: string(recording.StatePartial), gaps: 1, starts: []time.Time{from.Add(2 * time.Minute)}, ends: []time.Time{to},
					recordedFrom: from.Add(2 * time.Minute), history: 0, promotion: false,
					notified: []string{"recording_completed"}, audience: [][]uuid.UUID{{userID}},
				}
			},
		},
		{
			name: "バッファに穴があれば、続いたチャンクごとにセグメントに書き出し、穴を途切れた区間にする",
			chunks: []chunk{
				{start: from.Add(-time.Minute), end: new(from.Add(3 * time.Minute))},
				{start: from.Add(5 * time.Minute), end: nil},
			},
			want: func(userID uuid.UUID) *promoted {
				return &promoted{
					state: string(recording.StatePartial), gaps: 1, starts: []time.Time{from, from.Add(5 * time.Minute)},
					ends: []time.Time{from.Add(3 * time.Minute), to}, recordedFrom: from, history: 0, promotion: false,
					notified: []string{"recording_completed"}, audience: [][]uuid.UUID{{userID}},
				}
			},
		},
		{
			name:   "番組の終わりまで録れていなければ、終わりの欠けを途切れた区間にする",
			chunks: []chunk{{start: from.Add(-time.Minute), end: new(from.Add(5 * time.Minute))}},
			want: func(userID uuid.UUID) *promoted {
				return &promoted{
					state: string(recording.StatePartial), gaps: 1, starts: []time.Time{from}, ends: []time.Time{from.Add(5 * time.Minute)},
					recordedFrom: from, history: 0, promotion: false, notified: []string{"recording_completed"}, audience: [][]uuid.UUID{{userID}},
				}
			},
		},
		{
			name:   "書き出す範囲のチャンクがなければ、録画を失敗にする",
			chunks: []chunk{{start: to.Add(time.Minute), end: nil}},
			want: func(userID uuid.UUID) *promoted {
				return &promoted{
					state: string(recording.StateFailed), failureReason: new(string(recording.FailureProgramNotFound)), gaps: 0,
					starts: []time.Time{}, ends: []time.Time{}, recordedFrom: from, history: 0, promotion: false,
					notified: []string{"recording_failed"}, audience: [][]uuid.UUID{{userID}},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			_, store := f.newRecorderWithTimeshift(t, nil)
			tokyo, err := time.LoadLocation("Asia/Tokyo")
			require.NoError(t, err)
			for _, c := range tt.chunks {
				f.addChunk(t, store, grService(0), c, data)
			}
			ctx := t.Context()
			userID := id.New()
			_, err = f.pool.Exec(ctx, `INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, 'member', 'member', 'member', $2, $2)`,
				userID, f.clock.Now())
			require.NoError(t, err)
			recordingID := id.New()
			_, err = f.pool.Exec(ctx, `INSERT INTO recordings (id, service_id, event_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
				VALUES ($1, $2, 6, 'program', '', $3, $4, 'recording', $5, $6, $6)`,
				recordingID, grService(0), programStart(), programStart().Add(9*time.Minute), from, f.clock.Now())
			require.NoError(t, err)
			_, err = f.pool.Exec(ctx, `INSERT INTO timeshift_promotions (recording_id, service_id, from_at, to_at, created_by, created_at)
				VALUES ($1, $2, $3, $4, $5, $6)`, recordingID, grService(0), from, to, userID, f.clock.Now())
			require.NoError(t, err)

			worker := recording.NewPromoteWorker(f.pool, f.clock, store.store, nil, tokyo, slog.New(slog.DiscardHandler))
			job := &river.Job[recording.PromoteArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5}, Args: recording.PromoteArgs{RecordingID: recordingID}}
			require.NoError(t, worker.Work(ctx, job))

			assert.Equal(t, tt.want(userID), f.promotedOf(t, recordingID))
		})
	}
}

// TestPromoteWorkerSkipsGonePromotion は、昇格の行がもうなければ (録画を消したか、ほかで書き出し終えた)、何もせずに終えることを確かめる。
func TestPromoteWorkerSkipsGonePromotion(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	_, store := f.newRecorderWithTimeshift(t, nil)
	worker := recording.NewPromoteWorker(f.pool, f.clock, store.store, nil, time.UTC, slog.New(slog.DiscardHandler))
	job := &river.Job[recording.PromoteArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5}, Args: recording.PromoteArgs{RecordingID: id.New()}}
	require.NoError(t, worker.Work(t.Context(), job))
}
