package autocaption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

//go:generate go tool go-enum --values --nocomments

// State は、録画の字幕の自動生成の状態 (設計書 6.1)。
//
// ENUM(
// not_generated // 生成していず、生成するジョブもないこと。
// queued // 生成するジョブが動くのを待っていること。再試行を待っているときも含む。
// running // 生成していること。
// generated // 生成し終えたこと。台詞がなく、字幕が 1 つもなくても、生成し終えたとする。
// failed // 最後に入れた生成のジョブが、試す回数を使い切ったか、取りやめたこと。
// not_configured // 最後に入れた生成のジョブが、whisper-cli かモデルが見つからずに取りやめたこと。
// )
type State string

// Progress は、生成しているジョブの進み具合。
type Progress struct {
	Ratio float64
	// Remaining は残り時間。分からなければ nil。
	Remaining *time.Duration
	UpdatedAt time.Time
}

// Status は、録画の字幕の自動生成の状態。
type Status struct {
	State State
	// GeneratedAt は、最後に生成し終えた日時。生成していなければ nil。生成し直している間と、生成し直しに失敗した後も、前の日時を返す。
	GeneratedAt *time.Time
	// Model は、最後に生成し終えたときに使ったモデルの名前。生成していなければ空。
	Model string
	// Cues は、最後に生成し終えた字幕の数。
	Cues int
	// Progress は、生成しているときの、今の試行の進み具合。なければ nil。
	Progress *Progress
}

// latestJob は、録画の、最後に入れた生成のジョブ。
type latestJob struct {
	state       string
	attemptedAt *time.Time
	progress    *Progress
	// lastError は、最後の試行のエラー。なければ空。
	lastError string
}

// RecordingStatus は、録画 recordingID の字幕の自動生成の状態を返す。
// 最後に入れた生成のジョブ (River のテーブル) と、生成した字幕 (recording_auto_captions) から決める。
// River は終えたジョブを消すので、ジョブがなければ、生成した字幕だけで決める。
func RecordingStatus(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) (*Status, error) {
	status := &Status{State: StateNotGenerated, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
	summary, err := sqlc.New(db).GetRecordingAutoCaptionsSummary(ctx, recordingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("failed to get the captions of the recording: %w", err)
	default:
		status.State = StateGenerated
		status.GeneratedAt = &summary.GeneratedAt
		status.Model = summary.Model
		status.Cues = int(summary.CueCount)
	}

	return withJob(ctx, db, new(Args).Kind(), map[string]string{"recordingId": recordingID.String()}, status)
}

// Generated は、生成し終えた字幕の要約。
type Generated struct {
	At    time.Time
	Model string
	Cues  int
}

// JobStatus は、生成した字幕 generated (なければ nil) と、kind のジョブのうち、引数が match を含む最後のジョブから、字幕の自動生成の状態を返す。
// 録画でないファイル (ライブラリのファイル。#1734) のように、生成した字幕を DB の外に置くものに使う。
func JobStatus(ctx context.Context, db sqlc.DBTX, kind string, match map[string]string, generated *Generated) (*Status, error) {
	status := &Status{State: StateNotGenerated, GeneratedAt: nil, Model: "", Cues: 0, Progress: nil}
	if generated != nil {
		status.State = StateGenerated
		status.GeneratedAt = &generated.At
		status.Model = generated.Model
		status.Cues = generated.Cues
	}

	return withJob(ctx, db, kind, match, status)
}

// withJob は、生成した字幕から決めた status に、最後に入れた生成のジョブの状態を重ねて返す。
func withJob(ctx context.Context, db sqlc.DBTX, kind string, match map[string]string, status *Status) (*Status, error) {
	job, found, err := lastJob(ctx, db, kind, match)
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
		// 字幕を書いてから River がジョブを完了にするまでの間は、まだ動いているように見える。
		// 書いたことを通知した後に読み直した画面に、生成しているように出さないよう、今の試行で書いた字幕があれば生成し終えたとする。
		if status.GeneratedAt != nil && job.attemptedAt != nil && !status.GeneratedAt.Before(*job.attemptedAt) {
			return status, nil
		}
		status.State = StateRunning
		status.Progress = job.progress
	case "cancelled", "discarded":
		status.State = StateFailed
		if strings.Contains(job.lastError, ErrNotConfigured.Error()) {
			status.State = StateNotConfigured
		}
	}

	return status, nil
}

