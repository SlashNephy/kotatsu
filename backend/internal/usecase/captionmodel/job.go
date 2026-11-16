package captionmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/jobprogress"
)

const (
	// maxAttempts は、ダウンロードするジョブを試す回数の上限。通信が途切れたときに、やり直す。
	maxAttempts = 3
	// timeout は、ダウンロードするジョブの時間の上限。いちばん大きいモデル (550 MiB ほど) を、1 Mbps でも落とせるよう長くとる。
	// ジョブを止まったとみなす時間 (jobs の rescueStuckJobsAfter、3 時間) より短くする。
	timeout = 2*time.Hour + 30*time.Minute
	// partSuffix は、ダウンロードしている途中のファイルの名前の後ろに付けるもの。照合し終えてから、置き場所の名前に変える。
	partSuffix = ".part"
	// observeInterval は、進み具合を測る間隔。書き戻すのは ffprogress.WriteInterval ごと。
	observeInterval = time.Second
	// maxRedirects は、たどる転送の数の上限。
	maxRedirects = 5
)

var (
	// ErrUnknownModel は、一覧にないモデルを頼まれたことを表す。
	ErrUnknownModel = errors.New("unknown caption model")
	// ErrNoDataDir は、データディレクトリが設定されていず、モデルを置けないことを表す。
	ErrNoDataDir = errors.New("the data directory is not configured")
	// errChecksumMismatch は、ダウンロードしたファイルの SHA-256 が一覧と違うことを表す。入手先のファイルが変わったので、やり直さない。
	errChecksumMismatch = errors.New("the SHA-256 of the downloaded model does not match")
	// errSizeMismatch は、ダウンロードしたファイルの大きさが一覧と違うことを表す。途中で切れたかもしれないので、やり直す。
	errSizeMismatch = errors.New("the size of the downloaded model does not match")
	// errRedirectNotAllowed は、Hugging Face とその配信網でない所へ転送されたことを表す。
	errRedirectNotAllowed = errors.New("redirect to a host other than Hugging Face is not allowed")
)

// DownloadArgs は、モデルをダウンロードして置くジョブの引数。
type DownloadArgs struct {
	// Slot は、置く場所を決める種類。同じ種類のジョブは、同じファイルに書くので、重ねて入れない。
	Slot Kind `json:"kind" river:"unique"`
	// ModelID は、一覧のモデルの ID。
	ModelID string `json:"modelId"`
}

var _ river.JobArgsWithInsertOpts = new(DownloadArgs)

func (DownloadArgs) Kind() string { return "caption_model_download" }

func (DownloadArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttempts,
		// 同じ種類のジョブは、待っているか動いている間に重ねて入れない。終えたジョブには重ねてよい (置き直せるようにする)。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeStates()},
	}
}

// activeStates は、終えていないジョブの状態。
func activeStates() []rivertype.JobState {
	return []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
		rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// EnqueueTx は、tx の中で、モデル model をダウンロードするジョブを入れる。同じ種類のジョブを待っているか動かしていれば、入れずに false を返す。
func EnqueueTx(ctx context.Context, tx pgx.Tx, inserter Inserter, model *Model) (bool, error) {
	results, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{
		{Args: DownloadArgs{Slot: model.Kind, ModelID: model.ID}, InsertOpts: nil},
	})
	if err != nil {
		return false, fmt.Errorf("failed to insert the caption model download job: %w", err)
	}

	return len(results) == 1 && !results[0].UniqueSkippedAsDuplicate, nil
}

// Database は、ダウンロードに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Worker は、一覧のモデルをダウンロードし、SHA-256 を照合してから、データディレクトリの既定の場所に置く。
type Worker struct {
	river.WorkerDefaults[DownloadArgs]

	db      Database
	clock   clock.Clock
	logger  *slog.Logger
	client  *http.Client
	dataDir string
	// catalog は、モデルの一覧。テストでは、手元のサーバーの URL に差し替える。
	catalog func() []Model
}

var _ river.Worker[DownloadArgs] = new(Worker)

// NewWorker は Worker を返す。recorder は、外部との通信の記録 (設計書 11.8)。dataDir は、モデルを置くデータディレクトリ。
func NewWorker(db Database, clk clock.Clock, logger *slog.Logger, recorder *netactivity.Recorder, dataDir string) *Worker {
	return &Worker{
		WorkerDefaults: river.WorkerDefaults[DownloadArgs]{}, db: db, clock: clk, logger: logger,
		client: NewHTTPClient(recorder, nil), dataDir: dataDir, catalog: Catalog,
	}
}

// NewHTTPClient は、モデルをダウンロードする HTTP クライアントを返す。本文を読み終えるまで時間がかかるので、クライアントの時間の上限は設けず、
// ジョブの時間の上限で止める。転送は、HTTPS の Hugging Face とその配信網 (*.hf.co) にだけ従う。transport が nil なら既定のものを使う。
func NewHTTPClient(recorder *netactivity.Recorder, transport http.RoundTripper) *http.Client {
	return netactivity.NewClient(recorder, netactivity.PurposeCaptionModelDownload, netactivity.ClientOptions{
		Transport: transport, Timeout: 0, CheckRedirect: checkRedirect,
	})
}

