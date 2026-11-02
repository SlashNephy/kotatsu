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

// importTimeout は、取り込みのジョブの時間の上限 (付録 A)。Annict と AniList から作品 ID を引くときは、
// 間を空けて 1 つずつ、60 回ほど問い合わせるので、River の既定の 1 分では足りない。
const importTimeout = 15 * time.Minute

// Inserter は、ジョブを入れる River のクライアント。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// ImportArgs は、連携先の情報を取り込むジョブの引数。
type ImportArgs struct {
	// Rematch なら、取り込み終えたら、チャンネルの対応が変わっていなくても、これから 1 週間の番組を照合するジョブを入れる。
	Rematch bool `json:"rematch,omitempty"`
}

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

// Timeout は、取り込みのジョブの時間の上限を返す。
func (w *ImportWorker) Timeout(*river.Job[ImportArgs]) time.Duration {
	return importTimeout
}

func (w *ImportWorker) Work(ctx context.Context, job *river.Job[ImportArgs]) error {
	result, err := w.importer.Import(ctx, false)
	if err != nil {
		return err
	}
	if job.Args.Rematch && !result.Disabled && w.importer.inserter != nil {
		if err := pgx.BeginFunc(ctx, w.importer.db, func(tx pgx.Tx) error {
			return EnqueueMatchTx(ctx, tx, w.importer.inserter)
		}); err != nil {
			return err
		}
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
			return ImportArgs{Rematch: false}, nil
		},
		&river.PeriodicJobOpts{ID: "anime_import", RunOnStart: true},
	)
}

// EnqueueStart は、連携先の情報を取り込むジョブと、これから 1 週間の番組を照合するジョブを入れる。
// 管理者が連携を有効にしたときに、定期ジョブを待たずに始めるのに使う。照合は、取り込みが終わってから動くよう、取り込みのジョブが入れる。
func EnqueueStart(ctx context.Context, db Database, inserter Inserter) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: ImportArgs{Rematch: true}, InsertOpts: nil}}); err != nil {
			return fmt.Errorf("failed to enqueue the anime import: %w", err)
		}

		return nil
	})
}

// 照合のジョブの種類 (MatchArgs.Mode)。
const (
	// MatchModeChanges は、EPG の同期が「EPG が変わった」と通知したサービスと時間帯の番組を照合する。
	MatchModeChanges = "changes"
	// MatchModeHorizon は、これから 1 週間に放送される番組を照合し直す。
	MatchModeHorizon = "horizon"
	// MatchModeFinalize は、放送が終わった番組を照合し直し、結びつきを確定させる。
	MatchModeFinalize = "finalize"
	// MatchModeImported は、EPGStation から取り込んだ録画を照合する。
	MatchModeImported = "imported"
)

const (
	// matchChangesInterval は、EPG の変更の記録を読む間隔 (付録 A)。しょぼいカレンダーへの問い合わせは、キャッシュが古いときだけ行う。
	matchChangesInterval = time.Minute
	// matchHorizonInterval は、これから 1 週間に放送される番組を照合し直す間隔 (付録 A)。
	matchHorizonInterval = 6 * time.Hour
	// finalizeInterval は、放送が終わった番組の結びつきを確定させる間隔 (付録 A)。アーカイブへ移すまでの猶予 (24 時間) の間に、何度も機会がある。
	finalizeInterval = time.Hour
	// matchImportedInterval は、EPGStation から取り込んだ録画を照合する間隔 (付録 A)。少しずつ照合し、しょぼいカレンダーへの問い合わせを散らす。
	matchImportedInterval = 10 * time.Minute
)

// MatchArgs は、アニメの番組を照合するジョブの引数。
type MatchArgs struct {
	// Mode は、照合の種類 (MatchModeChanges など)。
	Mode string `json:"mode"`
}

var _ river.JobArgsWithInsertOpts = new(MatchArgs)

func (MatchArgs) Kind() string { return "anime_match" }

// InsertOpts は、同じ種類の照合を二重に動かさないようにする。前の照合が長引いても、次の照合はそれを待つ。
func (MatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// MatchWorker は、アニメの番組を照合するジョブを実行する。
type MatchWorker struct {
	river.WorkerDefaults[MatchArgs]

	matcher *Matcher
}

var _ river.Worker[MatchArgs] = new(MatchWorker)

// NewMatchWorker は MatchWorker を作る。
func NewMatchWorker(matcher *Matcher) *MatchWorker {
	return &MatchWorker{WorkerDefaults: river.WorkerDefaults[MatchArgs]{}, matcher: matcher}
}

func (w *MatchWorker) Work(ctx context.Context, job *river.Job[MatchArgs]) error {
	var (
		result *MatchResult
		err    error
	)
	switch job.Args.Mode {
	case MatchModeChanges:
		result, err = w.matcher.MatchChanges(ctx)
	case MatchModeHorizon:
		result, err = w.matcher.MatchHorizon(ctx)
	case MatchModeFinalize:
		result, err = w.matcher.Finalize(ctx)
	case MatchModeImported:
		result, err = w.matcher.MatchImported(ctx)
	default:
		return river.JobCancel(fmt.Errorf("unknown anime match mode %q", job.Args.Mode))
	}
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, result); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// MatchPeriodicJobs は、アニメの番組を照合する定期ジョブを返す。
// EPG の変更の記録は 1 分ごとに、これから 1 週間の番組は 6 時間ごとに、放送が終わった番組の確定は 1 時間ごとに、
// EPGStation から取り込んだ録画は 10 分ごとに照合する。
func MatchPeriodicJobs() []*river.PeriodicJob {
	periodic := func(mode string, interval time.Duration) *river.PeriodicJob {
		return river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) {
				return MatchArgs{Mode: mode}, nil
			},
			&river.PeriodicJobOpts{ID: "anime_match_" + mode, RunOnStart: true},
		)
	}

	return []*river.PeriodicJob{
		periodic(MatchModeChanges, matchChangesInterval),
		periodic(MatchModeHorizon, matchHorizonInterval),
		periodic(MatchModeFinalize, finalizeInterval),
		periodic(MatchModeImported, matchImportedInterval),
	}
}

// EnqueueMatchTx は、tx の中で、これから 1 週間の番組を照合し直すジョブを入れる。チャンネルの対応が変わったときに使う。
func EnqueueMatchTx(ctx context.Context, tx pgx.Tx, inserter Inserter) error {
	if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: MatchArgs{Mode: MatchModeHorizon}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the anime match: %w", err)
	}

	return nil
}
