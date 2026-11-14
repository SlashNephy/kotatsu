package cmdetect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

const (
	// LogoDetectorName と logoDetectorVersion は、LogoDetector の名前と版。規則や値を変えたら版を上げる。
	LogoDetectorName    = "logo"
	logoDetectorVersion = 1
	// ffprobeCommand は、PATH から探す ffprobe のコマンドの名前。FFmpeg に付いてくる。
	ffprobeCommand = "ffprobe"
	// rawReadBuffer は、FFmpeg が出す生のコマを読むバッファの大きさ。
	rawReadBuffer = 1 << 20
)

// errLogoUnusable は、その録画ではロゴを使えないことを表す。今の見分け方 (FFmpegDetector) で見分け直す。
var errLogoUnusable = errors.New("the logo cannot be used for this recording")

// LogoDetector は、映像の隅の局ロゴが出ているかどうかで、CM の区間を見分ける (#1505、設計書 6.1)。試験的な見分け方。
//
// 録画のキーフレームの上下の帯から、ロゴの枠と型を学習し (logo.Learn、logo.NewTemplate)、全コマのロゴの枠を切り出して、
// ロゴの出ていた区間を出す (logo.Intervals)。ロゴの出ていない所を CM とみなす。型は保存せず、録画ごとに学習する。
// ロゴを学習できない録画 (局ロゴのない通販の枠など) と、ロゴの出ていた区間が 1 つもない録画では、今の見分け方 (FFmpegDetector) で見分ける。
// FFmpeg は、子プロセスとして、OS の最低の優先度で動かす (#1511)。
type LogoDetector struct {
	lowerer  *procprio.Lowerer
	fallback Detector
	params   *logo.Params
}

var _ Detector = new(LogoDetector)

// NewLogoDetector は、PATH にある FFmpeg と ffprobe を使う LogoDetector を返す。見つからなければ ErrFFmpegNotFound を返す。
func NewLogoDetector(lowerer *procprio.Lowerer) (*LogoDetector, error) {
	fallback, err := NewFFmpegDetector(lowerer)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(ffprobeCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &LogoDetector{lowerer: lowerer, fallback: fallback, params: logo.DefaultParams()}, nil
}

func (*LogoDetector) Name() string { return LogoDetectorName }

func (*LogoDetector) Version() int32 { return logoDetectorVersion }

func (d *LogoDetector) Detect(ctx context.Context, input *Input) ([]Section, error) {
	sections, err := d.detect(ctx, input)
	if errors.Is(err, errLogoUnusable) {
		return d.fallback.Detect(ctx, input)
	}

	return sections, err
}

func (d *LogoDetector) detect(ctx context.Context, input *Input) ([]Section, error) {
	video, err := probeVideo(ctx, input.Path)
	if err != nil {
		return nil, err
	}
	p := d.params
	bandHeight := logo.Band(video.height, p)
	top, bottom := logo.NewAccumulator(video.width, bandHeight), logo.NewAccumulator(video.width, bandHeight)
	interval := max(p.LearnMinInterval.Seconds(), video.duration.Seconds()/float64(p.LearnFrames))
	learnFilter := fmt.Sprintf("fps=1/%f,split[a][b];[a]crop=%d:%d:0:0[t];[b]crop=%d:%d:0:%d[u];[t][u]vstack,format=gray",
		interval, video.width, bandHeight, video.width, bandHeight, video.height-bandHeight)
	half := video.width * bandHeight
	err = d.frames(ctx, input.Path, []string{"-skip_frame", "nokey"}, learnFilter, 2*half, func(buf []byte) {
		top.Add(&logo.Gray{Width: video.width, Height: bandHeight, Pix: buf[:half]}, p.EdgeStrength)
		bottom.Add(&logo.Gray{Width: video.width, Height: bandHeight, Pix: buf[half:]}, p.EdgeStrength)
	})
	if err != nil {
		return nil, err
	}
	learned, err := logo.Learn(top, bottom, video.width, video.height, p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLogoUnusable, err)
	}
	model, box := top, learned.Box
	if learned.Corner == logo.BottomRight || learned.Corner == logo.BottomLeft {
		model = bottom
		box.Y -= video.height - bandHeight
	}
	template, err := logo.NewTemplate(model.Crop(box), learned.Box, video.width, video.height, p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLogoUnusable, err)
	}

	// 切り出しは exact=1 にする。付けないと、yuv420p で奇数の位置が丸められ、細い縁の一致が崩れる。
	c := template.Crop
	measures := []logo.Measure{}
	err = d.frames(ctx, input.Path, nil, fmt.Sprintf("crop=%d:%d:%d:%d:exact=1,format=gray", c.W, c.H, c.X, c.Y), c.W*c.H, func(buf []byte) {
		measures = append(measures, template.Measure(&logo.Gray{Width: c.W, Height: c.H, Pix: buf}))
	})
	if err != nil {
		return nil, err
	}
	intervals := logo.Intervals(measures, video.fps, p)
	if len(intervals) == 0 {
		return nil, fmt.Errorf("%w: no frame shows the logo", errLogoUnusable)
	}

	return complementSections(intervals, len(measures), video.fps), nil
}

