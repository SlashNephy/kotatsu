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
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
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
	learnFilter := fmt.Sprintf("fps=1/%f,split[a][b];[a]crop=%d:%d:0:0:exact=1[t];[b]crop=%d:%d:0:%d:exact=1[u];[t][u]vstack,format=gray",
		interval, video.width, bandHeight, video.width, bandHeight, video.height-bandHeight)
	half := video.width * bandHeight
	_, err = d.frames(ctx, input.Path, []string{"-skip_frame", "nokey"}, learnFilter, 2*half, nil, func(buf []byte) {
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
	times, err := d.frames(ctx, input.Path, nil, fmt.Sprintf("crop=%d:%d:%d:%d:exact=1,format=gray", c.W, c.H, c.X, c.Y), c.W*c.H, input.Progress,
		func(buf []byte) {
			measures = append(measures, template.Measure(&logo.Gray{Width: c.W, Height: c.H, Pix: buf}))
		})
	if err != nil {
		return nil, err
	}
	// コマと時刻は順番で突き合わせるので、数が合わなければ、対応がずれている (#1505 の H)。
	if len(times) != len(measures) {
		return nil, fmt.Errorf("%w: %d frames but %d timestamps", errLogoUnusable, len(measures), len(times))
	}
	intervals := logo.Intervals(measures, video.fps, p)
	if len(intervals) == 0 {
		return nil, fmt.Errorf("%w: no frame shows the logo", errLogoUnusable)
	}

	return complementSections(intervals, times, video.fps), nil
}

// complementSections は、ロゴの出ていた区間 intervals の外を、CM の区間にする。times は、コマごとの時刻 (秒) で、
// 区間の時刻は、最初のコマ (ファイルの最初の映像) からの時間にする。最後のコマの終わりは、1 コマ分 (速さ fps) 後とする。
func complementSections(intervals []logo.Interval, times []float64, fps float64) []Section {
	n := len(times)
	at := func(frame int) time.Duration {
		t := 0.0
		if frame < n {
			t = times[frame] - times[0]
		} else if n > 0 {
			t = times[n-1] - times[0] + 1/fps
		}

		return time.Duration(t * float64(time.Second))
	}
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
// コマごとの時刻 (秒) を、showinfo の pts_time から読んで返す。コマが落ちたり、時刻が飛んだりしても、コマの番号でなく時刻で区間を出すためである。
// progress を渡せば、FFmpeg の -progress のまとまりと、それまでにファイルから流したバイト数を渡す (#1511)。
func (d *LogoDetector) frames(
	ctx context.Context, path string, pre []string, filter string, size int, progress func(*ffprogress.Block, int64), fn func([]byte),
) ([]float64, error) {
	file, err := sharedfile.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the recording file: %w", err)
	}
	defer func() { _ = file.Close() }()

	args := append([]string{"-nostdin", "-hide_banner", "-nostats", "-progress", "pipe:2", "-loglevel", "info"}, pre...)
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-vf", filter+",showinfo",
		"-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	cmd := exec.CommandContext(ctx, ffmpegCommand, args...)
	src := &countingReader{r: file, n: atomic.Int64{}}
	cmd.Stdin = src
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the FFmpeg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the FFmpeg stderr: %w", err)
	}
	if err := d.lowerer.Start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	var wg sync.WaitGroup
	var log *frameLog
	onBlock := func(*ffprogress.Block) {}
	if progress != nil {
		onBlock = func(b *ffprogress.Block) { progress(b, src.n.Load()) }
	}
	wg.Go(func() { log = parseFrameLog(stderr, onBlock) })
	r := bufio.NewReaderSize(stdout, rawReadBuffer)
	buf := make([]byte, size)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		fn(buf)
	}
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to read the frames with FFmpeg: %w: %s", err, strings.Join(log.tail, "; "))
	}

	return log.times, nil
}

// frameLog は、FFmpeg の標準エラー出力から読んだもの。
type frameLog struct {
	// times は、showinfo が出したコマごとの時刻 (秒)。
	times []float64
	// tail は、そのほかの最後の数行。失敗したときのエラーに含める。
	tail []string
}

// parseFrameLog は、FFmpeg の標準エラー出力から、showinfo のコマの時刻と、-progress のまとまりを読む。
func parseFrameLog(r io.Reader, onBlock func(*ffprogress.Block)) *frameLog {
	log := &frameLog{times: []float64{}, tail: []string{}}
	parser := ffprogress.NewParser(onBlock, func(line string) {
		if strings.Contains(line, "Parsed_showinfo") {
			if _, value, ok := strings.Cut(line, "pts_time:"); ok {
				value, _, _ = strings.Cut(strings.TrimSpace(value), " ")
				if t, err := strconv.ParseFloat(value, 64); err == nil {
					log.times = append(log.times, t)
				}
			}

			return
		}
		log.tail = append(log.tail, line)
		if len(log.tail) > maxStderrLines {
			log.tail = log.tail[1:]
		}
	})
	// 読み残すと FFmpeg が書き込みで止まるので、最後まで読む。
	_, _ = io.Copy(parser, r)
	parser.Flush()

	return log
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