// checkRedirect は、Hugging Face の resolve の URL から、配信網への転送にだけ従う。
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if !allowedHost(req.URL.Scheme, req.URL.Hostname()) {
		return fmt.Errorf("%w: %s", errRedirectNotAllowed, req.URL.Hostname())
	}

	return nil
}

// allowedHost は、ダウンロードで接続してよい先かを返す。HTTPS の huggingface.co と、その配信網 (hf.co の下) だけを許す。
func allowedHost(scheme, host string) bool {
	if scheme != "https" {
		return false
	}

	return host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co") || host == "hf.co" || strings.HasSuffix(host, ".hf.co")
}

func (w *Worker) Timeout(*river.Job[DownloadArgs]) time.Duration {
	return timeout
}

// Work は、モデルを .part に書き、大きさと SHA-256 を照合してから、置き場所の名前に変え、DB に記録する。
// 取りやめられたか、失敗したら、.part を消す。前に置いたモデルは、置き換えるまで残る。
func (w *Worker) Work(ctx context.Context, job *river.Job[DownloadArgs]) error {
	model, ok := find(w.catalog(), job.Args.ModelID)
	if !ok || model.Kind != job.Args.Slot {
		return river.JobCancel(fmt.Errorf("%w: %s", ErrUnknownModel, job.Args.ModelID))
	}
	if w.dataDir == "" {
		return river.JobCancel(ErrNoDataDir)
	}
	dest := Path(w.dataDir, model.Kind)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("failed to create the directory for the model: %w", err)
	}
	part := dest + partSuffix
	defer func() { _ = os.Remove(part) }()

	sum, err := w.download(ctx, job, model, part)
	if err != nil {
		return err
	}
	if sum != model.SHA256 {
		return river.JobCancel(fmt.Errorf("%w: got %s, want %s", errChecksumMismatch, sum, model.SHA256))
	}
	if err := os.Rename(part, dest); err != nil {
		return fmt.Errorf("failed to place the model: %w", err)
	}

	return pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		err := sqlc.New(tx).UpsertCaptionModel(ctx, sqlc.UpsertCaptionModelParams{
			Kind: string(model.Kind), ModelID: model.ID, Size: model.Size, SHA256: model.SHA256, Now: w.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to record the caption model: %w", err)
		}
		// 管理の画面 (録画の既定値) を読み直させる。
		_, err = notify.Publish(ctx, tx, w.clock, notify.AuditChannel)

		return err
	})
}

// download は、モデル model を part に書き、SHA-256 を返す。書いた大きさが一覧と違えば、errSizeMismatch を返す。
func (w *Worker) download(ctx context.Context, job *river.Job[DownloadArgs], model *Model, part string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, model.URL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create the request: %w", err)
	}
	// 個人やこのサーバーの分かる値は送らない。版とプロジェクトの URL だけを名乗る。
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download the model: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download the model: unexpected status %d", res.StatusCode)
	}
	if res.ContentLength >= 0 && res.ContentLength != model.Size {
		return "", fmt.Errorf("%w: the server sends %d bytes, want %d", errSizeMismatch, res.ContentLength, model.Size)
	}
	file, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to create the model file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var written atomic.Int64
	stop := w.observe(ctx, job, model.Size, written.Load)
	hash := sha256.New()
	// 一覧より大きいものは読み切らない。1 バイト多く読めたら、大きさが違うと分かる。
	n, err := io.Copy(io.MultiWriter(file, hash, counter{&written}), io.LimitReader(res.Body, model.Size+1))
	stop(err == nil && n == model.Size)
	if err != nil {
		return "", fmt.Errorf("failed to download the model: %w", err)
	}
	if n != model.Size {
		return "", fmt.Errorf("%w: got %d bytes, want %d", errSizeMismatch, n, model.Size)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("failed to write the model file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("failed to write the model file: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// counter は、書いたバイト数を数える。
type counter struct{ n *atomic.Int64 }

func (c counter) Write(p []byte) (int, error) {
	c.n.Add(int64(len(p)))

	return len(p), nil
}

// observe は、書いたバイト数 read を一定の間隔で測り、進み具合 (割合と残り時間) をジョブの画面と管理の画面に書き戻す。
// 返す関数で止める。done が true なら、終えたとして 100% を書く。
func (w *Worker) observe(ctx context.Context, job *river.Job[DownloadArgs], size int64, read func() int64) func(done bool) {
	logger := w.logger.With(slog.Int64("job_id", job.ID), slog.String("model", job.Args.ModelID))
	reporter := ffprogress.NewReporter(ffprogress.DefaultConfig(), ffprogress.Estimate{Frames: 0, Bytes: size}, w.clock, read,
		jobprogress.Saver(ctx, w.db, w.clock, logger, job.ID, job.Attempt), logger)
	block := func(end bool) *ffprogress.Block {
		return &ffprogress.Block{Frame: -1, TotalSize: -1, DropFrames: -1, Bitrate: "", Speed: "", End: end}
	}
	quit := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(observeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				reporter.Observe(ctx, block(false))
			}
		}
	}()

	return func(done bool) {
		close(quit)
		<-finished
		if done {
			reporter.Observe(ctx, block(true))
		}
		reporter.Finish(ctx)
	}
}
