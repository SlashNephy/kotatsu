package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"uuid"

	"filippo.io/age"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

const (
	// DefaultIntervalHours は、定期のバックアップを作成する間隔 (時間) の既定値 (付録 A)。
	DefaultIntervalHours = 24
	// DefaultGenerations は、定期のバックアップを残す世代の数の既定値 (付録 A)。
	DefaultGenerations = 7
	// maxIntervalHours と maxGenerations は、間隔と世代の数の上限。DB の制約と同じ。
	maxIntervalHours = 720
	maxGenerations   = 365
	// maxRecipients は、登録できる公開鍵の数の上限。
	maxRecipients = 16
	// keptRuns は、バックアップの結果を残す数。画面には最後の結果だけを出す。
	keptRuns = 20
	// checkInterval は、作成する時が来たかを確認する間隔。間隔は画面で変えるので、定期ジョブは短い間隔で動かし、最後の結果から決める。
	checkInterval = 15 * time.Minute
	// retryAfter は、失敗した後に、間隔を待たずに作り直すまでの時間。
	retryAfter = time.Hour
	// jobTimeout は、バックアップのジョブの時間の上限。DB が大きくても、数分で終わる見込み (開発用の DB で 3 秒)。
	jobTimeout = time.Hour
	// recordTimeout は、時間の上限を過ぎて取りやめたバックアップの結果を残すのに使う時間。
	recordTimeout = 30 * time.Second
	// FileNamePrefix と FileNameSuffix は、定期のバックアップのファイル名の前と後ろ。この形のファイルだけを、古い世代として消す。
	FileNamePrefix = "kotatsu-backup-"
	FileNameSuffix = ".age"
	// fileNameTimeLayout は、ファイル名に入れる、作成した時刻 (UTC) の形。名前の順が時刻の順になる。
	fileNameTimeLayout = "20060102T150405Z"
	// triggerSchedule と triggerManual は、backup_runs.trigger の値。
	triggerSchedule = "schedule"
	triggerManual   = "manual"
)

var (
	// ErrInvalidSettings は、定期のバックアップの設定の値が正しくないことを表す。
	ErrInvalidSettings = errors.New("invalid backup settings")
	// ErrNoBackupDir は、作成する先のディレクトリ (backup.dir) を設定していないことを表す。
	ErrNoBackupDir = errors.New("the backup directory (backup.dir) is not configured on nekonote")
	// ErrNoRuns は、まだ一度も作成していないことを表す。
	ErrNoRuns = errors.New("no backup has been written yet")
)

// Settings は、管理者の画面で変える、定期のバックアップの設定。
type Settings struct {
	// Recipients は、暗号化する age の公開鍵。空なら、定期のバックアップをしない。
	Recipients []string
	// IntervalHours は、作成する間隔 (時間)。
	IntervalHours int
	// Generations は、残す世代の数。
	Generations int
	// UpdatedAt は、最後に変えた日時。一度も変えていなければ nil。
	UpdatedAt *time.Time
}

// LoadSettings は、定期のバックアップの設定を読む。一度も変えていなければ、既定値を返す。
func LoadSettings(ctx context.Context, db sqlc.DBTX) (*Settings, error) {
	row, err := sqlc.New(db).GetBackupSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return &Settings{Recipients: []string{}, IntervalHours: DefaultIntervalHours, Generations: DefaultGenerations, UpdatedAt: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load the backup settings: %w", err)
	}

	return &Settings{Recipients: row.Recipients, IntervalHours: int(row.IntervalHours), Generations: int(row.Generations), UpdatedAt: &row.UpdatedAt}, nil
}

// SaveSettings は、定期のバックアップの設定を確認してから保存する。公開鍵は、前後の空白を除き、空の行と重なりを除いて持つ。
func SaveSettings(ctx context.Context, db sqlc.DBTX, now time.Time, s *Settings) error {
	recipients := []string{}
	for _, r := range s.Recipients {
		r = strings.TrimSpace(r)
		if r != "" && !slices.Contains(recipients, r) {
			recipients = append(recipients, r)
		}
	}
	switch {
	case len(recipients) > maxRecipients:
		return fmt.Errorf("%w: at most %d public keys can be registered", ErrInvalidSettings, maxRecipients)
	case s.IntervalHours < 1 || s.IntervalHours > maxIntervalHours:
		return fmt.Errorf("%w: the interval must be between 1 and %d hours", ErrInvalidSettings, maxIntervalHours)
	case s.Generations < 1 || s.Generations > maxGenerations:
		return fmt.Errorf("%w: the number of generations must be between 1 and %d", ErrInvalidSettings, maxGenerations)
	}
	if _, err := ParseRecipients(recipients); err != nil && len(recipients) > 0 {
		return err
	}
	if err := sqlc.New(db).SaveBackupSettings(ctx, sqlc.SaveBackupSettingsParams{
		Recipients: recipients, IntervalHours: int32(s.IntervalHours), Generations: int32(s.Generations), Now: now,
	}); err != nil {
		return fmt.Errorf("failed to save the backup settings: %w", err)
	}

	return nil
}

