package autocaption_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	poolID = "main"
	// serviceID は、テストのサービスの ID。放送のサービス ID (sid) は 1024。
	serviceID = 3273601024
	sid       = 1024
	// segmentA は、テストの録画の最初のセグメントの名前。
	segmentA = "a.m2ts"
	// stateCompleted は、録り終えた録画の状態。
	stateCompleted = "completed"
	// filesKey は、録画の完了のイベントの、セグメントの一覧のキー。
	filesKey = "files"
	// segmentB は、テストの録画の 2 つ目のセグメントの名前。
	segmentB = "b.m2ts"
	// generateKey は、録画の完了のイベントの、字幕を生成するかどうかのキー。
	generateKey = "generateCaptions"
	// fakeModel は、モックの Transcriber のモデルの名前。
	fakeModel = "ggml-fake.bin"
	// storedModel は、テストで DB に書く字幕のモデルの名前。
	storedModel = "ggml-model.bin"
	// recordingsVersion は、録画の変更を通知するチャンネルのバージョンを読む SQL。
	recordingsVersion = "SELECT coalesce((SELECT version FROM change_versions WHERE channel = 'recordings'), 0)"
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
	clk := clocktest.NewFake(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, $2, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $3, $3)`, serviceID, sid, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil), inserter: inserter}
}

// segmentAt は、録画を始めてから start 後に書き始めたセグメント。
type segmentAt struct {
	name  string
	start time.Duration
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
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, s.name), []byte("ts"), 0o600))
		_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', $3, $4, 2, $5, $5, $5)`, id.New(), recordingID, poolID, s.name, recordedFrom.Add(s.start))
		require.NoError(t, err)
	}

	return recordingID
}

// fakeAudio は、セグメントの名前ごとに、WAV の始まりの位置を決めた Extractor を作る。名前のないセグメントには音声がないとする。
// 取り出した WAV には、そのセグメントの名前を書く。
func fakeAudio(offsets map[string]time.Duration) func() (autocaption.Extractor, error) {
	return func() (autocaption.Extractor, error) {
		return func(_ context.Context, path string, service uint16, wav string) (time.Duration, error) {
			if service != sid {
				return 0, fmt.Errorf("unexpected service %d", service)
			}
			offset, ok := offsets[filepath.Base(path)]
			if !ok {
				return 0, autocaption.ErrNoAudio
			}

			return offset, os.WriteFile(wav, []byte(filepath.Base(path)), 0o600)
		}, nil
	}
}

// fakeTranscriber は、WAV に書いたセグメントの名前ごとに決めた字幕を返す Transcriber。
type fakeTranscriber struct {
	cues map[string][]autocaption.Cue
	// before は、字幕を返す前に呼ぶ。nil なら呼ばない。
	before func()
}

var _ autocaption.Transcriber = new(fakeTranscriber)

func (*fakeTranscriber) Model() string { return fakeModel }

func (f *fakeTranscriber) Transcribe(_ context.Context, wav string, progress func(ratio float64)) ([]autocaption.Cue, error) {
	name, err := os.ReadFile(wav)
	if err != nil {
		return nil, err
	}
	progress(0.5)
	if f.before != nil {
		f.before()
	}

	return f.cues[string(name)], nil
}

func (f *fixture) worker(transcriber autocaption.Transcriber, extractor func() (autocaption.Extractor, error)) *autocaption.Worker {
	return autocaption.NewWorker(f.pool, f.store, f.clock, slog.New(slog.DiscardHandler),
		func(context.Context) (autocaption.Transcriber, error) { return transcriber, nil }, extractor, "")
}

func work(t *testing.T, worker *autocaption.Worker, recordingID uuid.UUID) error {
	t.Helper()

	return worker.Work(t.Context(), &river.Job[autocaption.Args]{JobRow: new(rivertype.JobRow), Args: autocaption.Args{RecordingID: recordingID}})
}

