package epgstation_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epgstation"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

const (
	// missingServiceID は、Kotatsu にないサービスの ID。
	missingServiceID = 3273601025
	// eventID は、EPG にある番組 programID の eventId。EPGStation の番組の ID は、チャンネルの ID × 100000 + eventId になる。
	eventID = programID % 100000
	// encodedPool は、エンコード済みのファイルのディレクトリに対応付けるプール ID。
	encodedPool = "encoded"

	programName   = "こたつ特集"
	kotatsu       = "こたつ"
	mikan         = "みかん"
	regexpKeyword = "^こたつ (特集|入門)"
	encodedFile   = "こたつ特集.mp4"
	cleanFile     = "clean.m2ts"
	// segmentKind は、TS のセグメントを表す録画のファイルの種類。
	segmentKind = "segment"
)

type applyFixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	userID   uuid.UUID
	importer *epgstation.Importer
}

func newApplyFixture(t *testing.T) *applyFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, channelID, clk.Now())
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, false, true, 'こたつ特集', '', 'こたつ特集', '', '{}', '[]', '[]', $6, $6)`,
		channelID, eventID, int64(programID), clk.Now().Add(2*time.Hour), clk.Now().Add(150*time.Minute), clk.Now())
	require.NoError(t, err)
	userID := id.New()
	require.NoError(t, sqlc.New(pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: "mikan", DisplayName: mikan, Role: "admin", PasswordHash: nil, CreatedAt: clk.Now(),
	}))

	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)

	return &applyFixture{pool: pool, clock: clk, userID: userID, importer: epgstation.NewImporter(clk, audit.NewRecorder(clk), inserter)}
}

func (f *applyFixture) apply(t *testing.T, snapshot *epgstation.Snapshot) *epgstation.Result {
	t.Helper()

	result, err := f.importer.Apply(t.Context(), f.pool, snapshot, &epgstation.ApplyOptions{
		Pools:  map[string]string{recordedDirectory: mainPool, encodedDirectory: encodedPool},
		UserID: f.userID,
	})
	require.NoError(t, err)

	return result
}

// targetOf は、取り込んだデータから作った行の ID を返す。
func (f *applyFixture) targetOf(t *testing.T, kind epgstation.Kind, sourceID int64) uuid.UUID {
	t.Helper()

	targetID, err := sqlc.New(f.pool).GetImportedTarget(t.Context(), sqlc.GetImportedTargetParams{Source: "epgstation", Kind: string(kind), SourceID: sourceID})
	require.NoError(t, err)

	return targetID
}

func emptySnapshot() *epgstation.Snapshot {
	return &epgstation.Snapshot{
		Rules:          []*epgstation.Rule{},
		ManualReserves: []*epgstation.Reserve{},
		Recordings:     []*epgstation.Recording{},
		Histories:      []*epgstation.RecordedHistory{},
		Tags:           []*epgstation.Tag{},
	}
}

// importedRule は、取り込んだルールのうち、EPGStation のルールから決まる項目。
type importedRule struct {
	Name                string
	Memo                string
	Enabled             bool
	Conditions          *match.Conditions
	Overrides           *reserve.Overrides
	ValidFrom           *time.Time
	ValidUntil          *time.Time
	AvoidDuplicates     bool
	DuplicateWindowDays *int
}

// baseRule は、どの項目も空の EPGStation のルール。
func baseRule() *epgstation.Rule {
	return &epgstation.Rule{
		ID: 1, Enabled: true,
		Keyword:       epgstation.KeywordOption{Keyword: "", CaseSensitive: false, Regexp: false, Name: true, Description: false, Extended: false},
		IgnoreKeyword: epgstation.KeywordOption{Keyword: "", CaseSensitive: false, Regexp: false, Name: false, Description: false, Extended: false},
		ChannelTypes:  []string{}, ChannelIDs: []int64{}, Genres: []*epgstation.Genre{}, Times: []*epgstation.SearchTime{},
		SearchPeriods: []*epgstation.Period{}, TagIDs: []int64{},
	}
}

// baseConditions は、baseRule を取り込んだときの条件。
func baseConditions() *match.Conditions {
	return &match.Conditions{
		Keywords: []*match.Keyword{}, ExcludeKeywords: []*match.Keyword{}, ChannelTypes: []string{}, ServiceIDs: []int64{},
		Genres: []*match.Genre{}, ExcludeGenres: []*match.Genre{}, TimeRanges: []*match.TimeRange{},
		MinDurationSeconds: nil, MaxDurationSeconds: nil, FreeOnly: false,
	}
}

func noOverrides() *reserve.Overrides {
	return &reserve.Overrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil}
}

// TestApplyRules は、EPGStation のルールの各項目を、Kotatsu のルールの対応する項目に変換することを確かめる。
func TestApplyRules(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		modify func(r *epgstation.Rule)
		want   func(r *importedRule)
	}{
		{
			name: "キーワードは空白で区切り、すべてを含む条件にする",
			modify: func(r *epgstation.Rule) {
				r.Keyword = epgstation.KeywordOption{Keyword: "こたつ　みかん", CaseSensitive: true, Regexp: false, Name: true, Description: true, Extended: false}
				r.IgnoreKeyword = epgstation.KeywordOption{Keyword: "再放送 [二]", CaseSensitive: false, Regexp: false, Name: false, Description: false, Extended: true}
			},
			want: func(r *importedRule) {
				r.Name = "こたつ　みかん"
				fields := []match.Field{match.FieldName, match.FieldDescription}
				r.Conditions.Keywords = []*match.Keyword{
					{Text: kotatsu, Regexp: false, CaseSensitive: true, Fields: fields},
					{Text: mikan, Regexp: false, CaseSensitive: true, Fields: fields},
				}
				r.Conditions.ExcludeKeywords = []*match.Keyword{
					{Text: "再放送", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldExtended}},
					{Text: "[二]", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldExtended}},
				}
			},
		},
		{
			name: "正規表現は区切らず、照合する項目がなければ番組名と照合する",
			modify: func(r *epgstation.Rule) {
				r.Keyword = epgstation.KeywordOption{Keyword: regexpKeyword, CaseSensitive: false, Regexp: true, Name: false, Description: false, Extended: false}
			},
			want: func(r *importedRule) {
				r.Name = regexpKeyword
				r.Conditions.Keywords = []*match.Keyword{
					{Text: regexpKeyword, Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}},
				}
			},
		},
		{
			name: "チャンネルを指定していれば、放送波の種類より優先する",
			modify: func(r *epgstation.Rule) {
				r.ChannelTypes = []string{"GR", "BS"}
				r.ChannelIDs = []int64{channelID}
			},
			want: func(r *importedRule) { r.Conditions.ServiceIDs = []int64{channelID} },
		},
		{
			name: "放送波の種類、ジャンル、長さ、無料放送を写す",
			modify: func(r *epgstation.Rule) {
				r.ChannelTypes = []string{"GR", "BS"}
				r.Genres = []*epgstation.Genre{{Genre: 7, SubGenre: nil}, {Genre: 0, SubGenre: new(1)}}
				r.DurationMin, r.DurationMax = new(600), new(3600)
				r.FreeOnly = true
			},
			want: func(r *importedRule) {
				r.Conditions.ChannelTypes = []string{"GR", "BS"}
				r.Conditions.Genres = []*match.Genre{{Lv1: 7, Lv2: nil}, {Lv1: 0, Lv2: new(1)}}
				r.Conditions.MinDurationSeconds, r.Conditions.MaxDurationSeconds = new(600), new(3600)
				r.Conditions.FreeOnly = true
			},
		},
		{
			name: "時刻は時を分にし、曜日のビットを曜日の番号にする",
			modify: func(r *epgstation.Rule) {
				r.Times = []*epgstation.SearchTime{
					{Start: new(20), Range: new(3), Week: 0x3E},
					{Start: nil, Range: nil, Week: 0x41},
					{Start: new(1), Range: new(1), Week: 0},
				}
			},
			want: func(r *importedRule) {
				r.Conditions.TimeRanges = []*match.TimeRange{
					{Weekdays: []int{1, 2, 3, 4, 5}, StartMinute: 1200, DurationMinutes: 180},
					{Weekdays: []int{0, 6}, StartMinute: 0, DurationMinutes: 1440},
				}
				r.Memo = "EPGStation のルール #1 から取り込みました。\n取り込めなかった項目: 曜日のない時刻の条件"
			},
		},
		{
			name: "時刻指定のルールは、秒を分にして番組の時刻の条件にする",
			modify: func(r *epgstation.Rule) {
				r.IsTimeSpecification = true
				r.Times = []*epgstation.SearchTime{{Start: new(21*3600 + 1800), Range: new(1800), Week: 0x01}}
			},
			want: func(r *importedRule) {
				r.Conditions.TimeRanges = []*match.TimeRange{{Weekdays: []int{0}, StartMinute: 1290, DurationMinutes: 30}}
				r.Memo = "EPGStation のルール #1 から取り込みました。\n取り込めなかった項目: 時刻指定 (番組の時刻の条件として取り込んだ)"
			},
		},
		{
			name: "複数の期間は、最初から最後までの 1 つの期間にまとめる",
			modify: func(r *epgstation.Rule) {
				r.SearchPeriods = []*epgstation.Period{
					{StartAt: start.AddDate(0, 1, 0), EndAt: start.AddDate(0, 2, 0)},
					{StartAt: start, EndAt: start.AddDate(0, 0, 7)},
				}
			},
			want: func(r *importedRule) {
				r.ValidFrom, r.ValidUntil = new(start), new(start.AddDate(0, 2, 0))
				r.Memo = "EPGStation のルール #1 から取り込みました。\n取り込めなかった項目: 複数の期間 (最初から最後までの 1 つの期間にまとめた)"
			},
		},
		{
			name: "重複を避けないルールには、重複を避ける期間を付けない",
			modify: func(r *epgstation.Rule) {
				r.AvoidDuplicate = false
				r.PeriodToAvoidDuplicate = new(30)
			},
			want: func(*importedRule) {},
		},
		{
			name: "重複の回避と、その期間を写す",
			modify: func(r *epgstation.Rule) {
				r.AvoidDuplicate = true
				r.PeriodToAvoidDuplicate = new(30)
			},
			want: func(r *importedRule) {
				r.AvoidDuplicates = true
				r.DuplicateWindowDays = new(30)
			},
		},
		{
			name: "重複を避ける期間が 0 日なら、期間を区切らない",
			modify: func(r *epgstation.Rule) {
				r.AvoidDuplicate = true
				r.PeriodToAvoidDuplicate = new(0)
			},
			want: func(r *importedRule) { r.AvoidDuplicates = true },
		},
		{
			name: "保存先のディレクトリは、対応付けたプールにする",
			modify: func(r *epgstation.Rule) {
				r.Enabled = false
				r.ParentDirectoryName = recordedDirectory
			},
			want: func(r *importedRule) {
				r.Enabled = false
				r.Overrides.PoolID = new(mainPool)
			},
		},
		{
			name: "対応付けていない保存先と、取り込まない録画の設定は、末尾の欠けの許容を除いてメモに残す",
			modify: func(r *epgstation.Rule) {
				r.ParentDirectoryName = animeDirectory
				r.TagIDs = []int64{1}
				r.Directory = kotatsu
				r.RecordedFormat = recordedFormat
				r.AllowEndLack = true
			},
			want: func(r *importedRule) {
				r.Memo = "EPGStation のルール #1 から取り込みました。\n取り込めなかった項目: 保存先 anime (プールを対応付けていない)、" +
					"録画に付けるタグ、保存先のサブディレクトリ こたつ、ファイル名の形式 %TITLE%"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newApplyFixture(t)
			source := baseRule()
			tt.modify(source)
			snapshot := emptySnapshot()
			snapshot.Rules = []*epgstation.Rule{source}

			result := f.apply(t, snapshot)
			assert.Equal(t, map[epgstation.Kind]int{epgstation.KindRule: 1}, result.Imported)
			assert.Empty(t, result.Skipped)

			created, err := rule.NewRules(f.clock, audit.NewRecorder(f.clock)).Get(t.Context(), f.pool, f.targetOf(t, epgstation.KindRule, 1))
			require.NoError(t, err)
			want := &importedRule{
				Name: "EPGStation #1", Memo: "EPGStation のルール #1 から取り込みました。", Enabled: true,
				Conditions: baseConditions(), Overrides: noOverrides(),
				ValidFrom: nil, ValidUntil: nil, AvoidDuplicates: false, DuplicateWindowDays: nil,
			}
			tt.want(want)
			assert.Equal(t, want, &importedRule{
				Name: created.Name, Memo: created.Memo, Enabled: created.Enabled, Conditions: created.Conditions, Overrides: created.Overrides,
				ValidFrom: created.ValidFrom, ValidUntil: created.ValidUntil,
				AvoidDuplicates: created.AvoidDuplicates, DuplicateWindowDays: created.DuplicateWindowDays,
			})
			assert.Equal(t, &f.userID, created.CreatedBy)
		})
	}
}

// TestApplyRuleSkipsInvalidRule は、Kotatsu で使えない正規表現のルールを飛ばし、残りのルールを取り込むことを確かめる。
func TestApplyRuleSkipsInvalidRule(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t)
	invalid := baseRule()
	// RE2 は先読みを扱えない。
	invalid.Keyword = epgstation.KeywordOption{Keyword: "こたつ(?!特集)", CaseSensitive: false, Regexp: true, Name: true, Description: false, Extended: false}
	valid := baseRule()
	valid.ID = 2
	snapshot := emptySnapshot()
	snapshot.Rules = []*epgstation.Rule{invalid, valid}

	result := f.apply(t, snapshot)
	assert.Equal(t, map[epgstation.Kind]int{epgstation.KindRule: 1}, result.Imported)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, epgstation.KindRule, result.Skipped[0].Kind)
	assert.Equal(t, int64(1), result.Skipped[0].SourceID)
	assert.Contains(t, result.Skipped[0].Reason, "invalid regular expression")
	var rules int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM rules").Scan(&rules))
	assert.Equal(t, 1, rules)
}

// TestApplyRuleIsMatched は、取り込んだルールが、Kotatsu のルールの照合で番組に一致し、ルール予約を作ることを確かめる。
func TestApplyRuleIsMatched(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t)
	source := baseRule()
	source.Keyword = epgstation.KeywordOption{Keyword: "こたつ 特集", CaseSensitive: false, Regexp: false, Name: true, Description: false, Extended: false}
	source.ChannelTypes = []string{"GR"}
	snapshot := emptySnapshot()
	snapshot.Rules = []*epgstation.Rule{source}
	f.apply(t, snapshot)

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	reservations := reserve.NewReservations(f.clock, audit.NewRecorder(f.clock))
	engine := rule.NewEngine(f.pool, f.clock, location, func(context.Context, pgx.Tx) error { return nil }, reservations)
	result, err := engine.Run(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)

	var kind string
	var gotEventID int64
	var ruleID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT r.kind, r.event_id, rr.rule_id FROM reservations r
		JOIN reservation_rules rr ON rr.reservation_id = r.id`).Scan(&kind, &gotEventID, &ruleID))
	assert.Equal(t, string(reserve.KindRule), kind)
	assert.Equal(t, int64(eventID), gotEventID)
	assert.Equal(t, f.targetOf(t, epgstation.KindRule, 1), ruleID)
}

