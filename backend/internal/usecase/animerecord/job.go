package animerecord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// recordMaxAttempts は、記録を送る回数の上限。River の間隔で、1 時間ほど送り直してから諦める。
	recordMaxAttempts = 8
	// recordTimeout は、記録を送るジョブの時間の上限。1 話ごとに、作品とエピソードの一覧 (10 ページまで)、記録、作品の状態の
	// 十数回の問い合わせ (1 回 20 秒まで) をするので、River の既定の 1 分では足りない。途中で切れても、送れた話は残る。
	recordTimeout = 15 * time.Minute
	// pruneInterval は、見た区間の古い記録を消す間隔。
	pruneInterval = 24 * time.Hour
	// progressRetention は、見た区間を残す期間 (付録 A)。最後に足してから、この期間がたったものは消す。
	progressRetention = 90 * 24 * time.Hour
)

// 記録の状態 (anime_records の state)。
const (
	stateRecorded = "recorded"
	stateFailed   = "failed"
)

// RecordArgs は、ユーザーが見た番組の話を、1 つの記録先に記録するジョブの引数。トークンは載せず、ジョブが DB から引く。
type RecordArgs struct {
	UserID      uuid.UUID `json:"userId"`
	ServiceID   int64     `json:"serviceId"`
	StartAt     time.Time `json:"startAt"`
	Destination string    `json:"destination"`
}

var _ river.JobArgsWithInsertOpts = new(RecordArgs)

func (RecordArgs) Kind() string { return "anime_record" }

