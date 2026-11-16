package captionmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

//go:generate go tool go-enum --values --nocomments

// State は、種類ごとのモデルの置き場所の状態。
//
// ENUM(
// not_placed // 置いていない。
// downloading // ダウンロードを待っているか、ダウンロードしている。
// placed // 置いた。画面から置いたものも、手で置いたものも含む。
// failed // 最後のダウンロードが、試す回数を使い切ったか、照合に失敗して取りやめた。前に置いたモデルがあれば、それは残る。
// )
type State string

// Installed は、既定の場所に置いてあるモデル。
type Installed struct {
	// Model は、画面から置いたモデル。手で置いたファイルや、記録と大きさが違うファイルなら nil。
	Model *Model
	Size  int64
	// InstalledAt は、画面から置いた日時。手で置いたファイルなら nil。
	InstalledAt *time.Time
}

// Progress は、ダウンロードしているジョブの進み具合。
type Progress struct {
	Ratio float64
	// Remaining は残り時間。分からなければ nil。
	Remaining *time.Duration
	UpdatedAt time.Time
}

// Download は、種類ごとの、最後に入れたダウンロードのジョブ。終えていないか、失敗したものだけを返す。
type Download struct {
	JobID int64
	// Model は、ダウンロードしているか、ダウンロードに失敗したモデル。一覧から消えたモデルなら nil。
	Model *Model
	// Running は、動いているかどうか。false なら、待っているか、失敗した。
	Running bool
	// Failed は、失敗したかどうか。
	Failed bool
	// Progress は、動いているときの、今の試行の進み具合。まだ書いていなければ nil。
	Progress *Progress
	// AttemptedAt は、今の試行を始めた日時。始めていなければ nil。
	AttemptedAt *time.Time
}

// Status は、種類ごとのモデルの状態。
type Status struct {
	Kind  Kind
	State State
	// Installed は、既定の場所に置いてあるモデル。なければ nil。
	Installed *Installed
	// Download は、終えていないか、失敗した最後のダウンロード。なければ nil。
	Download *Download
	// Overridden は、設定でモデルの場所を書いていて、既定の場所のモデルを使わないかどうか。
	Overridden bool
}

