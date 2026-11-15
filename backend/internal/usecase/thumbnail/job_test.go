package thumbnail_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// segmentName は、テストの録画の最初のセグメントの、プール内の相対パス。
	segmentName = "録画.m2ts"
	// indexName は、生成物のディレクトリの、索引のファイルの名前。
	indexName = "index"
	// kindSegment は、recording_files の、セグメントの種類。
	kindSegment = "segment"
	// stateRecording と stateCompleted は、録画中と、録り終えた録画の状態。
	stateRecording = "recording"
	stateCompleted = "completed"
	// detectorFFmpeg と detectorLogo は、CM の区間の見分け方の名前。
	detectorFFmpeg = "ffmpeg"
	detectorLogo   = "logo"
)

type jobFixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	dir   string
	// dataRoot は、データディレクトリ。
	dataRoot string
	store    *kago.Store
	// segmentID は、recording で作った最初のセグメントのファイルの ID。
	segmentID uuid.UUID
}

func newJobFixture(t *testing.T) *jobFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	dir, dataRoot := t.TempDir(), t.TempDir()

	return &jobFixture{
		pool: pool, clock: clk, dir: dir, dataRoot: dataRoot, store: kago.New(map[string]string{poolID: dir}, nil).WithDataDir(dataRoot),
		segmentID: uuid.Nil(),
	}
}

// recording は、I フレームを 1 秒ごとに n 個持つセグメントと、その索引の録画を作る。
func (f *jobFixture) recording(t *testing.T, n int) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	now := f.clock.Now()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, 'completed', false, $3, $3, $3)`, recordingID, serviceID, now)
	require.NoError(t, err)
	seg := segmentAt(t, filepath.Join(f.dir, segmentName), 0, n, time.Second, 0)
	f.segmentID = f.segment(t, recordingID, segmentName, n, seg.Index)

	return recordingID
}

// segment は、I フレームを n 個持つセグメント name の行と、データディレクトリの entries の索引を作り、セグメントのファイルの ID を返す。
func (f *jobFixture) segment(t *testing.T, recordingID uuid.UUID, name string, n int, entries []ts.IndexEntry) uuid.UUID {
	t.Helper()

	segmentID := id.New()
	dataDir, err := f.store.CreateSegmentDir(segmentID)
	require.NoError(t, err)
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NoError(t, writer.Append(entry))
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.dataRoot, dataDir.Path, indexName), index.Bytes(), 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)`, segmentID, recordingID, kindSegment, poolID, name, n*keyframeSize, f.clock.Now())
	require.NoError(t, err)

	return segmentID
}

func (f *jobFixture) generate(t *testing.T, recordingID uuid.UUID, preview bool, newExtractor func() (thumbnail.Extractor, error)) error {
	t.Helper()

	worker := thumbnail.NewGenerateWorkerWith(f.pool, f.store, f.clock, preview, newExtractor)
	job := &river.Job[thumbnail.GenerateArgs]{JobRow: new(rivertype.JobRow), Args: thumbnail.GenerateArgs{RecordingID: recordingID}}

	return worker.Work(t.Context(), job)
}

// hasThumbnail は、最初のセグメントのディレクトリに、代表画像があるかを返す。
func (f *jobFixture) hasThumbnail(t *testing.T) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(f.dataRoot, f.segmentID.String(), thumbnail.ThumbnailFileName))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	require.NoError(t, err)

	return true
}

func fake() (thumbnail.Extractor, error) {
	return &fakeExtractor{broken: nil}, nil
}

// TestGenerateWorker は、録画の最初のセグメントの生成物のディレクトリに、代表画像とプレビュー画像を置き、
// DB には記録しないことを確かめる (設計書 5.6、6.1)。作り直しても、ファイルは増えない。
func TestGenerateWorker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		preview bool
		// wantFiles は、生成物のディレクトリにできるファイル。
		wantFiles []string
	}{
		{name: "プレビュー画像も作る", preview: true, wantFiles: []string{indexName, "preview.jpg", "preview.json", "thumbnail.jpg"}},
		{name: "プレビュー画像を無効にした", preview: false, wantFiles: []string{indexName, "thumbnail.jpg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture(t)
			recordingID := f.recording(t, 30)
			for range 2 {
				require.NoError(t, f.generate(t, recordingID, tt.preview, fake))
			}

			segmentDir := filepath.Join(f.dataRoot, f.segmentID.String())
			entries, err := os.ReadDir(segmentDir)
			require.NoError(t, err)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			assert.Equal(t, tt.wantFiles, names)
			if tt.preview {
				var manifest thumbnail.Manifest
				data, err := os.ReadFile(filepath.Join(segmentDir, "preview.json"))
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &manifest))
				assert.Equal(t, thumbnail.Manifest{
					Width: thumbnail.TileWidth, Height: thumbnail.TileHeight, Columns: thumbnail.Columns, Times: []float64{0, 10, 20},
				}, manifest)
			}
			// 画像は DB に記録しない。
			var rows int
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_files WHERE recording_id = $1", recordingID).Scan(&rows))
			assert.Equal(t, 1, rows)
		})
	}
}

