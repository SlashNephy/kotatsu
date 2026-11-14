package edcbimport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	// importSource は、import_sources の取り込み元の名前。
	importSource = "edcb"
	// autoAddCommentPrefix は、EDCB がキーワード予約から作った予約のコメントの先頭。ルールから作り直すので取り込まない。
	autoAddCommentPrefix = "EPG自動予約"
	// timeOnlyEventID は、番組に結び付かない時刻だけの予約のイベント ID。
	timeOnlyEventID = 0xFFFF
	// pgUniqueViolation は、一意性の制約に反したときの PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
)

// 取り込むデータの種類 (import_sources.kind)。
const (
	KindRule            = "rule"
	KindReserve         = "reserve"
	KindRecordedHistory = "recorded_history"
)

// recordedStatuses は、録画の履歴にする録画の結果 (EDCB の REC_END_STATUS)。
// 正常に終了した (1)、次の予約のために終了した (4)、開始時刻が変わった (6)、空き容量が足りず別のフォルダに保存した (11)。
func recordedStatuses() []uint32 {
	return []uint32{1, 4, 6, 11}
}

// Snapshot は、EpgTimerSrv から読んだ一覧。
type Snapshot struct {
	AutoAdds []*edcb.AutoAdd
	Reserves []*edcb.Reserve
	Recorded []*edcb.RecordedFile
}

// Reader は、EpgTimerSrv の一覧を読む。*edcb.Client が満たす。
type Reader interface {
	AutoAdds(ctx context.Context) ([]*edcb.AutoAdd, error)
	Reserves(ctx context.Context) ([]*edcb.Reserve, error)
	RecordedFiles(ctx context.Context) ([]*edcb.RecordedFile, error)
}

var _ Reader = new(edcb.Client)

// ReadOptions は Read の設定。0 の項目は既定値にする。
type ReadOptions struct {
	// BusyWait と BusyRetries は、EpgTimerSrv が忙しい (EPG を読み込み中など) と答えたときに、送り直すまでの待ち時間と回数。
	BusyWait    time.Duration
	BusyRetries int
}

const (
	// DefaultBusyWait と DefaultBusyRetries は、忙しいときの待ち時間と回数の既定 (付録 A)。
	// 2026年11月に Linux の EDCB で、地上波の EPG の読み込み直しは数秒で終わった。余裕を見て、最長 1 分待つ。
	DefaultBusyWait    = 5 * time.Second
	DefaultBusyRetries = 12
)

// Read は、3 つの一覧を、取り込みを始める前に全部読む。どれかを読めなければ、その理由を返す。
func Read(ctx context.Context, reader Reader, logger *slog.Logger, opts *ReadOptions) (*Snapshot, error) {
	wait, retries := DefaultBusyWait, DefaultBusyRetries
	if opts != nil {
		if opts.BusyWait > 0 {
			wait = opts.BusyWait
		}
		if opts.BusyRetries > 0 {
			retries = opts.BusyRetries
		}
	}
	autoAdds, err := withBusyRetry(ctx, logger, wait, retries, "keyword reservations", reader.AutoAdds)
	if err != nil {
		return nil, err
	}
	reserves, err := withBusyRetry(ctx, logger, wait, retries, "reservations", reader.Reserves)
	if err != nil {
		return nil, err
	}
	recorded, err := withBusyRetry(ctx, logger, wait, retries, "recorded files", reader.RecordedFiles)
	if err != nil {
		return nil, err
	}

	return &Snapshot{AutoAdds: autoAdds, Reserves: reserves, Recorded: recorded}, nil
}

