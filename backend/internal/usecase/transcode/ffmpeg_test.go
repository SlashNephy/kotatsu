package transcode_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// TestTranscodePresets は、選べる画質が、kyusu のプリセットにあり、映像を変換して 30fps で書くものであることを確かめる (#1253)。
func TestTranscodePresets(t *testing.T) {
	t.Parallel()

	for _, name := range domain.TranscodePresets() {
		preset, ok := kyusu.PresetByName(name)
		require.True(t, ok, name)
		assert.False(t, preset.Passthrough, name)
		assert.False(t, preset.Fields, name)
	}
}

// hwaccel は、GPU で復号させる FFmpeg の引数。x264Preset は、CPU でエンコードするときの速さの設定。
const (
	hwaccel    = "-hwaccel"
	x264Preset = "medium"
)

// TestFFmpegArgs は、エンコーダーの設定ごとに、映像のエンコーダーと、MP4 の書き方を確かめる (#1253)。
func TestFFmpegArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoder string
		preset  string
		want    []string
		notWant []string
		wantVF  string
	}{
		{name: "x264", encoder: "", preset: "720p", want: []string{"libx264", x264Preset}, notWant: []string{hwaccel, "hvc1"}, wantVF: "bwdif=0:-1:1,scale=-2:720,format=yuv420p"},
		{name: "x265", encoder: "", preset: "1080p-hevc", want: []string{"libx265", "hvc1"}, notWant: []string{hwaccel}, wantVF: "bwdif=0:-1:1,scale=-2:1080,format=yuv420p"},
		{name: "nvenc", encoder: config.EncoderNVENC, preset: "540p", want: []string{"h264_nvenc", hwaccel, "cuda"}, notWant: []string{x264Preset}, wantVF: "bwdif=0:-1:1,scale=-2:540,format=yuv420p"},
		{name: "vaapi", encoder: config.EncoderVAAPI, preset: "360p", want: []string{"h264_vaapi", "/dev/dri/renderD128"}, notWant: []string{x264Preset}, wantVF: "bwdif=0:-1:1,format=nv12,hwupload,scale_vaapi=w=-2:h=360"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			preset, ok := kyusu.PresetByName(tt.preset)
			require.True(t, ok)
			args := transcode.FFmpegArgs(&config.Kyusu{Encoder: tt.encoder, VAAPIDevice: "/dev/dri/renderD128"}, preset, "/pool/a.720p.mp4")
			for _, want := range tt.want {
				assert.Contains(t, args, want)
			}
			for _, notWant := range tt.notWant {
				assert.NotContains(t, args, notWant)
			}
			vf := slices.Index(args, "-vf")
			require.GreaterOrEqual(t, vf, 0)
			assert.Equal(t, tt.wantVF, args[vf+1])
			// 標準入力の TS を読み、索引を先頭に置いた MP4 を、最後の引数のパスに書く。
			assert.Equal(t, "pipe:0", args[slices.Index(args, "-i")+1])
			assert.Contains(t, strings.Join(args, " "), "-movflags +faststart -f mp4 /pool/a.720p.mp4")
		})
	}
}
