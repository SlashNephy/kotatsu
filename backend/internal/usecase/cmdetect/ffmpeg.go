package cmdetect

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ffmpegCommand は、PATH から探す FFmpeg のコマンドの名前。FFmpeg の入ったイメージ (-ffmpeg) には入っている。
	ffmpegCommand = "ffmpeg"
	// FFmpegDetectorName と ffmpegDetectorVersion は、FFmpegDetector の名前と版。規則や値を変えたら版を上げる。
	FFmpegDetectorName    = "ffmpeg"
	ffmpegDetectorVersion = 1
	// silenceNoise は、無音とみなす音量の上限。CM の境目は、デジタルの無音 (0) になる。
	// 本編の台詞の間は、BGM や環境音が小さく残ることが多いので、ずっと小さい音までを無音とみなさない。
	silenceNoise = "-80dB"
	// sceneThreshold は、シーンが切り替わったとみなす scdet の値の下限。既定 (10) と同じにする。
	sceneThreshold = "10"
	// silenceDuration は、無音とみなす長さの下限 (秒)。規則の minSilence と同じ値にする。
	silenceDuration = "0.3"
	// filterGraph は、映像を縮めてシーンの切り替わりを、音声から無音の区間を探す FFmpeg のフィルター。
	// 録画は、録画したサービスだけの TS なので (代表画像と同じく)、最初の映像と音声を読む。
	filterGraph = "[0:v:0]scale=160:90,scdet=threshold=" + sceneThreshold +
		",metadata=select:key=lavfi.scd.time,metadata=print:file=pipe\\\\:1[v];" +
		"[0:a:0]silencedetect=noise=" + silenceNoise + ":d=" + silenceDuration + "[a]"
	// maxStderrLines は、失敗したときに、エラーに含める FFmpeg の標準エラー出力の行の数。
	maxStderrLines = 5
)

// ErrFFmpegNotFound は、FFmpeg が PATH に見つからないことを表す。FFmpeg の入ったイメージ (-ffmpeg) を使うか、FFmpeg を入れる。
var ErrFFmpegNotFound = errors.New("FFmpeg is not found in PATH")

// ErrNoVideo は、ファイルに、サービスの映像が見つからないことを表す。
var ErrNoVideo = errors.New("the file has no video of the service")

// FFmpegDetector は、FFmpeg を子プロセスとして動かし、無音の区間とシーンの切り替わりから CM の区間を見分ける。
// 本体とはリンクしない (設計書 付録 B)。FFmpeg に流すのと同じ TS を ts.CueScanner にも読ませ、音声の構成の変化を手掛かりに足す。
// 映像は I フレームだけをデコードし、縮めてから比べるので、30 分の録画で CPU を 30 秒ほどしか使わない。
type FFmpegDetector struct{}

var _ Detector = new(FFmpegDetector)

// NewFFmpegDetector は、PATH にある FFmpeg を使う FFmpegDetector を返す。見つからなければ ErrFFmpegNotFound を返す。
func NewFFmpegDetector() (*FFmpegDetector, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &FFmpegDetector{}, nil
}

func (*FFmpegDetector) Name() string { return FFmpegDetectorName }

func (*FFmpegDetector) Version() int32 { return ffmpegDetectorVersion }

func (*FFmpegDetector) Detect(ctx context.Context, input *Input) ([]Section, error) {
	file, err := sharedfile.Open(input.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the recording file: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := ts.NewCueScanner(input.ServiceID)
	// 時刻を PTS のまま出させ (-copyts)、ts.CueScanner の時刻と比べられるようにする。I フレームだけをデコードする。
	// CM の境目の判断には、0.5 秒ごとの絵で足りる。シーンの切り替わりは標準出力に、無音の区間はログ (標準エラー出力) に出る。
	cmd := exec.CommandContext(ctx, ffmpegCommand, "-nostdin", "-hide_banner", "-nostats", "-loglevel", "info",
		"-threads", "1", "-filter_complex_threads", "1", "-skip_frame", "nokey", "-copyts", "-f", "mpegts", "-i", "pipe:0",
		"-filter_complex", filterGraph, "-map", "[v]", "-map", "[a]", "-f", "null", "-")
	cmd.Stdin = io.TeeReader(file, scanner)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the FFmpeg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the FFmpeg stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	var wg sync.WaitGroup
	var out *ffmpegOutput
	var log *ffmpegLog
	wg.Go(func() { out = parseStdout(stdout) })
	wg.Go(func() { log = parseStderr(stderr) })
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to detect CM sections with FFmpeg: %w: %s", err, strings.Join(log.tail, "; "))
	}

	sig, err := newSignals(scanner.Cues(), out.scenes, log.silences)
	if err != nil {
		return nil, err
	}
	spans := findSections(sig)
	sections := make([]Section, 0, len(spans))
	for _, s := range spans {
		sections = append(sections, Section{Start: seconds(s.start), End: seconds(s.end)})
	}

	return sections, nil
}

