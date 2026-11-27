package aribcaption

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
)

// enqueueBatch は、まとめて投入するときに、1 つのトランザクションで入れるジョブの数。
const enqueueBatch = 500

// PendingRecording は、ARIB 字幕の文を取り出す録画 (#2218)。
type PendingRecording struct {
	RecordingID uuid.UUID
	Name        string
	StartAt     time.Time
	// ExtractorVersion は、前に取り出したときの取り出し方のバージョン。取り出していなければ nil。
	ExtractorVersion *int32
}

// Pending は、ARIB 字幕の文を取り出す録画を、放送の開始の順に返す (#2218)。録画中でなく TS のセグメントを書いた録画のうち、
// 取り出していないか、取り出し方の古いバージョンで取り出した録画を選ぶ。all なら、今のバージョンで取り出した録画も選ぶ。
func Pending(ctx context.Context, db sqlc.DBTX, all bool) ([]PendingRecording, error) {
	rows, err := sqlc.New(db).ListRecordingsForARIBCaptionExtraction(ctx, sqlc.ListRecordingsForARIBCaptionExtractionParams{
		IncludeAll: all, ExtractorVersion: ExtractorVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings to extract the ARIB captions: %w", err)
	}
	recordings := make([]PendingRecording, 0, len(rows))
	for _, row := range rows {
		recordings = append(recordings, PendingRecording{
			RecordingID: row.RecordingID, Name: row.Name, StartAt: row.StartAt, ExtractorVersion: row.ExtractorVersion,
		})
	}

	return recordings, nil
}

// Beginner は、トランザクションを始められるもの。*pgxpool.Pool が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// EnqueueMany は、録画 recordingIDs の ARIB 字幕の文を取り出すジョブをまとめて投入し、入れた数を返す (#2218)。
// 待っている録画には重ねて入れない。録画を終えたときのジョブ (新しく録った録画) を待たせないよう、取り込んだ録画のジョブと同じく
// 優先度を下げて入れる。途中で失敗したら、それまでに入れた数と一緒にエラーを返す。
func EnqueueMany(ctx context.Context, db Beginner, inserter Inserter, recordingIDs []uuid.UUID) (int, error) {
	inserted := 0
	for start := 0; start < len(recordingIDs); start += enqueueBatch {
		ids := recordingIDs[start:min(start+enqueueBatch, len(recordingIDs))]
		params := make([]river.InsertManyParams, 0, len(ids))
		for _, id := range ids {
			opts := Args{RecordingID: id}.InsertOpts()
			opts.Priority = jobs.PriorityImported
			params = append(params, river.InsertManyParams{Args: Args{RecordingID: id}, InsertOpts: &opts})
		}
		batch := 0
		err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			results, err := inserter.InsertManyTx(ctx, tx, params)
			if err != nil {
				return fmt.Errorf("failed to insert the ARIB caption extraction jobs: %w", err)
			}
			for _, result := range results {
				if !result.UniqueSkippedAsDuplicate {
					batch++
				}
			}

			return nil
		})
		if err != nil {
			return inserted, errors.Join(err, fmt.Errorf("enqueued %d of %d recordings", inserted, len(recordingIDs)))
		}
		inserted += batch
	}

	return inserted, nil
}

// Queued は、録画 recordingIDs のうち、ARIB 字幕の文を取り出すジョブを待っているか動かしている録画の数を返す (#2218)。
// 管理画面に、まとめて入れたジョブがまだ終わっていないことを出すのに使う。
func Queued(ctx context.Context, db sqlc.DBTX, recordingIDs []uuid.UUID) (int, error) {
	if len(recordingIDs) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(recordingIDs))
	for _, id := range recordingIDs {
		ids = append(ids, id.String())
	}
	// River のテーブルは sqlc のスキーマにないので、直接引く。
	var queued int
	err := db.QueryRow(ctx, `SELECT count(DISTINCT args->>'recordingId') FROM river_job
		WHERE kind = $1 AND args->>'recordingId' = ANY($2::text[])
			AND state IN ('available', 'pending', 'retryable', 'running', 'scheduled')`,
		new(Args).Kind(), ids).Scan(&queued)
	if err != nil {
		return 0, fmt.Errorf("failed to count the queued ARIB caption extraction jobs: %w", err)
	}

	return queued, nil
}
