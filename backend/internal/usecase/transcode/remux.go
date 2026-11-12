package transcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
)

// ffprobeCommand は、変換した MP4 の時刻を読むのに使うコマンド。FFmpeg の入ったイメージ (Debian の ffmpeg パッケージ) に入っている。
const ffprobeCommand = "ffprobe"

var errNoVideoStream = errors.New("the MP4 has no video stream")

// MP4Info は、変換した MP4 から読んだ時刻。
type MP4Info struct {
	// VideoStart は、最初の映像のコマの時刻。変換は、録画を最初の I フレームから流すので、このコマが最初の I フレームに当たる。
	// FFmpeg は、TS の最初のパケット (映像より先に届く音声など) を MP4 の 0 にするので、映像は少し後から始まる (edit list で表す)。
	VideoStart time.Duration
	// Duration は、MP4 の長さ。
	Duration time.Duration
	// Chapters は、今入っているチャプターの数。
	Chapters int
}

// Chapterer は、変換した MP4 を読み、チャプターを書き直す。
type Chapterer interface {
	// Probe は、in の MP4 の時刻を読む。
	Probe(ctx context.Context, in string) (*MP4Info, error)
	// Rewrite は、in の MP4 を、ffmetadata 形式のファイル metadata のチャプターに替えて、out に写す。metadata が空なら、チャプターを外す。
	Rewrite(ctx context.Context, in, metadata, out string) error
}

// FFmpegChapterer は、FFmpeg で、変換した MP4 を再エンコードせずに写し直し、チャプターを書く。
// 録画の後処理なので、OS の最低の優先度で動かし、ライブ視聴の変換 (kyusu) に CPU を譲る (#1511)。
type FFmpegChapterer struct {
	lowerer *procprio.Lowerer
}

var _ Chapterer = new(FFmpegChapterer)

// NewFFmpegChapterer は FFmpegChapterer を返す。FFmpeg か ffprobe がなければ ErrFFmpegNotFound を返す。
// lowerer は、FFmpeg を最低の優先度で動かす。nil なら、優先度を変えない。
func NewFFmpegChapterer(lowerer *procprio.Lowerer) (*FFmpegChapterer, error) {
	for _, command := range []string{ffmpegCommand, ffprobeCommand} {
		if _, err := exec.LookPath(command); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
		}
	}

	return &FFmpegChapterer{lowerer: lowerer}, nil
}

// probeOutput は、ffprobe の JSON の出力のうち、読むところ。
type probeOutput struct {
	Streams []struct {
		StartTime string `json:"start_time"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Chapters []json.RawMessage `json:"chapters"`
}

func (c *FFmpegChapterer) Probe(ctx context.Context, in string) (*MP4Info, error) {
	cmd := exec.CommandContext(ctx, ffprobeCommand, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=start_time:format=duration", "-show_chapters", "-of", "json", in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: stderrLimit}
	if err := c.lowerer.Run(cmd); err != nil {
		return nil, fmt.Errorf("failed to probe the MP4: %w: %s", err, stderr.String())
	}
	var out probeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("failed to decode the ffprobe output: %w", err)
	}
	if len(out.Streams) == 0 {
		return nil, fmt.Errorf("%w: %s", errNoVideoStream, in)
	}
	start, err := parseSeconds(out.Streams[0].StartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to read the start of the video: %w", err)
	}
	duration, err := parseSeconds(out.Format.Duration)
	if err != nil {
		return nil, fmt.Errorf("failed to read the duration of the MP4: %w", err)
	}

	return &MP4Info{VideoStart: start, Duration: duration, Chapters: len(out.Chapters)}, nil
}

// Rewrite は、MP4 を再エンコードせずに写し直す。MP4 は、変換したときと同じく、索引を先頭に置き直す (faststart)。
//
// MP4 のチャプターは、FFmpeg が読むと、チャプターとは別に、データのストリーム (QuickTime のチャプタートラック) としても見える。
// -map 0 だけで写すと、前に書いたチャプタートラックがデータのストリームとして残り、書き直すたびに増え、外しても残るので、
// データのストリームを除く (-map -0:d?)。変換した MP4 は、映像と音声だけを持つ (FFmpegArgs) ので、ほかに除くものはない。
func (c *FFmpegChapterer) Rewrite(ctx context.Context, in, metadata, out string) error {
	var cmd *exec.Cmd
	if metadata == "" {
		cmd = exec.CommandContext(ctx, ffmpegCommand, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", in,
			"-map", "0", "-map", "-0:d?", "-map_chapters", "-1", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", out)
	} else {
		cmd = exec.CommandContext(ctx, ffmpegCommand, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", in,
			"-f", "ffmetadata", "-i", metadata,
			"-map", "0", "-map", "-0:d?", "-map_chapters", "1", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", out)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: stderrLimit}
	if err := c.lowerer.Run(cmd); err != nil {
		return fmt.Errorf("failed to rewrite the chapters of the MP4: %w: %s", err, stderr.String())
	}

	return nil
}

// parseSeconds は、ffprobe が出す秒の文字列を time.Duration にする。
func parseSeconds(s string) (time.Duration, error) {
	seconds, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds %q: %w", s, err)
	}

	return time.Duration(math.Round(seconds * float64(time.Second))), nil
}
