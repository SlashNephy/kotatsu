package dramarecord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	// recordMaxAttempts は、記録を送る回数の上限。River の間隔で、1 時間ほど送り直してから諦める。アニメの記録と同じ。
	recordMaxAttempts = 8
	// recordTimeout は、記録を送るジョブの時間の上限。
	recordTimeout = 5 * time.Minute
	// 記録の状態 (drama_records の state)。
	stateRecorded = "recorded"
	stateFailed   = "failed"
)

// errNoInserter は、記録のジョブを入れる River のクライアントがないことを表す。
var errNoInserter = errors.New("no job inserter for drama records")

// errEpisodeNotFound は、記録先が話を見つけられなかったことを表す。
var errEpisodeNotFound = fmt.Errorf("%w: the episode was not found on the destination", ErrPermanent)

// WatchedProgram は、見たと判定した、TMDb の話に結びついた番組。
type WatchedProgram struct {
	ServiceID int64
	StartAt   time.Time
	// Name は、番組名。通知に出す。
	Name     string
	SeriesID int32
	Season   int32
	Number   int32
}

// InsertRecords は、ユーザー userID が見た番組 p の話を、連携した記録先ごとに記録する行を tx で作り、新しく作った記録先のジョブを入れる。
// 同じユーザー、番組、話、記録先の組は 1 回だけ記録するので、見直しても、割合に届いた後の報告でも、行もジョブも増えない (設計書 3.8)。
// どの記録先とも連携していなければ、何もしない。
func InsertRecords(ctx context.Context, tx pgx.Tx, inserter anime.Inserter, userID uuid.UUID, p *WatchedProgram, now time.Time) error {
	q := sqlc.New(tx)
	connections, err := q.ListDramaRecordConnections(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to list the drama record connections: %w", err)
	}
	jobs := []river.InsertManyParams{}
	for i := range connections {
		destination := connections[i].Destination
		inserted, err := q.InsertDramaRecord(ctx, sqlc.InsertDramaRecordParams{
			ID: id.New(), UserID: userID, ServiceID: p.ServiceID, StartAt: p.StartAt, Destination: destination,
			TmdbSeriesID: p.SeriesID, SeasonNumber: p.Season, EpisodeNumber: p.Number, ProgramName: p.Name, Now: now,
		})
		if err != nil {
			return fmt.Errorf("failed to insert the drama record: %w", err)
		}
		if inserted > 0 {
			jobs = append(jobs, river.InsertManyParams{
				Args: RecordArgs{UserID: userID, ServiceID: p.ServiceID, StartAt: p.StartAt, Destination: destination}, InsertOpts: nil,
			})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	// ジョブを入れられないまま記録の行だけを残すと、送られないまま、次の報告でも作り直されない。行ごと取り消す。
	if inserter == nil {
		return errNoInserter
	}
	if _, err := inserter.InsertManyTx(ctx, tx, jobs); err != nil {
		return fmt.Errorf("failed to enqueue the drama record: %w", err)
	}

	return nil
}

// RecordArgs は、ユーザーが見た番組の話を、1 つの記録先に記録するジョブの引数。トークンは載せず、ジョブが DB から引く。
type RecordArgs struct {
	UserID      uuid.UUID `json:"userId"`
	ServiceID   int64     `json:"serviceId"`
	StartAt     time.Time `json:"startAt"`
	Destination string    `json:"destination"`
}

var _ river.JobArgsWithInsertOpts = new(RecordArgs)

func (RecordArgs) Kind() string { return "drama_record" }

// InsertOpts は、同じ番組と記録先のジョブを、終わるまでは 1 つにする。動いている間に増えた話は、ジョブが引き直して送る (Record)。
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

// recordOutcome は、1 回のジョブで送れた話と、諦めた話。
type recordOutcome struct {
	recorded []*sqlc.DramaRecord
	failed   []*sqlc.DramaRecord
	// url は、最初に記録した作品のページ。
	url string
	// reason は、最初に諦めた理由。
	reason string
}

// Record は、args の番組の、まだ送っていない話を、記録先に送る。同じ作品の話は 1 回の問い合わせにまとめる (Simkl の書き込みは
// 1 秒に 1 回まで)。送れた話は recorded にし、送り直しても送らない。記録先が見つけられなかった話と、送り直しても変わらない
// 失敗 (トークンが使えないなど) は failed にする。ほかのエラーは River の再試行に任せ、last (最後の試行) なら failed にする。
// 結果は、送れた話と送れなかった話に分けて、ユーザーに通知する。
func (s *Service) Record(ctx context.Context, db Database, args *RecordArgs, last bool) error {
	if _, err := s.destination(args.Destination); err != nil {
		return river.JobCancel(err)
	}
	outcome := &recordOutcome{recorded: []*sqlc.DramaRecord{}, failed: []*sqlc.DramaRecord{}, url: "", reason: ""}
	attempted := map[uuid.UUID]bool{}
	var retry error
	var c *session
	for {
		rows, err := s.untried(ctx, db, args, attempted)
		if err != nil {
			retry = err

			break
		}
		if len(rows) == 0 {
			break
		}
		if c == nil {
			if c, err = s.open(ctx, db, args.UserID, args.Destination); err != nil {
				if err := s.giveUp(ctx, db, rows, err, last, outcome); err != nil {
					retry = err
				}
				c = nil

				continue
			}
		}
		if err := s.send(ctx, db, c, args, rows, last, outcome); err != nil {
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
func (s *Service) untried(ctx context.Context, db sqlc.DBTX, args *RecordArgs, attempted map[uuid.UUID]bool) ([]sqlc.DramaRecord, error) {
	rows, err := sqlc.New(db).ListPendingDramaRecords(ctx, sqlc.ListPendingDramaRecordsParams{
		UserID: args.UserID, ServiceID: args.ServiceID, StartAt: args.StartAt, Destination: args.Destination,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the pending drama records: %w", err)
	}
	untried := []sqlc.DramaRecord{}
	for _, row := range rows {
		if !attempted[row.ID] {
			attempted[row.ID] = true
			untried = append(untried, row)
		}
	}

	return untried, nil
}

// giveUp は、送る前に失敗したとき、諦めるべき失敗か最後の試行なら、rows を記録できなかったことにする。そうでなければ、err を返して
// 再試行に回す。最後の試行で諦めないと、記録が pending のまま残る (#1416)。
func (s *Service) giveUp(ctx context.Context, db sqlc.DBTX, rows []sqlc.DramaRecord, err error, last bool, outcome *recordOutcome) error {
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

// send は、rows を作品ごとにまとめて送る。送り直すエラーがあれば、それを返す。
func (s *Service) send(ctx context.Context, db Database, c *session, args *RecordArgs, rows []sqlc.DramaRecord, last bool, outcome *recordOutcome) error {
	bySeries := map[int32][]*sqlc.DramaRecord{}
	order := []int32{}
	for i := range rows {
		series := rows[i].TmdbSeriesID
		if _, ok := bySeries[series]; !ok {
			order = append(order, series)
		}
		bySeries[series] = append(bySeries[series], &rows[i])
	}
	var retry error
	for _, series := range order {
		group := bySeries[series]
		episodes := make([]*Episode, 0, len(group))
		for _, row := range group {
			episodes = append(episodes, &Episode{Season: row.SeasonNumber, Number: row.EpisodeNumber, WatchedAt: row.WatchedAt})
		}
		var recorded *Recorded
		err := c.do(ctx, func(ctx context.Context, access string) error {
			var err error
			recorded, err = c.destination.RecordEpisodes(ctx, c.app, access, series, episodes)

			return err
		})
		if err != nil {
			if errors.Is(err, ErrPermanent) || last {
				for _, row := range group {
					if err := s.finish(ctx, db, row, stateFailed, nil, err, outcome); err != nil {
						return err
					}
				}

				continue
			}
			s.logger.WarnContext(ctx, "failed to send a drama record; will retry", slog.String("destination", args.Destination), slog.Any("error", err))
			retry = err

			continue
		}
		// 記録を付けたら、ジョブの期限が来ていても、付けたことを残す。
		if err := s.finishSent(context.WithoutCancel(ctx), db, args, group, recorded, outcome); err != nil {
			return err
		}
	}

	return retry
}

// finishSent は、送った話 group を、記録先が見つけられなかった話は failed に、ほかは recorded にする。
// 1 話でも記録できたら、記録先が作品をウォッチリストに入れた状態を、取り込んだことにする。見ただけの作品を、次の取り込みで、
// 見ている作品に足さないため (Simkl は、見た記録を付けると、作品を「見ている」に入れる)。
func (s *Service) finishSent(ctx context.Context, db sqlc.DBTX, args *RecordArgs, group []*sqlc.DramaRecord, recorded *Recorded, outcome *recordOutcome) error {
	anyRecorded := false
	for _, row := range group {
		key := EpisodeKey{Season: row.SeasonNumber, Number: row.EpisodeNumber}
		if slices.Contains(recorded.NotFound, key) {
			if err := s.finish(ctx, db, row, stateFailed, nil, errEpisodeNotFound, outcome); err != nil {
				return err
			}

			continue
		}
		var url *string
		if recorded.URL != "" {
			url = new(recorded.URL)
		}
		if err := s.finish(ctx, db, row, stateRecorded, url, nil, outcome); err != nil {
			return err
		}
		anyRecorded = true
	}
	if !anyRecorded {
		return nil
	}
	status := recorded.Status
	if status == "" {
		status = StatusWatching
	}
	if err := sqlc.New(db).InsertDramaWatchlistImportIfAbsent(ctx, sqlc.InsertDramaWatchlistImportIfAbsentParams{
		UserID: args.UserID, Destination: args.Destination, TmdbSeriesID: group[0].TmdbSeriesID, Status: status, Now: s.clock.Now(),
	}); err != nil {
		// できたらよい操作なので、失敗してもログに残すだけにし、記録は付け直さない。
		s.logger.WarnContext(ctx, "failed to note the watchlist status after recording", slog.String("destination", args.Destination), slog.Any("error", err))
	}

	return nil
}

// finish は、記録 row を state にし、結果を outcome に足す。
func (s *Service) finish(ctx context.Context, db sqlc.DBTX, row *sqlc.DramaRecord, state string, url *string, cause error, outcome *recordOutcome) error {
	var reason *string
	if cause != nil {
		reason = new(failureReason(cause))
	}
	if err := sqlc.New(db).FinishDramaRecord(ctx, sqlc.FinishDramaRecordParams{State: state, URL: url, Error: reason, Now: s.clock.Now(), ID: row.ID}); err != nil {
		return fmt.Errorf("failed to finish the drama record: %w", err)
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
	case errors.Is(err, ErrReauthorizationRequired):
		return "連携し直してください"
	case errors.Is(err, errEpisodeNotFound):
		return "Simkl に話が見つかりません"
	default:
		return shorten(strings.TrimPrefix(err.Error(), ErrPermanent.Error()+": "))
	}
}

// notify は、送れた話と諦めた話を、それぞれ 1 件の通知にして、見たユーザーだけに届ける。
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
			if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindDramaRecorded, payload, audience); err != nil {
				return err
			}
		}
		if len(outcome.failed) > 0 {
			payload := recordPayload(args, outcome.failed)
			payload["reason"] = outcome.reason
			if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindDramaRecordFailed, payload, audience); err != nil {
				return err
			}
		}

		return nil
	})
}

// recordPayload は、通知の材料を作る。番組名 (EPG の名前のまま。作品名は文を作るときに読む)、記録先、話 (「シーズン1 第3話」の形) を載せる。
// TMDb から引いた作品名は、TMDb の利用規約の保持期間を超えて通知に残さないよう、載せない。
func recordPayload(args *RecordArgs, rows []*sqlc.DramaRecord) map[string]any {
	episodes := make([]string, 0, len(rows))
	for _, row := range rows {
		if text := EpisodeLabel(row.SeasonNumber, row.EpisodeNumber); !slices.Contains(episodes, text) {
			episodes = append(episodes, text)
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

// EpisodeLabel は、TMDb のシーズンと話を、画面と同じ「シーズン7 第7話」の形にする。シーズン 0 は「特別編 第1話」にする。
func EpisodeLabel(season, number int32) string {
	if season == 0 {
		return "特別編 第" + strconv.Itoa(int(number)) + "話"
	}

	return "シーズン" + strconv.Itoa(int(season)) + " 第" + strconv.Itoa(int(number)) + "話"
}
