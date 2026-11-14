package tvtplay_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/tvtplay"
)

// segmentAt は、録画の始まりから offset 後に始まり、length の長さのセグメントの行を作る。
func (f *chapterFixture) segmentAt(t *testing.T, recordingID uuid.UUID, rel string, offset, length time.Duration) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', 'main', $3, 188, $4, $5, $4)`, id.New(), recordingID, rel, f.startedAt.Add(offset), f.startedAt.Add(offset+length))
	require.NoError(t, err)
}

// otherFile は、その他のファイルの行を作る。
func (f *chapterFixture) otherFile(t *testing.T, recordingID uuid.UUID, rel string) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, created_at)
		VALUES ($1, $2, 'other', 'main', $3, 1, $4, $4)`, id.New(), recordingID, rel, f.startedAt)
	require.NoError(t, err)
}

// detected は、CM の区間を見分けた記録と、区間を作る。
func (f *chapterFixture) detected(t *testing.T, recordingID uuid.UUID, sections ...[2]time.Duration) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
		VALUES ($1, 'ffmpeg', 1, $2)`, recordingID, f.startedAt)
	require.NoError(t, err)
	for _, s := range sections {
		f.section(t, recordingID, s[0], s[1])
	}
}

func (f *chapterFixture) enable(t *testing.T, enabled bool) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO system_settings (id, recording_defaults, archive_retention_days, updated_at, tvtplay_chapters_enabled)
		VALUES (1, '{}', 30, $1, $2) ON CONFLICT (id) DO UPDATE SET tvtplay_chapters_enabled = EXCLUDED.tvtplay_chapters_enabled`, f.startedAt, enabled)
	require.NoError(t, err)
}

// otherFiles は、録画のその他のファイルのパスを返す。
func (f *chapterFixture) otherFiles(t *testing.T, recordingID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT path FROM recording_files WHERE recording_id = $1 AND kind = 'other' ORDER BY path", recordingID)
	require.NoError(t, err)
	defer rows.Close()
	paths := []string{}
	for rows.Next() {
		var p string
		require.NoError(t, rows.Scan(&p))
		paths = append(paths, p)
	}
	require.NoError(t, rows.Err())

	return paths
}

func (f *chapterFixture) work(t *testing.T, dir string, recordingID uuid.UUID) {
	t.Helper()

	worker := tvtplay.NewWriteWorker(f.pool, kago.New(map[string]string{"main": dir}, nil), clocktest.NewFake(f.startedAt.Add(time.Hour)))
	require.NoError(t, worker.Work(t.Context(), &river.Job[tvtplay.WriteArgs]{Args: tvtplay.WriteArgs{RecordingID: recordingID}}))
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// TestWriteWorker は、CM の区間を、セグメントごとの始まりからの位置にずらして、セグメントの隣の chapters フォルダーに書き、
// その他のファイルとして記録することを確かめる。前の名前のファイルは消し、記録のないファイルは上書きしない。
func TestWriteWorker(t *testing.T) {
	t.Parallel()

	f := newChapterFixture(t)
	f.enable(t, true)
	dir := t.TempDir()
	recordingID := f.recording(t, 2*time.Minute, `[]`)
	// 2 本のセグメント。2 本目は、1 本目と 1 秒重なって始まる (引き継ぎ)。
	f.segmentAt(t, recordingID, "番組/こたつ特集.m2ts", 0, time.Minute)
	f.segmentAt(t, recordingID, "番組/こたつ特集-2.m2ts", 59*time.Second, 61*time.Second)
	// CM は、1 本目の中と、2 本目にまたがるもの。
	f.detected(t, recordingID, [2]time.Duration{10 * time.Second, 20 * time.Second}, [2]time.Duration{50 * time.Second, 70 * time.Second})
	// 名前を付け直す前に書いたファイル。
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "古い名前", "chapters"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "古い名前", "chapters", "前.chapter"), []byte("old"), 0o600))
	f.otherFile(t, recordingID, "古い名前/chapters/前.chapter")

	f.work(t, dir, recordingID)

	assert.Equal(t, "\xef\xbb\xbfc-10000cix-20000cox-50000cix-0eox-c", readFile(t, filepath.Join(dir, "番組", "chapters", "こたつ特集.chapter")))
	assert.Equal(t, "\xef\xbb\xbfc-0cix-11000cox-c", readFile(t, filepath.Join(dir, "番組", "chapters", "こたつ特集-2.chapter")))
	assert.NoDirExists(t, filepath.Join(dir, "古い名前"))
	assert.Equal(t, []string{"番組/chapters/こたつ特集-2.chapter", "番組/chapters/こたつ特集.chapter"}, f.otherFiles(t, recordingID))
}

// TestWriteWorkerKeepsForeignFiles は、記録のないチャプターのファイル (comskip-tvtplay が書いたものなど) を上書きせず、
// CM の区間がなくなったら、前に書いたファイルを消すことを確かめる。
func TestWriteWorkerKeepsForeignFiles(t *testing.T) {
	t.Parallel()

	f := newChapterFixture(t)
	f.enable(t, true)
	dir := t.TempDir()
	foreign := f.recording(t, time.Minute, `[]`)
	f.segmentAt(t, foreign, "a.m2ts", 0, time.Minute)
	f.detected(t, foreign, [2]time.Duration{10 * time.Second, 20 * time.Second})
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "chapters"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "chapters", "a.chapter"), []byte("comskip"), 0o600))

	f.work(t, dir, foreign)

	assert.Equal(t, "comskip", readFile(t, filepath.Join(dir, "chapters", "a.chapter")))
	assert.Empty(t, f.otherFiles(t, foreign))

	noCM := f.recording(t, time.Minute, `[]`)
	f.segmentAt(t, noCM, "b.m2ts", 0, time.Minute)
	f.detected(t, noCM)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "chapters", "b.chapter"), []byte("old"), 0o600))
	f.otherFile(t, noCM, "chapters/b.chapter")

	f.work(t, dir, noCM)

	assert.NoFileExists(t, filepath.Join(dir, "chapters", "b.chapter"))
	assert.Empty(t, f.otherFiles(t, noCM))
}

// TestWriteWorkerWhileDisabled は、管理者が無効にしていれば、書かず、前に書いたファイルも消さないことを確かめる。
func TestWriteWorkerWhileDisabled(t *testing.T) {
	t.Parallel()

	f := newChapterFixture(t)
	dir := t.TempDir()
	recordingID := f.recording(t, time.Minute, `[]`)
	f.segmentAt(t, recordingID, "a.m2ts", 0, time.Minute)
	f.detected(t, recordingID, [2]time.Duration{10 * time.Second, 20 * time.Second})
	f.otherFile(t, recordingID, "chapters/old.chapter")

	f.work(t, dir, recordingID)

	assert.NoDirExists(t, filepath.Join(dir, "chapters"))
	assert.Equal(t, []string{"chapters/old.chapter"}, f.otherFiles(t, recordingID))
}

// TestWriteWorkerSkipsGaps は、途切れた区間を挟むセグメント (つないだセグメント) では、ファイルにない区間の分を詰めた位置にすることを確かめる。
func TestWriteWorkerSkipsGaps(t *testing.T) {
	t.Parallel()

	f := newChapterFixture(t)
	f.enable(t, true)
	dir := t.TempDir()
	// 1 分から 1 分 30 秒まで途切れた、つないだ 1 本のセグメント。
	recordingID := f.recording(t, 30*time.Minute, `[{"from":"2026-11-15T21:01:00Z","to":"2026-11-15T21:01:30Z"}]`)
	f.segmentAt(t, recordingID, "a.m2ts", 0, 30*time.Minute)
	f.detected(t, recordingID, [2]time.Duration{10 * time.Minute, 11 * time.Minute}, [2]time.Duration{29*time.Minute + 50*time.Second, 30 * time.Minute})

	f.work(t, dir, recordingID)

	assert.Equal(t, "\xef\xbb\xbfc-570000cix-630000cox-1760000cix-0eox-c", readFile(t, filepath.Join(dir, "chapters", "a.chapter")))
}

// inserter は、投入したジョブの引数を覚える。
type inserter struct {
	args []river.JobArgs
}

func (i *inserter) InsertManyTx(_ context.Context, _ pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	for _, p := range params {
		i.args = append(i.args, p.Args)
	}

	return []*rivertype.JobInsertResult{}, nil
}

// TestEnqueuer は、管理者が有効にしていれば、見分け終えたとき、つないだとき、名前を付け直したときにジョブを投入し、
// 無効なら投入しないことを確かめる。
func TestEnqueuer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled *bool
		want    int
	}{
		{name: "設定の行がなければ、既定の無効として投入しない", enabled: nil, want: 0},
		{name: "無効なら投入しない", enabled: new(false), want: 0},
		{name: "有効なら、どの契機でも投入する", enabled: new(true), want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newChapterFixture(t)
			if tt.enabled != nil {
				f.enable(t, *tt.enabled)
			}
			recordingID := f.recording(t, time.Minute, `[]`)
			ins := &inserter{}
			e := tvtplay.NewEnqueuer(ins)
			require.NoError(t, pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
				return errors.Join(e.OnDetected(t.Context(), tx, recordingID), e.OnJoined(t.Context(), tx, recordingID), e.OnRenamed(t.Context(), tx, recordingID))
			}))
			assert.Len(t, ins.args, tt.want)
			for _, args := range ins.args {
				assert.Equal(t, tvtplay.WriteArgs{RecordingID: recordingID}, args)
			}
		})
	}
}