// ParseRecipients は、age の公開鍵 (age1...) を読む。
func ParseRecipients(keys []string) ([]age.Recipient, error) {
	if len(keys) == 0 {
		return nil, ErrNoRecipients
	}
	recipients := make([]age.Recipient, 0, len(keys))
	for _, key := range keys {
		// 1 行に 1 つのキーだけを受け付ける。コメント (#) や複数の行を渡されても、別のキーとして読まない。
		if strings.ContainsAny(key, "\n#") {
			return nil, fmt.Errorf("%w: invalid age public key", ErrInvalidSettings)
		}
		parsed, err := age.ParseRecipients(strings.NewReader(key))
		if err != nil || len(parsed) != 1 {
			return nil, fmt.Errorf("%w: invalid age public key %q", ErrInvalidSettings, key)
		}
		recipients = append(recipients, parsed[0])
	}

	return recipients, nil
}

// Run は、バックアップの結果。
type Run struct {
	ID uuid.UUID
	// Manual は、管理者が画面から頼んだものかどうか。
	Manual     bool
	StartedAt  time.Time
	FinishedAt time.Time
	// FileName と SizeBytes は、作成したファイルの名前と大きさ。失敗したら nil。
	FileName  *string
	SizeBytes *int64
	// Error は、失敗したときの理由。成功したら nil。
	Error *string
}

// LatestRun は、最後のバックアップの結果を返す。まだ作成していなければ ErrNoRuns を返す。
func LatestRun(ctx context.Context, db sqlc.DBTX) (*Run, error) {
	row, err := sqlc.New(db).GetLatestBackupRun(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoRuns
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load the latest backup run: %w", err)
	}

	return &Run{
		ID: row.ID, Manual: row.Trigger == triggerManual, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		FileName: row.FileName, SizeBytes: row.SizeBytes, Error: row.Error,
	}, nil
}

// RunArgs は、バックアップを作成するジョブの引数。
type RunArgs struct {
	// Manual は、管理者が画面から頼んだものかどうか。頼まれたものは、間隔を待たずに作成する。
	Manual bool `json:"manual" river:"unique"`
}

var _ river.JobArgsWithInsertOpts = new(RunArgs)

func (RunArgs) Kind() string { return "backup" }

func (RunArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: jobs.QueueBackup,
		// 失敗したら、次に確認する時 (retryAfter の後) に作り直すので、River では再試行しない。
		MaxAttempts: 1,
		// 待っているか動いている間は、同じ種類 (定期か、頼まれたものか) を重ねて入れない。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// PeriodicJob は、作成する時が来たかを checkInterval ごとに確認する定期ジョブを返す。nekonote が起動したときにも確認する。
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(checkInterval),
		func() (river.JobArgs, *river.InsertOpts) { return RunArgs{Manual: false}, nil },
		&river.PeriodicJobOpts{ID: "backup", RunOnStart: true},
	)
}

// Inserter は、ジョブを入れる。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// EnqueueManualTx は、tx の中で、今すぐ作成するジョブを入れる。頼まれたものを待っているか動かしていれば、入れずに false を返す。
func EnqueueManualTx(ctx context.Context, tx pgx.Tx, inserter Inserter) (bool, error) {
	results, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: RunArgs{Manual: true}, InsertOpts: nil}})
	if err != nil {
		return false, fmt.Errorf("failed to insert the backup job: %w", err)
	}

	return len(results) == 1 && !results[0].UniqueSkippedAsDuplicate, nil
}

// WorkerOptions は、NewWorker に渡す依存。
type WorkerOptions struct {
	DB *pgxpool.Pool
	// Store は、キャプチャの画像を読むデータディレクトリを持つ Store。
	Store *kago.Store
	// SecretKey は、アプリの秘密キー。
	SecretKey string
	// Dir は、作成する先のディレクトリ (backup.dir)。空なら、作成できない。
	Dir    string
	Clock  clock.Clock
	Logger *slog.Logger
}

// Worker は、定期と、頼まれたバックアップを、ディレクトリに作成する。作成し終えたら、残す世代の数より古いファイルを消す。
type Worker struct {
	river.WorkerDefaults[RunArgs]

	opts *WorkerOptions
}

var _ river.Worker[RunArgs] = new(Worker)

// NewWorker は Worker を作る。
func NewWorker(opts *WorkerOptions) *Worker {
	return &Worker{WorkerDefaults: river.WorkerDefaults[RunArgs]{}, opts: opts}
}

func (w *Worker) Timeout(*river.Job[RunArgs]) time.Duration {
	return jobTimeout
}

