package epgstation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

const (
	// importSource は、import_sources に記録する取り込み元のソフトの名前。
	importSource = "epgstation"
	// mirakurunIDBase は、Mirakurun の ID の桁の区切り。番組の ID は「サービスの ID × 100000 + eventId」になる。
	mirakurunIDBase = 100000
	// maxRuleNameLength は、ルールの名前の上限 (Kotatsu のルールの名前の上限と同じ)。
	maxRuleNameLength = 200
	// maxDuplicateWindowDays は、重複を避ける期間の上限 (Kotatsu のルールの上限と同じ)。
	maxDuplicateWindowDays = 3650
	// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
)

// Kind は、取り込むデータの種類。import_sources の kind に記録する。
type Kind string

const (
	// KindRule はルール。
	KindRule Kind = "rule"
	// KindReserve は手動予約。
	KindReserve Kind = "reserve"
	// KindRecorded は録画済みの番組。
	KindRecorded Kind = "recorded"
	// KindRecordedHistory は、重複の回避に使う録画履歴。
	KindRecordedHistory Kind = "recorded_history"
)

// ApplyOptions は、取り込みの条件。
type ApplyOptions struct {
	// Pools は、EPGStation の録画の保存先ディレクトリの名前から、Kotatsu のプール ID への対応。
	Pools map[string]string
	// UserID は、取り込んだルールと予約を作ったことにするユーザー。
	UserID uuid.UUID
}

// Result は取り込みの結果。
type Result struct {
	// Imported は、種類ごとの、取り込んだ数。
	Imported map[Kind]int
	// AlreadyImported は、種類ごとの、前に取り込んでいたので飛ばした数。
	AlreadyImported map[Kind]int
	// Skipped は、取り込めなかったデータと、その理由。
	Skipped []*Skipped
}

// Skipped は、取り込めなかったデータ。
type Skipped struct {
	Kind     Kind
	SourceID int64
	Reason   string
}

// skipError は、そのデータを取り込まずに次へ進むことを表す。Error は飛ばした理由を返す。
type skipError struct {
	reason string
}

var _ error = new(skipError)

func (e *skipError) Error() string {
	return e.reason
}

func skip(format string, args ...any) error {
	return &skipError{reason: fmt.Sprintf(format, args...)}
}

// Importer は、読み取った EPGStation のデータを Kotatsu に書き込む。
type Importer struct {
	clock        clock.Clock
	rules        *rule.Rules
	reservations *reserve.Reservations
	inserter     analysis.Inserter
}

// NewImporter は Importer を作る。inserter は、取り込んだ録画を解析するジョブを投入する。
func NewImporter(clk clock.Clock, recorder audit.Recorder, inserter analysis.Inserter) *Importer {
	return &Importer{
		clock: clk, rules: rule.NewRules(clk, recorder), reservations: reserve.NewReservations(clk, recorder), inserter: inserter,
	}
}

// Database は、取り込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Apply は、snapshot を Kotatsu に取り込む。
// データごとに 1 つのトランザクションで書き込み、取り込んだことを import_sources に記録する。
// 前に取り込んだデータは飛ばすので、何度実行しても同じものを二重に作らない。
func (im *Importer) Apply(ctx context.Context, db Database, snapshot *Snapshot, opts *ApplyOptions) (*Result, error) {
	serviceIDs, err := sqlc.New(db).ListServiceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	a := &applier{
		importer: im,
		db:       db,
		opts:     opts,
		actor:    audit.User(opts.UserID),
		services: map[int64]bool{},
		tags:     map[int64]string{},
		result:   &Result{Imported: map[Kind]int{}, AlreadyImported: map[Kind]int{}, Skipped: []*Skipped{}},
	}
	for _, serviceID := range serviceIDs {
		a.services[serviceID] = true
	}
	for _, tag := range snapshot.Tags {
		a.tags[tag.ID] = tag.Name
	}

	for _, r := range snapshot.Rules {
		if err := a.apply(ctx, KindRule, r.ID, func(tx pgx.Tx) (uuid.UUID, error) { return a.rule(ctx, tx, r) }); err != nil {
			return nil, err
		}
	}
	for _, r := range snapshot.ManualReserves {
		if err := a.apply(ctx, KindReserve, r.ID, func(tx pgx.Tx) (uuid.UUID, error) { return a.reserve(ctx, tx, r) }); err != nil {
			return nil, err
		}
	}
	for _, r := range snapshot.Recordings {
		if err := a.apply(ctx, KindRecorded, r.ID, func(tx pgx.Tx) (uuid.UUID, error) { return a.recordingAndPublish(ctx, tx, r) }); err != nil {
			return nil, err
		}
	}
	for _, h := range snapshot.Histories {
		if err := a.apply(ctx, KindRecordedHistory, h.ID, func(tx pgx.Tx) (uuid.UUID, error) { return a.history(ctx, tx, h) }); err != nil {
			return nil, err
		}
	}

	return a.result, nil
}

