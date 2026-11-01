package anime

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// importCheckInterval は、取り込み直す時期が来たかを確かめる間隔。取り込めなかった取り込み元を、1 時間後に問い合わせ直すのにも使う。
const importCheckInterval = time.Hour

// Inserter は、ジョブを入れる River のクライアント。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// ImportArgs は、連携先の情報を取り込むジョブの引数。
type ImportArgs struct{}

var _ river.JobArgs = new(ImportArgs)

func (ImportArgs) Kind() string { return "anime_import" }

// ImportWorker は、連携先の情報を取り込むジョブを実行する。
type ImportWorker struct {
	river.WorkerDefaults[ImportArgs]

	importer *Importer
}

var _ river.Worker[ImportArgs] = new(ImportWorker)

// NewImportWorker は ImportWorker を作る。
func NewImportWorker(importer *Importer) *ImportWorker {
	return &ImportWorker{WorkerDefaults: river.WorkerDefaults[ImportArgs]{}, importer: importer}
}

func (w *ImportWorker) Work(ctx context.Context, _ *river.Job[ImportArgs]) error {
	result, err := w.importer.Import(ctx, false)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, result); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// ImportPeriodicJob は、取り込み直す時期が来たかを 1 時間ごとに確かめる定期ジョブを返す。
// 取り込み直すのは、前に問い合わせてから 1 日 (取り込めなかったときは 1 時間) が過ぎた取り込み元だけである。
func ImportPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(importCheckInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return ImportArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "anime_import", RunOnStart: true},
	)
}

// EnqueueImport は、連携先の情報を取り込むジョブを入れる。管理者が連携を有効にしたときに、1 時間を待たずに取り込むのに使う。
func EnqueueImport(ctx context.Context, db Database, inserter Inserter) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: ImportArgs{}, InsertOpts: nil}}); err != nil {
			return fmt.Errorf("failed to enqueue the anime import: %w", err)
		}

		return nil
	})
}
