package transcode_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

const (
	// stepTranscode と stepDetect は、TestChaptersOrder の段。変換を終えるか、CM の区間を見分け終える。
	stepTranscode = "transcode"
	stepDetect    = "detect"
	// chaptersKind は、チャプターを書くジョブの種類。
	chaptersKind = "recording_chapters"
	// output360p は、segmentName を 360p で変換した MP4 の名前。
	output360p = "anime/foo.360p.mp4"
	// fakeDuration は、fakeChapterer が返す MP4 の長さ。
	fakeDuration = 30 * time.Minute
)

// fakeChapterer は、MP4 の中身を、書いた ffmetadata の中身にする Chapterer。チャプターの数は、中身の [CHAPTER] の数。
type fakeChapterer struct {
	// metadata は、Rewrite に渡された ffmetadata の中身。チャプターを外すときは空。
	metadata []string
	// failOn は、何回目 (1 から) の Rewrite を失敗させるか。0 なら失敗させない。
	failOn int
	// cancel は、Rewrite の間に、ジョブを中止させる。
	cancel context.CancelFunc
}

var _ transcode.Chapterer = new(fakeChapterer)

var errRewrite = errors.New("rewrite failed")

func (*fakeChapterer) Probe(_ context.Context, in string) (*transcode.MP4Info, error) {
	content, err := os.ReadFile(in)
	if err != nil {
		return nil, err
	}

	return &transcode.MP4Info{VideoStart: 0, Duration: fakeDuration, Chapters: strings.Count(string(content), "[CHAPTER]")}, nil
}

func (c *fakeChapterer) Rewrite(ctx context.Context, _, metadata, out string) error {
	content := ""
	if metadata != "" {
		b, err := os.ReadFile(metadata)
		if err != nil {
			return err
		}
		content = string(b)
	}
	c.metadata = append(c.metadata, content)
	// 書きかけのファイルを残してから失敗する。
	if err := os.WriteFile(out, []byte("partial"), 0o600); err != nil {
		return err
	}
	if c.cancel != nil {
		c.cancel()

		return ctx.Err()
	}
	if len(c.metadata) == c.failOn {
		return errRewrite
	}
	if metadata == "" {
		return os.WriteFile(out, []byte{}, 0o600)
	}

	return copyFile(metadata, out)
}

// copyFile は、src のファイルを dst に写す。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}

func runChapters(ctx context.Context, f *fixture, chapterer transcode.Chapterer, recordingID uuid.UUID) error {
	worker := transcode.NewChaptersWorker(f.pool, f.store, f.clock, func() (transcode.Chapterer, error) { return chapterer, nil })

	return worker.Work(ctx, &river.Job[transcode.ChaptersArgs]{JobRow: new(rivertype.JobRow), Args: transcode.ChaptersArgs{RecordingID: recordingID}})
}

// transcoded は、録画 recordingID の、name の変換した MP4 (中身は content) を、その他のファイルとして置く。
func (f *fixture) transcoded(t *testing.T, recordingID uuid.UUID, name string, content []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), content, 0o600))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		SELECT $1, $2, 'other', $3, $4, $5, recorded_from, recorded_until, recorded_from FROM recordings WHERE id = $2`,
		id.New(), recordingID, poolID, name, len(content))
	require.NoError(t, err)
}

// detect は、CM の区間を見分けるジョブが書くのと同じく、録画の CM の区間 (FFmpegDetector のもの) を sections に置き換え、
// 同じトランザクションの中で、見分け終えたことを ChaptersEnqueuer に渡す。
func (f *fixture) detect(t *testing.T, recordingID uuid.UUID, sections ...cmdetect.Section) {
	t.Helper()

	require.NoError(t, pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
			VALUES ($1, $2, 1, $3) ON CONFLICT (recording_id, detector) DO UPDATE SET detected_at = EXCLUDED.detected_at`,
			recordingID, cmdetect.FFmpegDetectorName, f.clock.Now())
		if err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), "DELETE FROM recording_cm_sections WHERE recording_id = $1", recordingID); err != nil {
			return err
		}
		for _, s := range sections {
			_, err := tx.Exec(t.Context(), `INSERT INTO recording_cm_sections (recording_id, detector, start_offset_ms, end_offset_ms)
				VALUES ($1, $2, $3, $4)`, recordingID, cmdetect.FFmpegDetectorName, s.Start.Milliseconds(), s.End.Milliseconds())
			if err != nil {
				return err
			}
		}

		return transcode.NewChaptersEnqueuer(f.inserter).OnDetected(t.Context(), tx, recordingID)
	}))
}