// applier は、1 回の取り込みの状態を持つ。
type applier struct {
	importer *Importer
	db       Database
	opts     *ApplyOptions
	actor    *audit.Actor
	// services は、Kotatsu にあるサービスの ID の集合。
	services map[int64]bool
	// tags は、EPGStation のタグの ID から名前への対応。
	tags   map[int64]string
	result *Result
}

// errAlreadyImported は、前に取り込んだデータであることを表す。トランザクションを巻き戻すのに使う。
var errAlreadyImported = errors.New("already imported")

// apply は、1 つのデータを 1 つのトランザクションで取り込む。create が作った行の ID を import_sources に記録する。
// create が skipError を返したら、巻き戻して理由を結果に残し、次へ進む。
func (a *applier) apply(ctx context.Context, kind Kind, sourceID int64, create func(tx pgx.Tx) (uuid.UUID, error)) error {
	err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		_, err := q.GetImportedTarget(ctx, sqlc.GetImportedTargetParams{Source: importSource, Kind: string(kind), SourceID: sourceID})
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
		err = q.InsertImportSource(ctx, sqlc.InsertImportSourceParams{
			Source: importSource, Kind: string(kind), SourceID: sourceID, TargetID: targetID, Now: a.importer.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to record the imported %s %d: %w", kind, sourceID, err)
		}

		return nil
	})
	switch {
	case err == nil:
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
// Kotatsu の検証に通らないデータや、EPG にない番組の予約は、取り込みを止めずに飛ばす。
func isSkip(err error) bool {
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return true
	}

	if _, ok := errors.AsType[*skipError](err); ok {
		return true
	}

	return errors.Is(err, rule.ErrInvalidRule) ||
		errors.Is(err, match.ErrInvalidConditions) ||
		errors.Is(err, reserve.ErrInvalidReservation) ||
		errors.Is(err, reserve.ErrInvalidSettings) ||
		errors.Is(err, reserve.ErrProgramNotFound)
}

// poolOf は、EPGStation の保存先ディレクトリに対応付けた Kotatsu のプール ID を返す。
// 空の名前は EPGStation の既定の保存先なので、Kotatsu の既定のプールを使うよう nil を返す。
func (a *applier) poolOf(directory string) (*string, bool) {
	if directory == "" {
		return nil, true
	}
	poolID, ok := a.opts.Pools[directory]
	if !ok {
		return nil, false
	}

	return &poolID, true
}

// rule は、EPGStation のルールから Kotatsu のルールを作る。
func (a *applier) rule(ctx context.Context, tx pgx.Tx, r *Rule) (uuid.UUID, error) {
	created, err := a.importer.rules.Create(ctx, tx, a.actor, a.ruleInput(r))
	if err != nil {
		return uuid.Nil, err
	}

	return created.ID, nil
}

