package drama

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Inserter は、ジョブを入れる River のクライアント。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// 照合のジョブの種類 (MatchArgs.Mode)。
const (
	// MatchModeChanges は、EPG の同期が「EPG が変わった」と通知したサービスと時間帯の番組を照合する。
	MatchModeChanges = "changes"
	// MatchModeHorizon は、これから 1 週間に放送される番組を照合し直す。
	MatchModeHorizon = "horizon"
	// MatchModeFinalize は、放送が終わった番組を照合し直し、結びつきを確定させる。
	MatchModeFinalize = "finalize"
)

const (
	// matchChangesInterval は、EPG の変更の記録を読む間隔 (付録 A)。TMDb への問い合わせは、キャッシュが古いときだけ行う。
	matchChangesInterval = time.Minute
	// matchHorizonInterval は、これから 1 週間に放送される番組を照合し直す間隔 (付録 A)。
	matchHorizonInterval = 6 * time.Hour
	// finalizeInterval は、放送が終わった番組の結びつきを確定させる間隔 (付録 A)。
	finalizeInterval = time.Hour
	// matchTimeout は、照合のジョブの時間の上限。問い合わせの間を空けて、上限 (DefaultMaxRequests) まで問い合わせることがある。
	matchTimeout = 5 * time.Minute
)

// MatchArgs は、ドラマの番組を照合するジョブの引数。
type MatchArgs struct {
	// Mode は、照合の種類 (MatchModeChanges など)。
	Mode string `json:"mode"`
}

var _ river.JobArgsWithInsertOpts = new(MatchArgs)

func (MatchArgs) Kind() string { return "drama_match" }

// InsertOpts は、同じ種類の照合を二重に動かさないようにする。前の照合が長引いても、次の照合はそれを待つ。
func (MatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// MatchWorker は、ドラマの番組を照合するジョブを実行する。
type MatchWorker struct {
	river.WorkerDefaults[MatchArgs]

	matcher *Matcher
}

var _ river.Worker[MatchArgs] = new(MatchWorker)

// NewMatchWorker は MatchWorker を作る。
func NewMatchWorker(matcher *Matcher) *MatchWorker {
	return &MatchWorker{WorkerDefaults: river.WorkerDefaults[MatchArgs]{}, matcher: matcher}
}

// Timeout は、照合のジョブの時間の上限を返す。
func (w *MatchWorker) Timeout(*river.Job[MatchArgs]) time.Duration {
	return matchTimeout
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
	default:
		return river.JobCancel(fmt.Errorf("unknown drama match mode %q", job.Args.Mode))
	}
	if err != nil {
		return err
	}
	// 照合しきれなかった番組は、次の定期の照合 (6 時間後など) を待たずに、少し空けてから続きを照合する。
	// 変更の記録の照合は、1 分ごとに読み直すので続けない。
	if result.Stopped && job.Args.Mode != MatchModeChanges {
		return river.JobSnooze(result.ContinueAfter)
	}
	if err := river.RecordOutput(ctx, result); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// MatchPeriodicJobs は、ドラマの番組を照合する定期ジョブを返す。
// EPG の変更の記録は 1 分ごとに、これから 1 週間の番組は 6 時間ごとに、放送が終わった番組の確定は 1 時間ごとに照合する。
func MatchPeriodicJobs() []*river.PeriodicJob {
	periodic := func(mode string, interval time.Duration) *river.PeriodicJob {
		return river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) {
				return MatchArgs{Mode: mode}, nil
			},
			&river.PeriodicJobOpts{ID: "drama_match_" + mode, RunOnStart: true},
		)
	}

	return []*river.PeriodicJob{
		periodic(MatchModeChanges, matchChangesInterval),
		periodic(MatchModeHorizon, matchHorizonInterval),
		periodic(MatchModeFinalize, finalizeInterval),
	}
}

// EnqueueMatchTx は、tx の中で、これから 1 週間の番組を照合し直すジョブを入れる。
// 管理者が連携を有効にしたときと、上書きを変えたときに、6 時間ごとの照合し直しを待たずに照合するのに使う。
func EnqueueMatchTx(ctx context.Context, tx pgx.Tx, inserter Inserter) error {
	if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{
		{Args: MatchArgs{Mode: MatchModeHorizon}, InsertOpts: nil},
		{Args: MatchArgs{Mode: MatchModeFinalize}, InsertOpts: nil},
	}); err != nil {
		return fmt.Errorf("failed to enqueue the drama match: %w", err)
	}

	return nil
}

// EnqueueStart は、これから 1 週間の番組と、放送が終わった番組を照合するジョブを入れる。管理者が連携を有効にしたときに使う。
func EnqueueStart(ctx context.Context, db Database, inserter Inserter) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return EnqueueMatchTx(ctx, tx, inserter)
	})
}
