package fileimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/analysis"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// importSource は、import_sources に記録する取り込み元の名前。
	importSource = "files"
	// kindRecorded は、import_sources の kind。EPGStation から取り込んだ録画と同じく、アニメの作品との結びつきの対象にする (設計書 3.8)。
	kindRecorded = "recorded"
	// DefaultMinAge は、取り込むファイルの、最後に書いてから空ける時間の既定値。書いている途中のファイルを取り込まないためである。
	DefaultMinAge = 10 * time.Minute
	// serviceIDBase は、Kotatsu のサービス ID (network_id * 100000 + service_id) の桁の区切り。
	serviceIDBase = 100000
	// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
	// auditTarget は、録画の監査ログの対象の種類。
	auditTarget = "recording"
)

var (
	// ErrInvalidDirectory は、プールの中のディレクトリとして使えないパスであることを表す。
	ErrInvalidDirectory = errors.New("invalid directory in the pool")
	// errDryRun は、dry run で書き込みを巻き戻すのに使う。
	errDryRun = errors.New("dry run")
)

// extensions は、取り込む TS の拡張子 (小文字)。
func extensions() []string {
	return []string{".ts", ".m2ts"}
}

// Options は、取り込みの条件。
type Options struct {
	// PoolID と Dir は、取り込むファイルを探すプールと、プールの中のディレクトリ (/ 区切り)。空か . ならプールの全体。
	PoolID string
	Dir    string
	// UserID は、監査ログに残す、取り込んだユーザー。dry run では uuid.Nil() でよい。
	UserID uuid.UUID
	// MinAge は、最後に書いてからこれだけたっていないファイルを、書いている途中とみなして飛ばす時間。
	MinAge time.Duration
	// DryRun なら、書き込みを巻き戻す。
	DryRun bool
}

// Result は、取り込みの結果。
type Result struct {
	// Imported は、取り込んだ (dry run では取り込む) ファイル。
	Imported []*Imported
	// AlreadyImported は、前に取り込んだので飛ばしたファイルの数。
	AlreadyImported int
	// AlreadyRecorded は、もう録画のファイルとして記録されているので飛ばしたファイル (Kotatsu が録った録画など) の数。
	AlreadyRecorded int
	// Skipped は、取り込めなかったファイルと、その理由。
	Skipped []*Skipped
}

// Imported は、取り込んだファイルと、録画にした番組。
type Imported struct {
	Path        string
	ServiceName string
	Name        string
	StartAt     time.Time
}

// Skipped は、取り込めなかったファイル。
type Skipped struct {
	Path   string
	Reason string
}

// skipError は、そのファイルを取り込まずに次へ進むことを表す。Error は飛ばした理由を返す。
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

// Database は、取り込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Importer は、プールにある TS を録画として取り込む。
type Importer struct {
	clock    clock.Clock
	recorder audit.Recorder
	inserter analysis.Inserter
	store    *kago.Store
}

// NewImporter は Importer を返す。inserter は、取り込んだ録画を解析するジョブを投入する。store は、このインスタンスにマウントされたプール。
func NewImporter(clk clock.Clock, recorder audit.Recorder, inserter analysis.Inserter, store *kago.Store) *Importer {
	return &Importer{clock: clk, recorder: recorder, inserter: inserter, store: store}
}