// InsertOpts は、同じ番組と記録先のジョブを、終わるまでは 1 つにする (River は、動いているジョブも重ねない)。
// 動いている間に、照合し直しなどで同じ番組と記録先の話が増えると、ジョブは入らない。そこで、ジョブは、送る話がなくなるまで引き直す (Record)。
func (RecordArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: recordMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// RecordWorker は、記録を送るジョブを実行する。
type RecordWorker struct {
	river.WorkerDefaults[RecordArgs]

	service *Service
	db      Database
}

var _ river.Worker[RecordArgs] = new(RecordWorker)

// NewRecordWorker は RecordWorker を作る。
func NewRecordWorker(service *Service, db Database) *RecordWorker {
	return &RecordWorker{WorkerDefaults: river.WorkerDefaults[RecordArgs]{}, service: service, db: db}
}

// Timeout は、記録を送るジョブの時間の上限を返す。
func (w *RecordWorker) Timeout(*river.Job[RecordArgs]) time.Duration {
	return recordTimeout
}

func (w *RecordWorker) Work(ctx context.Context, job *river.Job[RecordArgs]) error {
	return w.service.Record(ctx, w.db, &job.Args, job.Attempt >= job.MaxAttempts)
}

// Record は、args の番組の、まだ送っていない話を、記録先に 1 話ずつ送る。送れた話は recorded にし、送り直しても送らない。
// 送り直しても記録できない話 (エピソードが見つからない、トークンが使えないなど) は failed にする。
// ほかのエラーは、River の再試行に任せる。last なら、最後の試行なので、送れなかった話を failed にする。
// 結果は、送れた話と送れなかった話に分けて、ユーザーに通知する。
func (s *Service) Record(ctx context.Context, db Database, args *RecordArgs, last bool) error {
	destination, err := s.destination(args.Destination)
	if err != nil {
		return river.JobCancel(err)
	}
	outcome := &recordOutcome{recorded: []*sqlc.AnimeRecord{}, failed: []*sqlc.AnimeRecord{}, url: "", reason: ""}
	// attempted は、このジョブで送ろうとした記録。送り直す記録は、River の再試行に任せ、このジョブでは引き直さない。
	attempted := map[uuid.UUID]bool{}
	var retry error
	for {
		rows, err := s.untried(ctx, db, args, attempted)
		if err != nil {
			retry = err

			break
		}
		if len(rows) == 0 {
			break
		}
		if err := s.send(ctx, db, destination, args, rows, last, outcome); err != nil {
			retry = err
		}
	}
	// 通知は、ジョブの期限が来ても、送れた話を知らせられるよう、期限で取り消さない context で書く。
	if err := s.notify(context.WithoutCancel(ctx), db, args, outcome); err != nil {
		return err
	}

	return retry
}

// untried は、args の番組と記録先の、まだ送っていない記録のうち、このジョブで送ろうとしていないものを返し、送ろうとしたものに足す。
// 動いている間に増えた記録も送るため、送る記録がなくなるまで引き直す。
func (s *Service) untried(ctx context.Context, db sqlc.DBTX, args *RecordArgs, attempted map[uuid.UUID]bool) ([]sqlc.AnimeRecord, error) {
	rows, err := sqlc.New(db).ListPendingAnimeRecords(ctx, sqlc.ListPendingAnimeRecordsParams{
		UserID: args.UserID, ServiceID: args.ServiceID, StartAt: args.StartAt, Destination: args.Destination,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the pending anime records: %w", err)
	}
	untried := []sqlc.AnimeRecord{}
	for _, row := range rows {
		if !attempted[row.ID] {
			attempted[row.ID] = true
			untried = append(untried, row)
		}
	}

	return untried, nil
}

// recordOutcome は、1 回のジョブで送れた話と、諦めた話。
type recordOutcome struct {
	recorded []*sqlc.AnimeRecord
	failed   []*sqlc.AnimeRecord
	// url は、最初に記録した作品かエピソードのページ。
	url string
	// reason は、最初に諦めた理由。
	reason string
}

// send は、話を 1 話ずつ送る。送り直すエラーがあれば、それを返す。
func (s *Service) send(
	ctx context.Context, db Database, destination Destination, args *RecordArgs, rows []sqlc.AnimeRecord, last bool, outcome *recordOutcome,
) error {
	token, err := s.prepare(ctx, db, args)
	if err != nil {
		if !errors.Is(err, ErrPermanent) && !last {
			return err
		}
		for i := range rows {
			if err := s.finish(ctx, db, &rows[i], stateFailed, nil, err, outcome); err != nil {
				return err
			}
		}

		return nil
	}
	userSettings, err := LoadSettings(ctx, db, args.UserID)
	if err != nil {
		return err
	}
	tids := make([]int32, 0, len(rows))
	for i := range rows {
		tids = append(tids, rows[i].SyobocalTid)
	}
	works, err := anime.ListWorks(ctx, db, tids)
	if err != nil {
		return err
	}
	var retry error
	for i := range rows {
		row := &rows[i]
		episode := &Episode{Number: row.EpisodeNumber, Subtitle: row.Subtitle, First: row.IsFirst, Last: row.IsLast}
		recorded, err := destination.RecordEpisode(ctx, token, destination.WorkIDs(works[row.SyobocalTid]), episode, userSettings.Rules)
		switch {
		case err == nil:
			// 記録を付けたら、ジョブの期限が来ていても、付けたことを残す。残せないと、送り直しで記録が二重になる。
			if err = s.finish(context.WithoutCancel(ctx), db, row, stateRecorded, &recorded.URL, nil, outcome); err == nil {
				s.followUp(ctx, args, recorded)
			}
		case errors.Is(err, ErrPermanent) || last:
			err = s.finish(ctx, db, row, stateFailed, nil, err, outcome)
		default:
			s.logger.WarnContext(ctx, "failed to send an anime record; will retry",
				slog.String("destination", args.Destination), slog.Any("error", err))
			retry = err

			continue
		}
		if err != nil {
			return err
		}
	}

	return retry
}

// followUp は、記録を付けた後の作品の状態の変更を行う。できたらよい操作なので、失敗してもログに残すだけにし、記録は付け直さない。
func (s *Service) followUp(ctx context.Context, args *RecordArgs, recorded *Recorded) {
	if recorded.FollowUp == nil {
		return
	}
	if err := recorded.FollowUp(ctx); err != nil {
		s.logger.WarnContext(ctx, "failed to update the work status after recording",
			slog.String("destination", args.Destination), slog.Any("error", err))
	}
}

// prepare は、記録を送れるかを確かめ、ユーザーのトークンを返す。連携が無効か、連携していなければ、ErrPermanent を包んで返す。
func (s *Service) prepare(ctx context.Context, db sqlc.DBTX, args *RecordArgs) (string, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return "", err
	}
	if !system.AnimeLinkingEnabled {
		return "", fmt.Errorf("%w: %w", ErrPermanent, ErrUnavailable)
	}
	token, err := s.token(ctx, db, args.UserID, args.Destination)
	if errors.Is(err, ErrNotConnected) {
		return "", fmt.Errorf("%w: %w", ErrPermanent, err)
	}

	return token, err
}

// finish は、記録 row を state にし、結果を outcome に足す。
func (s *Service) finish(ctx context.Context, db sqlc.DBTX, row *sqlc.AnimeRecord, state string, url *string, cause error, outcome *recordOutcome) error {
	var reason *string
	if cause != nil {
		reason = new(failureReason(cause))
	}
	if err := sqlc.New(db).FinishAnimeRecord(ctx, sqlc.FinishAnimeRecordParams{State: state, URL: url, Error: reason, Now: s.clock.Now(), ID: row.ID}); err != nil {
		return fmt.Errorf("failed to finish the anime record: %w", err)
	}
	if state == stateRecorded {
		outcome.recorded = append(outcome.recorded, row)
		if outcome.url == "" && url != nil {
			outcome.url = *url
		}

		return nil
	}
	outcome.failed = append(outcome.failed, row)
	if outcome.reason == "" && reason != nil {
		outcome.reason = *reason
	}

	return nil
}

// failureReason は、記録できなかった理由を、通知に出す文にする。トークンなどの中身は、もともとエラーに入れない。
func failureReason(err error) string {
	switch {
	case errors.Is(err, ErrNotConnected):
		return "連携していません"
	case errors.Is(err, ErrUnavailable):
		return "連携が無効になっています"
	default:
		return shorten(strings.TrimPrefix(err.Error(), ErrPermanent.Error()+": "))
	}
}

// notify は、送れた話と諦めた話を、それぞれ 1 件の通知にして、ユーザーだけに届ける。
func (s *Service) notify(ctx context.Context, db Database, args *RecordArgs, outcome *recordOutcome) error {
	if len(outcome.recorded) == 0 && len(outcome.failed) == 0 {
		return nil
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		audience := &notification.Audience{UserIDs: []uuid.UUID{args.UserID}, ByRule: false}
		if len(outcome.recorded) > 0 {
			payload := recordPayload(args, outcome.recorded)
			if outcome.url != "" {
				payload["externalUrl"] = outcome.url
			}
			if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindAnimeRecorded, payload, audience); err != nil {
				return err
			}
		}
		if len(outcome.failed) > 0 {
			payload := recordPayload(args, outcome.failed)
			payload["reason"] = outcome.reason
			if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindAnimeRecordFailed, payload, audience); err != nil {
				return err
			}
		}

		return nil
	})
}