// chaptersJobs は、投入されたチャプターを書くジョブの、キューの並び。
func (f *fixture) chaptersJobs(t *testing.T) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT queue FROM river_job WHERE kind = $1 ORDER BY id", chaptersKind)
	require.NoError(t, err)
	queues, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return queues
}

// leftovers は、dir に残った一時ファイル (.part と .ffmetadata) の名前を返す。
func leftovers(t *testing.T, dir string) []string {
	t.Helper()

	names := []string{}
	for _, pattern := range []string{"*.part", "*.ffmetadata"} {
		matches, err := filepath.Glob(filepath.Join(dir, "anime", pattern))
		require.NoError(t, err)
		names = append(names, matches...)
	}

	return names
}

func cmSection(start, end time.Duration) cmdetect.Section {
	return cmdetect.Section{Start: start, End: end}
}

// wantMetadata は、録画の終わり (fakeDuration) までの、CM の区間 cms のチャプターの ffmetadata。
func wantMetadata(cms ...cmdetect.Section) string {
	var b strings.Builder
	b.WriteString(";FFMETADATA1\n")
	chapter := func(start, end time.Duration, title string) {
		b.WriteString("[CHAPTER]\nTIMEBASE=1/1000\nSTART=" + strconv.FormatInt(start.Milliseconds(), 10) +
			"\nEND=" + strconv.FormatInt(end.Milliseconds(), 10) + "\ntitle=" + title + "\n")
	}
	var cursor time.Duration
	for _, cm := range cms {
		chapter(cursor, cm.Start, "本編")
		chapter(cm.Start, cm.End, "CM")
		cursor = cm.End
	}
	chapter(cursor, fakeDuration, "本編")

	return b.String()
}

// TestChaptersWorkerRedetect は、CM の区間を見分け直すたびに、変換した MP4 のチャプターを書き直し、
// CM が見つからなくなったら外すことを確かめる (#1510)。外すチャプターもなければ、写し直さない。
func TestChaptersWorkerRedetect(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, tsPackets(10))
	f.transcoded(t, recordingID, outputName, []byte("mp4"))
	chapterer := new(fakeChapterer)
	first := []cmdetect.Section{cmSection(5*time.Minute, 6*time.Minute), cmSection(20*time.Minute, 21*time.Minute)}
	second := []cmdetect.Section{cmSection(10*time.Minute, 12*time.Minute)}

	tests := []struct {
		name     string
		sections []cmdetect.Section
		// want は、MP4 の中身 (fakeChapterer が書いた ffmetadata)。wantRewrites は、それまでに写し直した回数。
		want         string
		wantRewrites int
	}{
		{name: "detected", sections: first, want: wantMetadata(first...), wantRewrites: 1},
		{name: "redetected", sections: second, want: wantMetadata(second...), wantRewrites: 2},
		{name: "no CM", sections: []cmdetect.Section{}, want: "", wantRewrites: 3},
		{name: "still no CM", sections: []cmdetect.Section{}, want: "", wantRewrites: 3},
	}
	for _, tt := range tests {
		f.detect(t, recordingID, tt.sections...)
		require.NoError(t, runChapters(t.Context(), f, chapterer, recordingID), tt.name)
		written, err := os.ReadFile(filepath.Join(f.dir, outputName))
		require.NoError(t, err)
		assert.Equal(t, tt.want, string(written), tt.name)
		assert.Len(t, chapterer.metadata, tt.wantRewrites, tt.name)
		// 書き直した大きさを記録し直す。
		assert.Equal(t, []otherFile{{path: outputName, size: int64(len(written))}}, f.otherFiles(t, recordingID), tt.name)
		assert.Empty(t, leftovers(t, f.dir), tt.name)
	}
}

