// Package transcode は、録画を終えた後に、録画を変換して MP4 を足す (#1253、設計書 6.1)。
// 元の TS は残し、変換した MP4 は、録画の「その他のファイル」として記録する。
package transcode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

const (
	// ffmpegCommand は、変換に使う FFmpeg のコマンド。
	ffmpegCommand = "ffmpeg"
	// audioBitrate は、音声のビットレート (kbps)。ファイルに残すので、ライブ視聴 (kyusu) より高くする。
	audioBitrate = 192
	// stderrLimit は、失敗の説明に添える FFmpeg の標準エラー出力の長さ。
	stderrLimit = 4096
)

// ErrFFmpegNotFound は、FFmpeg が見つからないことを表す。
var ErrFFmpegNotFound = errors.New("FFmpeg is not found")

// Encoder は、TS を読んで MP4 を書く。
type Encoder interface {
	Encode(ctx context.Context, src io.Reader, preset *kyusu.Preset, out string) error
}

// FFmpegEncoder は、FFmpeg で変換する。映像のエンコーダーは、kyusu と同じ設定 (config.Kyusu の encoder) を使う。
type FFmpegEncoder struct {
	cfg *config.Kyusu
}

var _ Encoder = new(FFmpegEncoder)

// NewFFmpegEncoder は FFmpegEncoder を返す。FFmpeg がなければ ErrFFmpegNotFound を返す。
func NewFFmpegEncoder(cfg *config.Kyusu) (*FFmpegEncoder, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &FFmpegEncoder{cfg: cfg}, nil
}

// Encode は、src の TS を、preset の画質の MP4 にして out に書く。
func (e *FFmpegEncoder) Encode(ctx context.Context, src io.Reader, preset *kyusu.Preset, out string) error {
	// コマンドは定数で、引数は設定のエンコーダーと固定の表の画質、録画のセグメントから作った出力のパスから組み立てる。
	cmd := exec.CommandContext(ctx, ffmpegCommand, FFmpegArgs(e.cfg, preset, out)...) //nolint:gosec // G204 の誤検知。理由は上のとおり。
	cmd.Stdin = src
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: stderrLimit}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to transcode the recording: %w: %s", err, stderr.String())
	}

	return nil
}

// FFmpegArgs は、標準入力の TS を、preset の画質の MP4 にして out に書く FFmpeg の引数を返す。
// 映像は、kyusu と同じく、CPU の yadif でインターレースを解除してから縮め、8 ビットにする (設計書 7.3)。
// 音声は、最初の音声を AAC にする。MP4 は、先頭に索引を置き (faststart)、読み込み終える前から再生できるようにする。
func FFmpegArgs(cfg *config.Kyusu, preset *kyusu.Preset, out string) []string {
	height := strconv.Itoa(preset.Height)
	yadif := "yadif=0:-1:1"
	filter := yadif + ",scale=-2:" + height + ",format=yuv420p"
	device := []string{}
	options := []string{}
	var encoder string
	switch cfg.Encoder {
	case config.EncoderVAAPI:
		device = []string{"-vaapi_device", cfg.VAAPIDevice}
		filter = yadif + ",format=nv12,hwupload,scale_vaapi=w=-2:h=" + height
		encoder = map[kyusu.Codec]string{kyusu.CodecH264: "h264_vaapi", kyusu.CodecHEVC: "hevc_vaapi"}[preset.Codec]
	case config.EncoderQSV:
		device = []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
		filter = yadif + ",format=nv12,hwupload=extra_hw_frames=64,scale_qsv=w=-1:h=" + height
		encoder = map[kyusu.Codec]string{kyusu.CodecH264: "h264_qsv", kyusu.CodecHEVC: "hevc_qsv"}[preset.Codec]
	case config.EncoderNVENC:
		device = []string{"-hwaccel", "cuda"}
		encoder = map[kyusu.Codec]string{kyusu.CodecH264: "h264_nvenc", kyusu.CodecHEVC: "hevc_nvenc"}[preset.Codec]
	default:
		// ファイルに残すので、ライブ視聴 (veryfast) より、速さより大きさを優先する。
		encoder = map[kyusu.Codec]string{kyusu.CodecH264: "libx264", kyusu.CodecHEVC: "libx265"}[preset.Codec]
		options = []string{"-preset", "medium"}
	}
	if preset.Codec == kyusu.CodecHEVC {
		// Safari は、hvc1 の印のない HEVC の MP4 を再生しない。
		options = append(options, "-tag:v", "hvc1")
	}
	bitrate := preset.VideoBitrate

	args := make([]string, 0, 48)
	args = append(args, "-hide_banner", "-loglevel", "error", "-nostdin", "-y")
	args = append(args, device...)
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0?")
	args = append(args, "-vf", filter, "-c:v", encoder)
	args = append(args, options...)
	args = append(args,
		"-b:v", strconv.Itoa(bitrate)+"k", "-maxrate", strconv.Itoa(bitrate*3/2)+"k", "-bufsize", strconv.Itoa(bitrate*2)+"k",
		"-c:a", "aac", "-b:a", strconv.Itoa(audioBitrate)+"k", "-ac", "2", "-ar", "48000",
		"-movflags", "+faststart", "-f", "mp4", out,
	)

	return args
}

// limitedWriter は、書かれたものの先頭 limit バイトだけを残す。
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}

	return len(p), nil
}
