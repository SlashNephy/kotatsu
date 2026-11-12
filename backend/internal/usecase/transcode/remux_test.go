package transcode_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// testTS は、合成した 720×480 の約 6 秒の TS (サービス 1024)。I フレームは 0.5 秒ごとにあり、音声は映像より少し先に始まる。
const testTS = "../../../testdata/ts/test.ts"

// newChapterer は、本物の FFmpeg を使う Chapterer を返す。FFmpeg か ffprobe のない環境 (CI など) では飛ばす。
func newChapterer(t *testing.T) *transcode.FFmpegChapterer {
	t.Helper()

	chapterer, err := transcode.NewFFmpegChapterer(nil)
	if errors.Is(err, transcode.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	return chapterer
}

// encodeMP4 は、src の TS を、本物の FFmpeg で 360p の MP4 に変換して、dir に書く。
func encodeMP4(t *testing.T, src, dir string) string {
	t.Helper()

	encoder, err := transcode.NewFFmpegEncoder(&config.Kyusu{Encoder: "", VAAPIDevice: "", WorkDir: ""}, nil)
	require.NoError(t, err)
	preset, ok := kyusu.PresetByName("360p")
	require.True(t, ok)
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out := filepath.Join(dir, "test.360p.mp4")
	require.NoError(t, encoder.Encode(t.Context(), in, preset, out))

	return out
}

// probed は、ffprobe が報告する MP4 のストリームとチャプター。
type probed struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		StartTime string `json:"start_time"`
	} `json:"streams"`
	Chapters []struct {
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// probe は、ffprobe -show_chapters で、MP4 のストリームとチャプターを読む。
func probe(t *testing.T, path string) *probed {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_streams", "-show_chapters", "-show_format", "-of", "json", path).Output()
	require.NoError(t, err)
	var p probed
	require.NoError(t, json.Unmarshal(out, &p))

	return &p
}

// probedChapter は、ffprobe が読んだチャプターの、名前と始まりと終わり (ミリ秒)。
type probedChapter struct {
	title      string
	start, end int64
}

func (p *probed) chapters(t *testing.T) []probedChapter {
	t.Helper()

	chapters := make([]probedChapter, 0, len(p.Chapters))
	for _, c := range p.Chapters {
		chapters = append(chapters, probedChapter{title: c.Tags["title"], start: millis(t, c.StartTime), end: millis(t, c.EndTime)})
	}

	return chapters
}

// millis は、ffprobe の秒の文字列を、ミリ秒に丸める。
func millis(t *testing.T, s string) int64 {
	t.Helper()

	f, err := strconv.ParseFloat(s, 64)
	require.NoError(t, err)

	return time.Duration(f * float64(time.Second)).Round(time.Millisecond).Milliseconds()
}

// TestFFmpegChapterer は、本物の FFmpeg で、変換した MP4 に ffmetadata のチャプターを写し、書き直し、外せることを確かめる (#1510)。
// 写し直しても、映像と音声のストリーム、映像の始まりの時刻、長さは変わらず、前のチャプターのトラックも残らない。
func TestFFmpegChapterer(t *testing.T) {
	t.Parallel()

	chapterer := newChapterer(t)
	dir := t.TempDir()
	original := encodeMP4(t, testTS, dir)
	before, err := chapterer.Probe(t.Context(), original)
	require.NoError(t, err)
	// 音声が映像より先に始まるので、MP4 の映像は 0 より後から始まる。
	require.Positive(t, before.VideoStart)
	assert.Zero(t, before.Chapters)
	sec := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	chapters := []transcode.Chapter{
		{Start: 0, End: sec(1.5), Title: "本編"},
		{Start: sec(1.5), End: sec(3.25), Title: "CM"},
		{Start: sec(3.25), End: before.Duration, Title: "本編"},
	}
	want := []probedChapter{
		{title: "本編", start: 0, end: 1500},
		{title: "CM", start: 1500, end: 3250},
		{title: "本編", start: 3250, end: before.Duration.Milliseconds()},
	}

	tests := []struct {
		name     string
		chapters []transcode.Chapter
		want     []probedChapter
		// wantStreams は、写した MP4 のストリームの数。チャプターを書くと、映像と音声に、チャプターのトラックが 1 つ加わる。
		wantStreams int
	}{
		{name: "write", chapters: chapters, want: want, wantStreams: 3},
		{name: "rewrite", chapters: chapters, want: want, wantStreams: 3},
		{name: "strip", chapters: []transcode.Chapter{}, want: []probedChapter{}, wantStreams: 2},
	}
	// 前の段の MP4 を写し直していく。
	in := original
	for i, tt := range tests {
		metadata := ""
		if len(tt.chapters) > 0 {
			metadata = filepath.Join(dir, strconv.Itoa(i)+".ffmetadata")
			require.NoError(t, os.WriteFile(metadata, transcode.FFMetadata(tt.chapters), 0o600))
		}
		out := filepath.Join(dir, strconv.Itoa(i)+".mp4")
		require.NoError(t, chapterer.Rewrite(t.Context(), in, metadata, out), tt.name)

		p := probe(t, out)
		assert.Equal(t, tt.want, p.chapters(t), tt.name)
		assert.Len(t, p.Streams, tt.wantStreams, tt.name)
		after, err := chapterer.Probe(t.Context(), out)
		require.NoError(t, err)
		assert.Equal(t, before.VideoStart, after.VideoStart, tt.name)
		assert.Equal(t, before.Duration, after.Duration, tt.name)
		assert.Equal(t, len(tt.want), after.Chapters, tt.name)
		in = out
	}
}