// ruleInput は、EPGStation のルールを Kotatsu のルールの内容に変換する。
// Kotatsu で表せない項目は、捨てたことをメモに残す。
func (a *applier) ruleInput(r *Rule) *rule.Input {
	notes := []string{}
	conditions := &match.Conditions{
		Keywords:           keywords(&r.Keyword),
		KeywordMatch:       match.KeywordMatchAll,
		ExcludeKeywords:    keywords(&r.IgnoreKeyword),
		ChannelTypes:       []string{},
		ServiceIDs:         []int64{},
		ExcludeServiceIDs:  []int64{},
		Genres:             []*match.Genre{},
		ExcludeGenres:      []*match.Genre{},
		TimeRanges:         []*match.TimeRange{},
		MinDurationSeconds: r.DurationMin,
		MaxDurationSeconds: r.DurationMax,
		FreeOnly:           r.FreeOnly,
		Marks:              []string{},
		ExcludeMarks:       []string{},
	}
	// EPGStation は、チャンネルを指定すると、放送波の種類を見ない。
	if len(r.ChannelIDs) > 0 {
		conditions.ServiceIDs = append(conditions.ServiceIDs, r.ChannelIDs...)
	} else {
		conditions.ChannelTypes = append(conditions.ChannelTypes, r.ChannelTypes...)
	}
	for _, genre := range r.Genres {
		conditions.Genres = append(conditions.Genres, &match.Genre{Lv1: genre.Genre, Lv2: genre.SubGenre})
	}
	for _, t := range r.Times {
		timeRange, ok := timeRangeOf(t, r.IsTimeSpecification)
		if !ok {
			notes = append(notes, "曜日のない時刻の条件")

			continue
		}
		conditions.TimeRanges = append(conditions.TimeRanges, timeRange)
	}
	if r.IsTimeSpecification {
		notes = append(notes, "時刻指定 (番組の時刻の条件として取り込んだ)")
	}

	var validFrom, validUntil *time.Time
	for _, period := range r.SearchPeriods {
		if validFrom == nil || period.StartAt.Before(*validFrom) {
			validFrom = new(period.StartAt.UTC())
		}
		if validUntil == nil || period.EndAt.After(*validUntil) {
			validUntil = new(period.EndAt.UTC())
		}
	}
	if len(r.SearchPeriods) > 1 {
		notes = append(notes, "複数の期間 (最初から最後までの 1 つの期間にまとめた)")
	}

	var window *int
	if r.AvoidDuplicate && r.PeriodToAvoidDuplicate != nil && *r.PeriodToAvoidDuplicate > 0 {
		window = new(min(*r.PeriodToAvoidDuplicate, maxDuplicateWindowDays))
	}

	overrides := new(reserve.Overrides)
	poolID, ok := a.poolOf(r.ParentDirectoryName)
	if !ok {
		notes = append(notes, "保存先 "+r.ParentDirectoryName+" (プールを対応付けていない)")
	}
	overrides.PoolID = poolID
	notes = append(notes, droppedFields(r.TagIDs, r.Directory, r.RecordedFormat)...)

	return &rule.Input{
		Name:                ruleName(r),
		Memo:                memo("ルール", r.ID, notes),
		Enabled:             r.Enabled,
		Conditions:          conditions,
		Overrides:           overrides,
		ValidFrom:           validFrom,
		ValidUntil:          validUntil,
		AvoidDuplicates:     r.AvoidDuplicate,
		DuplicateWindowDays: window,
		FollowEventRelay:    false,
	}
}

// keywords は、EPGStation のキーワードを Kotatsu のキーワードに変換する。
// EPGStation は、正規表現でないキーワードを空白で区切り、すべてを含む番組を探すので、区切った語ごとのキーワードにする。
func keywords(option *KeywordOption) []*match.Keyword {
	result := []*match.Keyword{}
	if option.Keyword == "" {
		return result
	}
	fields := []match.Field{}
	if option.Name {
		fields = append(fields, match.FieldName)
	}
	if option.Description {
		fields = append(fields, match.FieldDescription)
	}
	if option.Extended {
		fields = append(fields, match.FieldExtended)
	}
	if len(fields) == 0 {
		fields = append(fields, match.FieldName)
	}
	texts := []string{option.Keyword}
	if !option.Regexp {
		texts = strings.Fields(option.Keyword)
	}
	for _, text := range texts {
		result = append(result, &match.Keyword{
			Text: text, Regexp: option.Regexp, CaseSensitive: option.CaseSensitive, Fields: slices.Clone(fields),
		})
	}

	return result
}

// timeRangeOf は、EPGStation の時刻の条件を Kotatsu の時刻の範囲に変換する。曜日がなければ false を返す。
// 番組の照合では Start と Range が時、時刻指定のルールでは秒を表す。Start がなければ終日とする。
func timeRangeOf(t *SearchTime, timeSpecification bool) (*match.TimeRange, bool) {
	weekdays := []int{}
	for weekday := range 7 {
		if t.Week&(1<<weekday) != 0 {
			weekdays = append(weekdays, weekday)
		}
	}
	if len(weekdays) == 0 {
		return nil, false
	}
	startMinute, durationMinutes := 0, 24*60
	if t.Start != nil && t.Range != nil {
		if timeSpecification {
			startMinute, durationMinutes = *t.Start/60, max(*t.Range/60, 1)
		} else {
			startMinute, durationMinutes = *t.Start*60, *t.Range*60
		}
	}

	return &match.TimeRange{Weekdays: weekdays, StartMinute: startMinute, DurationMinutes: durationMinutes}, true
}

// ruleName は、ルールの名前を、キーワードから作る。キーワードがなければ、EPGStation のルールの ID を使う。
func ruleName(r *Rule) string {
	name := strings.TrimSpace(r.Keyword.Keyword)
	if name == "" {
		return "EPGStation #" + strconv.FormatInt(r.ID, 10)
	}
	if utf8.RuneCountInString(name) > maxRuleNameLength {
		name = string([]rune(name)[:maxRuleNameLength])
	}

	return name
}

