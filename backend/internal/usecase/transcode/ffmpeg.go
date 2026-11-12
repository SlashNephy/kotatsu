// Package transcode は、録画を終えた後に、録画を変換して MP4 を足す (#1253、設計書 6.1)。
// 元の TS は残し、変換した MP4 は、録画の「その他のファイル」として記録する。
package transcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

const (
	// ffmpegCommand は、変換に使う FFmpeg のコマンド。
	ffmpegCommand = "ffmpeg"
	// audioBitrate は、音声のビットレート (kbps)。ファイルに残すので、ライブ視聴 (kyusu) より高くする。
	audioBitrate = 192
	// stderrLimit は、失敗の説明に添える FFmpeg の標準エラー出力の長さ。
	stderrLimit = 4096
	// waitDelay は、FFmpeg を止めてから、標準出力を読み終えるまで待つ時間の上限。FFmpeg が作った子プロセスが出力を握ったままでも、
	// ジョブを終えられるようにする。
	waitDelay = 10 * time.Second
)

// ErrFFmpegNotFound は、FFmpeg が見つからないことを表す。
var ErrFFmpegNotFound = errors.New("FFmpeg is not found")

// Encoder は、TS を読んで MP4 を書く。
type Encoder interface {
	// Encode は、src の TS を、preset の画質の MP4 にして out に書く。progress には、FFmpeg の -progress の出力を書く。
	// fields なら、インターレースをフィールドごとに 1 コマにして、60 コマで出す (#1509)。
	Encode(ctx context.Context, src io.Reader, preset *kyusu.Preset, fields bool, out string, progress io.Writer) error
	// ProbeFrameRate は、TS のファイル path の最初の映像のフレームレートを読む。読めなければ 0 を返す。
	// 録画の映像の形式に、フレームレートがないとき (HEVC) に、進み具合の見込みのコマ数を求めるのに使う (#1511)。
	ProbeFrameRate(ctx context.Context, path string) (float64, error)
	// MeasureFields は、TS のファイル path の、offset から length の間の、フィールドどうしの輝度の差を測る (#1509)。
	// 出すコマ数を自動で選ぶときに、映像が 24p 系、30p、60i のどれかを見分けるのに使う。
	MeasureFields(ctx context.Context, path string, offset, length time.Duration) (*FieldDiffs, error)
}

// FFmpegEncoder は、FFmpeg で変換する。映像のエンコーダーは、kyusu と同じ設定 (config.Kyusu の encoder) を使う。
// 録画の後処理なので、OS の最低の優先度で動かし、ライブ視聴の変換 (kyusu) に CPU を譲る (#1511)。
type FFmpegEncoder struct {
	cfg     *config.Kyusu
	lowerer *procprio.Lowerer
}

var _ Encoder = new(FFmpegEncoder)

// NewFFmpegEncoder は FFmpegEncoder を返す。FFmpeg がなければ ErrFFmpegNotFound を返す。
// ffprobe は、進み具合の見込みにしか使わないので、なくても変換する。
// lowerer は、FFmpeg を最低の優先度で動かす。nil なら、優先度を変えない。
func NewFFmpegEncoder(cfg *config.Kyusu, lowerer *procprio.Lowerer) (*FFmpegEncoder, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &FFmpegEncoder{cfg: cfg, lowerer: lowerer}, nil
}

// Encode は、src の TS を、preset の画質の MP4 にして out に書く。-progress の出力は、標準出力から progress に書く。
func (e *FFmpegEncoder) Encode(ctx context.Context, src io.Reader, preset *kyusu.Preset, fields bool, out string, progress io.Writer) error {
	// コマンドは定数で、引数は設定のエンコーダーと固定の表の画質、録画のセグメントから作った出力のパスから組み立てる。
	cmd := exec.CommandContext(ctx, ffmpegCommand, FFmpegArgs(e.cfg, preset, fields, out)...) //nolint:gosec // G204 の誤検知。理由は上のとおり。
	cmd.Stdin = src
	cmd.Stdout = progress
	cmd.WaitDelay = waitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: stderrLimit}
	if err := e.lowerer.Run(cmd); err != nil {
		return fmt.Errorf("failed to transcode the recording: %w: %s", err, stderr.String())
	}

	return nil
}

func (e *FFmpegEncoder) ProbeFrameRate(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, ffprobeCommand, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=avg_frame_rate,r_frame_rate", "-of", "json", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: stderrLimit}
	if err := e.lowerer.Run(cmd); err != nil {
		return 0, fmt.Errorf("failed to probe the frame rate: %w: %s", err, stderr.String())
	}
	var probed struct {
		Streams []struct {
			AvgFrameRate string `json:"avg_frame_rate"`
			RFrameRate   string `json:"r_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probed); err != nil {
		return 0, fmt.Errorf("failed to decode the ffprobe output: %w", err)
	}
	if len(probed.Streams) == 0 {
		return 0, nil
	}
	if rate := parseRate(probed.Streams[0].AvgFrameRate); rate > 0 {
		return rate, nil
	}

	return parseRate(probed.Streams[0].RFrameRate), nil
}

// parseRate は、ffprobe が出す「30000/1001」の形のフレームレートを読む。読めないか 0/0 なら 0 を返す。
func parseRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d == 0 {
		return 0
	}

	return n / d
}

// FFmpegArgs は、標準入力の TS を、preset の画質の MP4 にして out に書く FFmpeg の引数を返す。
// 映像は、kyusu と同じく、CPU の bwdif でインターレースを解除してから縮め、8 ビットにする (設計書 7.3)。
// bwdif は、インターレースの印のあるコマだけを解除する (deint=interlaced)。fields なら、フィールドごとに 1 コマにし (send_field)、
// kyusu の 60fps の画質と同じく 59.94 コマに固定して、ビットレートを 1.4 倍にする (#1509)。そうでなければ、コマごとに解除する (send_frame)。
// 音声は、最初の音声を AAC にする。MP4 は、先頭に索引を置き (faststart)、読み込み終える前から再生できるようにする。
func FFmpegArgs(cfg *config.Kyusu, preset *kyusu.Preset, fields bool, out string) []string {
	height := strconv.Itoa(preset.Height)
	deinterlace := "bwdif=0:-1:1"
	bitrate := preset.VideoBitrate
	if fields {
		deinterlace = "bwdif=1:-1:1,fps=60000/1001"
		bitrate = bitrate * fieldBitrateNumerator / fieldBitrateDenominator
	}
	filter := deinterlace + ",scale=-2:" + height + ",format=yuv420p"
	device := []string{}
	options := []string{}
	var encoder string
	switch cfg.Encoder {
	case config.EncoderVAAPI:
		device = []string{"-vaapi_device", cfg.VAAPIDevice}
		filter = deinterlace + ",format=nv12,hwupload,scale_vaapi=w=-2:h=" + height
		encoder = map[kyusu.Codec]string{kyusu.CodecH264: "h264_vaapi", kyusu.CodecHEVC: "hevc_vaapi"}[preset.Codec]
	case config.EncoderQSV:
		device = []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
		filter = deinterlace + ",format=nv12,hwupload=extra_hw_frames=64,scale_qsv=w=-1:h=" + height
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
	args := make([]string, 0, 48)
	// 進み具合は、-progress のまとまりを標準出力に出させて読む (#1511)。標準エラー出力の統計の行は出さない。
	args = append(args, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-nostats", "-progress", "pipe:1")
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
