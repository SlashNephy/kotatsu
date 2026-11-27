package aribcaption_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/tstest"
	"github.com/SlashNephy/kotatsu/internal/usecase/aribcaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	poolID = "main"
	// serviceID は、テストのサービスの ID。放送のサービス ID (sid) は 1024。
	serviceID = 3273601024
	sid       = 1024
	// segmentA と segmentB は、テストの録画のセグメントの名前。
	segmentA = "a.m2ts"
	segmentB = "b.m2ts"
	// wantText は、captionSegment のセグメントから取り出す字幕の文。
	wantText = "日本あ"
	// stateCompleted は、録り終えた録画の状態。
	stateCompleted = "completed"
	// filesKey は、録画の完了のイベントの、セグメントの一覧のキー。
	filesKey = "files"
	// recordingsVersion は、録画の変更を通知するチャンネルのバージョンを読む SQL。
	recordingsVersion = "SELECT coalesce((SELECT version FROM change_versions WHERE channel = 'recordings'), 0)"
)

// 字幕の文の 8 単位符号 (ARIB STD-B24)。初めは GL が漢字、GR がひらがな。
const (
	// nihon は「日本」(漢字、GL)。
	nihon = "\x46\x7C\x4B\x5C"
	// hiraganaA は「あ」(ひらがな、GR)。
	hiraganaA = "\xA2"
	// clearScreen は、画面の消去 (CS)。
	clearScreen = "\x0C"
)

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dir      string
	store    *kago.Store
	inserter *jobs.Client
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	clk := clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, $2, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $3, $3)`, serviceID, sid, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil), inserter: inserter}
}

// segmentAt は、録画を始めてから start 後に書き始めた、内容が data のセグメント。
type segmentAt struct {
	name  string
	start time.Duration
	data  []byte
}

// recording は、状態が state で、segments のセグメントを持つ録画を作る。録画を始めた日時は、今の 1 時間前。
func (f *fixture) recording(t *testing.T, state string, segments ...segmentAt) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	recordedFrom := f.clock.Now().Add(-time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $4, $5, false, 0, 0, 0, $3, $4, $3, $3)`,
		recordingID, serviceID, recordedFrom, recordedFrom.Add(30*time.Minute), state)
	require.NoError(t, err)
	for _, s := range segments {
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, s.name), s.data, 0o600))
		_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', $3, $4, $5, $6, $6, $6)`, id.New(), recordingID, poolID, s.name, len(s.data), recordedFrom.Add(s.start))
		require.NoError(t, err)
	}

	return recordingID
}

func (f *fixture) worker() *aribcaption.Worker {
	return aribcaption.NewWorker(f.pool, f.store, f.clock, slog.New(slog.DiscardHandler))
}

func work(t *testing.T, worker *aribcaption.Worker, recordingID uuid.UUID) error {
	t.Helper()

	return worker.Work(t.Context(), &river.Job[aribcaption.Args]{JobRow: new(rivertype.JobRow), Args: aribcaption.Args{RecordingID: recordingID}})
}

// storedCue は、DB の cues に書いた字幕。
type storedCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// stored は、録画の ARIB 字幕の文と、検索の gram を読む。
func (f *fixture) stored(t *testing.T, recordingID uuid.UUID) ([]storedCue, []string) {
	t.Helper()

	row, err := sqlc.New(f.pool).GetRecordingARIBCaptions(t.Context(), recordingID)
	require.NoError(t, err)
	assert.Equal(t, int32(aribcaption.ExtractorVersion), row.ExtractorVersion)
	var cues []storedCue
	require.NoError(t, json.Unmarshal(row.Cues, &cues))
	var grams []string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT search_grams FROM recording_arib_captions WHERE recording_id = $1", recordingID).Scan(&grams))

	return cues, grams
}

// version は、録画の変更を通知するチャンネルのバージョンを返す。
func (f *fixture) version(t *testing.T) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.pool.QueryRow(t.Context(), recordingsVersion).Scan(&version))

	return version
}

// captionSegment は、最初の I フレームの PTS が base 秒で、at 秒に「日本」、at+3 秒に「あ」を出し、at+5 秒に画面を消すセグメントを作る。
func captionSegment(base, at uint64) []byte {
	s := tstest.NewStream(sid).Frame((base - 1) * sec).Keyframe(base * sec).Management(base * sec)
	s.Statement(at*sec, []byte(clearScreen+nihon)).Statement((at+3)*sec, []byte(hiraganaA)).Statement((at+5)*sec, []byte(clearScreen))

	return s.Keyframe((at + 10) * sec).Bytes()
}

// TestWorker は、セグメントごとの字幕を、最初の I フレームを 0 にした時間に、セグメントを書き始めた時間を足して、録画を始めた日時からの時間に
// そろえて書くことを確認する。字幕の文から検索の gram も作り、録画の変更を通知する。
func TestWorker(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted,
		segmentAt{name: segmentA, start: 0, data: captionSegment(100, 102)},
		segmentAt{name: segmentB, start: 10 * time.Minute, data: captionSegment(500, 520)})
	before := f.version(t)

	require.NoError(t, work(t, f.worker(), recordingID))

	cues, grams := f.stored(t, recordingID)
	assert.Equal(t, []storedCue{
		{StartMs: 2000, EndMs: 7000, Text: wantText},
		{StartMs: (10*60 + 20) * 1000, EndMs: (10*60 + 25) * 1000, Text: wantText},
	}, cues)
	assert.Subset(t, grams, []string{"日本", "本あ"})
	assert.Greater(t, f.version(t), before)
}