// TestGenerateWorkerPairsIndexesByPath は、始まりが同じセグメントが 2 つあっても、それぞれのファイルの ID の生成物のディレクトリの索引を使うことを確かめる。
// EPGStation から取り込んだ録画では、どのファイルも番組の始まりを始まりとして記録する。
func TestGenerateWorkerPairsIndexesByPath(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	recordingID := f.recording(t, 30)
	// 後から足す、0 秒と 30 秒の位置にだけ I フレームを索引したセグメント。始まりは最初のセグメントと同じにする。
	// 最初のセグメントの I フレームは 0〜29 秒にあるので、このセグメントの索引を使ったときだけ、30 秒のコマができる。
	const second = "録画-2.m2ts"
	seg := segmentAt(t, filepath.Join(f.dir, second), 0, 31, time.Second, 100)
	f.segment(t, recordingID, second, 31, []ts.IndexEntry{seg.Index[0], seg.Index[30]})

	require.NoError(t, f.generate(t, recordingID, true, fake))
	var manifest thumbnail.Manifest
	data, err := os.ReadFile(filepath.Join(f.dataRoot, f.segmentID.String(), "preview.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &manifest))
	// それぞれのセグメントの索引から、10 秒ごとのコマを取り出す。
	assert.Equal(t, []float64{0, 10, 20, 30}, manifest.Times)
}

// TestGenerateWorkerCancels は、FFmpeg がないか、録画が消されたか、画像を作れるセグメントがなければ、送り直さずに取りやめることを確かめる。
func TestGenerateWorkerCancels(t *testing.T) {
	t.Parallel()

	missing := func() (thumbnail.Extractor, error) {
		return nil, fmt.Errorf("%w: exec: \"ffmpeg\": executable file not found in $PATH", thumbnail.ErrFFmpegNotFound)
	}
	tests := []struct {
		name         string
		newExtractor func() (thumbnail.Extractor, error)
		prepare      func(t *testing.T, f *jobFixture, recordingID uuid.UUID) uuid.UUID
		wantErr      string
	}{
		{
			name: "FFmpeg がない", newExtractor: missing,
			prepare: func(_ *testing.T, _ *jobFixture, recordingID uuid.UUID) uuid.UUID { return recordingID },
			wantErr: "FFmpeg is not found in PATH",
		},
		{
			name: "録画が消された", newExtractor: fake,
			prepare: func(_ *testing.T, _ *jobFixture, _ uuid.UUID) uuid.UUID { return id.New() },
			wantErr: "the recording no longer exists",
		},
		{
			name: "索引がない", newExtractor: fake,
			prepare: func(t *testing.T, f *jobFixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				require.NoError(t, os.Remove(filepath.Join(f.dataRoot, kago.IndexLocation(f.segmentID).Path)))

				return recordingID
			},
			wantErr: "the recording has no segment with an index on the mounted pools",
		},
		{
			name: "プールがこのインスタンスにマウントされていない", newExtractor: fake,
			prepare: func(t *testing.T, f *jobFixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				f.store = kago.New(map[string]string{"sub": f.dir}, nil).WithDataDir(f.dataRoot)

				return recordingID
			},
			wantErr: "the recording has no segment with an index on the mounted pools",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture(t)
			target := tt.prepare(t, f, f.recording(t, 3))
			err := f.generate(t, target, true, tt.newExtractor)
			var cancel *rivertype.JobCancelError
			require.ErrorAs(t, err, &cancel)
			require.ErrorContains(t, err, tt.wantErr)
			assert.False(t, f.hasThumbnail(t))
		})
	}
}

// deletingExtractor は、最初に呼ばれたときに録画を消す。画像を作っている間に録画が消されたときを再現する。
type deletingExtractor struct {
	fakeExtractor

	t           *testing.T
	pool        *pgxpool.Pool
	recordingID uuid.UUID
	deleted     bool
}

func (e *deletingExtractor) Extract(ctx context.Context, r io.Reader, size thumbnail.Size) ([]byte, error) {
	if !e.deleted {
		_, err := e.pool.Exec(ctx, "DELETE FROM recordings WHERE id = $1", e.recordingID)
		require.NoError(e.t, err)
		e.deleted = true
	}

	return e.fakeExtractor.Extract(ctx, r, size)
}

// TestGenerateWorkerRemovesImagesOfDeletedRecording は、画像を作っている間に録画が消されたら、書いた画像を消して取りやめることを確かめる。
// 録画の削除は、そのときにあったファイルしか消さないので、残すとデータディレクトリに持ち主のない画像が残る。
func TestGenerateWorkerRemovesImagesOfDeletedRecording(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	recordingID := f.recording(t, 3)
	extractor := &deletingExtractor{broken: nil, t: t, pool: f.pool, recordingID: recordingID, deleted: false}
	err := f.generate(t, recordingID, true, func() (thumbnail.Extractor, error) { return extractor, nil })
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	require.ErrorContains(t, err, "the recording no longer exists")
	assert.NoDirExists(t, filepath.Join(f.dataRoot, f.segmentID.String()))
}

// TestGenerateWorkerLive は、録画中の録画の代表画像 (#1236) を、書いている最中のセグメントから作り、プレビュー画像は作らず、
// 録画を終えた録画では作らないことを確かめる。
func TestGenerateWorkerLive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
		// want は、代表画像を作るかどうか。
		want bool
	}{
		{name: "録画中なら作る", state: stateRecording, want: true},
		{name: "録画を終えていれば作らない", state: stateCompleted, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newJobFixture(t)
			recordingID := f.recording(t, 30)
			// 書いている最中のセグメントは、大きさが 0 で、終わりが NULL。
			_, err := f.pool.Exec(t.Context(), `UPDATE recording_files SET size = 0, ended_at = NULL WHERE id = $1`, f.segmentID)
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), `UPDATE recordings SET state = $2 WHERE id = $1`, recordingID, tt.state)
			require.NoError(t, err)

			worker := thumbnail.NewGenerateWorkerWith(f.pool, f.store, f.clock, true, fake)
			err = worker.Work(t.Context(), &river.Job[thumbnail.GenerateArgs]{
				JobRow: new(rivertype.JobRow), Args: thumbnail.GenerateArgs{RecordingID: recordingID, Live: true, ThumbnailOnly: false},
			})
			if !tt.want {
				var cancel *rivertype.JobCancelError
				require.ErrorAs(t, err, &cancel)
				assert.False(t, f.hasThumbnail(t))

				return
			}
			require.NoError(t, err)
			assert.True(t, f.hasThumbnail(t))
			_, err = os.Stat(filepath.Join(f.dataRoot, f.segmentID.String(), thumbnail.PreviewFileName))
			assert.ErrorIs(t, err, os.ErrNotExist, "the preview is made after the recording ends")
		})
	}
}

