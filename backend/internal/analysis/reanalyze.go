package analysis

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

// enqueueBatch は、1 つのトランザクションで投入する解析のジョブの数。
const enqueueBatch = 500

// TxBeginner は、ジョブを投入するトランザクションを始める。*pgxpool.Pool が満たす。
type TxBeginner interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Candidate は、解析し直す候補の録画。
type Candidate struct {
	RecordingID uuid.UUID
	// Missing は、書いたもののあるセグメントの索引か、代表画像がないかどうか。
	Missing bool
}

// Candidates は、録画中でない、書いたもののあるセグメントを持つ録画を、録画を始めた日時の新しい順に返す (#897)。
// 生成物 (索引と代表画像) がデータディレクトリにない録画は、Missing を true にする。
func Candidates(ctx context.Context, db sqlc.DBTX, store *kago.Store) ([]*Candidate, error) {
	rows, err := sqlc.New(db).ListAnalyzableRecordingSegments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording segments: %w", err)
	}
	checker := store.NewChecker()
	defer checker.Close()
	exists := func(location *kago.Location) (bool, error) {
		found, err := checker.Exists(location.PoolID, location.Path)
		if err != nil {
			return false, fmt.Errorf("failed to inspect the data directory: %w", err)
		}

		return found, nil
	}
	candidates := []*Candidate{}
	for i := range rows {
		row := &rows[i]
		// 行は、録画ごとにセグメントの始まりの順に並ぶので、録画の最初の行が、画像を置くセグメント (thumbnail.ImageSegment)。
		first := len(candidates) == 0 || candidates[len(candidates)-1].RecordingID != row.RecordingID
		if first {
			candidates = append(candidates, &Candidate{RecordingID: row.RecordingID, Missing: false})
		}
		candidate := candidates[len(candidates)-1]
		if candidate.Missing {
			continue
		}
		found, err := exists(kago.IndexLocation(row.ID))
		if err != nil {
			return nil, err
		}
		if found && first {
			found, err = exists(kago.DataLocation(row.ID, thumbnail.ThumbnailFileName))
			if err != nil {
				return nil, err
			}
		}
		candidate.Missing = !found
	}

	return candidates, nil
}

// Enqueue は、録画 ids を解析し直すジョブを、ids の順に投入し、新しく投入した数を返す。
// 同じ録画のジョブを待っていれば、重ねて投入しない。新しく録った録画のジョブより後に回す (AnalyzeArgs の優先度のまま)。
func Enqueue(ctx context.Context, db TxBeginner, inserter Inserter, ids []uuid.UUID) (int, error) {
	inserted := 0
	for start := 0; start < len(ids); start += enqueueBatch {
		params := make([]river.InsertManyParams, 0, min(enqueueBatch, len(ids)-start))
		for _, id := range ids[start:min(start+enqueueBatch, len(ids))] {
			params = append(params, river.InsertManyParams{Args: AnalyzeArgs{RecordingID: id}, InsertOpts: nil})
		}
		batch := 0
		err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			results, err := inserter.InsertManyTx(ctx, tx, params)
			if err != nil {
				return fmt.Errorf("failed to enqueue the analysis: %w", err)
			}
			for _, result := range results {
				if !result.UniqueSkippedAsDuplicate {
					batch++
				}
			}

			return nil
		})
		if err != nil {
			return inserted, errors.Join(err, fmt.Errorf("enqueued %d of %d recordings", inserted, len(ids)))
		}
		inserted += batch
	}

	return inserted, nil
}