// droppedFields は、Kotatsu に取り込まない録画の設定のうち、値が入っているものの説明を返す。
// 末尾の欠けの許容 (allowEndLack) は、EPGStation の既定で有効なので、メモに残すと、ほとんどのルールに同じ注記が付く。そのため残さない。
func droppedFields(tagIDs []int64, directory, recordedFormat string) []string {
	notes := []string{}
	if len(tagIDs) > 0 {
		notes = append(notes, "録画に付けるタグ")
	}
	if directory != "" {
		notes = append(notes, "保存先のサブディレクトリ "+directory)
	}
	if recordedFormat != "" {
		notes = append(notes, "ファイル名の形式 "+recordedFormat)
	}

	return notes
}

// memo は、取り込み元と、取り込めなかった項目をメモにする。
func memo(kind string, sourceID int64, notes []string) string {
	text := fmt.Sprintf("EPGStation の%s #%d から取り込みました。", kind, sourceID)
	if len(notes) > 0 {
		text += "\n取り込めなかった項目: " + strings.Join(notes, "、")
	}

	return text
}

// reserve は、EPGStation の手動予約から Kotatsu の予約を作る。開始済みの予約は取り込まない。
func (a *applier) reserve(ctx context.Context, tx pgx.Tx, r *Reserve) (uuid.UUID, error) {
	if !r.StartAt.After(a.importer.clock.Now()) {
		return uuid.Nil, skip("the reservation has already started")
	}
	if !a.services[r.ChannelID] {
		return uuid.Nil, skip("service %d is not in Kotatsu", r.ChannelID)
	}
	poolID, ok := a.poolOf(r.ParentDirectoryName)
	if !ok {
		return uuid.Nil, skip("directory %q is not mapped to a pool", r.ParentDirectoryName)
	}
	overrides := &reserve.Overrides{PoolID: poolID, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil, JoinSegments: nil}

	created, err := a.createReservation(ctx, tx, r, overrides)
	if err != nil {
		return uuid.Nil, err
	}
	if r.IsSkip {
		if _, err := a.importer.reservations.Update(ctx, tx, a.actor, created.ID, &reserve.Update{
			Enabled: new(false), FollowEventRelay: nil, Overrides: nil, StartAt: nil, EndAt: nil, Name: nil,
		}); err != nil {
			return uuid.Nil, err
		}
	}

	return created.ID, nil
}

