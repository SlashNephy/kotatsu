package cmdetect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
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
	// maxCMRatio は、CM の割合の上限 (#1508)。これ以上を CM とした結果は使わない。ロゴでない縁を覚えたときや、
	// 型が番組のロゴと合わないときは、「先頭の数秒だけ本編」のような結果が出る。CM の自動スキップがそれを信じると本編を飛ばすので、
	// CM が残るほうを選ぶ。普通の番組の CM は 2〜3 割ほどなので、それより上に置く (仮の値)。
	maxCMRatio = 0.5
	// missingGap は、映像の欠けとみなすコマの時刻の空き (#1508)。放送では静止した場面でもコマは途切れないので、
	// これだけ空いた所は、元の TS で映像をデコードできなかった所である。
	missingGap = time.Second
)

// errLogoUnusable は、その録画ではロゴを使えないことを表す。今の見分け方 (FFmpegDetector) で見分け直す。
var errLogoUnusable = errors.New("the logo cannot be used for this recording")

// LogoDetector は、映像の隅の局ロゴが出ているかどうかで、CM の区間を見分ける (#1505、設計書 6.1)。試験的な見分け方。
//
// 録画のキーフレームの上下の帯から、ロゴの枠と型を学習し (logo.Learn、logo.NewTemplate)、全コマのロゴの枠を切り出して、
// ロゴの出ていた区間を出す (logo.Intervals)。ロゴの出ていない所を CM とみなす。型は保存せず、録画ごとに学習する。
// ロゴを学習できない録画 (局ロゴのない通販の枠など) と、ロゴの出ていた区間が 1 つもない録画では、今の見分け方 (FFmpegDetector) で見分ける。
// ロゴで決めた CM の割合が上限 (maxCMRatio) 以上の録画も、今の見分け方で見分け直し、それも上限以上なら CM なしにする (#1508)。
// FFmpeg は、子プロセスとして、OS の最低の優先度で動かす (#1511)。
type LogoDetector struct {
	lowerer  *procprio.Lowerer
	fallback Detector
	params   *logo.Params
}

var (
	_ Detector = new(LogoDetector)
	_ Reporter = new(LogoDetector)
)

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

// Detect は、ロゴで見分ける。ロゴを使えないか、ロゴで決めた CM の割合が上限以上なら、今の見分け方で見分け直す (#1508)。
// 見分け直した結果も上限以上なら、CM なしにする。
func (d *LogoDetector) Detect(ctx context.Context, input *Input) ([]Section, error) {
	sections, _, err := d.DetectWithReport(ctx, input)

	return sections, err
}