// Apply は、プール opts.PoolID のディレクトリ opts.Dir の中の TS を、ファイルごとに 1 つのトランザクションで録画として取り込む。
// 取り込んだことを import_sources に、プールとパス、大きさ、更新日時から作った ID で記録するので、何度実行しても二重に取り込まない。
// もう録画のファイルとして記録されているファイルも取り込まない。
func (im *Importer) Apply(ctx context.Context, db Database, opts *Options) (*Result, error) {
	dir, err := cleanDir(opts.Dir)
	if err != nil {
		return nil, err
	}
	entries, err := im.store.Scan(opts.PoolID)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(db).ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	services := make(map[int64]string, len(rows))
	for _, row := range rows {
		services[row.ID] = row.Name
	}
	actor := audit.User(opts.UserID)
	if opts.UserID == uuid.Nil() {
		// dry run では、ユーザーを指定しなくてよい。書き込みは巻き戻すので、取り込んだ人は残らない。
		actor = audit.System(importSource)
	}

	result := &Result{Imported: []*Imported{}, AlreadyImported: 0, AlreadyRecorded: 0, Skipped: []*Skipped{}}
	a := &applier{importer: im, db: db, opts: opts, actor: actor, services: services, result: result}
	now := im.clock.Now()
	for _, entry := range entries {
		if entry.SegmentDir || !inDir(entry.Path, dir) || !slices.Contains(extensions(), strings.ToLower(path.Ext(entry.Path))) {
			continue
		}
		if now.Sub(entry.ModifiedAt) < opts.MinAge {
			result.Skipped = append(result.Skipped, &Skipped{
				Path: entry.Path, Reason: fmt.Sprintf("modified within the last %s; it may still be being written", opts.MinAge),
			})

			continue
		}
		if err := a.apply(ctx, entry); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// cleanDir は、プールの中のディレクトリを、/ 区切りの、プールの直下からの相対パスにする。プールの全体なら空を返す。
func cleanDir(dir string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(dir))
	if dir == "" || cleaned == "." {
		return "", nil
	}
	if !filepath.IsLocal(filepath.FromSlash(cleaned)) {
		return "", fmt.Errorf("%w: %q", ErrInvalidDirectory, dir)
	}

	return cleaned, nil
}

// inDir は、プールの中のパス rel が、ディレクトリ dir の中にあるかを返す。dir が空ならプールの全体。
func inDir(rel, dir string) bool {
	return dir == "" || strings.HasPrefix(rel, dir+"/")
}

// sourceID は、プールのファイルの、import_sources の ID を返す。プールとパス、大きさ、更新日時の SHA-256 の先頭の 8 バイト。
// ファイルを書き換えれば、別のファイルとして取り込み直せる。
func sourceID(poolID string, entry *kago.Entry) int64 {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%d\x00%d", poolID, entry.Path, entry.Size, entry.ModifiedAt.UnixNano()))
	var id int64
	// 8 バイトを読むだけなので、失敗しない。
	_ = binary.Read(bytes.NewReader(sum[:8]), binary.BigEndian, &id)

	return id
}

// applier は、1 回の取り込みの状態を持つ。
type applier struct {
	importer *Importer
	db       Database
	opts     *Options
	actor    *audit.Actor
	// services は、Kotatsu にあるサービスの ID から名前への対応。
	services map[int64]string
	result   *Result
}

// apply は、1 つのファイルを取り込む。取り込めないファイルは、理由を結果に残して nil を返す。
func (a *applier) apply(ctx context.Context, entry *kago.Entry) error {
	q := sqlc.New(a.db)
	source := sourceID(a.opts.PoolID, entry)
	_, err := q.GetImportedTarget(ctx, sqlc.GetImportedTargetParams{Source: importSource, Kind: kindRecorded, SourceID: source})
	switch {
	case err == nil:
		a.result.AlreadyImported++

		return nil
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("failed to look up the imported file %s: %w", entry.Path, err)
	}

	recorded, err := q.RecordingFileExistsAt(ctx, sqlc.RecordingFileExistsAtParams{PoolID: a.opts.PoolID, Path: entry.Path})
	if err != nil {
		return fmt.Errorf("failed to look up the recording file %s: %w", entry.Path, err)
	}
	if recorded {
		a.result.AlreadyRecorded++

		return nil
	}
	// ファイルを読む間は、トランザクションを開かない。
	recording, err := a.read(entry)
	if err != nil {
		if _, ok := errors.AsType[*skipError](err); ok {
			a.result.Skipped = append(a.result.Skipped, &Skipped{Path: entry.Path, Reason: err.Error()})

			return nil
		}

		return err
	}
	err = pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		if err := a.insert(ctx, tx, entry, source, recording); err != nil {
			return err
		}
		if a.opts.DryRun {
			return errDryRun
		}

		return nil
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		// 調べてから書くまでの間に、ほかで取り込んだか記録した。
		a.result.AlreadyImported++

		return nil
	}
	if err != nil && !errors.Is(err, errDryRun) {
		return fmt.Errorf("failed to import %s: %w", entry.Path, err)
	}
	a.result.Imported = append(a.result.Imported, &Imported{
		Path: entry.Path, ServiceName: a.services[recording.serviceID], Name: recording.name, StartAt: recording.startAt,
	})

	return nil
}