// TestWorker は、セグメントごとの字幕を、セグメントを書き始めた時間と、WAV の始まりの位置だけずらして、録画を始めた日時からの時間にそろえて
// 1 つにまとめて書くことを確認する。最初の I フレームより前で終わる字幕は破棄し、前で始まる字幕は 0 からにする。音声のないセグメントは飛ばす。
func TestWorker(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted,
		segmentAt{name: segmentA, start: 0}, segmentAt{name: segmentB, start: 10 * time.Minute}, segmentAt{name: "silent.m2ts", start: 20 * time.Minute})
	transcriber := &fakeTranscriber{cues: map[string][]autocaption.Cue{
		segmentA: {
			{Start: 0, End: 500 * time.Millisecond, Text: "I フレームより前"},
			{Start: 800 * time.Millisecond, End: 2 * time.Second, Text: "またがる"},
			{Start: 5 * time.Second, End: 7 * time.Second, Text: "あ"},
		},
		segmentB: {{Start: time.Second, End: 3 * time.Second, Text: "い"}},
	}, before: nil}
	worker := f.worker(transcriber, fakeAudio(map[string]time.Duration{segmentA: -time.Second, segmentB: 250 * time.Millisecond}))
	before := f.version(t)
	require.NoError(t, work(t, worker, recordingID))

	cues, err := autocaption.RecordingCues(t.Context(), f.pool, recordingID)
	require.NoError(t, err)
	assert.Equal(t, []autocaption.Cue{
		{Start: 0, End: time.Second, Text: "またがる"},
		{Start: 4 * time.Second, End: 6 * time.Second, Text: "あ"},
		{Start: 10*time.Minute + 1250*time.Millisecond, End: 10*time.Minute + 3250*time.Millisecond, Text: "い"},
	}, cues)
	status, err := autocaption.RecordingStatus(t.Context(), f.pool, recordingID)
	require.NoError(t, err)
	assert.Equal(t, &autocaption.Status{State: autocaption.StateGenerated, GeneratedAt: new(f.clock.Now()), Model: fakeModel, Cues: 3, Progress: nil}, status)
	assert.Greater(t, f.version(t), before, "録画の詳細に読み直させる")
	// 字幕の文で探すための gram も書いた字幕から作る (#2214)。
	var grams []string
	var fresh bool
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT search_grams, search_grams_generated_at = generated_at FROM recording_auto_captions WHERE recording_id = $1", recordingID).
		Scan(&grams, &fresh))
	assert.Equal(t, []string{"あ", "い", "が", "がる", "た", "たが", "ま", "また", "る"}, grams)
	assert.True(t, fresh)
}

// TestWorkerReplaces は、生成し直したら、前の字幕を置き換えることを確認する。
func TestWorkerReplaces(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0})
	audio := fakeAudio(map[string]time.Duration{segmentA: 0})
	first := &fakeTranscriber{cues: map[string][]autocaption.Cue{segmentA: {{Start: 0, End: time.Second, Text: "前"}}}, before: nil}
	require.NoError(t, work(t, f.worker(first, audio), recordingID))
	second := &fakeTranscriber{cues: map[string][]autocaption.Cue{segmentA: {{Start: time.Second, End: 2 * time.Second, Text: "後"}}}, before: nil}
	require.NoError(t, work(t, f.worker(second, audio), recordingID))

	cues, err := autocaption.RecordingCues(t.Context(), f.pool, recordingID)
	require.NoError(t, err)
	assert.Equal(t, []autocaption.Cue{{Start: time.Second, End: 2 * time.Second, Text: "後"}}, cues)
}

// TestWorkerCancels は、生成できない録画と、whisper-cli かモデルのない nekonote では、再試行せずに取りやめることを確認する。
func TestWorkerCancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
		// segments は録画のセグメント。
		segments []segmentAt
		// transcriberErr は、Transcriber を作るときの誤り。
		transcriberErr error
		// extractorErr は、Extractor を作るときの誤り。
		extractorErr error
		wantErr      error
	}{
		{
			name: "whisper-cli かモデルがなければ取りやめる", state: stateCompleted, segments: []segmentAt{{name: segmentA, start: 0}},
			transcriberErr: fmt.Errorf("%w: the model is not found", autocaption.ErrNotConfigured), wantErr: autocaption.ErrNotConfigured,
		},
		{
			name: "FFmpeg がなければ取りやめる", state: stateCompleted, segments: []segmentAt{{name: segmentA, start: 0}},
			extractorErr: autocaption.ErrFFmpegNotFound, wantErr: autocaption.ErrFFmpegNotFound,
		},
		{name: "録画中なら取りやめる", state: "recording", segments: []segmentAt{{name: segmentA, start: 0}}},
		{name: "セグメントがなければ取りやめる", state: stateCompleted, segments: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tt.state, tt.segments...)
			transcriber := &fakeTranscriber{cues: map[string][]autocaption.Cue{}, before: nil}
			worker := autocaption.NewWorker(f.pool, f.store, f.clock, slog.New(slog.DiscardHandler),
				func(context.Context) (autocaption.Transcriber, error) {
					if tt.transcriberErr != nil {
						return nil, tt.transcriberErr
					}

					return transcriber, nil
				},
				func() (autocaption.Extractor, error) {
					if tt.extractorErr != nil {
						return nil, tt.extractorErr
					}

					return fakeAudio(map[string]time.Duration{segmentA: 0})()
				}, "")
			err := work(t, worker, recordingID)
			var cancel *river.JobCancelError
			require.ErrorAs(t, err, &cancel)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			cues, err := autocaption.RecordingCues(t.Context(), f.pool, recordingID)
			require.NoError(t, err)
			assert.Empty(t, cues)
		})
	}
}

