package thumbnail_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// segmentName は、テストの録画の最初のセグメントの、プール内の相対パス。
	segmentName = "録画.m2ts"
	// indexName は、付属ファイルのディレクトリの、索引のファイルの名前。
	indexName = "index"
	// kindThumbnail と kindPreview は、recording_files の、代表画像とプレビュー画像の種類。
	kindThumbnail = "thumbnail"
	kindPreview   = "preview"
)

type jobFixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	dir   string
	store *kago.Store
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
	dir := t.TempDir()

	return &jobFixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil)}
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
	sidecar, err := f.store.CreateSidecarDir(poolID, segmentName)
	require.NoError(t, err)
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	for _, entry := range seg.Index {
		require.NoError(t, writer.Append(entry))
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, sidecar, indexName), index.Bytes(), 0o600))
	for _, file := range []struct {
		kind, path string
		size       int
	}{{"segment", segmentName, n * keyframeSize}, {indexName, sidecar + "/" + indexName, index.Len()}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)`, id.New(), recordingID, file.kind, poolID, file.path, file.size, now)
		require.NoError(t, err)
	}

	return recordingID
}

func (f *jobFixture) generate(t *testing.T, recordingID uuid.UUID, preview bool, newExtractor func() (thumbnail.Extractor, error)) error {
	t.Helper()

	worker := thumbnail.NewGenerateWorkerWith(f.pool, f.store, f.clock, preview, newExtractor)
	job := &river.Job[thumbnail.GenerateArgs]{JobRow: new(rivertype.JobRow), Args: thumbnail.GenerateArgs{RecordingID: recordingID}}

	return worker.Work(t.Context(), job)
}

// images は、録画のファイルとして記録した画像の、種類からプール内の相対パスと大きさへの対応を返す。
func (f *jobFixture) images(t *testing.T, recordingID uuid.UUID) map[string]string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(),
		"SELECT kind, path || ' ' || size FROM recording_files WHERE recording_id = $1 AND kind IN ('thumbnail', 'preview')", recordingID)
	require.NoError(t, err)
	type row struct {
		Kind string
		File string
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	require.NoError(t, err)
	images := map[string]string{}
	for _, r := range got {
		images[r.Kind] = r.File
	}

	return images
}

func fake() (thumbnail.Extractor, error) {
	return &fakeExtractor{broken: nil}, nil
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)

	return info.Size()
}

// TestGenerateWorker は、録画の最初のセグメントの付属ファイルのディレクトリに、代表画像とプレビュー画像を置き、
// 録画のファイルとして記録することを確かめる (設計書 5.6、6.1)。作り直しても、ファイルと行は増えない。
func TestGenerateWorker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		preview bool
		// wantFiles は、付属ファイルのディレクトリにできるファイル。
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

			sidecar := filepath.Join(f.dir, segmentName+".kotatsu")
			entries, err := os.ReadDir(sidecar)
			require.NoError(t, err)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			assert.Equal(t, tt.wantFiles, names)

			want := map[string]string{
				kindThumbnail: fmt.Sprintf("%s.kotatsu/thumbnail.jpg %d", segmentName, fileSize(t, filepath.Join(sidecar, "thumbnail.jpg"))),
			}
			if tt.preview {
				want[kindPreview] = fmt.Sprintf("%s.kotatsu/preview.jpg %d", segmentName, fileSize(t, filepath.Join(sidecar, "preview.jpg")))
				var manifest thumbnail.Manifest
				data, err := os.ReadFile(filepath.Join(sidecar, "preview.json"))
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &manifest))
				assert.Equal(t, thumbnail.Manifest{
					Width: thumbnail.TileWidth, Height: thumbnail.TileHeight, Columns: thumbnail.Columns, Times: []float64{0, 10, 20},
				}, manifest)
			}
			assert.Equal(t, want, f.images(t, recordingID))
		})
	}
}

// TestGenerateWorkerPairsIndexesByPath は、始まりが同じセグメントが 2 つあっても、それぞれの付属ファイルのディレクトリの索引を使うことを確かめる。
// EPGStation から取り込んだ録画では、どのファイルも番組の始まりを始まりとして記録する。
func TestGenerateWorkerPairsIndexesByPath(t *testing.T) {
	t.Parallel()

	f := newJobFixture(t)
	recordingID := f.recording(t, 30)
	// 後から足す、0 秒と 30 秒の位置にだけ I フレームを索引したセグメント。始まりは最初のセグメントと同じにする。
	// 最初のセグメントの I フレームは 0〜29 秒にあるので、このセグメントの索引を使ったときだけ、30 秒のコマができる。
	const second = "録画-2.m2ts"
	seg := segmentAt(t, filepath.Join(f.dir, second), 0, 31, time.Second, 100)
	sidecar, err := f.store.CreateSidecarDir(poolID, second)
	require.NoError(t, err)
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	for _, i := range []int{0, 30} {
		require.NoError(t, writer.Append(seg.Index[i]))
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, sidecar, indexName), index.Bytes(), 0o600))
	for _, file := range []struct {
		kind, path string
		size       int
	}{{"segment", second, 31 * keyframeSize}, {indexName, sidecar + "/" + indexName, index.Len()}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)`, id.New(), recordingID, file.kind, poolID, file.path, file.size, f.clock.Now())
		require.NoError(t, err)
	}

	require.NoError(t, f.generate(t, recordingID, true, fake))
	var manifest thumbnail.Manifest
	data, err := os.ReadFile(filepath.Join(f.dir, segmentName+".kotatsu", "preview.json"))
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

				_, err := f.pool.Exec(t.Context(), "DELETE FROM recording_files WHERE recording_id = $1 AND kind = 'index'", recordingID)
				require.NoError(t, err)

				return recordingID
			},
			wantErr: "the recording has no segment with an index on the mounted pools",
		},
		{
			name: "プールがこのインスタンスにマウントされていない", newExtractor: fake,
			prepare: func(t *testing.T, f *jobFixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				f.store = kago.New(map[string]string{"sub": f.dir}, nil)

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
			assert.Empty(t, f.images(t, target))
		})
	}
}

// TestEnqueuer は、録画の完了のイベントを配るときだけ、その録画の画像を作るジョブを、重いジョブのキューに投入することを確かめる。