// TestChaptersOrder は、変換と CM の区間の見分けのどちらが先に終わっても、チャプターを書くジョブを 1 つだけ入れ、
// 1 回だけ正しく書くことを確かめる (#1510)。ジョブは media のキューに入れる。
func TestChaptersOrder(t *testing.T) {
	t.Parallel()

	sections := []cmdetect.Section{cmSection(5*time.Minute, 6*time.Minute)}
	tests := []struct {
		name  string
		steps []string
		// wantAfter は、それぞれの段の後に投入されているジョブの数。
		wantAfter []int
	}{
		{name: "transcoded first", steps: []string{stepTranscode, stepDetect}, wantAfter: []int{0, 1}},
		{name: "detected first", steps: []string{stepDetect, stepTranscode}, wantAfter: []int{0, 1}},
		{name: "redetected while waiting", steps: []string{stepDetect, stepTranscode, stepDetect, stepTranscode}, wantAfter: []int{0, 1, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tsPackets(10))
			for i, step := range tt.steps {
				switch step {
				case stepTranscode:
					require.NoError(t, work(t, f, new(copyEncoder), recordingID, preset720p))
				case stepDetect:
					f.detect(t, recordingID, sections...)
				}
				assert.Len(t, f.chaptersJobs(t), tt.wantAfter[i], "after %s", step)
			}
			assert.Equal(t, []string{jobs.QueueMedia}, f.chaptersJobs(t))

			chapterer := new(fakeChapterer)
			require.NoError(t, runChapters(t.Context(), f, chapterer, recordingID))
			written, err := os.ReadFile(filepath.Join(f.dir, outputName))
			require.NoError(t, err)
			assert.Equal(t, wantMetadata(sections...), string(written))
			assert.Len(t, chapterer.metadata, 1)
		})
	}
}

// TestChaptersWorkerFails は、書き直せないか中止されたら、一時ファイルを消し、元の MP4 をどれも置き換えないことを確かめる (#1510)。
// 複数の画質の MP4 があれば、全部を書き終えるまで、どれも置き換えない。
func TestChaptersWorkerFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		failOn  int
		cancel  bool
		wantErr error
	}{
		{name: "first file fails", failOn: 1, wantErr: errRewrite},
		{name: "second file fails", failOn: 2, wantErr: errRewrite},
		{name: "canceled", cancel: true, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tsPackets(10))
			originals := map[string]string{outputName: "mp4 720p", output360p: "mp4 360p"}
			for name, content := range originals {
				f.transcoded(t, recordingID, name, []byte(content))
			}
			f.detect(t, recordingID, cmSection(5*time.Minute, 6*time.Minute))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			chapterer := &fakeChapterer{metadata: nil, failOn: tt.failOn, cancel: nil}
			if tt.cancel {
				chapterer.cancel = cancel
			}

			require.ErrorIs(t, runChapters(ctx, f, chapterer, recordingID), tt.wantErr)
			for name, content := range originals {
				written, err := os.ReadFile(filepath.Join(f.dir, name))
				require.NoError(t, err)
				assert.Equal(t, content, string(written), name)
			}
			assert.Empty(t, leftovers(t, f.dir))
		})
	}
}

// TestChaptersWorkerSkips は、Kotatsu が変換した MP4 のない録画では、何も写さずに終えることを確かめる (#1510)。
// EPGStation から取り込んだ MP4 のように、変換した名前でないその他のファイルは書き換えない。
func TestChaptersWorkerSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		other string
	}{
		{name: "no other file", other: ""},
		{name: "imported MP4", other: "anime/foo.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tsPackets(10))
			if tt.other != "" {
				f.transcoded(t, recordingID, tt.other, []byte("imported"))
			}
			f.detect(t, recordingID, cmSection(5*time.Minute, 6*time.Minute))
			assert.Empty(t, f.chaptersJobs(t), "nothing to rewrite")

			chapterer := new(fakeChapterer)
			require.NoError(t, runChapters(t.Context(), f, chapterer, recordingID))
			assert.Empty(t, chapterer.metadata)
			if tt.other != "" {
				written, err := os.ReadFile(filepath.Join(f.dir, tt.other))
				require.NoError(t, err)
				assert.Equal(t, "imported", string(written))
			}
		})
	}
}

// ffprobeFirstPTS は、ffprobe で、TS の最初の映像のパケットと、最初の I フレームの PTS (90 kHz) を読む。
func ffprobeFirstPTS(t *testing.T, path string) (int64, int64) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts,flags", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	first, key := int64(-1), int64(-1)
	for line := range strings.Lines(string(out)) {
		pts, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		value, err := strconv.ParseInt(pts, 10, 64)
		if err != nil {
			continue
		}
		if first < 0 {
			first = value
		}
		if strings.HasPrefix(flags, "K") {
			key = value

			break
		}
	}
	require.GreaterOrEqual(t, first, int64(0))
	require.GreaterOrEqual(t, key, int64(0))

	return first, key
}