// TestWorkerRetriesWhenSegmentsChanged は、字幕を作っている間に、セグメントをつないだり名前を変えたりしたら、書かずにやり直すことを確認する。
// 字幕を作るのは長くかかるので、その間は録画ごとのロックを持たない。
func TestWorkerRetriesWhenSegmentsChanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0})
	transcriber := &fakeTranscriber{cues: map[string][]autocaption.Cue{segmentA: {{Start: 0, End: time.Second, Text: "あ"}}}, before: func() {
		_, err := f.pool.Exec(context.Background(), "UPDATE recording_files SET path = 'renamed.m2ts' WHERE recording_id = $1", recordingID)
		if err != nil {
			panic(err)
		}
	}}
	err := work(t, f.worker(transcriber, fakeAudio(map[string]time.Duration{segmentA: 0})), recordingID)
	require.Error(t, err)
	var cancel *river.JobCancelError
	require.NotErrorAs(t, err, &cancel, "やり直す")
	cues, err := autocaption.RecordingCues(t.Context(), f.pool, recordingID)
	require.NoError(t, err)
	assert.Empty(t, cues)
}

// TestWorkerSnoozesWhileLocked は、ほかでセグメントをつないでいる (録画ごとのロックを持っている) 間は、読まずに後でやり直すことを確認する。
func TestWorkerSnoozesWhileLocked(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0})
	// セグメントをつなぐジョブと同じロックを、ほかの接続で持つ。
	conn, err := f.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	locked, err := sqlc.New(conn).TryLockRecordingJoin(t.Context(), recordingID.String())
	require.NoError(t, err)
	require.True(t, locked)
	defer func() { _, _ = sqlc.New(conn).UnlockRecordingJoin(context.Background(), recordingID.String()) }()

	err = work(t, f.worker(&fakeTranscriber{cues: map[string][]autocaption.Cue{}, before: nil}, fakeAudio(map[string]time.Duration{segmentA: 0})), recordingID)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
}

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
			name: "生成する設定の録画を終えたら、生成するジョブを入れる", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), generateKey: true}, want: true,
		},
		{
			name: "生成しない設定の録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), generateKey: false}, want: false,
		},
		{
			name: "生成するかどうかを載せていないイベント (前の版の mikan が出したもの) では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment()}, want: false,
		},
		{
			name: "CM の区間を見分けるだけの録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), "detectCm": true}, want: false,
		},
		{
			name: "セグメントのない録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: []map[string]string{}, generateKey: true}, want: false,
		},
		{
			name: "録画の失敗では、何もしない", kind: domain.NotificationKindRecordingFailed,
			payload: map[string]any{filesKey: oneSegment(), generateKey: true}, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := id.New()
			tt.payload["recordingId"] = recordingID.String()
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload))
			_, err := notification.NewDispatcher(f.pool, f.clock, autocaption.NewEnqueuer(f.inserter, f.clock)).DispatchOnce(t.Context())
			require.NoError(t, err)

			want := map[uuid.UUID]string{}
			if tt.want {
				want[recordingID] = jobs.QueueCaptions
			}
			assert.Equal(t, want, f.queued(t))
		})
	}
}

// queued は、入れた字幕の生成のジョブを、録画の ID ごとのキューの名前で返す。
func (f *fixture) queued(t *testing.T) map[uuid.UUID]string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT queue, args FROM river_job WHERE kind = $1", new(autocaption.Args).Kind())
	require.NoError(t, err)
	defer rows.Close()
	queued := map[uuid.UUID]string{}
	for rows.Next() {
		var queue string
		var raw json.RawMessage
		require.NoError(t, rows.Scan(&queue, &raw))
		var args autocaption.Args
		require.NoError(t, json.Unmarshal(raw, &args))
		queued[args.RecordingID] = queue
	}
	require.NoError(t, rows.Err())

	return queued
}

// version は、録画の変更を通知するチャンネルのバージョンを返す。
func (f *fixture) version(t *testing.T) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.pool.QueryRow(t.Context(), recordingsVersion).Scan(&version))

	return version
}

// TestEnqueue は、録画の詳細から頼まれたら、設定にかかわらずジョブを入れ、待っている間は重ねて入れないことを確認する。
func TestEnqueue(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, stateCompleted, segmentAt{name: segmentA, start: 0})
	before := f.version(t)
	inserted, err := autocaption.Enqueue(t.Context(), f.pool, f.inserter, f.clock, recordingID)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Greater(t, f.version(t), before, "録画の詳細に、待っていると出させる")
	inserted, err = autocaption.Enqueue(t.Context(), f.pool, f.inserter, f.clock, recordingID)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.Equal(t, map[uuid.UUID]string{recordingID: jobs.QueueCaptions}, f.queued(t))
}