// withBusyRetry は、EpgTimerSrv が忙しいと答えたら、wait だけ待って retries 回まで送り直す。
func withBusyRetry[T any](ctx context.Context, logger *slog.Logger, wait time.Duration, retries int, name string, read func(context.Context) (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		v, err := read(ctx)
		if !errors.Is(err, edcb.ErrBusy) || attempt >= retries {
			if err != nil {
				return v, fmt.Errorf("failed to read the %s from EpgTimerSrv: %w", name, err)
			}

			return v, nil
		}
		logger.InfoContext(ctx, "EpgTimerSrv is busy; retrying", slog.String("list", name), slog.Duration("wait", wait))
		select {
		case <-ctx.Done():
			var zero T

			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// ApplyOptions は、取り込みの条件。
type ApplyOptions struct {
	// UserID は、取り込んだルールと予約を作ったことにするユーザー。dry run でユーザーを指定しなければ、零値でよい。
	UserID uuid.UUID
	// DryRun なら、取り込みを試すだけで、書き込みは巻き戻す。何が取り込まれるかを見るのに使う。
	DryRun bool
}

// Result は取り込みの結果。
type Result struct {
	Imported        map[string]int
	AlreadyImported map[string]int
	// Skipped は、取り込めなかったデータと、その理由。
	Skipped []*Skipped
	// SkippedAutoReserves は、キーワード予約から作られた予約で、ルールから作り直すので飛ばした数。
	SkippedAutoReserves int
}

// Skipped は、取り込めなかったデータ。
type Skipped struct {
	Kind     string
	SourceID int64
	Reason   string
}

// skipError は、そのデータを取り込まずに次へ進むことを表す。
type skipError struct {
	reason string
}

func (e *skipError) Error() string {
	return e.reason
}

func skip(format string, args ...any) error {
	return &skipError{reason: fmt.Sprintf(format, args...)}
}

// errDryRun は、dry run で書き込みを巻き戻すのに使う。
var errDryRun = errors.New("dry run")

// errAlreadyImported は、前に取り込んだデータであることを表す。
var errAlreadyImported = errors.New("already imported")

// Database は、取り込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Importer は、読んだ EDCB の一覧を Kotatsu に書き込む。
type Importer struct {
	clock        clock.Clock
	rules        *rule.Rules
	reservations *reserve.Reservations
}

// NewImporter は Importer を返す。
func NewImporter(clk clock.Clock, recorder audit.Recorder) *Importer {
	return &Importer{clock: clk, rules: rule.NewRules(clk, recorder), reservations: reserve.NewReservations(clk, recorder)}
}

// Apply は、snapshot を Kotatsu に取り込む。データごとに 1 つのトランザクションで書き込み、取り込んだことを import_sources に記録する。
// 前に取り込んだデータは飛ばすので、何度実行しても同じものを二重に作らない。
// 取り込む順は、録画の履歴、ルール、手動の予約。録画済みの回を、ルールが動き出した時点で予約してしまわないためである。
func (im *Importer) Apply(ctx context.Context, db Database, snapshot *Snapshot, opts *ApplyOptions) (*Result, error) {
	serviceIDs, err := sqlc.New(db).ListServiceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	actor := audit.User(opts.UserID)
	if opts.UserID == uuid.Nil() {
		// dry run では、ユーザーを指定しなくてよい。書き込みは巻き戻すので、作った人は残らない。
		actor = audit.System(importSource)
	}
	a := &applier{
		importer: im, db: db, opts: opts, actor: actor, services: map[int64]bool{},
		result: &Result{Imported: map[string]int{}, AlreadyImported: map[string]int{}, Skipped: []*Skipped{}, SkippedAutoReserves: 0},
	}
	for _, serviceID := range serviceIDs {
		a.services[serviceID] = true
	}

	seen := map[historyKey]bool{}
	for _, f := range snapshot.Recorded {
		// 同じ鍵が一覧の中に 2 回出れば、録画の履歴にするもののうち、先のものだけを使う。下見と取り込みで件数が食い違わないようにするためである。
		// 録れていない記録は、鍵をふさがない。先に失敗した記録があっても、後の正常に録れた記録を履歴にする (#1725)。
		key := keyOf(f)
		if seen[key] {
			continue
		}
		if historyCandidate(f) {
			seen[key] = true
		}
		if err := a.apply(ctx, KindRecordedHistory, int64(f.ID), func(tx pgx.Tx) (uuid.UUID, error) { return a.history(ctx, tx, f) }); err != nil {
			return nil, err
		}
	}
	for _, r := range snapshot.AutoAdds {
		if err := a.apply(ctx, KindRule, int64(r.ID), func(tx pgx.Tx) (uuid.UUID, error) { return a.rule(ctx, tx, r) }); err != nil {
			return nil, err
		}
	}
	for _, r := range snapshot.Reserves {
		if strings.HasPrefix(r.Comment, autoAddCommentPrefix) {
			a.result.SkippedAutoReserves++

			continue
		}
		if err := a.apply(ctx, KindReserve, int64(r.ID), func(tx pgx.Tx) (uuid.UUID, error) { return a.reserve(ctx, tx, r) }); err != nil {
			return nil, err
		}
	}

	return a.result, nil
}

// historyKey は、録画の履歴を二重に取り込まないための鍵。局と、予約したときの開始 (なければ開始)。
type historyKey struct {
	edcb.ServiceKey

	start time.Time
}

func keyOf(f *edcb.RecordedFile) historyKey {
	start := f.StartEPG
	if start.IsZero() {
		start = f.Start
	}

	return historyKey{ServiceKey: f.ServiceKey, start: start.Truncate(time.Second)}
}

// recordedNormally は、EDCB の録画済みが、録れたものかどうかを返す。
func recordedNormally(f *edcb.RecordedFile) bool {
	return slices.Contains(recordedStatuses(), f.Status)
}

// historyCandidate は、EDCB の録画済みが、番組名と状態から、録画の履歴にするものかどうかを返す。
func historyCandidate(f *edcb.RecordedFile) bool {
	return strings.TrimSpace(f.Title) != "" && recordedNormally(f)
}

// applier は、1 回の取り込みの状態を持つ。
type applier struct {
	importer *Importer
	db       Database
	opts     *ApplyOptions
	actor    *audit.Actor
	services map[int64]bool
	result   *Result
}

// apply は、1 つのデータを 1 つのトランザクションで取り込み、create が作った行の ID を import_sources に記録する。
// dry run なら、最後に巻き戻す。create が飛ばすエラーを返したら、理由を結果に残して次へ進む。
func (a *applier) apply(ctx context.Context, kind string, sourceID int64, create func(tx pgx.Tx) (uuid.UUID, error)) error {
	err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		_, err := q.GetImportedTarget(ctx, sqlc.GetImportedTargetParams{Source: importSource, Kind: kind, SourceID: sourceID})
		switch {
		case err == nil:
			return errAlreadyImported
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("failed to look up the imported %s %d: %w", kind, sourceID, err)
		}
		targetID, err := create(tx)
		if err != nil {
			return err
		}
		if err := q.InsertImportSource(ctx, sqlc.InsertImportSourceParams{
			Source: importSource, Kind: kind, SourceID: sourceID, TargetID: targetID, Now: a.importer.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to record the imported %s %d: %w", kind, sourceID, err)
		}
		if a.opts.DryRun {
			return errDryRun
		}

		return nil
	})
	switch {
	case err == nil, errors.Is(err, errDryRun):
		a.result.Imported[kind]++
	case errors.Is(err, errAlreadyImported):
		a.result.AlreadyImported[kind]++
	case isSkip(err):
		a.result.Skipped = append(a.result.Skipped, &Skipped{Kind: kind, SourceID: sourceID, Reason: err.Error()})
	default:
		return fmt.Errorf("failed to import %s %d: %w", kind, sourceID, err)
	}

	return nil
}

// isSkip は、err が、そのデータだけを取り込まずに済ませてよいエラーかどうかを返す。
func isSkip(err error) bool {
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return true
	}
	if _, ok := errors.AsType[*skipError](err); ok {
		return true
	}

	return isUnsupported(err) ||
		errors.Is(err, rule.ErrInvalidRule) ||
		errors.Is(err, match.ErrInvalidConditions) ||
		errors.Is(err, domain.ErrInvalidReservation) ||
		errors.Is(err, reserve.ErrInvalidSettings) ||
		errors.Is(err, reserve.ErrProgramNotFound)
}

// rule は、EDCB のキーワード予約から Kotatsu のルールを作る。
func (a *applier) rule(ctx context.Context, tx pgx.Tx, r *edcb.AutoAdd) (uuid.UUID, error) {
	input, err := ruleInput(r, a.services)
	if err != nil {
		return uuid.Nil(), err
	}
	created, err := a.importer.rules.Create(ctx, tx, a.actor, input)
	if err != nil {
		return uuid.Nil(), err
	}

	return created.ID, nil
}

// reserve は、EDCB の手動の予約から Kotatsu の番組予約を作る。終わりが過ぎたもの、録らないもの、時刻だけの予約は取り込まない。
// 番組は (ONID, SID, イベント ID) で引き、Kotatsu の番組表になければ飛ばす。番組表が埋まってから、もう一度実行してもらう。
func (a *applier) reserve(ctx context.Context, tx pgx.Tx, r *edcb.Reserve) (uuid.UUID, error) {
	if !r.Start.Add(r.Duration).After(a.importer.clock.Now()) {
		return uuid.Nil(), skip("the reservation has already ended")
	}
	if !r.RecSetting.Records() {
		return uuid.Nil(), skip("the reservation does not record (recording mode %d)", r.RecSetting.RecMode)
	}
	if r.EventID == timeOnlyEventID {
		return uuid.Nil(), skip("the reservation is time-only")
	}
	serviceID := int64(r.ONID)*serviceIDBase + int64(r.SID)
	if !a.services[serviceID] {
		return uuid.Nil(), skip("service %d is not in Kotatsu", serviceID)
	}
	reservation, created, err := a.importer.reservations.CreateProgram(ctx, tx, a.actor, &reserve.ProgramInput{
		ServiceID: serviceID, EventID: int64(r.EventID), FollowEventRelay: false, Overrides: new(domain.RecordingOverrides),
	})
	if err != nil {
		return uuid.Nil(), err
	}
	if !created {
		return uuid.Nil(), skip("the program is already reserved")
	}

	return reservation.ID, nil
}

// history は、EDCB の録画済みを、Kotatsu の重複の回避に使う録画履歴にする。番組名が空のものと、録れていないものは飛ばす。
func (a *applier) history(ctx context.Context, tx pgx.Tx, f *edcb.RecordedFile) (uuid.UUID, error) {
	if strings.TrimSpace(f.Title) == "" {
		return uuid.Nil(), skip("the recorded file has no title")
	}
	if !recordedNormally(f) {
		return uuid.Nil(), skip("the recording did not finish normally (status %d)", f.Status)
	}
	serviceID := int64(f.ONID)*serviceIDBase + int64(f.SID)
	if !a.services[serviceID] {
		return uuid.Nil(), skip("service %d is not in Kotatsu", serviceID)
	}
	historyID := id.New()
	// ミリ秒は捨てる。
	start := f.Start.Truncate(time.Second).UTC()
	err := sqlc.New(tx).InsertRecordingHistory(ctx, sqlc.InsertRecordingHistoryParams{
		ID: historyID, ServiceID: serviceID, TitleKey: rule.TitleKey(f.Title), Name: f.Title, Series: episode.SeriesOf(f.Title),
		StartAt: start, EndAt: start.Add(f.Duration), Now: a.importer.clock.Now(),
	})
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to insert the recording history: %w", err)
	}

	return historyID, nil
}
