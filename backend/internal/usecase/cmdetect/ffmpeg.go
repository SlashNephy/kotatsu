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
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ffmpegCommand は、PATH から探す FFmpeg のコマンドの名前。FFmpeg の入ったイメージ (-ffmpeg) には入っている。
	ffmpegCommand = "ffmpeg"
	// FFmpegDetectorName と ffmpegDetectorVersion は、FFmpegDetector の名前とバージョン。規則や値を変えたらバージョンを上げる。
	// バージョン 2 で、区間の時刻の基準を、最初の映像の PES から、最初の I フレームに移した (#1811)。
	FFmpegDetectorName    = "ffmpeg"
	ffmpegDetectorVersion = 2
	// firstKeyframeFFmpegVersion は、区間の時刻の基準を最初の I フレームにした、FFmpegDetector の最初のバージョン。
	firstKeyframeFFmpegVersion = 2
	// silenceNoise は、無音とみなす音量の上限。CM の境目は、デジタルの無音 (0) になる。
	// 本編の台詞の間は、BGM や環境音が小さく残ることが多いので、ずっと小さい音までを無音とみなさない。
	silenceNoise = "-80dB"
	// sceneThreshold は、シーンが切り替わったとみなす scdet の値の下限。既定 (10) と同じにする。
	sceneThreshold = "10"
	// silenceDuration は、無音とみなす長さの下限 (秒)。規則の minSilence と同じ値にする。
	silenceDuration = "0.3"
	// maxStderrLines は、失敗したときに、エラーに含める FFmpeg の標準エラー出力の行の数。
	maxStderrLines = 5
)

// ErrFFmpegNotFound は、FFmpeg が PATH に見つからないことを表す。FFmpeg の入ったイメージ (-ffmpeg) を使うか、FFmpeg を入れる。
var ErrFFmpegNotFound = errors.New("FFmpeg is not found in PATH")

// ErrNoVideo は、ファイルに、サービスの映像が見つからないことを表す。
var ErrNoVideo = errors.New("the file has no video of the service")

// FFmpegDetector は、FFmpeg を子プロセスとして動かし、無音の区間とシーンの切り替わりから CM の区間を見分ける。
// 本体とはリンクしない (設計書 付録 B)。FFmpeg に流すのと同じ TS を ts.CueScanner にも読ませ、音声の構成の変化を手掛かりに足す。
// 映像は I フレームだけをデコードし、縮めてから比較するので、30 分の録画で CPU を 30 秒ほどしか使わない。
// 録画の後処理なので、OS の最低の優先度で動かし、ライブ視聴の変換 (kyusu) に CPU を譲る (#1511)。
type FFmpegDetector struct {
	lowerer *procprio.Lowerer
}

var _ Detector = new(FFmpegDetector)

// NewFFmpegDetector は、PATH にある FFmpeg を使う FFmpegDetector を返す。見つからなければ ErrFFmpegNotFound を返す。
// lowerer は、FFmpeg を最低の優先度で動かす。nil なら、優先度を変えない。
func NewFFmpegDetector(lowerer *procprio.Lowerer) (*FFmpegDetector, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return &FFmpegDetector{lowerer: lowerer}, nil
}

func (*FFmpegDetector) Name() string { return FFmpegDetectorName }

func (*FFmpegDetector) Version() int32 { return ffmpegDetectorVersion }

func (d *FFmpegDetector) Detect(ctx context.Context, input *Input) ([]Section, error) {
	sig, err := d.signals(ctx, input)
	if err != nil {
		return nil, err
	}
	spans := findSections(sig)
	sections := make([]Section, 0, len(spans))
	for _, s := range spans {
		// 最初の I フレームより前 (デコードできず、再生にも入らない所) は切る。
		if start, end := max(s.start, 0), s.end; end > start {
			sections = append(sections, Section{Start: seconds(start), End: seconds(end)})
		}
	}

	return sections, nil
}

