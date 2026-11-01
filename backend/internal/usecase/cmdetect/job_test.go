package cmdetect_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
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
	// filesKey は、録画の完了のイベントの、セグメントの一覧のキー。
	filesKey = "files"
	// detectKey は、録画の完了のイベントの、CM の区間を見分けるかどうかのキー。
	detectKey = "detectCm"
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
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, $2, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $3, $3)`, serviceID, sid, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{
		pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil).WithDataDir(t.TempDir()), inserter: inserter,
	}
}

// segmentAt は、録画を始めてから start 後に書き始めたセグメント。
type segmentAt struct {
	name  string
	start time.Duration
}

// recording は、状態が state で、segments のセグメントを持つ録画を作る。
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

// fakeDetector は、ファイルの名前ごとに決めた区間を返す Detector。名前のないファイルでは、映像がないとする。
type fakeDetector struct {
	sections map[string][]cmdetect.Section
	// inputs は、渡されたファイルの名前とサービス ID。
	inputs []cmdetect.Input
}

var _ cmdetect.Detector = new(fakeDetector)

func (*fakeDetector) Name() string { return "fake" }

func (*fakeDetector) Version() int32 { return 3 }

func (d *fakeDetector) Detect(_ context.Context, input *cmdetect.Input) ([]cmdetect.Section, error) {
	d.inputs = append(d.inputs, cmdetect.Input{Path: filepath.Base(input.Path), ServiceID: input.ServiceID})
	sections, ok := d.sections[filepath.Base(input.Path)]
	if !ok {
		return nil, cmdetect.ErrNoVideo
	}

	return sections, nil
}

func (f *fixture) detect(t *testing.T, detector cmdetect.Detector, recordingID uuid.UUID) error {
	t.Helper()

	worker := cmdetect.NewDetectWorkerWith(f.pool, f.store, f.clock, func() (cmdetect.Detector, error) { return detector, nil })

	return worker.Work(t.Context(), &river.Job[cmdetect.DetectArgs]{JobRow: new(rivertype.JobRow), Args: cmdetect.DetectArgs{RecordingID: recordingID}})
}

// sections は、録画の、見分け方 fake の区間を、始まりの順に返す。
func (f *fixture) sections(t *testing.T, recordingID uuid.UUID) [][2]time.Duration {
	t.Helper()

	rows, err := f.pool.Query(t.Context(),
		"SELECT start_offset_ms, end_offset_ms FROM recording_cm_sections WHERE recording_id = $1 AND detector = 'fake' ORDER BY start_offset_ms", recordingID)
	require.NoError(t, err)
	got := [][2]time.Duration{}
	for rows.Next() {
		var start, end int64
		require.NoError(t, rows.Scan(&start, &end))
		got = append(got, [2]time.Duration{time.Duration(start) * time.Millisecond, time.Duration(end) * time.Millisecond})
	}
	require.NoError(t, rows.Err())

	return got
}

func TestDetectWorker(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 区間は、セグメントの始まりを足して、録画を始めた日時からの時間にする。
	recordingID := f.recording(t, "completed",
		segmentAt{name: segmentA, start: 0},
		segmentAt{name: segmentB, start: 20 * time.Minute},
		segmentAt{name: "empty.m2ts", start: 29 * time.Minute})
	detector := &fakeDetector{sections: map[string][]cmdetect.Section{
		segmentA: {{Start: time.Minute, End: 2 * time.Minute}, {Start: 1190 * time.Second, End: 1200 * time.Second}},
		// 1 つ目のセグメントの終わりに続く区間は、1 つにまとめる。
		segmentB: {{Start: 0, End: 30 * time.Second}, {Start: 5 * time.Minute, End: 6 * time.Minute}},
	}}
	require.NoError(t, f.detect(t, detector, recordingID))

	assert.Equal(t, [][2]time.Duration{
		{time.Minute, 2 * time.Minute},
		{1190 * time.Second, 1230 * time.Second},
		{25 * time.Minute, 26 * time.Minute},
	}, f.sections(t, recordingID))
	// TS の中から、録画したサービスのストリームを選ばせる。映像のないセグメントは飛ばす。
	assert.Equal(t, []cmdetect.Input{{Path: segmentA, ServiceID: sid}, {Path: segmentB, ServiceID: sid}, {Path: "empty.m2ts", ServiceID: sid}}, detector.inputs)

	var version int32
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT detector_version FROM recording_cm_detections WHERE recording_id = $1 AND detector = 'fake'", recordingID).Scan(&version))
	assert.Equal(t, int32(3), version)

	// 見分け直したら、前の区間を置き換える。CM が見つからなくても、見分けた記録は残す。
	require.NoError(t, f.detect(t, &fakeDetector{sections: map[string][]cmdetect.Section{segmentA: {}, segmentB: {}}}, recordingID))
	assert.Empty(t, f.sections(t, recordingID))
	var detections int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_cm_detections WHERE recording_id = $1", recordingID).Scan(&detections))
	assert.Equal(t, 1, detections)
}

func TestDetectWorkerCancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
		// unmounted は、録画のセグメントを、マウントされていないプールに置くかどうか。
		unmounted bool
	}{
		{name: "録画中の録画は見分けない", state: "recording"},
		{name: "セグメントがマウントされていなければ取りやめる", state: "completed", unmounted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tt.state, segmentAt{name: segmentA, start: 0})
			if tt.unmounted {
				_, err := f.pool.Exec(t.Context(), "UPDATE recording_files SET pool_id = 'other' WHERE recording_id = $1", recordingID)
				require.NoError(t, err)
			}
			err := f.detect(t, &fakeDetector{sections: map[string][]cmdetect.Section{segmentA: {{Start: 0, End: time.Minute}}}}, recordingID)
			var cancel *rivertype.JobCancelError
			require.ErrorAs(t, err, &cancel)
			assert.Empty(t, f.sections(t, recordingID))
		})
	}
}

func TestDetectWorkerWithoutFFmpeg(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, "completed", segmentAt{name: segmentA, start: 0})
	worker := cmdetect.NewDetectWorkerWith(f.pool, f.store, f.clock, func() (cmdetect.Detector, error) {
		return nil, errors.Join(cmdetect.ErrFFmpegNotFound, errors.New("exec: not found"))
	})
	err := worker.Work(t.Context(), &river.Job[cmdetect.DetectArgs]{JobRow: new(rivertype.JobRow), Args: cmdetect.DetectArgs{RecordingID: recordingID}})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
}

func TestRecordingSections(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, "completed")
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at) VALUES
		($1, 'ffmpeg', 1, $2), ($1, 'fake', 1, $2)`, recordingID, f.clock.Now())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_cm_sections (recording_id, detector, start_offset_ms, end_offset_ms) VALUES
		($1, 'ffmpeg', 600000, 690000), ($1, 'ffmpeg', 0, 59500), ($1, 'fake', 100, 200)`, recordingID)
	require.NoError(t, err)

	// 画面に出すのは、いま使っている見分け方の区間だけ。
	got, err := cmdetect.RecordingSections(t.Context(), f.pool, recordingID)
	require.NoError(t, err)
	assert.Equal(t, []cmdetect.Section{{Start: 0, End: 59500 * time.Millisecond}, {Start: 10 * time.Minute, End: 690 * time.Second}}, got)
}

// oneSegment は、録画の完了のイベントに載せる、セグメント 1 つの一覧。
func oneSegment() []map[string]string {
	return []map[string]string{{"poolId": poolID, "path": segmentA}}
}

func TestEnqueuer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    domain.NotificationKind
		payload map[string]any
		want    bool
	}{
		{
			name: "見分ける設定の録画を終えたら、見分けるジョブを入れる", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), detectKey: true}, want: true,
		},
		{
			name: "見分けない設定の録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment(), detectKey: false}, want: false,
		},
		{
			name: "見分けるかどうかを載せていないイベント (前の版の mikan が出したもの) では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: oneSegment()}, want: false,
		},
		{
			name: "セグメントのない録画では、何もしない", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{filesKey: []map[string]string{}, detectKey: true}, want: false,
		},
		{
			name: "録画の失敗では、何もしない", kind: domain.NotificationKindRecordingFailed,
			payload: map[string]any{filesKey: oneSegment(), detectKey: true}, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := id.New()
			tt.payload["recordingId"] = recordingID.String()
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload))
			_, err := notification.NewDispatcher(f.pool, f.clock, cmdetect.NewEnqueuer(f.inserter)).DispatchOnce(t.Context())
			require.NoError(t, err)

			rows, err := f.pool.Query(t.Context(), "SELECT queue, args FROM river_job WHERE kind = 'recording_cm_detect'")
			require.NoError(t, err)
			defer rows.Close()
			queued := []uuid.UUID{}
			for rows.Next() {
				var queue string
				var raw json.RawMessage
				require.NoError(t, rows.Scan(&queue, &raw))
				assert.Equal(t, jobs.QueueCM, queue)
				var args cmdetect.DetectArgs
				require.NoError(t, json.Unmarshal(raw, &args))
				queued = append(queued, args.RecordingID)
			}
			require.NoError(t, rows.Err())
			if tt.want {
				assert.Equal(t, []uuid.UUID{recordingID}, queued)
			} else {
				assert.Empty(t, queued)
			}
		})
	}
}