// importedReservation は、取り込んだ予約のうち、EPGStation の予約から決まる項目。
type importedReservation struct {
	Kind             reserve.Kind
	ServiceID        int64
	EventID          *int64
	StartAt          time.Time
	Name             string
	Enabled          bool
	FollowEventRelay bool
	Overrides        *reserve.Overrides
}

// TestApplyReserves は、手動予約を番組予約か時刻予約として取り込み、取り込めないものを理由と一緒に飛ばすことを確かめる。
func TestApplyReserves(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	reserveAt := func(id int64, startAt time.Time) *epgstation.Reserve {
		return &epgstation.Reserve{
			ID: id, ChannelID: channelID, ChannelType: "GR", StartAt: startAt, EndAt: startAt.Add(30 * time.Minute),
			Name: programName, TagIDs: []int64{},
		}
	}
	tests := []struct {
		name       string
		reserve    func() *epgstation.Reserve
		want       *importedReservation
		wantReason string
	}{
		{
			name: "時刻予約",
			reserve: func() *epgstation.Reserve {
				r := reserveAt(1, now.Add(time.Hour))
				r.IsTimeSpecified = true
				r.ParentDirectoryName = recordedDirectory

				return r
			},
			want: &importedReservation{
				Kind: reserve.KindTime, ServiceID: channelID, EventID: nil, StartAt: now.Add(time.Hour), Name: programName,
				Enabled: true, FollowEventRelay: false,
				Overrides: &reserve.Overrides{PoolID: new(mainPool), FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
			},
		},
		{
			name: "番組予約は、番組情報を EPG から写し、スキップは無効にする",
			reserve: func() *epgstation.Reserve {
				r := reserveAt(1, now.Add(2*time.Hour))
				r.ProgramID = new(int64(programID))
				r.IsSkip = true
				r.IsEventRelay = true

				return r
			},
			want: &importedReservation{
				Kind: reserve.KindProgram, ServiceID: channelID, EventID: new(int64(eventID)), StartAt: now.Add(2 * time.Hour), Name: programName,
				Enabled: false, FollowEventRelay: true, Overrides: noOverrides(),
			},
		},
		{
			name: "EPG にない番組は飛ばす",
			reserve: func() *epgstation.Reserve {
				r := reserveAt(1, now.Add(2*time.Hour))
				r.ProgramID = new(int64(programID + 1))

				return r
			},
			wantReason: "program not found: service 3273601024, event 12346",
		},
		{
			name:       "開始済みの予約は飛ばす",
			reserve:    func() *epgstation.Reserve { return reserveAt(1, now) },
			wantReason: "the reservation has already started",
		},
		{
			name: "Kotatsu にないサービスの予約は飛ばす",
			reserve: func() *epgstation.Reserve {
				r := reserveAt(1, now.Add(time.Hour))
				r.ChannelID = missingServiceID

				return r
			},
			wantReason: "service 3273601025 is not in Kotatsu",
		},
		{
			name: "プールを対応付けていない保存先の予約は飛ばす",
			reserve: func() *epgstation.Reserve {
				r := reserveAt(1, now.Add(time.Hour))
				r.IsTimeSpecified = true
				r.ParentDirectoryName = animeDirectory

				return r
			},
			wantReason: `directory "anime" is not mapped to a pool`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newApplyFixture(t)
			snapshot := emptySnapshot()
			snapshot.ManualReserves = []*epgstation.Reserve{tt.reserve()}

			result := f.apply(t, snapshot)
			if tt.wantReason != "" {
				assert.Empty(t, result.Imported)
				assert.Equal(t, []*epgstation.Skipped{{Kind: epgstation.KindReserve, SourceID: 1, Reason: tt.wantReason}}, result.Skipped)
				var reservations int
				require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM reservations").Scan(&reservations))
				assert.Zero(t, reservations)

				return
			}
			assert.Equal(t, map[epgstation.Kind]int{epgstation.KindReserve: 1}, result.Imported)
			created, err := reserve.NewReservations(f.clock, audit.NewRecorder(f.clock)).Get(t.Context(), f.pool, f.targetOf(t, epgstation.KindReserve, 1))
			require.NoError(t, err)
			assert.Equal(t, tt.want, &importedReservation{
				Kind: created.Kind, ServiceID: created.ServiceID, EventID: created.EventID, StartAt: created.StartAt, Name: created.Name,
				Enabled: created.Enabled, FollowEventRelay: created.FollowEventRelay, Overrides: created.Overrides,
			})
		})
	}
}