// recordPayload は、通知の材料を作る。番組名、記録先、話数 (「#1、#2」の形) を載せる。
func recordPayload(args *RecordArgs, rows []*sqlc.AnimeRecord) map[string]any {
	episodes := []string{}
	for _, row := range rows {
		if row.EpisodeNumber != nil {
			episodes = append(episodes, "#"+strconv.FormatFloat(*row.EpisodeNumber, 'f', -1, 64))
		}
	}
	name := ""
	if len(rows) > 0 {
		name = rows[0].ProgramName
	}

	return map[string]any{
		"name": name, "destination": args.Destination, "episodes": strings.Join(episodes, "、"),
		"serviceId": args.ServiceID, "startAt": args.StartAt.UTC().Format(time.RFC3339Nano),
	}
}

// PruneArgs は、見た区間の古い記録を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "anime_watch_progress_prune" }

// PruneWorker は、見た区間の古い記録を消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	service *Service
	db      sqlc.DBTX
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(service *Service, db sqlc.DBTX) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, service: service, db: db}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	q := sqlc.New(w.db)
	now := w.service.clock.Now()
	if _, err := q.PruneAnimeWatchProgress(ctx, now.Add(-progressRetention)); err != nil {
		return fmt.Errorf("failed to prune the watch progress: %w", err)
	}
	if _, err := q.DeleteExpiredAnimeRecordAuthorizations(ctx, now); err != nil {
		return fmt.Errorf("failed to delete expired anime record authorizations: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、見た区間の古い記録を 1 日ごとに消す定期ジョブを返す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "anime_watch_progress_prune", RunOnStart: false},
	)
}
