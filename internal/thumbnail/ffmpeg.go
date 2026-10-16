package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	// ffmpegCommand は、PATH から探す FFmpeg のコマンドの名前。FFmpeg の入ったイメージ (-ffmpeg) には入っている。
	ffmpegCommand = "ffmpeg"
	// extractTimeout は、FFmpeg で 1 枚を取り出す時間の上限。
	extractTimeout = 30 * time.Second
	// maxStderrBytes は、失敗したときに、エラーに含める FFmpeg の標準エラー出力の長さの上限。
	maxStderrBytes = 1024

	// 画素の縦横比 (SAR) を戻してから、決めた大きさに収めて縮め、余白は黒で埋める。4:3 や 3:2 の映像でも歪まない。
	thumbnailFilter = "scale=iw*sar:ih,scale=480:270:force_original_aspect_ratio=decrease,pad=480:270:(ow-iw)/2:(oh-ih)/2,setsar=1"
	tileFilter      = "scale=iw*sar:ih,scale=320:180:force_original_aspect_ratio=decrease,pad=320:180:(ow-iw)/2:(oh-ih)/2,setsar=1"
)

// ErrFFmpegNotFound は、FFmpeg が PATH に見つからないことを表す。FFmpeg の入ったイメージ (-ffmpeg) を使うか、FFmpeg を入れる。
var ErrFFmpegNotFound = errors.New("FFmpeg is not found in PATH")

// FFmpeg は、FFmpeg を子プロセスとして動かし、TS からフレームを取り出す。本体とはリンクしない (設計書 付録 B)。
type FFmpeg struct{}

var _ Extractor = new(FFmpeg)

// NewFFmpeg は、PATH にある FFmpeg を使う FFmpeg を返す。見つからなければ ErrFFmpegNotFound を返す。
func NewFFmpeg() (*FFmpeg, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &FFmpeg{}, nil
}

// Extract は、r から読む TS の最初のキーフレームを、size の大きさの JPEG にする。
// B フレームはデコードしないので、TS の途中から渡しても、すぐに 1 枚を返す。
// P フレームまで捨てる (nokey) と、I フレームをフィールド単位で符号化した MPEG-2 (I フィールドと P フィールドの対) では、
// 片方のフィールドの行が未初期化のまま出て、1 行おきに緑の縞になる (#424)。
func (f *FFmpeg) Extract(ctx context.Context, r io.Reader, size Size) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	var cmd *exec.Cmd
	switch size {
	case SizeThumbnail:
		cmd = exec.CommandContext(ctx, ffmpegCommand, "-hide_banner", "-loglevel", "error",
			"-skip_frame", "bidir", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-an", "-sn", "-frames:v", "1",
			"-vf", thumbnailFilter, "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
	case SizeTile:
		cmd = exec.CommandContext(ctx, ffmpegCommand, "-hide_banner", "-loglevel", "error",
			"-skip_frame", "bidir", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-an", "-sn", "-frames:v", "1",
			"-vf", tileFilter, "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
	default:
		return nil, fmt.Errorf("unknown frame size: %d", size)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r, &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > maxStderrBytes {
			message = strings.ToValidUTF8(message[:maxStderrBytes], "")
		}

		return nil, fmt.Errorf("failed to extract a frame with FFmpeg: %w: %s", err, message)
	}
	if stdout.Len() == 0 {
		return nil, errors.New("FFmpeg produced no frame")
	}

	return stdout.Bytes(), nil
}