// createReservation は、時刻予約か番組予約を作る。番組予約の番組がすでに予約されていれば、取り込まない。
func (a *applier) createReservation(ctx context.Context, tx pgx.Tx, r *Reserve, overrides *reserve.Overrides) (*reserve.Reservation, error) {
	if r.IsTimeSpecified {
		return a.importer.reservations.CreateTime(ctx, tx, a.actor, &reserve.TimeInput{
			ServiceID: r.ChannelID, StartAt: r.StartAt.UTC(), EndAt: r.EndAt.UTC(), Name: r.Name, Overrides: overrides,
		})
	}
	if r.ProgramID == nil {
		return nil, skip("the program reservation has no program")
	}
	reservation, created, err := a.importer.reservations.CreateProgram(ctx, tx, a.actor, &reserve.ProgramInput{
		ServiceID: *r.ProgramID / mirakurunIDBase, EventID: *r.ProgramID % mirakurunIDBase,
		FollowEventRelay: r.IsEventRelay, Overrides: overrides,
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, skip("the program is already reserved")
	}

	return reservation, nil
}

// recordingAndPublish は、録画を取り込み、録画の変更を知らせる (#868)。
func (a *applier) recordingAndPublish(ctx context.Context, tx pgx.Tx, r *Recording) (uuid.UUID, error) {
	recordingID, err := a.recording(ctx, tx, r)
	if err != nil || recordingID == uuid.Nil {
		return recordingID, err
	}
	if _, err := notify.Publish(ctx, tx, a.importer.clock, notify.RecordingsChannel); err != nil {
		return uuid.Nil, err
	}

	return recordingID, nil
}

// recording は、EPGStation の録画済みの番組と、そのファイルを Kotatsu の録画として記録する。
// 録画中の番組と、プールを対応付けたディレクトリにファイルがない録画は取り込まない。
func (a *applier) recording(ctx context.Context, tx pgx.Tx, r *Recording) (uuid.UUID, error) {
	if r.IsRecording {
		return uuid.Nil, skip("the program is still being recorded")
	}
	if !a.services[r.ChannelID] {
		return uuid.Nil, skip("service %d is not in Kotatsu", r.ChannelID)
	}
	type mappedFile struct {
		poolID string
		file   *VideoFile
	}
	files := []*mappedFile{}
	for _, file := range r.Files {
		if poolID, ok := a.opts.Pools[file.ParentDirectoryName]; ok {
			files = append(files, &mappedFile{poolID: poolID, file: file})
		}
	}
	if len(files) == 0 {
		return uuid.Nil, skip("no file is in a directory mapped to a pool")
	}

	q := sqlc.New(tx)
	now := a.importer.clock.Now()
	startAt, endAt := r.StartAt.UTC(), r.EndAt.UTC()
	var eventID *int64
	if r.ProgramID != nil {
		eventID = new(*r.ProgramID % mirakurunIDBase)
	}
	drop := r.Drop
	if drop == nil {
		drop = &DropLog{ErrorCount: 0, DropCount: 0, ScramblingCount: 0}
	}
	// Kotatsu の録画と同じく、ドロップがあればエラーありの完了にする。
	state := "completed"
	if drop.DropCount > 0 {
		state = "completed_with_errors"
	}
	recordingID := id.New()
	err := q.InsertImportedRecording(ctx, sqlc.InsertImportedRecordingParams{
		ID: recordingID, ServiceID: r.ChannelID, EventID: eventID, Name: r.Name, Description: r.Description,
		StartAt: startAt, EndAt: endAt, State: state, Protected: r.IsProtected,
		Drops: drop.DropCount, Errors: drop.ErrorCount, Scrambled: drop.ScramblingCount, Now: now,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert the recording: %w", err)
	}
	for _, f := range files {
		kind := "other"
		if f.file.Type == VideoFileTypeTS {
			kind = "segment"
		}
		err := q.InsertImportedRecordingFile(ctx, sqlc.InsertImportedRecordingFileParams{
			ID: id.New(), RecordingID: recordingID, Kind: kind, PoolID: f.poolID, Path: f.file.FilePath, Size: f.file.Size,
			StartedAt: startAt, EndedAt: &endAt, Now: now,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to insert the file %s/%s: %w", f.poolID, f.file.FilePath, err)
		}
	}
	for _, tagID := range r.TagIDs {
		name, ok := a.tags[tagID]
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		kotatsuTagID, err := q.UpsertTag(ctx, sqlc.UpsertTagParams{ID: id.New(), Name: strings.TrimSpace(name), Now: now})
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to create the tag %q: %w", name, err)
		}
		if err := q.InsertRecordingTag(ctx, sqlc.InsertRecordingTagParams{RecordingID: recordingID, TagID: kotatsuTagID}); err != nil {
			return uuid.Nil, fmt.Errorf("failed to tag the recording: %w", err)
		}
	}

	// TS のセグメントがあれば、nekonote が索引とドロップの集計を作り、続けて画像を作る (設計書 6.3)。
	// エンコード済みのファイルしかない録画には、作るものがない。
	if slices.ContainsFunc(files, func(f *mappedFile) bool { return f.file.Type == VideoFileTypeTS }) {
		params := []river.InsertManyParams{{Args: analysis.AnalyzeArgs{RecordingID: recordingID}, InsertOpts: nil}}
		if _, err := a.importer.inserter.InsertManyTx(ctx, tx, params); err != nil {
			return uuid.Nil, fmt.Errorf("failed to enqueue the analysis: %w", err)
		}
	}

	return recordingID, nil
}

// history は、EPGStation の録画履歴を、Kotatsu の重複の回避に使う録画履歴として記録する。
// EPGStation の録画履歴には終わりの時刻しかないので、始まりも終わりの時刻にする。
func (a *applier) history(ctx context.Context, tx pgx.Tx, h *RecordedHistory) (uuid.UUID, error) {
	if !a.services[h.ChannelID] {
		return uuid.Nil, skip("service %d is not in Kotatsu", h.ChannelID)
	}
	historyID := id.New()
	endAt := h.EndAt.UTC()
	err := sqlc.New(tx).InsertRecordingHistory(ctx, sqlc.InsertRecordingHistoryParams{
		ID: historyID, ServiceID: h.ChannelID, TitleKey: rule.TitleKey(h.Name), Name: h.Name,
		StartAt: endAt, EndAt: endAt, Now: a.importer.clock.Now(),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert the recording history: %w", err)
	}

	return historyID, nil
}