// TestWorkerRecordsEmptyCaptions は、字幕のない録画も、取り出したことが分かるよう、空の字幕で書くことを確認する。
func TestWorkerRecordsEmptyCaptions(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted,
		segmentAt{name: segmentA, start: 0, data: tstest.NewStream(sid).Keyframe(100 * sec).Keyframe(110 * sec).Bytes()})

	require.NoError(t, work(t, f.worker(), recordingID))

	cues, grams := f.stored(t, recordingID)
	assert.Empty(t, cues)
	assert.Empty(t, grams)
}

// TestWorkerReplaces は、取り出し直したら、前の字幕を置き換えることを確認する。
func TestWorkerReplaces(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0, data: captionSegment(100, 102)})
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_arib_captions (recording_id, extractor_version, extracted_at, cues, search_grams)
		VALUES ($1, 0, $2, '[{"startMs":0,"endMs":1,"text":"古い"}]', '{古い}')`, recordingID, f.clock.Now())
	require.NoError(t, err)

	require.NoError(t, work(t, f.worker(), recordingID))

	cues, grams := f.stored(t, recordingID)
	assert.Equal(t, []storedCue{{StartMs: 2000, EndMs: 7000, Text: wantText}}, cues)
	assert.NotContains(t, grams, "古い")
}

// TestWorkerCancels は、取り出せない録画のジョブを、やり直さずに取りやめることを確認する。
func TestWorkerCancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture) uuid.UUID
	}{
		{
			name:  "録画が消えた",
			setup: func(*testing.T, *fixture) uuid.UUID { return id.New() },
		},
		{
			name: "録画中",
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				return f.recording(t, "recording", segmentAt{name: segmentA, start: 0, data: captionSegment(100, 102)})
			},
		},
		{
			name: "セグメントがない",
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				return f.recording(t, stateCompleted)
			},
		},
		{
			name: "マウントしていないプールのセグメントがある",
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0, data: captionSegment(100, 102)})
				_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
					VALUES ($1, $2, 'segment', 'other', 'b.m2ts', 2, $3, $3, $3)`, id.New(), recordingID, f.clock.Now())
				require.NoError(t, err)

				return recordingID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			err := work(t, f.worker(), tt.setup(t, f))
			var cancel *river.JobCancelError
			require.ErrorAs(t, err, &cancel)
		})
	}
}

// TestWorkerSnoozesWhileLocked は、ほかでセグメントをつないでいる間は、読まずに後でやり直すことを確認する。
func TestWorkerSnoozesWhileLocked(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0, data: captionSegment(100, 102)})
	conn, err := f.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	locked, err := sqlc.New(conn).TryLockRecordingJoin(t.Context(), recordingID.String())
	require.NoError(t, err)
	require.True(t, locked)

	err = work(t, f.worker(), recordingID)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
}

// TestEnqueuer は、TS のセグメントを書いた録画を終えたら、設定にかかわらず、取り出すジョブを重いジョブのキューに入れることを確認する。
func TestEnqueuer(t *testing.T) {
	t.Parallel()

	oneSegment := func() []map[string]string { return []map[string]string{{"poolId": poolID, "path": segmentA}} }
	tests := []struct {
		name    string
		kind    domain.NotificationKind
		payload map[string]any
		want    bool
	}{
		{
			name: "録画を終えたら、取り出すジョブを入れる", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment()}, want: true,
		},
		{
			name: "字幕を自動生成しない設定の録画でも、取り出す", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), "generateCaptions": false}, want: true,
		},
		{
			name: "セグメントのない録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: []map[string]string{}}, want: false,
		},
		{
			name: "録画の失敗では、何もしない", kind: domain.NotificationKindRecordingFailed,
			payload: map[string]any{filesKey: oneSegment()}, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := id.New()
			tt.payload["recordingId"] = recordingID.String()
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload))
			_, err := notification.NewDispatcher(f.pool, f.clock, aribcaption.NewEnqueuer(f.inserter)).DispatchOnce(t.Context())
			require.NoError(t, err)

			want := map[uuid.UUID]string{}
			if tt.want {
				want[recordingID] = jobs.QueueMedia
			}
			assert.Equal(t, want, f.queued(t))
		})
	}
}

// queued は、入れた取り出しのジョブを、録画の ID ごとのキューの名前で返す。
func (f *fixture) queued(t *testing.T) map[uuid.UUID]string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT queue, args FROM river_job WHERE kind = $1", new(aribcaption.Args).Kind())
	require.NoError(t, err)
	defer rows.Close()
	queued := map[uuid.UUID]string{}
	for rows.Next() {
		var queue string
		var raw json.RawMessage
		require.NoError(t, rows.Scan(&queue, &raw))
		var args aribcaption.Args
		require.NoError(t, json.Unmarshal(raw, &args))
		queued[args.RecordingID] = queue
	}
	require.NoError(t, rows.Err())

	return queued
}