// DetectWithReport は、Detect と同じ区間と、ロゴで決められなかった理由と映像の欠けの長さを返す (#1508)。
func (d *LogoDetector) DetectWithReport(ctx context.Context, input *Input) ([]Section, *Report, error) {
	video, err := probeVideo(ctx, input.Path)
	if err != nil {
		return nil, nil, err
	}
	report := &Report{Note: "", Missing: 0}
	// 見分け直した結果の割合は、録画の長さで割る。長さは、ロゴで測れたならコマの時刻から、測れなければ ffprobe から求める。
	// 映像の欠けは外さない。今の見分け方の区間とは時刻の基準が違い、重なりを求められないためである。
	duration := video.duration
	result, err := d.detect(ctx, input, video)
	if err == nil {
		report.Missing = totalLength(result.missing) + result.edgeMissing
		ratio := cmRatio(result.sections, result.missing, result.duration)
		if ratio < maxCMRatio {
			return result.sections, report, nil
		}
		duration = result.duration
		err = fmt.Errorf("%w: %.0f%% of the recording was detected as CM (%s)", errLogoUnusable, ratio*100, result.detail(d.params))
	}
	if !errors.Is(err, errLogoUnusable) {
		return nil, nil, err
	}
	report.Note = err.Error()
	sections, err := d.fallback.Detect(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	// 長さが分からなければ (ffprobe が長さを返さない TS)、割合を確かめられないので、今の見分け方の結果をそのまま使う。
	// 今の見分け方を選んだときと同じ結果になる。
	if duration > 0 {
		if ratio := cmRatio(sections, []Section{}, duration); ratio >= maxCMRatio {
			report.Note += fmt.Sprintf("; the redetected result was dropped: %.0f%% of the recording was detected as CM", ratio*100)

			return []Section{}, report, nil
		}
	}

	return sections, report, nil
}

// totalLength は、区間の長さの合計を返す。
func totalLength(sections []Section) time.Duration {
	var total time.Duration
	for _, s := range sections {
		total += s.End - s.Start
	}

	return total
}

// trimMissing は、CM の端にかかった映像の欠けを、CM から切り落とす (#1508)。始まりが欠けの中なら欠けの終わりまで、
// 終わりが欠けの中なら欠けの始まりまでずらし、長さが 0 以下になった区間は捨てる。CM の内側の欠けは、CM のまま残す。
// 外すと、映像が何度も途切れる所で、CM が細かく分かれてしまうためである。
func trimMissing(sections, missing []Section) []Section {
	out := make([]Section, 0, len(sections))
	for _, s := range sections {
		for _, m := range missing {
			if s.Start >= m.Start && s.Start < m.End {
				s.Start = m.End
			}
			if s.End > m.Start && s.End <= m.End {
				s.End = m.Start
			}
		}
		if s.End > s.Start {
			out = append(out, s)
		}
	}

	return out
}

// logoResult は、ロゴで決めた結果。
type logoResult struct {
	sections []Section
	// missing は、映像の欠け (コマの時刻が missingGap 以上空いた所)。
	missing []Section
	// duration は、最初のコマから最後のコマの終わりまでの長さ。
	duration time.Duration
	// edgeMissing は、ファイルの始まりから最初のコマまでと、最後のコマの終わりからファイルの終わりまでの欠けの長さ (#1508)。
	// それぞれ missingGap 以上のときだけ数える。区間の時刻は最初のコマから数えるので、missing には入れない。
	edgeMissing time.Duration
	// 以下は、ロゴの結果を捨てたときに、理由の詳細に書くもの (#1508)。
	corner    logo.Corner
	box       logo.Box
	measures  []logo.Measure
	intervals []logo.Interval
	times     []float64
	fps       float64
}

// maxDetailSections は、理由の詳細に書く CM の区間の数の上限。理由の文が長くなりすぎないようにする。
const maxDetailSections = 10

// detail は、ロゴの結果を捨てた理由の詳細を返す (#1508)。当てた型の在り処と大きさ、ロゴが出ていたとしたコマの割合、
// 点数の分布、ロゴで CM とした区間とその中の点数の中央値。本編なのに CM とした区間の点数が、続いているとみなす点数に近ければ、
// ロゴは写っていたのに届かなかったことが、0 に近ければ、型と番組のロゴが合っていないことが分かる。
func (r *logoResult) detail(p *logo.Params) string {
	n := len(r.measures)
	if n == 0 {
		return "no frame was measured"
	}
	shown := 0
	for _, iv := range r.intervals {
		shown += iv.End - iv.Start
	}
	var appear, between, below int
	for _, m := range r.measures {
		switch {
		case m.Score >= p.AppearScore:
			appear++
		case m.Score >= p.ContinueScore:
			between++
		default:
			below++
		}
	}
	percent := func(count int) float64 { return float64(count) * 100 / float64(n) }
	var b strings.Builder
	fmt.Fprintf(&b, "logo at %s x=%d y=%d w=%d h=%d; logo shown in %.0f%% of the frames; scores >=%.2f: %.0f%%, %.2f-%.2f: %.0f%%, <%.2f: %.0f%%",
		r.corner, r.box.X, r.box.Y, r.box.W, r.box.H, percent(shown),
		p.AppearScore, percent(appear), p.ContinueScore, p.AppearScore, percent(between), p.ContinueScore, percent(below))
	b.WriteString("; CM by the logo (median score):")
	cursor := 0
	written := 0
	for i := 0; i <= len(r.intervals); i++ {
		end := n
		next := n
		if i < len(r.intervals) {
			end, next = r.intervals[i].Start, r.intervals[i].End
		}
		if end > cursor {
			if written == maxDetailSections {
				b.WriteString(" ...")

				break
			}
			fmt.Fprintf(&b, " %.1f-%.1fs (%.2f)", r.at(cursor), r.at(end), medianScore(r.measures[cursor:end]))
			written++
		}
		cursor = next
	}

	return b.String()
}

// at は、frame 番目のコマの始まりの時刻を、最初のコマからの秒で返す。最後のコマの後なら、最後のコマの終わりにする。
func (r *logoResult) at(frame int) float64 {
	if frame < len(r.times) {
		return r.times[frame] - r.times[0]
	}

	return r.times[len(r.times)-1] - r.times[0] + 1/r.fps
}

// medianScore は、measures の点数の中央値を返す。
func medianScore(measures []logo.Measure) float64 {
	scores := make([]float64, 0, len(measures))
	for _, m := range measures {
		scores = append(scores, m.Score)
	}
	slices.Sort(scores)
	if len(scores) == 0 {
		return 0
	}

	return scores[len(scores)/2]
}

// cmRatio は、CM の割合を返す (#1508)。映像の欠けは、分母からも分子からも外す。分母が 0 以下なら 0 にする。
func cmRatio(sections, missing []Section, duration time.Duration) float64 {
	total := duration
	for _, m := range missing {
		total -= m.End - m.Start
	}
	if total <= 0 {
		return 0
	}
	var cm time.Duration
	for _, s := range sections {
		cm += s.End - s.Start
		for _, m := range missing {
			cm -= max(min(s.End, m.End)-max(s.Start, m.Start), 0)
		}
	}

	return float64(cm) / float64(total)
}

// missingSections は、コマごとの時刻 times (秒) から、映像の欠けを、最初のコマからの時間で返す。
func missingSections(times []float64, fps float64) []Section {
	out := []Section{}
	at := func(t float64) time.Duration { return time.Duration((t - times[0]) * float64(time.Second)) }
	for i := 1; i < len(times); i++ {
		if time.Duration((times[i]-times[i-1])*float64(time.Second)) >= missingGap {
			out = append(out, Section{Start: at(times[i-1] + 1/fps), End: at(times[i])})
		}
	}

	return out
}

func (d *LogoDetector) detect(ctx context.Context, input *Input, video *videoInfo) (*logoResult, error) {
	p := d.params
	bandHeight := logo.Band(video.height, p)
	top, bottom := logo.NewAccumulator(video.width, bandHeight), logo.NewAccumulator(video.width, bandHeight)
	interval := max(p.LearnMinInterval.Seconds(), video.duration.Seconds()/float64(p.LearnFrames))
	learnFilter := fmt.Sprintf("fps=1/%f,split[a][b];[a]crop=%d:%d:0:0:exact=1[t];[b]crop=%d:%d:0:%d:exact=1[u];[t][u]vstack,format=gray",
		interval, video.width, bandHeight, video.width, bandHeight, video.height-bandHeight)
	half := video.width * bandHeight
	_, err := d.frames(ctx, input.Path, []string{"-skip_frame", "nokey"}, learnFilter, 2*half, nil, func(buf []byte) {
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

	missing := missingSections(times, video.fps)

	return &logoResult{
		sections:    trimMissing(complementSections(intervals, times, video.fps), missing),
		missing:     missing,
		duration:    time.Duration((times[len(times)-1] - times[0] + 1/video.fps) * float64(time.Second)),
		edgeMissing: edgeMissing(times, video),
		corner:      learned.Corner, box: learned.Box, measures: measures, intervals: intervals, times: times, fps: video.fps,
	}, nil
}

// edgeMissing は、ファイルの始まりから最初のコマまでと、最後のコマの終わりからファイルの終わり (ffprobe の長さ) までのうち、
// missingGap 以上空いたものの長さの合計を返す (#1508)。コマの時刻は、FFmpeg がファイルの始まりを 0 に寄せたものなので、
// 最初のコマの時刻が、そのまま始まりの空きになる。長さが分からなければ (ffprobe が長さを返さない TS)、終わりの空きは数えない。
func edgeMissing(times []float64, video *videoInfo) time.Duration {
	if len(times) == 0 {
		return 0
	}
	var total time.Duration
	if head := time.Duration(times[0] * float64(time.Second)); head >= missingGap {
		total += head
	}
	if video.duration > 0 {
		end := time.Duration((times[len(times)-1] + 1/video.fps) * float64(time.Second))
		if tail := video.duration - end; tail >= missingGap {
			total += tail
		}
	}

	return total
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