// TestChaptersEndToEnd は、本物の FFmpeg で、GOP の途中から録り始めた録画を変換し、チャプターを書いて、
// チャプターの時刻が、変換で捨てた先頭と、MP4 で映像が音声より遅れて始まる分を詰めたものになることを確かめる (#1510)。
// FFmpeg のない環境 (CI など) では飛ばす。
func TestChaptersEndToEnd(t *testing.T) {
	t.Parallel()

	chapterer := newChapterer(t)
	f := newFixture(t)
	// セグメントの中身は、後で合成した TS の途中からに替える。
	recordingID := f.recording(t, tsPackets(1))
	var segmentID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM recording_files WHERE recording_id = $1 AND kind = 'segment'", recordingID).Scan(&segmentID))
	// 最初の GOP の途中のパケットから録り始めたことにする。最初の I フレームまでの映像は、変換が捨てる。
	src, err := os.Open(testTS)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)
	offset := info.Size() / 12 / ts.PacketSize * ts.PacketSize
	segment, err := os.Create(filepath.Join(f.dir, segmentName))
	require.NoError(t, err)
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	// 録画と同じく、書きながら解析して索引を作る。
	_, err = io.Copy(io.MultiWriter(segment, ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})), io.NewSectionReader(src, offset, info.Size()-offset))
	require.NoError(t, err)
	require.NoError(t, segment.Close())
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET size = $1 WHERE id = $2", info.Size()-offset, segmentID)
	require.NoError(t, err)
	location := kago.IndexLocation(segmentID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dataDir, filepath.Dir(location.Path)), 0o750))
	indexFile, err := os.Create(filepath.Join(f.dataDir, location.Path))
	require.NoError(t, err)
	_, err = io.Copy(indexFile, &index)
	require.NoError(t, err)
	require.NoError(t, indexFile.Close())
	cm := cmSection(2*time.Second, 4*time.Second)
	f.detect(t, recordingID, cm)

	encoder, err := transcode.NewFFmpegEncoder(&config.Kyusu{Encoder: "", VAAPIDevice: "", WorkDir: ""}, nil)
	require.NoError(t, err)
	require.NoError(t, work(t, f, encoder, recordingID, "360p"))
	require.Len(t, f.chaptersJobs(t), 1)
	require.NoError(t, runChapters(t.Context(), f, chapterer, recordingID))

	// CM の区間は、ファイルの最初の映像からの時間。MP4 は、最初の I フレームを、映像の始まり (音声の始まりより後) に置く。
	firstVideo, firstKey := ffprobeFirstPTS(t, filepath.Join(f.dir, segmentName))
	require.Greater(t, firstKey, firstVideo, "the recording starts in the middle of a GOP")
	mp4 := probe(t, filepath.Join(f.dir, output360p))
	var videoStart int64
	for _, s := range mp4.Streams {
		if s.CodecType == "video" {
			videoStart = millis(t, s.StartTime)
		}
	}
	require.Positive(t, videoStart, "the audio starts before the video")
	skipped := (firstKey - firstVideo) * 1000 / ts.PTSClock
	start, end := cm.Start.Milliseconds()-skipped+videoStart, cm.End.Milliseconds()-skipped+videoStart
	duration := millis(t, mp4.Format.Duration)
	got := mp4.chapters(t)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"本編", "CM", "本編"}, []string{got[0].title, got[1].title, got[2].title})
	// ミリ秒に切り捨てて書くので、1 ミリ秒までずれてよい。
	assert.Zero(t, got[0].start)
	assert.InDelta(t, start, got[0].end, 1)
	assert.InDelta(t, start, got[1].start, 1)
	assert.InDelta(t, end, got[1].end, 1)
	assert.InDelta(t, end, got[2].start, 1)
	assert.InDelta(t, duration, got[2].end, 1)
	t.Logf("skipped %d ms, video starts at %d ms, CM chapter [%d, %d] ms", skipped, videoStart, got[1].start, got[1].end)
}
