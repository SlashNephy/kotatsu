package transcode_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// secondSegmentName は、テストの録画の 2 つ目のセグメントの名前。
const secondSegmentName = "anime/foo-1.m2ts"

// setVideo は、録画 recordingID のセグメントの映像の形式を video にする。
func (f *fixture) setVideo(t *testing.T, recordingID uuid.UUID, video *ts.VideoFormat) {
	t.Helper()

	b, err := json.Marshal(video)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET video = $2 WHERE recording_id = $1 AND kind = 'segment'", recordingID, b)
	require.NoError(t, err)
}

// addSegment は、録画 recordingID に、最初のセグメントの後に続く 30 分の 2 つ目のセグメントを足す。
func (f *fixture) addSegment(t *testing.T, recordingID uuid.UUID, content []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(f.dir, secondSegmentName), content, 0o600))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		SELECT $1, recording_id, 'segment', pool_id, $2, $3, ended_at, ended_at + interval '30 minutes', created_at
		FROM recording_files WHERE recording_id = $4 AND kind = 'segment'`,
		id.New(), secondSegmentName, len(content), recordingID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET recorded_until = recorded_until + interval '30 minutes' WHERE id = $1", recordingID)
	require.NoError(t, err)
}

// transcodeFrameRate は、録画 recordingID の変換した MP4 の記録に残したコマ数を返す。
func (f *fixture) transcodeFrameRate(t *testing.T, recordingID uuid.UUID) *int32 {
	t.Helper()

	var rate *int32
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT transcode_frame_rate FROM recording_files WHERE recording_id = $1 AND kind = 'other'", recordingID).Scan(&rate))

	return rate
}

// TestWorkerFrameRate は、出すコマ数の設定と、映像の形式、窓の測り方から、フィールドごとに 1 コマにするかを決め、
// 変換した MP4 の記録にコマ数を残すことを確かめる (#1509)。
func TestWorkerFrameRate(t *testing.T) {
	t.Parallel()

	interlaced := &ts.VideoFormat{Codec: "mpeg2", Width: 1440, Height: 1080, Interlaced: true, FrameRate: 30000.0 / 1001}
	tests := []struct {
		name      string
		frameRate string
		video     *ts.VideoFormat
		// probe は、ffprobe で読むフレームレート。diffs は、窓を測ったときの差の列 (nil なら測れない)。
		probe      float64
		diffs      *transcode.FieldDiffs
		wantFields bool
		wantRate   int32
		// wantWindows は、測った窓の数。
		wantWindows int
	}{
		{name: "fixed 30", frameRate: domain.TranscodeFrameRate30, video: interlaced, diffs: interlaced60(10), wantFields: false, wantRate: 30, wantWindows: 0},
		{name: "fixed 60", frameRate: domain.TranscodeFrameRate60, video: interlaced, diffs: telecined(0), wantFields: true, wantRate: 60, wantWindows: 0},
		{name: "auto telecine", frameRate: domain.TranscodeFrameRateAuto, video: interlaced, diffs: telecined(0), wantFields: false, wantRate: 30, wantWindows: 9},
		{name: "auto 30p", frameRate: domain.TranscodeFrameRateAuto, video: interlaced, diffs: progressive30(10), wantFields: false, wantRate: 30, wantWindows: 9},
		{name: "auto 60i", frameRate: domain.TranscodeFrameRateAuto, video: interlaced, diffs: interlaced60(10), wantFields: true, wantRate: 60, wantWindows: 9},
		// 足す前の版が入れたジョブは、既定値 (自動) で出す。
		{name: "older job", frameRate: "", video: interlaced, diffs: telecined(0), wantFields: false, wantRate: 30, wantWindows: 9},
		// 測れた窓が 1 つもなければ、60 コマにする。
		{name: "auto unmeasured", frameRate: domain.TranscodeFrameRateAuto, video: interlaced, diffs: nil, wantFields: true, wantRate: 60, wantWindows: 9},
		// 映像の形式が分からなければ、インターレースとみなして測る。
		{name: "auto without the video format", frameRate: domain.TranscodeFrameRateAuto, video: nil, diffs: telecined(0), wantFields: false, wantRate: 30, wantWindows: 9},
		// プログレッシブの映像は、測らずに元のコマ数のまま出す。HEVC は、フレームレートを ffprobe で読む。
		{
			name: "auto progressive", frameRate: domain.TranscodeFrameRateAuto,
			video: &ts.VideoFormat{Codec: "h264", Width: 1280, Height: 720, Interlaced: false, FrameRate: 60000.0 / 1001},
			diffs: interlaced60(10), wantFields: false, wantRate: 60, wantWindows: 0,
		},
		{
			name: "auto progressive HEVC", frameRate: domain.TranscodeFrameRateAuto,
			video: &ts.VideoFormat{Codec: "hevc", Width: 3840, Height: 2160, Interlaced: false, FrameRate: 0}, probe: 60000.0 / 1001,
			diffs: interlaced60(10), wantFields: false, wantRate: 60, wantWindows: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tsPackets(10))
			if tt.video != nil {
				f.setVideo(t, recordingID, tt.video)
			}
			encoder := &copyEncoder{presets: []string{}, rate: tt.probe, diffs: tt.diffs}
			args := transcode.Args{RecordingID: recordingID, Preset: preset720p, FrameRate: tt.frameRate}
			require.NoError(t, workJob(t.Context(), f, encoder, &rivertype.JobRow{ID: 1, Attempt: 1}, args))
			assert.Equal(t, []bool{tt.wantFields}, encoder.fields)
			assert.Equal(t, new(tt.wantRate), f.transcodeFrameRate(t, recordingID))
			assert.Len(t, encoder.measured, tt.wantWindows)
		})
	}
}

// TestWorkerFrameRateWindows は、窓を、セグメントを並べた録画全体に散らして置き、位置ごとに当たるセグメントのファイルを、
// その中の位置で測ることを確かめる (#1509)。セグメントの終わりをはみ出す窓は、セグメントの中に収まるよう前にずらす。
func TestWorkerFrameRateWindows(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, tsPackets(10))
	f.addSegment(t, recordingID, tsPackets(10))
	encoder := &copyEncoder{presets: []string{}, diffs: telecined(0)}
	args := transcode.Args{RecordingID: recordingID, Preset: preset720p, FrameRate: domain.TranscodeFrameRateAuto}
	require.NoError(t, workJob(t.Context(), f, encoder, &rivertype.JobRow{ID: 1, Attempt: 1}, args))

	// 60 分の録画に、20 秒の窓を 9 つ置くと、(3600 − 20) 秒を 10 等分した 358 秒ごとになる。
	first, second := filepath.Base(segmentName), filepath.Base(secondSegmentName)
	assert.Equal(t, []measurement{
		{file: first, offset: 358 * time.Second},
		{file: first, offset: 716 * time.Second},
		{file: first, offset: 1074 * time.Second},
		{file: first, offset: 1432 * time.Second},
		// 1790 秒からの窓は、30 分のセグメントの終わりをはみ出すので、1780 秒からにする。
		{file: first, offset: 1780 * time.Second},
		{file: second, offset: (2148 - 1800) * time.Second},
		{file: second, offset: (2506 - 1800) * time.Second},
		{file: second, offset: (2864 - 1800) * time.Second},
		{file: second, offset: (3222 - 1800) * time.Second},
	}, encoder.measured)
	assert.Equal(t, []bool{false}, encoder.fields)
}

// TestWorkerUnknownFrameRate は、知らないコマ数の設定のジョブを、やり直さずに取りやめることを確かめる (#1509)。
func TestWorkerUnknownFrameRate(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, tsPackets(10))
	encoder := new(copyEncoder)
	err := workJob(t.Context(), f, encoder, &rivertype.JobRow{ID: 1, Attempt: 1}, transcode.Args{RecordingID: recordingID, Preset: preset720p, FrameRate: "24"})
	require.ErrorContains(t, err, "unknown transcode frame rate")
	assert.Empty(t, encoder.presets)
}