// importedFile は、取り込んだ録画のファイル。
type importedFile struct {
	Kind   string
	PoolID string
	Path   string
	Size   int64
}

// TestApplyRecordings は、録画済みの番組を、プールを対応付けたディレクトリのファイルと一緒に取り込むことを確かめる。
func TestApplyRecordings(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 10, 3, 12, 30, 0, 0, time.UTC)
	f := newApplyFixture(t)
	snapshot := emptySnapshot()
	// タグ 2 と 3 は、前後の空白を落とすと同じ名前になる。
	snapshot.Tags = []*epgstation.Tag{
		{ID: 1, Name: animeTag, Color: "#ff0000"}, {ID: 2, Name: " こたつ ", Color: "#00ff00"}, {ID: 3, Name: kotatsu, Color: "#0000ff"},
	}
	recording := func(id int64) *epgstation.Recording {
		return &epgstation.Recording{
			ID: id, ChannelID: channelID, StartAt: recordedAt, EndAt: recordedAt.Add(30 * time.Minute), Name: programName, Description: mikan,
			Genres: []*epgstation.Genre{}, TagIDs: []int64{},
			Files: []*epgstation.VideoFile{{ID: id, ParentDirectoryName: recordedDirectory, FilePath: "a.m2ts", Type: epgstation.VideoFileTypeTS, Name: "TS", Size: 1}},
		}
	}
	full := recording(1)
	full.ProgramID = new(int64(programID))
	full.IsProtected = true
	full.Drop = &epgstation.DropLog{ErrorCount: 1, DropCount: 2, ScramblingCount: 3}
	// 同じタグを 2 度付けても、Kotatsu では 1 つのタグにまとめ、取り込みを止めない。存在しないタグ 4 は付けない。
	full.TagIDs = []int64{1, 1, 2, 3, 4}
	full.Files = []*epgstation.VideoFile{
		{ID: 1, ParentDirectoryName: recordedDirectory, FilePath: "こたつ特集.m2ts", Type: epgstation.VideoFileTypeTS, Name: "TS", Size: 100},
		{ID: 2, ParentDirectoryName: encodedDirectory, FilePath: encodedFile, Type: epgstation.VideoFileTypeEncoded, Name: encodeName, Size: 50},
		{ID: 3, ParentDirectoryName: animeDirectory, FilePath: encodedFile, Type: epgstation.VideoFileTypeEncoded, Name: "H.265", Size: 40},
	}
	clean := recording(2)
	clean.Files[0].FilePath = cleanFile
	inProgress := recording(3)
	inProgress.IsRecording = true
	unmapped := recording(4)
	unmapped.Files[0].ParentDirectoryName = animeDirectory
	missingService := recording(5)
	missingService.ChannelID = missingServiceID
	// 別の録画と同じファイルを指す録画は、Kotatsu のファイルの一意性に反するので飛ばす。
	sameFile := recording(6)
	sameFile.Files[0].FilePath = cleanFile
	// ドロップがなければ、エラーやスクランブルがあっても、Kotatsu の録画と同じく完了にする。
	noDrops := recording(7)
	noDrops.Files[0].FilePath = "no-drops.m2ts"
	noDrops.Drop = &epgstation.DropLog{ErrorCount: 1, DropCount: 0, ScramblingCount: 1}
	// エンコード済みのファイルしかない録画は、取り込むが、解析するものがない。
	encodedOnly := recording(8)
	encodedOnly.Files[0] = &epgstation.VideoFile{
		ID: 8, ParentDirectoryName: encodedDirectory, FilePath: "encoded-only.mp4", Type: epgstation.VideoFileTypeEncoded, Name: encodeName, Size: 1,
	}
	snapshot.Recordings = []*epgstation.Recording{full, clean, inProgress, unmapped, missingService, sameFile, noDrops, encodedOnly}

	result := f.apply(t, snapshot)
	assert.Equal(t, map[epgstation.Kind]int{epgstation.KindRecorded: 4}, result.Imported)
	reasons := map[int64]string{}
	for _, skipped := range result.Skipped {
		reasons[skipped.SourceID] = skipped.Reason
	}
	assert.Len(t, reasons, 4)
	assert.Equal(t, "the program is still being recorded", reasons[3])
	assert.Equal(t, "no file is in a directory mapped to a pool", reasons[4])
	assert.Equal(t, "service 3273601025 is not in Kotatsu", reasons[5])
	assert.Contains(t, reasons[6], "duplicate key value violates unique constraint")

	// TS のある録画だけ、索引と集計を作るジョブを、重いジョブのキューに投入する。飛ばした録画のジョブは、巻き戻して残さない。
	rows, err := f.pool.Query(t.Context(), "SELECT queue, (args->>'recordingId')::uuid FROM river_job WHERE kind = 'recording_analyze'")
	require.NoError(t, err)
	type queuedJob struct {
		Queue       string
		RecordingID uuid.UUID
	}
	queued, err := pgx.CollectRows(rows, pgx.RowToStructByPos[queuedJob])
	require.NoError(t, err)
	sourceIDs := []int64{1, 2, 7}
	want := make([]queuedJob, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		want = append(want, queuedJob{Queue: jobs.QueueMedia, RecordingID: f.targetOf(t, epgstation.KindRecorded, sourceID)})
	}
	assert.ElementsMatch(t, want, queued)

	type importedRecording struct {
		ServiceID     int64
		EventID       *int64
		Name          string
		Description   string
		StartAt       time.Time
		EndAt         time.Time
		State         string
		Protected     bool
		Drops         int64
		Errors        int64
		Scrambled     int64
		RecordedFrom  time.Time
		RecordedUntil *time.Time
	}
	tests := []struct {
		name     string
		sourceID int64
		want     *importedRecording
		files    []*importedFile
		tags     []string
	}{
		{
			name: "ドロップのある録画は、エラーありの完了にする", sourceID: 1,
			want: &importedRecording{
				ServiceID: channelID, EventID: new(int64(eventID)), Name: programName, Description: mikan,
				StartAt: recordedAt, EndAt: recordedAt.Add(30 * time.Minute), State: "completed_with_errors", Protected: true,
				Drops: 2, Errors: 1, Scrambled: 3, RecordedFrom: recordedAt, RecordedUntil: new(recordedAt.Add(30 * time.Minute)),
			},
			files: []*importedFile{
				{Kind: "other", PoolID: encodedPool, Path: encodedFile, Size: 50},
				{Kind: segmentKind, PoolID: mainPool, Path: "こたつ特集.m2ts", Size: 100},
			},
			tags: []string{kotatsu, animeTag},
		},
		{
			name: "ドロップの集計がない録画は、完了にする", sourceID: 2,
			want: &importedRecording{
				ServiceID: channelID, EventID: nil, Name: programName, Description: mikan,
				StartAt: recordedAt, EndAt: recordedAt.Add(30 * time.Minute), State: "completed", Protected: false,
				Drops: 0, Errors: 0, Scrambled: 0, RecordedFrom: recordedAt, RecordedUntil: new(recordedAt.Add(30 * time.Minute)),
			},
			files: []*importedFile{{Kind: segmentKind, PoolID: mainPool, Path: cleanFile, Size: 1}},
			tags:  []string{},
		},
		{
			name: "ドロップのない録画は、エラーやスクランブルがあっても完了にする", sourceID: 7,
			want: &importedRecording{
				ServiceID: channelID, EventID: nil, Name: programName, Description: mikan,
				StartAt: recordedAt, EndAt: recordedAt.Add(30 * time.Minute), State: "completed", Protected: false,
				Drops: 0, Errors: 1, Scrambled: 1, RecordedFrom: recordedAt, RecordedUntil: new(recordedAt.Add(30 * time.Minute)),
			},
			files: []*importedFile{{Kind: segmentKind, PoolID: mainPool, Path: "no-drops.m2ts", Size: 1}},
			tags:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recordingID := f.targetOf(t, epgstation.KindRecorded, tt.sourceID)
			rows, err := f.pool.Query(t.Context(), `SELECT service_id, event_id, name, description, start_at, end_at, state, protected,
				drops, errors, scrambled, recorded_from, recorded_until FROM recordings WHERE id = $1`, recordingID)
			require.NoError(t, err)
			got, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByPos[importedRecording])
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			rows, err = f.pool.Query(t.Context(), "SELECT kind, pool_id, path, size FROM recording_files WHERE recording_id = $1 ORDER BY kind", recordingID)
			require.NoError(t, err)
			files, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[importedFile])
			require.NoError(t, err)
			assert.Equal(t, tt.files, files)

			rows, err = f.pool.Query(t.Context(), `SELECT t.name FROM recording_tags rt JOIN tags t ON t.id = rt.tag_id
				WHERE rt.recording_id = $1`, recordingID)
			require.NoError(t, err)
			tags, err := pgx.CollectRows(rows, pgx.RowTo[string])
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.tags, tags)
		})
	}
}

