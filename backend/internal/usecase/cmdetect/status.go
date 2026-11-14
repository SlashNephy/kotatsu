package cmdetect

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

//go:generate go tool go-enum --values --nocomments

// State は、録画の CM の区間の見分けの状態 (#1571、設計書 6.1)。
//
// ENUM(
// not_detected // 見分けていず、見分けるジョブもないこと。
// queued // 見分けるジョブが動くのを待っていること。試し直しを待っているときも含む。
// running // 見分けていること。
// detected // 見分け終えたこと。CM が見つからなくても、見分け終えたとする。
// failed // 最後に入れた見分けのジョブが、試す回数を使い切ったか、取りやめたこと。
// )
type State string

// Progress は、見分けているジョブの進み具合 (#1511)。
type Progress struct {
	Ratio float64
	// Remaining は残り時間。分からなければ nil。
	Remaining *time.Duration
	UpdatedAt time.Time
}

// Status は、録画の CM の区間の見分けの状態。
type Status struct {
	State State
	// DetectedAt は、最後に見分け終えた日時。見分けていなければ nil。見分け直している間と、見分け直しに失敗した後も、前の日時を返す。
	DetectedAt *time.Time
	// Progress は、見分けているときの、今の試行の進み具合。なければ nil。
	Progress *Progress
	// Note は、最後に見分け終えた見分けで、いつもの見分け方で決められなかった理由 (#1508)。決められたか、見分けていなければ空。
	Note string
	// VideoMissing は、最後に見分け終えた見分けで測った、映像の欠けの長さの合計 (#1508)。
	VideoMissing time.Duration
}

// latestJob は、録画の、最後に入れた見分けのジョブ。
type latestJob struct {
	state       string
	attemptedAt *time.Time
	progress    *Progress
}

// RecordingStatus は、録画 recordingID の CM の区間の見分けの状態を返す (#1571)。
// 最後に入れた見分けのジョブ (River の表) と、見分けた記録 (recording_cm_detections) から決める。
// River は終えたジョブを消すので、ジョブがなければ、見分けた記録だけで決める。
func RecordingStatus(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) (*Status, error) {
	status := &Status{State: StateNotDetected, DetectedAt: nil, Progress: nil, Note: "", VideoMissing: 0}
	// 見分け方が複数あれば、最後に見分けたものを使う (#1505)。区間 (RecordingSections) と同じ見分けを指す。
	latest, err := sqlc.New(db).GetLatestRecordingCMDetection(ctx, recordingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("failed to get the CM detection of the recording: %w", err)
	default:
		status.DetectedAt = &latest.DetectedAt
		status.State = StateDetected
		status.Note = latest.Note
		status.VideoMissing = time.Duration(latest.VideoMissingMs) * time.Millisecond
	}
	job, found, err := lastJob(ctx, db, recordingID)
	if err != nil {
		return nil, err
	}
	if !found {
		return status, nil
	}
	switch job.state {
	case "available", "scheduled", "pending", "retryable":
		status.State = StateQueued
	case "running":
		// 区間を書いてから River がジョブを完了にするまでの間は、まだ動いているように見える。
		// 区間を書いたことを知らせた後に読み直した画面に、見分けているように出さないよう、今の試行で書いた記録があれば見分け終えたとする。
		if status.DetectedAt != nil && job.attemptedAt != nil && !status.DetectedAt.Before(*job.attemptedAt) {
			return status, nil
		}
		status.State = StateRunning
		status.Progress = job.progress
	case "cancelled", "discarded":
		status.State = StateFailed
	}

	return status, nil
}

// lastJob は、録画 recordingID の、最後に入れた見分けのジョブと、今の試行の進み具合を返す。ジョブがなければ false を返す。
// River の表は sqlc のスキーマにないので、直接読む。
func lastJob(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) (*latestJob, bool, error) {
	var (
		job         latestJob
		ratio       *float64
		remainingMs *int64
		updatedAt   *time.Time
	)
	err := db.QueryRow(ctx, `SELECT j.state::text, j.attempted_at, p.ratio, p.remaining_ms, p.updated_at
		FROM river_job j
		LEFT JOIN job_progress p ON p.job_id = j.id AND p.attempt = j.attempt
		WHERE j.kind = $1 AND j.args @> jsonb_build_object('recordingId', $2::text)
		ORDER BY j.id DESC
		LIMIT 1`, new(DetectArgs).Kind(), recordingID.String()).Scan(&job.state, &job.attemptedAt, &ratio, &remainingMs, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get the CM detection job of the recording: %w", err)
	}
	if ratio != nil && updatedAt != nil {
		job.progress = &Progress{Ratio: *ratio, Remaining: nil, UpdatedAt: *updatedAt}
		if remainingMs != nil {
			job.progress.Remaining = new(time.Duration(*remainingMs) * time.Millisecond)
		}
	}

	return &job, true, nil
}

// LatestReports は、録画 recordingIDs ごとの、最後に見分け終えた見分けの添え書きを、まとめて引く (#1508)。録画の一覧の印に使う。
// 見分けていない録画は、結果に含めない。
func LatestReports(ctx context.Context, db sqlc.DBTX, recordingIDs []uuid.UUID) (map[uuid.UUID]Report, error) {
	rows, err := sqlc.New(db).ListLatestRecordingCMDetectionNotes(ctx, recordingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load the CM detection notes: %w", err)
	}
	reports := make(map[uuid.UUID]Report, len(rows))
	for _, row := range rows {
		reports[row.RecordingID] = Report{Note: row.Note, Missing: time.Duration(row.VideoMissingMs) * time.Millisecond}
	}

	return reports, nil
}
