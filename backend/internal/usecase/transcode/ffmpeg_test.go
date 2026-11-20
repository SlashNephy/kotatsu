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
// 60 コマで出すかは、画質ではなく、出すコマ数で選ぶ (#1509)。
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
// bitrate720p は、720p の 30 コマの画質の映像のビットレート。
const (
	hwaccel     = "-hwaccel"
	x264Preset  = "medium"
	bitrate720p = "3500k"
)

// TestFFmpegArgs は、エンコーダーの設定ごとに、映像のエンコーダーと、MP4 の書き方を確かめる (#1253)。
// フィールドごとに 1 コマにするときは、bwdif をフィールド単位にして 59.94 コマに固定し、ビットレートを 1.4 倍にする (#1509)。
func TestFFmpegArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoder string
		preset  string
		fields  bool
		want    []string
		notWant []string
		wantVF  string
	}{
		{name: "x264", encoder: "", preset: preset720p, want: []string{"libx264", x264Preset, bitrate720p}, notWant: []string{hwaccel, "hvc1"}, wantVF: "bwdif=0:-1:1,scale=-2:720,format=yuv420p"},
		{name: "x265", encoder: "", preset: "1080p-hevc", want: []string{"libx265", "hvc1"}, notWant: []string{hwaccel}, wantVF: "bwdif=0:-1:1,scale=-2:1080,format=yuv420p"},
		{name: "nvenc", encoder: config.EncoderNVENC, preset: "540p", want: []string{"h264_nvenc", hwaccel, "cuda"}, notWant: []string{x264Preset}, wantVF: "bwdif=0:-1:1,scale=-2:540,format=yuv420p"},
		// VAAPI では、復号から縮小までを GPU でし、bwdif の代わりに deinterlace_vaapi で解除する (#1970)。
		// QSV でも、復号から縮小までは VAAPI でし、hwmap で QSV のフレームにしてエンコーダーに渡す (#1999)。
		{name: "vaapi", encoder: config.EncoderVAAPI, preset: "360p", want: []string{"h264_vaapi", "vaapi=va:/dev/dri/renderD128", hwaccel}, notWant: []string{x264Preset}, wantVF: "format=nv12|vaapi,hwupload,deinterlace_vaapi=rate=frame:auto=1,scale_vaapi=w=-2:h=360:format=nv12"},
		{name: "vaapi fields", encoder: config.EncoderVAAPI, preset: preset720p, fields: true, want: []string{"h264_vaapi", "4900k"}, notWant: []string{bitrate720p}, wantVF: "format=nv12|vaapi,hwupload,deinterlace_vaapi=rate=field:auto=1,fps=60000/1001,scale_vaapi=w=-2:h=720:format=nv12"},
		{name: "x264 fields", encoder: "", preset: preset720p, fields: true, want: []string{"libx264", "4900k"}, notWant: []string{bitrate720p}, wantVF: "bwdif=1:-1:1,fps=60000/1001,scale=-2:720,format=yuv420p"},
		{name: "qsv fields", encoder: config.EncoderQSV, preset: "1080p-hevc", fields: true, want: []string{"hevc_qsv", "5600k", "vaapi=va:/dev/dri/renderD128", hwaccel}, notWant: []string{x264Preset}, wantVF: "format=nv12|vaapi,hwupload,deinterlace_vaapi=rate=field:auto=1,fps=60000/1001,scale_vaapi=w=-2:h=1080:format=nv12,hwmap=derive_device=qsv,format=qsv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			preset, ok := kyusu.PresetByName(tt.preset)
			require.True(t, ok)
			args := transcode.FFmpegArgs(&config.Kyusu{Encoder: tt.encoder, VAAPIDevice: "/dev/dri/renderD128"}, preset, tt.fields, "/pool/a.720p.mp4")
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