// Work は、作成する時が来ていれば (頼まれたものなら、いつでも)、作成して結果を残す。
// 公開鍵を登録していなければ、定期のものは何もしない。
func (w *Worker) Work(ctx context.Context, job *river.Job[RunArgs]) error {
	settings, err := LoadSettings(ctx, w.opts.DB)
	if err != nil {
		return err
	}
	if !job.Args.Manual {
		due, err := w.due(ctx, settings)
		if err != nil || !due {
			return err
		}
	}

	started := w.opts.Clock.Now()
	fileName, size, runErr := w.write(ctx, settings, started)
	// 時間の上限を過ぎて ctx が終わっていても、失敗したことを画面に出せるよう、取りやめを引き継がずに結果を残す。
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := w.record(recordCtx, job.Args.Manual, started, fileName, size, runErr); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return runErr
	}
	w.opts.Logger.InfoContext(ctx, "backup written", slog.String("file", fileName), slog.Int64("bytes", size), slog.Bool("manual", job.Args.Manual))
	if err := Prune(w.opts.Dir, settings.Generations); err != nil {
		// 作成は済んだので、古いファイルを消せなくても失敗にせず、ログに出して次に消す。
		w.opts.Logger.WarnContext(ctx, "failed to remove old backups", slog.Any("error", err))
	}

	return nil
}

// due は、定期のバックアップを作成する時が来たかを返す。公開鍵がなければ、来ていないとみなす。
// 最後の作成が成功していれば間隔を、失敗していれば retryAfter (間隔より短ければ間隔) を待つ。
func (w *Worker) due(ctx context.Context, settings *Settings) (bool, error) {
	if len(settings.Recipients) == 0 {
		return false, nil
	}
	latest, err := LatestRun(ctx, w.opts.DB)
	if errors.Is(err, ErrNoRuns) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	wait := time.Duration(settings.IntervalHours) * time.Hour
	if latest.Error != nil {
		wait = min(wait, retryAfter)
	}

	return !w.opts.Clock.Now().Before(latest.StartedAt.Add(wait)), nil
}

// write は、作成する先のディレクトリに、時刻の名前のファイルを書く。書いたファイルの名前と大きさを返す。
func (w *Worker) write(ctx context.Context, settings *Settings, now time.Time) (string, int64, error) {
	if w.opts.Dir == "" {
		return "", 0, ErrNoBackupDir
	}
	recipients, err := ParseRecipients(settings.Recipients)
	if err != nil {
		return "", 0, err
	}
	name := FileNamePrefix + now.UTC().Format(fileNameTimeLayout) + FileNameSuffix
	result, err := WriteFile(ctx, &WriteOptions{
		DB: w.opts.DB, Store: w.opts.Store, SecretKey: w.opts.SecretKey, Clock: w.opts.Clock, Logger: w.opts.Logger,
	}, filepath.Join(w.opts.Dir, name), recipients)
	if err != nil {
		return "", 0, err
	}

	return name, result.Bytes, nil
}

// record は、バックアップの結果を残し、古い結果を消し、管理者の画面に変わったことを通知する。
func (w *Worker) record(ctx context.Context, manual bool, started time.Time, fileName string, size int64, runErr error) error {
	trigger := triggerSchedule
	if manual {
		trigger = triggerManual
	}
	params := sqlc.InsertBackupRunParams{
		ID: id.New(), Trigger: trigger, StartedAt: started, FinishedAt: w.opts.Clock.Now(), FileName: nil, SizeBytes: nil, Error: nil,
	}
	if runErr != nil {
		params.Error = new(runErr.Error())
	} else {
		params.FileName, params.SizeBytes = new(fileName), new(size)
	}

	return pgx.BeginFunc(ctx, w.opts.DB, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.InsertBackupRun(ctx, params); err != nil {
			return fmt.Errorf("failed to record the backup run: %w", err)
		}
		if err := q.PruneBackupRuns(ctx, keptRuns); err != nil {
			return fmt.Errorf("failed to remove old backup runs: %w", err)
		}
		_, err := notify.Publish(ctx, tx, w.opts.Clock, notify.AuditChannel)

		return err
	})
}

// Prune は、dir の中の定期のバックアップ (FileNamePrefix と FileNameSuffix の名前のファイル) を、新しいものから generations 個だけ残して消す。
// ほかの名前のファイルと、書きかけの一時ファイルは消さない。
func Prune(dir string, generations int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to list the backup directory: %w", err)
	}
	names := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, FileNamePrefix) || !strings.HasSuffix(name, FileNameSuffix) {
			continue
		}
		// ファイルシステムによっては、一覧で種類が分からないので (DT_UNKNOWN)、ファイルの情報で確認する。
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	errs := []error{}
	for _, name := range names[:max(len(names)-generations, 0)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove the old backup %s: %w", name, err))
		}
	}

	return errors.Join(errs...)
}