// Statuses は、種類ごとのモデルの状態を、Whisper、VAD の順に返す。
// 置いたモデルは、DB の記録と、データディレクトリ dataDir のファイルから決める。データディレクトリをマウントしていなければ (空なら)、
// 記録だけで決める。ダウンロードは、River の表の、種類ごとの最後のジョブから決める。cfg は、モデルの場所を書いたかを見る設定。
func Statuses(ctx context.Context, db sqlc.DBTX, dataDir string, cfg *config.Captions) ([]*Status, error) {
	records, err := sqlc.New(db).ListCaptionModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the caption models: %w", err)
	}
	jobs, err := lastJobs(ctx, db)
	if err != nil {
		return nil, err
	}
	statuses := make([]*Status, 0, len(KindValues()))
	for _, kind := range KindValues() {
		var record *sqlc.CaptionModel
		for i := range records {
			if records[i].Kind == string(kind) {
				record = &records[i]
			}
		}
		installed, placed, err := installedOf(kind, record, dataDir)
		if err != nil {
			return nil, err
		}
		status := &Status{Kind: kind, State: StateNotPlaced, Installed: nil, Download: jobs[kind], Overridden: Overridden(cfg, kind)}
		if placed {
			status.Installed = installed
		}
		if placedByAttempt(status.Installed, status.Download) {
			// 置いて記録してから River がジョブを完了にするまでの間は、まだ動いているように見える。
			// 置いたことを知らせた後に読み直した画面に、ダウンロードしているように出さないよう、今の試行で置いていれば終えたとする。
			status.Download = nil
		}
		switch {
		case status.Download != nil && !status.Download.Failed:
			status.State = StateDownloading
		case status.Download != nil:
			status.State = StateFailed
		case placed:
			status.State = StatePlaced
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// placedByAttempt は、動いているダウンロード download が、今の試行で installed を置き終えたかを返す。
func placedByAttempt(installed *Installed, download *Download) bool {
	if installed == nil || installed.InstalledAt == nil || download == nil || !download.Running || download.AttemptedAt == nil {
		return false
	}

	return !installed.InstalledAt.Before(*download.AttemptedAt)
}

// installedOf は、種類 kind の既定の場所のモデルを返す。置いていなければ false を返す。記録があっても、ファイルがなければ置いていないとし、
// 大きさが記録と違えば、手で置き換えたものとする。
func installedOf(kind Kind, record *sqlc.CaptionModel, dataDir string) (*Installed, bool, error) {
	var recorded *Installed
	if record != nil {
		recorded = &Installed{Model: nil, Size: record.Size, InstalledAt: &record.InstalledAt}
		if model, ok := Lookup(record.ModelID); ok {
			recorded.Model = model
		}
	}
	if dataDir == "" {
		if recorded == nil {
			return nil, false, nil
		}

		return recorded, true, nil
	}
	info, err := os.Stat(Path(dataDir, kind))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to stat the caption model: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if recorded != nil && recorded.Size == info.Size() {
		return recorded, true, nil
	}

	return &Installed{Model: nil, Size: info.Size(), InstalledAt: nil}, true, nil
}

// lastJobs は、種類ごとに、最後に入れたダウンロードのジョブのうち、終えていないものと失敗したものを返す。
// 取りやめた (画面から取り消した) ジョブと、終えたジョブは返さない。River の表は sqlc のスキーマにないので、直接読む。
func lastJobs(ctx context.Context, db sqlc.DBTX) (map[Kind]*Download, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT ON (j.args->>'kind') j.id, j.args, j.state::text, j.attempted_at,
			j.errors[array_length(j.errors, 1)]->>'error', p.ratio, p.remaining_ms, p.updated_at
		FROM river_job j
		LEFT JOIN job_progress p ON p.job_id = j.id AND p.attempt = j.attempt
		WHERE j.kind = $1
		ORDER BY j.args->>'kind', j.id DESC`, new(DownloadArgs).Kind())
	if err != nil {
		return nil, fmt.Errorf("failed to list the caption model download jobs: %w", err)
	}
	defer rows.Close()
	downloads := map[Kind]*Download{}
	for rows.Next() {
		var (
			id          int64
			rawArgs     []byte
			state       string
			attemptedAt *time.Time
			lastError   *string
			ratio       *float64
			remainingMs *int64
			updatedAt   *time.Time
		)
		if err := rows.Scan(&id, &rawArgs, &state, &attemptedAt, &lastError, &ratio, &remainingMs, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to read a caption model download job: %w", err)
		}
		var args DownloadArgs
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("failed to decode a caption model download job: %w", err)
		}
		download := downloadOf(id, &args, state, lastError)
		if download == nil {
			continue
		}
		download.AttemptedAt = attemptedAt
		if download.Running && ratio != nil && updatedAt != nil {
			download.Progress = &Progress{Ratio: *ratio, Remaining: nil, UpdatedAt: *updatedAt}
			if remainingMs != nil {
				download.Progress.Remaining = new(time.Duration(*remainingMs) * time.Millisecond)
			}
		}
		downloads[args.Slot] = download
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the caption model download jobs: %w", err)
	}

	return downloads, nil
}

// downloadOf は、ジョブの状態 state から、画面に出すダウンロードを返す。終えたジョブと、画面から取り消したジョブは nil を返す。
func downloadOf(id int64, args *DownloadArgs, state string, lastError *string) *Download {
	download := &Download{JobID: id, Model: nil, Running: false, Failed: false, Progress: nil, AttemptedAt: nil}
	if model, ok := Lookup(args.ModelID); ok {
		download.Model = model
	}
	switch state {
	case "available", "scheduled", "pending", "retryable":
		return download
	case "running":
		download.Running = true

		return download
	case "discarded":
		download.Failed = true

		return download
	case "cancelled":
		// 待っている間に取り消したジョブにはエラーがなく、動いている間に取り消したジョブには、River が取り消したことを残す。
		if lastError == nil || strings.Contains(*lastError, cancelledRemotely) {
			return nil
		}
		download.Failed = true

		return download
	default:
		return nil
	}
}

// cancelledRemotely は、動いているジョブを取り消したときに、River が残すエラーの文 (river.ErrJobCancelledRemotely)。
const cancelledRemotely = "job cancelled remotely"

// ErrNotFound は、取り消すジョブや、消すモデルがないことを表す。
var ErrNotFound = errors.New("not found")

// ErrDownloading は、ダウンロードしている間に、そのモデルを消そうとしたことを表す。
var ErrDownloading = errors.New("the model is being downloaded")

// ActiveJob は、種類 kind の、終えていないダウンロードのジョブの ID を返す。なければ ErrNotFound を返す。
func ActiveJob(ctx context.Context, db sqlc.DBTX, kind Kind) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM river_job WHERE kind = $1 AND args->>'kind' = $2
		AND state NOT IN ('completed', 'cancelled', 'discarded') ORDER BY id DESC LIMIT 1`,
		new(DownloadArgs).Kind(), string(kind)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get the caption model download job: %w", err)
	}

	return id, nil
}

// Remove は、データディレクトリ dataDir の既定の場所から、種類 kind のモデルを消し、tx で記録を消す。
// ダウンロードしている間は ErrDownloading を、ファイルも記録もなければ ErrNotFound を返す。消したモデルの記録 (なければ nil) を返す。
func Remove(ctx context.Context, tx pgx.Tx, dataDir string, kind Kind) (*sqlc.CaptionModel, error) {
	if dataDir == "" {
		return nil, ErrNoDataDir
	}
	if _, err := ActiveJob(ctx, tx, kind); err == nil {
		return nil, ErrDownloading
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	q := sqlc.New(tx)
	records, err := q.ListCaptionModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the caption models: %w", err)
	}
	var removed *sqlc.CaptionModel
	for i := range records {
		if records[i].Kind == string(kind) {
			removed = &records[i]
		}
	}
	deleted, err := q.DeleteCaptionModel(ctx, string(kind))
	if err != nil {
		return nil, fmt.Errorf("failed to delete the caption model: %w", err)
	}
	err = os.Remove(Path(dataDir, kind))
	if errors.Is(err, os.ErrNotExist) {
		if deleted == 0 {
			return nil, ErrNotFound
		}

		return removed, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to remove the caption model: %w", err)
	}

	return removed, nil
}