// TestApplyHistories は、録画履歴を、正規化した番組名と一緒に取り込むことを確かめる。
func TestApplyHistories(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t)
	endAt := time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC)
	snapshot := emptySnapshot()
	snapshot.Histories = []*epgstation.RecordedHistory{
		{ID: 1, Name: "[新]こたつ特集 #1", ChannelID: channelID, EndAt: endAt},
		{ID: 2, Name: "こたつ特集 #2", ChannelID: missingServiceID, EndAt: endAt},
	}

	result := f.apply(t, snapshot)
	assert.Equal(t, map[epgstation.Kind]int{epgstation.KindRecordedHistory: 1}, result.Imported)
	assert.Equal(t, []*epgstation.Skipped{
		{Kind: epgstation.KindRecordedHistory, SourceID: 2, Reason: "service 3273601025 is not in Kotatsu"},
	}, result.Skipped)

	type history struct {
		ServiceID int64
		TitleKey  string
		Name      string
		StartAt   time.Time
		EndAt     time.Time
	}
	rows, err := f.pool.Query(t.Context(), "SELECT service_id, title_key, name, start_at, end_at FROM recording_history WHERE id = $1",
		f.targetOf(t, epgstation.KindRecordedHistory, 1))
	require.NoError(t, err)
	got, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByPos[history])
	require.NoError(t, err)
	assert.Equal(t, &history{ServiceID: channelID, TitleKey: rule.TitleKey("[新]こたつ特集 #1"), Name: "[新]こたつ特集 #1", StartAt: endAt, EndAt: endAt}, got)
}