// signals は、録画のファイルを FFmpeg で読み、無音の区間、シーンの切り替わり、音声の構成の変化を返す。
// 局ロゴの見分け方も、境目の候補を作るのに使う (#1506)。
func (d *FFmpegDetector) signals(ctx context.Context, input *Input) (*signals, error) {
	file, err := input.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	scanner := ts.NewCueScanner(input.ServiceID)
	// 時刻を PTS のまま出させ (-copyts)、ts.CueScanner の時刻と比べられるようにする。I フレームだけをデコードする。
	// CM の境目の判断には、0.5 秒ごとの絵で足りる。シーンの切り替わりは標準出力に、無音の区間はログ (標準エラー出力) に出る。
	// 進み具合 (#1511) の -progress のまとまりは、標準出力をシーンの切り替わりに使っているので、標準エラー出力に混ぜて出させ、
	// 行の形で見分ける。fd 3 に出させる (ExtraFiles) 方法は、Windows で使えない。
	graph := filterGraph(input)
	cmd := exec.CommandContext(ctx, ffmpegCommand, "-nostdin", "-hide_banner", "-nostats", "-progress", "pipe:2", "-loglevel", "info",
		"-threads", "1", "-filter_complex_threads", "1", "-skip_frame", "nokey", "-copyts", "-f", "mpegts", "-i", "pipe:0",
		"-filter_complex", graph, "-map", "[v]", "-map", "[a]", "-f", "null", "-")
	src := &countingReader{r: file, n: atomic.Int64{}}
	cmd.Stdin = io.TeeReader(src, scanner)
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
	var out *ffmpegOutput
	var log *ffmpegLog
	wg.Go(func() { out = parseStdout(stdout) })
	var onBlock func(*ffprogress.Block)
	if input.Progress != nil {
		onBlock = func(b *ffprogress.Block) { input.Progress(b, src.n.Load()) }
	}
	wg.Go(func() { log = parseStderr(stderr, onBlock) })
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to detect CM sections with FFmpeg: %w: %s", err, strings.Join(log.tail, "; "))
	}

	return newSignals(scanner.Cues(), out.scenes, log.silences)
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
	// 読み残すと FFmpeg が書き込みで止まるので、読めなくなっても最後まで破棄する。
	_, _ = io.Copy(io.Discard, r)

	return out
}

// ffmpegLog は、FFmpeg のログから読んだ無音の区間 (秒、PTS のまま) と、失敗したときに出す末尾の行。
type ffmpegLog struct {
	silences []span
	tail     []string
}

// parseStderr は、silencedetect フィルターが出す silence_start と silence_end の行を読む。
// -progress のまとまりの行は、onBlock に渡し、ログの末尾には残さない。onBlock が nil なら破棄する。
func parseStderr(r io.Reader, onBlock func(*ffprogress.Block)) *ffmpegLog {
	log := &ffmpegLog{silences: []span{}, tail: []string{}}
	start := math.NaN()
	if onBlock == nil {
		onBlock = func(*ffprogress.Block) {}
	}
	parser := ffprogress.NewParser(onBlock, func(line string) {
		if _, value, ok := strings.Cut(line, "silence_start: "); ok {
			if t, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				start = t
			}

			return
		}
		if _, value, ok := strings.Cut(line, "silence_end: "); ok {
			value, _, _ = strings.Cut(value, " ")
			if t, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(start) {
				log.silences = append(log.silences, span{start: start, end: t})
			}
			start = math.NaN()

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

// countingReader は、読んだバイト数を数える。進み具合の読み位置に使う。
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))

	return n, err
}

// newSignals は、PTS のままの時刻を、ファイルの最初の I フレームからの秒にそろえる。PTS が一周していても、差が半周より短ければ正しく求まる。
// 再生の時間は、録画のインデックスの最初の I フレームを 0 にするので、それとそろえる (#1811)。録画の先頭の、最初の I フレームより前の PES は、
// デコードできず、再生にも入らない。本番の録画では 0.3〜0.5 秒あり、最初の映像の PES を 0 にすると、CM の区間がその分遅れた。
// I フレームの探し方を知らない映像で、最初の I フレームが分からなければ、最初の映像の PES を 0 にする。
func newSignals(cues *ts.Cues, scenes []float64, silences []span) (*signals, error) {
	if cues.FirstVideoPTS == nil || cues.LastVideoPTS == nil {
		return nil, ErrNoVideo
	}
	base := *cues.FirstVideoPTS
	if cues.FirstKeyframePTS != nil {
		base = *cues.FirstKeyframePTS
	}
	fromPTS := func(pts int64) float64 { return float64(ts.PTSDiff(base, pts)) / ts.PTSClock }
	fromSeconds := func(t float64) float64 { return fromPTS(int64(math.Round(t * ts.PTSClock))) }

	sig := &signals{
		origin:       base,
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
