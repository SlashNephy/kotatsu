package anime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// nicologInterval は、録画に対応する NICOLOG のファイルを探す間隔。
	nicologInterval = time.Hour
	// maxNicologRecordings は、1 回のジョブで探す録画の数の上限。個人が運営するサービスなので、問い合わせを控えめにする。
	maxNicologRecordings = 20
	// nicologRecheckInterval は、見つからなかった録画を調べ直す間隔。nicologRecheckWindow は、録画を終えてから調べ直し続ける期間。
	nicologRecheckInterval = 24 * time.Hour
	nicologRecheckWindow   = 14 * 24 * time.Hour
	// nicologRequestInterval は、NICOLOG への問い合わせの間隔の下限。
	nicologRequestInterval = 2 * time.Second
)

// errNicologFileNotFound は、録画に対応する NICOLOG のファイルが見つからないことを表す。
var errNicologFileNotFound = errors.New("the nicolog file is not found")

// NicologClient は、NICOLOG への問い合わせ。
type NicologClient interface {
	List(ctx context.Context, path string) ([]*nicolog.Entry, error)
}

var _ NicologClient = new(nicolog.Client)

// NicologMatcher は、録画に対応する NICOLOG のファイルを探す (#1363、設計書 7.4)。
type NicologMatcher struct {
	db     Database
	clock  clock.Clock
	client NicologClient
	logger *slog.Logger
	// interval は、問い合わせの間隔。
	interval time.Duration
}

// NewNicologMatcher は NicologMatcher を返す。
func NewNicologMatcher(db Database, clk clock.Clock, client NicologClient, interval time.Duration, logger *slog.Logger) *NicologMatcher {
	return &NicologMatcher{db: db, clock: clk, client: client, interval: interval, logger: logger}
}

// DefaultNicologRequestInterval は、NICOLOG への問い合わせの間隔の既定値。
const DefaultNicologRequestInterval = nicologRequestInterval

// Match は、まだ調べていない録画の、NICOLOG のファイルを探す。管理者が NICOLOG のソースをオプトインしていなければ、問い合わせない。
func (m *NicologMatcher) Match(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	system, err := settings.Load(ctx, m.db)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(system.CommentSources, domain.CommentSourceNicolog) {
		result.Disabled = true

		return result, nil
	}
	now := m.clock.Now()
	q := sqlc.New(m.db)
	recordings, err := q.ListRecordingsForNicolog(ctx, sqlc.ListRecordingsForNicologParams{
		CheckedBefore: now.Add(-nicologRecheckInterval), RecordedSince: now.Add(-nicologRecheckWindow), MaxRecordings: maxNicologRecordings,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings to look up: %w", err)
	}
	if len(recordings) == 0 {
		return result, nil
	}
	root, err := m.client.List(ctx, nicolog.RootPath)
	result.Requests++
	if err != nil {
		return nil, fmt.Errorf("failed to list the nicolog titles: %w", err)
	}
	folders := map[string]string{}
	for _, entry := range root {
		if entry.IsDir {
			folders[TitleKey(entry.Name)] = entry.Name
		}
	}
	listings := map[string][]*nicolog.Entry{}
	var failures error
	for _, recording := range recordings {
		path, err := m.find(ctx, folders, listings, recording.Title, recording.EpisodeCount, result)
		if err != nil && !errors.Is(err, errNicologFileNotFound) {
			result.Failed++
			failures = errors.Join(failures, err)

			continue
		}
		if err := q.UpsertRecordingNicologFile(ctx, sqlc.UpsertRecordingNicologFileParams{RecordingID: recording.ID, Path: path, Now: now}); err != nil {
			return nil, fmt.Errorf("failed to save the nicolog file: %w", err)
		}
		result.Programs++
		if err == nil {
			result.Linked++
		}
	}
	if result.Failed > 0 && result.Linked == 0 && result.Programs == 0 {
		return nil, failures
	}

	return result, nil
}

// find は、作品 title の話 count のファイルのパスを返す。見つからなければ nil と errNicologFileNotFound を返す。作品名は、正規化して完全に一致するものだけを使う。
func (m *NicologMatcher) find(ctx context.Context, folders map[string]string, listings map[string][]*nicolog.Entry, title string, count float64, result *MatchResult) (*string, error) {
	folder, ok := folders[TitleKey(title)]
	if !ok || count != float64(int(count)) {
		return nil, errNicologFileNotFound
	}
	path := nicolog.RootPath + "/" + folder
	entries, ok := listings[folder]
	if !ok {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.interval):
		}
		var err error
		entries, err = m.client.List(ctx, path)
		result.Requests++
		if err != nil {
			return nil, fmt.Errorf("failed to list the nicolog files of a title: %w", err)
		}
		listings[folder] = entries
	}
	name := folder + " " + strconv.Itoa(int(count)) + "話上映会.xml"
	for _, entry := range entries {
		if !entry.IsDir && entry.Name == name {
			return new(path + "/" + name), nil
		}
	}

	return nil, errNicologFileNotFound
}

// NicologArgs は、NICOLOG のファイルを探すジョブの引数。
type NicologArgs struct{}

var _ river.JobArgsWithInsertOpts = new(NicologArgs)

func (NicologArgs) Kind() string { return "anime_nicolog" }

// InsertOpts は、同じジョブを二重に動かさないようにする。
func (NicologArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// NicologWorker は、NICOLOG のファイルを探すジョブを実行する。
type NicologWorker struct {
	river.WorkerDefaults[NicologArgs]

	matcher *NicologMatcher
}

var _ river.Worker[NicologArgs] = new(NicologWorker)

// NewNicologWorker は NicologWorker を作る。
func NewNicologWorker(matcher *NicologMatcher) *NicologWorker {
	return &NicologWorker{WorkerDefaults: river.WorkerDefaults[NicologArgs]{}, matcher: matcher}
}

func (w *NicologWorker) Work(ctx context.Context, _ *river.Job[NicologArgs]) error {
	result, err := w.matcher.Match(ctx)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, result); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// NicologPeriodicJob は、NICOLOG のファイルを定期的に探すジョブを返す。
func NicologPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(nicologInterval),
		func() (river.JobArgs, *river.InsertOpts) { return NicologArgs{}, nil },
		&river.PeriodicJobOpts{ID: "anime_nicolog", RunOnStart: true},
	)
}