// ffmpegOutput は、FFmpeg の標準出力から読んだ、シーンが切り替わった時刻 (秒、PTS のまま)。
type ffmpegOutput struct {
	scenes []float64
}

// parseStdout は、metadata フィルターが出す、シーンが切り替わったフレームの lavfi.scd.time を読む。
func parseStdout(r io.Reader) *ffmpegOutput {
	out := &ffmpegOutput{scenes: []float64{}}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "lavfi.scd.time="); ok {
			if t, err := strconv.ParseFloat(value, 64); err == nil {
				out.scenes = append(out.scenes, t)
			}
		}
	}
	// 読み残すと FFmpeg が書き込みで止まるので、読めなくなっても最後まで捨てる。
	_, _ = io.Copy(io.Discard, r)

	return out
}

// ffmpegLog は、FFmpeg のログから読んだ無音の区間 (秒、PTS のまま) と、失敗したときに出す末尾の行。
type ffmpegLog struct {
	silences []span
	tail     []string
}

// parseStderr は、silencedetect フィルターが出す silence_start と silence_end の行を読む。
func parseStderr(r io.Reader) *ffmpegLog {
	log := &ffmpegLog{silences: []span{}, tail: []string{}}
	start := math.NaN()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if _, value, ok := strings.Cut(line, "silence_start: "); ok {
			if t, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				start = t
			}

			continue
		}
		if _, value, ok := strings.Cut(line, "silence_end: "); ok {
			value, _, _ = strings.Cut(value, " ")
			if t, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(start) {
				log.silences = append(log.silences, span{start: start, end: t})
			}
			start = math.NaN()

			continue
		}
		log.tail = append(log.tail, line)
		if len(log.tail) > maxStderrLines {
			log.tail = log.tail[1:]
		}
	}
	_, _ = io.Copy(io.Discard, r)

	return log
}

// newSignals は、PTS のままの時刻を、ファイルの最初の映像からの秒にそろえる。PTS が一周していても、差が半周より短ければ正しく求まる。
func newSignals(cues *ts.Cues, scenes []float64, silences []span) (*signals, error) {
	if cues.FirstVideoPTS == nil || cues.LastVideoPTS == nil {
		return nil, ErrNoVideo
	}
	base := *cues.FirstVideoPTS
	fromPTS := func(pts int64) float64 { return float64(ts.PTSDiff(base, pts)) / ts.PTSClock }
	fromSeconds := func(t float64) float64 { return fromPTS(int64(math.Round(t * ts.PTSClock))) }

	sig := &signals{
		silences:     make([]span, 0, len(silences)),
		scenes:       make([]float64, 0, len(scenes)),
		audioChanges: make([]float64, 0, len(cues.AudioChanges)),
		duration:     fromPTS(*cues.LastVideoPTS),
	}
	for _, s := range silences {
		sig.silences = append(sig.silences, span{start: fromSeconds(s.start), end: fromSeconds(s.end)})
	}
	for _, t := range scenes {
		sig.scenes = append(sig.scenes, fromSeconds(t))
	}
	for _, pts := range cues.AudioChanges {
		sig.audioChanges = append(sig.audioChanges, fromPTS(pts))
	}

	return sig, nil
}

// seconds は、秒を time.Duration にする。
func seconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}