// recording は、ファイルから求めた、取り込む録画。
type recording struct {
	serviceID    int64
	event        *Event
	name         string
	description  string
	startAt      time.Time
	endAt        time.Time
	recordedFrom time.Time
	// recordedUntil は、録った終わり。
	recordedUntil time.Time
}

// read は、ファイルの頭と終わりを読み、取り込む録画を求める。取り込めなければ skipError を返す。
func (a *applier) read(entry *kago.Entry) (*recording, error) {
	f, err := a.importer.store.Open(a.opts.PoolID, entry.Path)
	if err != nil {
		return nil, skip("failed to open: %v", err)
	}
	defer f.Close()
	probe, err := ReadProbe(f, entry.Size)
	if err != nil {
		return nil, skip("%v", err)
	}
	if len(probe.Services) == 0 {
		return nil, skip("no service with video was found in the first %d MiB", headLimit>>20)
	}
	service, serviceID, ok := a.service(probe)
	if !ok {
		descriptions := make([]string, 0, len(probe.Services))
		for _, s := range probe.Services {
			descriptions = append(descriptions, describeService(probe.NetworkID, s))
		}

		return nil, skip("the service is not in Kotatsu (%s)", strings.Join(descriptions, ", "))
	}

	r := &recording{
		serviceID: serviceID, event: service.Program(probe.RecordedFrom, probe.RecordedUntil), name: "", description: "",
		startAt: time.Time{}, endAt: time.Time{}, recordedFrom: time.Time{}, recordedUntil: time.Time{},
	}
	if r.event != nil && strings.TrimSpace(r.event.Name) == "" {
		r.event = nil
	}
	// 録った範囲は、TOT から求める。
	switch {
	case probe.RecordedFrom != nil && probe.RecordedUntil != nil:
		r.recordedFrom, r.recordedUntil = *probe.RecordedFrom, *probe.RecordedUntil
	case r.event != nil && r.event.Start != nil && r.event.Duration != nil:
		// TOT が読めなければ、番組の時間にする。
		r.recordedFrom, r.recordedUntil = *r.event.Start, r.event.Start.Add(*r.event.Duration)
	default:
		// どちらも読めなければ、ファイルの更新日時にする。
		r.recordedFrom, r.recordedUntil = entry.ModifiedAt, entry.ModifiedAt
	}
	if r.event == nil {
		// EIT[p/f] が読めなければ、ファイル名を番組名に、録った範囲を番組の時間にする。
		r.name = strings.TrimSuffix(path.Base(entry.Path), path.Ext(entry.Path))
		r.startAt, r.endAt = r.recordedFrom, r.recordedUntil

		return r, nil
	}
	r.name, r.description = r.event.Name, r.event.Description
	r.startAt, r.endAt = r.recordedFrom, r.recordedUntil
	if r.event.Start != nil && r.event.Duration != nil {
		r.startAt, r.endAt = *r.event.Start, r.event.Start.Add(*r.event.Duration)
	}

	return r, nil
}

// service は、TS のサービスのうち、Kotatsu にある最初のものと、その Kotatsu のサービス ID を返す。
// SDT を読めず、ネットワークが分からなければ、サービス ID が同じ Kotatsu のサービスが 1 つだけのときに、それにする。
func (a *applier) service(probe *Probe) (*Service, int64, bool) {
	for _, s := range probe.Services {
		if probe.NetworkID != nil {
			key := int64(*probe.NetworkID)*serviceIDBase + int64(s.ServiceID)
			if _, ok := a.services[key]; ok {
				return s, key, true
			}

			continue
		}
		matched := []int64{}
		for key := range a.services {
			if key%serviceIDBase == int64(s.ServiceID) {
				matched = append(matched, key)
			}
		}
		if len(matched) == 1 {
			return s, matched[0], true
		}
	}

	return nil, 0, false
}