// complementSections は、ロゴの出ていた区間 intervals の外 (n コマの中) を、CM の区間にする。
// コマの番号は、ファイルの最初の映像からの時間に、速さ fps で直す。
func complementSections(intervals []logo.Interval, n int, fps float64) []Section {
	at := func(frame int) time.Duration { return time.Duration(float64(frame) / fps * float64(time.Second)) }
	sections := []Section{}
	cursor := 0
	for _, iv := range intervals {
		if iv.Start > cursor {
			sections = append(sections, Section{Start: at(cursor), End: at(iv.Start)})
		}
		cursor = iv.End
	}
	if cursor < n {
		sections = append(sections, Section{Start: at(cursor), End: at(n)})
	}

	return sections
}

// frames は、path の TS を FFmpeg でデコードし、最初の映像に filter を掛けたグレースケールの生のコマを、size バイトずつ fn に渡す。
// pre は、入力の前に付ける引数。コマは間引かず増やさない (-fps_mode passthrough)。
func (d *LogoDetector) frames(ctx context.Context, path string, pre []string, filter string, size int, fn func([]byte)) error {
	file, err := sharedfile.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open the recording file: %w", err)
	}
	defer func() { _ = file.Close() }()

	args := append([]string{"-nostdin", "-hide_banner", "-nostats", "-loglevel", "error"}, pre...)
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-vf", filter, "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	cmd := exec.CommandContext(ctx, ffmpegCommand, args...)
	cmd.Stdin = file
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open the FFmpeg stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := d.lowerer.Start(cmd); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	r := bufio.NewReaderSize(stdout, rawReadBuffer)
	buf := make([]byte, size)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		fn(buf)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to read the frames with FFmpeg: %w: %s", err, lastLines(stderr.String(), maxStderrLines))
	}

	return nil
}

// lastLines は、s の最後の n 行を "; " でつないで返す。
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")

	return strings.Join(lines[max(len(lines)-n, 0):], "; ")
}

// videoInfo は、録画の最初の映像の大きさと速さと、録画の長さ。
type videoInfo struct {
	width    int
	height   int
	fps      float64
	duration time.Duration
}

// probeVideo は、ffprobe で、path の最初の映像の大きさと速さと、録画の長さを読む。
func probeVideo(ctx context.Context, path string) (*videoInfo, error) {
	out, err := exec.CommandContext(ctx, ffprobeCommand, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate:format=duration", "-of", "json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to probe the recording with ffprobe: %w", err)
	}
	var probed struct {
		Streams []struct {
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probed); err != nil {
		return nil, fmt.Errorf("failed to decode the ffprobe output: %w", err)
	}
	if len(probed.Streams) == 0 || probed.Streams[0].Width == 0 || probed.Streams[0].Height == 0 {
		return nil, ErrNoVideo
	}
	s := probed.Streams[0]
	fps := frameRate(s.AvgFrameRate)
	if fps <= 0 {
		return nil, fmt.Errorf("%w: unknown frame rate %q", ErrNoVideo, s.AvgFrameRate)
	}
	seconds, _ := strconv.ParseFloat(probed.Format.Duration, 64)

	return &videoInfo{width: s.Width, height: s.Height, fps: fps, duration: time.Duration(seconds * float64(time.Second))}, nil
}

// frameRate は、ffprobe の "30000/1001" の形の速さを数にする。読めなければ 0 を返す。
func frameRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	n, err1 := strconv.ParseFloat(num, 64)
	if !ok {
		if err1 != nil {
			return 0
		}

		return n
	}
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}

	return n / d
}