// lastJob は、kind のジョブのうち、引数が match を含む最後に入れたジョブと、今の試行の進み具合を返す。ジョブがなければ false を返す。
// River のテーブルは sqlc のスキーマにないので、直接読む。
func lastJob(ctx context.Context, db sqlc.DBTX, kind string, match map[string]string) (*latestJob, bool, error) {
	args, err := json.Marshal(match)
	if err != nil {
		return nil, false, fmt.Errorf("failed to encode the job arguments: %w", err)
	}
	var (
		job         latestJob
		ratio       *float64
		remainingMs *int64
		updatedAt   *time.Time
		lastError   *string
	)
	err = db.QueryRow(ctx, `SELECT j.state::text, j.attempted_at, j.errors[array_length(j.errors, 1)]->>'error', p.ratio, p.remaining_ms, p.updated_at
		FROM river_job j
		LEFT JOIN job_progress p ON p.job_id = j.id AND p.attempt = j.attempt
		WHERE j.kind = $1 AND j.args @> $2::jsonb
		ORDER BY j.id DESC
		LIMIT 1`, kind, string(args)).Scan(&job.state, &job.attemptedAt, &lastError, &ratio, &remainingMs, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get the caption generation job: %w", err)
	}
	if lastError != nil {
		job.lastError = *lastError
	}
	if ratio != nil && updatedAt != nil {
		job.progress = &Progress{Ratio: *ratio, Remaining: nil, UpdatedAt: *updatedAt}
		if remainingMs != nil {
			job.progress.Remaining = new(time.Duration(*remainingMs) * time.Millisecond)
		}
	}

	return &job, true, nil
}

// RecordingCues は、録画 recordingID の自動生成した字幕を、始まりの順に返す。生成していなければ空のスライスを返す。
// 時刻は、録画を始めた日時からの時間。
func RecordingCues(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) ([]Cue, error) {
	row, err := sqlc.New(db).GetRecordingAutoCaptions(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []Cue{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the captions of the recording: %w", err)
	}

	return decodeCues(row.Cues)
}

// RecordingsCues は、録画 recordingIDs の自動生成した字幕を、録画ごとに始まりの順に返す。字幕を生成していない録画は含めない。
// 字幕の文で探すとき (#2214) に、候補の録画の字幕をまとめて読む。時刻は録画を始めた日時からの時間。
func RecordingsCues(ctx context.Context, db sqlc.DBTX, recordingIDs []uuid.UUID) (map[uuid.UUID][]Cue, error) {
	rows, err := sqlc.New(db).ListRecordingAutoCaptionCues(ctx, recordingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list the captions of the recordings: %w", err)
	}
	out := make(map[uuid.UUID][]Cue, len(rows))
	for _, row := range rows {
		cues, err := decodeCues(row.Cues)
		if err != nil {
			return nil, fmt.Errorf("recording %s: %w", row.RecordingID, err)
		}
		out[row.RecordingID] = cues
	}

	return out, nil
}

// decodeCues は、DB の cues (JSONB) を字幕にする。
func decodeCues(raw []byte) ([]Cue, error) {
	var stored []storedCue
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("failed to decode the captions of the recording: %w", err)
	}
	cues := make([]Cue, 0, len(stored))
	for _, c := range stored {
		cues = append(cues, Cue{Start: time.Duration(c.StartMs) * time.Millisecond, End: time.Duration(c.EndMs) * time.Millisecond, Text: c.Text})
	}

	return cues, nil
}

// searchGrams は、字幕の文で探すための gram を返す (#2214、設計書 3.5)。字幕の文を正規化して作る。
func searchGrams(cues []Cue) []string {
	texts := make([]string, 0, len(cues))
	for _, c := range cues {
		texts = append(texts, domain.Normalize(c.Text))
	}

	return match.TextGrams(texts)
}