// fakeInserter は、入れたジョブを覚える。
type fakeInserter struct {
	args []river.JobArgs
}

func (f *fakeInserter) InsertManyTx(_ context.Context, _ pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	for _, param := range params {
		f.args = append(f.args, param.Args)
	}

	return []*rivertype.JobInsertResult{}, nil
}

// TestLiveWorker は、録り始めてから LiveInterval たった録画中の録画だけに、今の様子の代表画像を作るジョブを入れることを確かめる (#1236)。
func TestLiveWorker(t *testing.T) {
	t.Parallel()
	f := newJobFixture(t)
	now := f.clock.Now()
	recordings := map[string]uuid.UUID{}
	for _, recording := range []struct {
		name  string
		state string
		from  time.Time
	}{
		{name: "old", state: stateRecording, from: now.Add(-thumbnail.LiveInterval)},
		{name: "new", state: stateRecording, from: now.Add(-time.Minute)},
		{name: stateCompleted, state: stateCompleted, from: now.Add(-time.Hour)},
	} {
		recordingID := id.New()
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
			state, protected, recorded_from, created_at, updated_at)
			VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $3, $3)`, recordingID, serviceID, recording.from, recording.state)
		require.NoError(t, err)
		recordings[recording.name] = recordingID
	}
	inserter := new(fakeInserter)

	require.NoError(t, thumbnail.NewLiveWorker(f.pool, inserter, f.clock).Work(t.Context(), &river.Job[thumbnail.LiveArgs]{JobRow: new(rivertype.JobRow), Args: thumbnail.LiveArgs{}}))
	assert.Equal(t, []river.JobArgs{thumbnail.GenerateArgs{RecordingID: recordings["old"], Live: true, ThumbnailOnly: false}}, inserter.args)
}

// thumbnailMarker は、最初のセグメントのディレクトリの代表画像にした、I フレームの通し番号を返す。
func (f *jobFixture) thumbnailMarker(t *testing.T) int {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(f.dataRoot, f.segmentID.String(), thumbnail.ThumbnailFileName))
	require.NoError(t, err)

	return marker(decode(t, data), thumbnail.ThumbnailWidth/2, thumbnail.ThumbnailHeight/2)
}

// cmSection は、録画を始めた日時からの時間 (秒) の CM の区間。
type cmSection struct {
	start, end int64
}

// cmDetection は、見分け方 detector で、at に見分けた CM の区間。
type cmDetection struct {
	detector string
	at       time.Duration
	sections []cmSection
}

// TestGenerateWorkerAvoidsCM は、最後に見分けた見分け方の CM の区間を避けて、代表画像を作ることを確かめる (#1810)。
// 区間は録画を始めた日時からの時間なので、セグメントが録画を始めた日時より後に始まれば、その分ずらして比べる。
func TestGenerateWorkerAvoidsCM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		detections []cmDetection
		// lateStart は、セグメントの始まりが、録画を始めた日時より後にずれている長さ。
		lateStart time.Duration
		// want は、代表画像にした I フレームの通し番号。
		want int
	}{
		// 1 秒ごとの 30 個の I フレーム (0〜29 秒) の 25% は、7 秒の I フレーム (通し番号 8)。
		{name: "見分けていなければ 25% の位置", detections: nil, lateStart: 0, want: 8},
		// 余白を足した 4〜11 秒を避け、11 秒の I フレーム (通し番号 12) を選ぶ。
		{name: "CM の区間を避ける", detections: []cmDetection{{detector: detectorFFmpeg, at: 0, sections: []cmSection{{start: 5, end: 10}}}}, lateStart: 0, want: 12},
		{
			name: "最後に見分けた見分け方の区間を使う",
			detections: []cmDetection{
				{detector: detectorFFmpeg, at: 0, sections: []cmSection{{start: 5, end: 10}}},
				{detector: detectorLogo, at: time.Minute, sections: []cmSection{{start: 20, end: 25}}},
			},
			lateStart: 0, want: 8,
		},
		// 録画の時間の 8〜13 秒は、セグメントの 5〜10 秒。
		{name: "セグメントの始まりのずれを引く", detections: []cmDetection{{detector: detectorFFmpeg, at: 0, sections: []cmSection{{start: 8, end: 13}}}}, lateStart: 3 * time.Second, want: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newJobFixture(t)
			recordingID := f.recording(t, 30)
			_, err := f.pool.Exec(t.Context(), `UPDATE recording_files SET started_at = started_at + $2 WHERE id = $1`, f.segmentID, tt.lateStart)
			require.NoError(t, err)
			for _, detection := range tt.detections {
				_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
					VALUES ($1, $2, 1, $3)`, recordingID, detection.detector, f.clock.Now().Add(detection.at))
				require.NoError(t, err)
				for _, section := range detection.sections {
					_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_sections (recording_id, detector, start_offset_ms, end_offset_ms)
						VALUES ($1, $2, $3, $4)`, recordingID, detection.detector, section.start*1000, section.end*1000)
					require.NoError(t, err)
				}
			}

			require.NoError(t, f.generate(t, recordingID, false, fake))
			assert.Equal(t, tt.want, f.thumbnailMarker(t))
		})
	}
}

// TestGenerateWorkerThumbnailOnly は、代表画像だけを作り直すジョブ (#1810) が、プレビュー画像を作らないことを確かめる。
func TestGenerateWorkerThumbnailOnly(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	recordingID := f.recording(t, 30)
	worker := thumbnail.NewGenerateWorkerWith(f.pool, f.store, f.clock, true, fake)
	require.NoError(t, worker.Work(t.Context(), &river.Job[thumbnail.GenerateArgs]{
		JobRow: new(rivertype.JobRow), Args: thumbnail.GenerateArgs{RecordingID: recordingID, Live: false, ThumbnailOnly: true},
	}))
	assert.True(t, f.hasThumbnail(t))
	assert.NoFileExists(t, filepath.Join(f.dataRoot, f.segmentID.String(), thumbnail.PreviewFileName))
}

// TestCMEnqueuer は、CM の区間を見分け終えたら、代表画像だけを作り直すジョブを入れることを確かめる (#1810)。
func TestCMEnqueuer(t *testing.T) {
	t.Parallel()

	inserter := new(fakeInserter)
	recordingID := id.New()
	require.NoError(t, thumbnail.NewCMEnqueuer(inserter).OnDetected(t.Context(), nil, recordingID))
	assert.Equal(t, []river.JobArgs{thumbnail.GenerateArgs{RecordingID: recordingID, Live: false, ThumbnailOnly: true}}, inserter.args)
}