// TestApplyIsIdempotent は、同じデータを 2 度取り込んでも、前に取り込んだものを作り直さないことを確かめる。
func TestApplyIsIdempotent(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t)
	now := f.clock.Now()
	snapshot := emptySnapshot()
	snapshot.Rules = []*epgstation.Rule{baseRule()}
	snapshot.ManualReserves = []*epgstation.Reserve{{
		ID: 1, IsTimeSpecified: true, ChannelID: channelID, ChannelType: "GR", StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour),
		Name: programName, TagIDs: []int64{},
	}}
	snapshot.Recordings = []*epgstation.Recording{{
		ID: 1, ChannelID: channelID, StartAt: now.Add(-time.Hour), EndAt: now, Name: programName, Genres: []*epgstation.Genre{}, TagIDs: []int64{},
		Files: []*epgstation.VideoFile{{ID: 1, ParentDirectoryName: recordedDirectory, FilePath: "a.m2ts", Type: epgstation.VideoFileTypeTS, Name: "TS", Size: 1}},
	}}
	snapshot.Histories = []*epgstation.RecordedHistory{{ID: 1, Name: programName, ChannelID: channelID, EndAt: now}}
	all := map[epgstation.Kind]int{
		epgstation.KindRule: 1, epgstation.KindReserve: 1, epgstation.KindRecorded: 1, epgstation.KindRecordedHistory: 1,
	}

	first := f.apply(t, snapshot)
	assert.Equal(t, all, first.Imported)
	assert.Empty(t, first.AlreadyImported)

	second := f.apply(t, snapshot)
	assert.Empty(t, second.Imported)
	assert.Equal(t, all, second.AlreadyImported)
	assert.Empty(t, second.Skipped)
	var counts [4]int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM rules), (SELECT count(*) FROM reservations),
		(SELECT count(*) FROM recordings), (SELECT count(*) FROM recording_history)`).Scan(&counts[0], &counts[1], &counts[2], &counts[3]))
	assert.Equal(t, [4]int{1, 1, 1, 1}, counts)
	// 録画を解析するジョブも、2 度目の取り込みでは投入しない。
	var analyses int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = 'recording_analyze'").Scan(&analyses))
	assert.Equal(t, 1, analyses)
}