// describeService は、飛ばした理由に出す、TS のサービスの説明を返す。
func describeService(networkID *uint16, s *Service) string {
	network := "unknown network"
	if networkID != nil {
		network = fmt.Sprintf("network %d", *networkID)
	}
	if s.Name == "" {
		return fmt.Sprintf("%s, service %d", network, s.ServiceID)
	}

	return fmt.Sprintf("%s, service %d %q", network, s.ServiceID, s.Name)
}

// insert は、録画と、そのセグメントのファイルを記録し、取り込んだことを import_sources と監査ログに残し、解析するジョブを投入する。
func (a *applier) insert(ctx context.Context, tx pgx.Tx, entry *kago.Entry, source int64, r *recording) error {
	q := sqlc.New(tx)
	now := a.importer.clock.Now()
	var eventID *int64
	extended := []byte("{}")
	if r.event != nil {
		eventID = new(int64(r.event.EventID))
		encoded, err := encodeExtended(r.event.Extended)
		if err != nil {
			return err
		}
		extended = encoded
	}
	recordingID := id.New()
	recordedFrom, recordedUntil := r.recordedFrom.UTC(), r.recordedUntil.UTC()
	err := q.InsertFileImportedRecording(ctx, sqlc.InsertFileImportedRecordingParams{
		ID: recordingID, ServiceID: r.serviceID, EventID: eventID, Name: r.name, Description: r.description, Extended: extended,
		NameNormalized: new(domain.Normalize(r.name)), DescriptionNormalized: new(domain.Normalize(r.description)),
		StartAt: r.startAt.UTC(), EndAt: r.endAt.UTC(), RecordedFrom: recordedFrom, RecordedUntil: &recordedUntil, Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to insert the recording: %w", err)
	}
	err = q.InsertExternalRecordingFile(ctx, sqlc.InsertExternalRecordingFileParams{
		ID: id.New(), RecordingID: recordingID, PoolID: a.opts.PoolID, Path: entry.Path, Size: entry.Size,
		StartedAt: recordedFrom, EndedAt: &recordedUntil, Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to insert the recording file: %w", err)
	}
	err = q.InsertImportSource(ctx, sqlc.InsertImportSourceParams{
		Source: importSource, Kind: kindRecorded, SourceID: source, TargetID: recordingID, Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to record the imported file: %w", err)
	}
	err = a.importer.recorder.Record(ctx, tx, &audit.Event{
		Actor: a.actor, Action: "recording.imported", TargetType: auditTarget, TargetID: recordingID.String(),
		Summary:  map[string]any{"name": r.name, "pool": a.opts.PoolID, "path": entry.Path},
		SourceIP: netip.Addr{}, UserAgent: "",
	})
	if err != nil {
		return err
	}
	// nekonote が、索引とドロップの集計を作り、続けて画像を作る (設計書 6.3)。
	params := []river.InsertManyParams{{Args: analysis.AnalyzeArgs{RecordingID: recordingID}, InsertOpts: nil}}
	if _, err := a.importer.inserter.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("failed to enqueue the analysis: %w", err)
	}
	if _, err := notify.Publish(ctx, tx, a.importer.clock, notify.RecordingsChannel); err != nil {
		return err
	}

	return nil
}

// encodeExtended は、EIT[p/f] の詳細を、番組表の extended (見出しから本文への対応の JSON) にする。同じ見出しが続けば、本文をつなぐ。
func encodeExtended(items []*ts.ExtendedItem) ([]byte, error) {
	extended := make(map[string]string, len(items))
	for _, item := range items {
		extended[item.Heading] += item.Text
	}
	encoded, err := json.Marshal(extended)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the extended description: %w", err)
	}

	return encoded, nil
}
