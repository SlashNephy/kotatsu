package cmdetect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

const (
	// LogoDetectorName と logoDetectorVersion は、LogoDetector の名前と版。規則や値を変えたら版を上げる。
	// 版 3 で、縦が 1080 ラインより大きい映像 (BS4K、8K) も、縮めてロゴで見分けるようにした (#1505)。
	// 版 4 で、隅を横切る帯 (番組の話題の帯) の端のアイコンを、局ロゴとして覚えないようにした (#1883)。
	LogoDetectorName    = "logo"
	logoDetectorVersion = 4
	// ffprobeCommand は、PATH から探す ffprobe のコマンドの名前。FFmpeg に付いてくる。
	ffprobeCommand = "ffprobe"
	// rawReadBuffer は、FFmpeg が出す生のコマを読むバッファの大きさ。
	rawReadBuffer = 1 << 20
	// skipFrameOption は、デコードするコマを選ぶ FFmpeg の入力の引数。
	skipFrameOption = "-skip_frame"
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

// LogoDetector は、映像の隅の局ロゴが出ているかどうかで、CM の区間を見分ける (#1505、設計書 6.1)。今の見分け方より精度が高い代わりに、全コマを読むので、時間と CPU を多く使う。
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
	// signalsOf は、今の見分け方の材料 (無音などの境目の候補) を読む (#1506)。nil なら読まず、ロゴの区間の外を CM にする。
	signalsOf func(ctx context.Context, input *Input) (*signals, error)
}

var (
	_ Detector         = new(LogoDetector)
	_ Reporter         = new(LogoDetector)
	_ MaterialDetector = new(LogoDetector)
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

	return &LogoDetector{lowerer: lowerer, fallback: fallback, params: logo.DefaultParams(), signalsOf: fallback.signals}, nil
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
	sections, report, _, err := d.DetectWithMaterial(ctx, input)

	return sections, report, err
}

// maxLogoHeight は、局ロゴを縮めずに学習する映像の縦の上限 (ライン)。これより大きい映像は、縮めてから学習し測る (#1505)。
// パラメーター (膨らませる半径、括る距離、ずらす量など) を画素で持ち、地上波の 1440×1080 に合わせてあるためである。
// 2026 年 11 月に BS4K の録画で試したとき、4K のままでは、常に出ている局ロゴも学習できなかった。
const maxLogoHeight = 1080

// logoScale は、局ロゴを学習し測るときに、映像を縮める倍率を返す (#1505)。縦が maxLogoHeight 以下なら 1 (縮めない) で、
// 4K (2160 ライン) なら 2、8K (4320 ライン) なら 4。整数にするのは、縮めたコマの枠を、元のコマの枠に倍率を掛けるだけで移せるようにして、
// 学習 (縮めてから帯を切り出す) と全コマの読み出し (枠を切り出してから縮める) で、同じ画素になるようにするためである。
func logoScale(height int) int {
	return max(1, (height+maxLogoHeight-1)/maxLogoHeight)
}

// DetectWithMaterial は、DetectWithReport と同じ結果と、読み直さずに判定し直すための材料を返す (#1507)。
// ロゴを学習できなかったか、コマと時刻の数が合わなかった回は、材料を返さない (nil)。読み直したほうがよいからである。
func (d *LogoDetector) DetectWithMaterial(ctx context.Context, input *Input) ([]Section, *Report, *Material, error) {
	video, err := probeVideo(ctx, input)
	if err != nil {
		return nil, nil, nil, err
	}
	material, err := d.measure(ctx, input, video)
	if err != nil && !errors.Is(err, errLogoUnusable) {
		return nil, nil, nil, err
	}
	report := &Report{Note: "", Missing: 0}
	// 見分け直した結果の割合は、録画の長さで割る。長さは、ロゴで測れたならコマの時刻から、測れなければ ffprobe から求める。
	// 映像の欠けは外さない。今の見分け方の区間とは時刻の基準が違い、重なりを求められないためである。
	duration := video.duration
	if err == nil {
		var sections []Section
		sections, duration, err = d.decide(material, input.Program, report)
		if err == nil {
			return sections, report, material, nil
		}
	}
	report.Note = err.Error()
	sections, err := d.fallback.Detect(ctx, input)
	if err != nil {
		return nil, nil, nil, err
	}
	// 今の見分け方は、CM が 2 本以上続かないと CM にしないので、録画の前後のマージンに入った前後の番組を落とす。
	// ロゴを学習できて材料を読めていれば、番組の時刻に近い無音の境目で、前後を CM にする (#1506)。
	if material != nil && material.bounds != nil && input.Program != nil {
		sections = programEdges(sections, material.bounds, material.boundsDuration, input.Program, material.offset)
	}
	// 長さが分からなければ (ffprobe が長さを返さない TS)、割合を確かめられないので、今の見分け方の結果をそのまま使う。
	// 今の見分け方を選んだときと同じ結果になる。
	if duration > 0 {
		if ratio := cmRatio(sections, []Section{}, duration); ratio >= maxCMRatio {
			report.Note += fmt.Sprintf("; the redetected result was dropped: %.0f%% of the recording was detected as CM", ratio*100)

			return []Section{}, report, material, nil
		}
	}

	return sections, report, material, nil
}

// Redetect は、材料から、読み直さずに判定し直す (#1507)。ロゴで決められなければ (ロゴの出ていた区間がないか、
// CM の割合が上限以上なら)、今の見分け方で読み直す必要があるので、ok を false にする。
func (d *LogoDetector) Redetect(material *Material, program *Program) ([]Section, *Report, bool) {
	report := &Report{Note: "", Missing: 0}
	sections, _, err := d.decide(material, program, report)
	if err != nil {
		return nil, nil, false
	}

	return sections, report, true
}

// decide は、材料からロゴで決める。決められれば、区間を返し、report に映像の欠けの長さを書く。
// 決められなければ、errLogoUnusable を包んだ理由と、見分け直した結果の割合を割る長さ (コマの時刻から求めたもの) を返す。
func (d *LogoDetector) decide(material *Material, program *Program, report *Report) ([]Section, time.Duration, error) {
	result, err := d.evaluate(material, program)
	if err != nil {
		return nil, material.Duration, err
	}
	report.Missing = totalLength(result.missing) + result.edgeMissing
	ratio := cmRatio(clipSections(result.sections, result.scope), clipSections(result.missing, result.scope), result.scope.End-result.scope.Start)
	if ratio < maxCMRatio {
		return shiftSections(result.sections, material.playbackShift()), result.duration, nil
	}

	return nil, result.duration, fmt.Errorf("%w: %.0f%% of the recording was detected as CM (%s)", errLogoUnusable, ratio*100, result.detail(d.params))
}

// playbackShift は、最初のコマからの時間に足すと、再生の時間 (録画の索引の最初の I フレームを 0 にした時間) になる量 (秒) を返す (#1811)。
// 今の見分け方の基準 (sig.origin) は、索引と同じ探し方の最初の I フレームなので、最初のコマの、今の見分け方の基準での時刻になる。
// 録画の頭の I フレームの前で PMT が届いた録画では、FFmpeg がデコードする最初のコマ (ffprobe でも同じ) は、索引の最初の I フレームより
// 前にある (本番の録画で 0.5 秒)。今の見分け方の材料を読めなかった回は、基準が分からないので 0 にする。
func (m *Material) playbackShift() float64 {
	if m.bounds == nil {
		return 0
	}

	return m.Times[0] + m.offset
}

// shiftSections は、区間を shift 秒ずらす。0 より前は切り、長さがなくなった区間は捨てる。
func shiftSections(sections []Section, shift float64) []Section {
	out := make([]Section, 0, len(sections))
	d := seconds(shift)
	for _, s := range sections {
		if start, end := max(s.Start+d, 0), s.End+d; end > start {
			out = append(out, Section{Start: start, End: end})
		}
	}

	return out
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
	// scope は、CM の割合を数える範囲 (最初のコマからの時間)。番組の窓を置いたら窓、置かなければ録画全体 (#1506)。
	// 窓の外は前後の番組なので、CM にするのは当然で、割合に数えると、遅れて始まった放送で上限を超えてしまう。
	scope Section
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

// clipSections は、区間を scope に収める。scope の外の部分は切り、長さがなくなった区間は捨てる。
func clipSections(sections []Section, scope Section) []Section {
	out := make([]Section, 0, len(sections))
	for _, s := range sections {
		if start, end := max(s.Start, scope.Start), min(s.End, scope.End); end > start {
			out = append(out, Section{Start: start, End: end})
		}
	}

	return out
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

// Material は、1 つのファイルをロゴで見分けた材料 (#1507)。数値だけなので、DB に残して、読み直さずに判定し直せる。
type Material struct {
	// Fps は、コマの速さ。Duration は、ffprobe で読んだファイルの長さ (分からなければ 0)。
	Fps      float64
	Duration time.Duration
	// Corner と Box は、当てた型の隅と枠。理由の詳細に書く。
	Corner logo.Corner
	Box    logo.Box
	// Times は、コマごとの時刻 (秒)。Measures は、コマごとの測り。同じ数だけある。
	Times    []float64
	Measures []logo.Measure
	// bounds は、今の見分け方の材料から作った境目の候補 (#1506)。boundsDuration は、その基準での録画の長さ。
	// offset は、コマの時刻に足すと、今の見分け方の基準 (ファイルの最初の映像からの秒) になる量。材料を読めなかった回は、bounds が nil。
	bounds         []boundary
	boundsDuration float64
	offset         float64
}

// measure は、ロゴを学習し、全コマを測って、材料を返す。ロゴを学習できなければ、errLogoUnusable を包んで返す。
// 縦が maxLogoHeight より大きい映像は、logoScale の倍率で縮めて学習し測る (#1505)。枠や型は、縮めたコマの座標で持つ。
func (d *LogoDetector) measure(ctx context.Context, input *Input, video *videoInfo) (*Material, error) {
	p := d.params
	scale := logoScale(video.height)
	width, height := video.width/scale, video.height/scale
	top, bottom, err := d.learnBands(ctx, input, video, scale)
	if err != nil {
		return nil, err
	}
	learned, err := logo.Learn(top, bottom, width, height, p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLogoUnusable, err)
	}
	bandHeight := logo.Band(height, p)
	model, box := top, learned.Box
	if learned.Corner == logo.BottomRight || learned.Corner == logo.BottomLeft {
		model = bottom
		box.Y -= height - bandHeight
	}
	template, err := logo.NewTemplate(model.Crop(box), learned.Box, width, height, p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLogoUnusable, err)
	}

	// 今の見分け方の材料は、全コマを読む間に、別の FFmpeg で並べて読む (#1505)。材料を読む FFmpeg は 1 スレッドなので、
	// 続けて読むと、4K では、その分 (録画の長さの 0.2 倍ほど) がまるごと足される。
	// 読めなければ (音声のない録画など)、ロゴの区間の外を CM にする。ファイルの始まりが分からなければ、時刻をそろえられないので読まない。
	// 進み具合は、全コマを読む間だけ出すので、材料を読む FFmpeg からは出さない。
	signalsCtx, cancelSignals := context.WithCancel(ctx)
	defer cancelSignals()
	var wg sync.WaitGroup
	var sig *signals
	var sigErr error
	if d.signalsOf != nil && video.firstFrame >= 0 {
		wg.Go(func() {
			quiet := *input
			quiet.Progress = nil
			sig, sigErr = d.signalsOf(signalsCtx, &quiet)
		})
	}
	// 全コマのロゴの枠を切り出して測る。切り出しは exact=1 にする。付けないと、yuv420p で奇数の位置が丸められ、細い縁の一致が崩れる。
	pre, filter := cropFilter(template.Crop, scale)
	c := template.Crop
	measures := []logo.Measure{}
	times, err := d.frames(ctx, input, pre, filter, c.W*c.H, input.Progress, func(buf []byte) {
		measures = append(measures, template.Measure(&logo.Gray{Width: c.W, Height: c.H, Pix: buf}))
	})
	if err != nil {
		cancelSignals()
		wg.Wait()

		return nil, err
	}
	wg.Wait()
	// コマと時刻は順番で突き合わせるので、数が合わなければ、対応がずれている (#1505 の H)。
	if len(times) != len(measures) {
		return nil, fmt.Errorf("%w: %d frames but %d timestamps", errLogoUnusable, len(measures), len(times))
	}
	if len(times) == 0 {
		return nil, ErrNoVideo
	}
	// 縮める映像では、参照されないコマをデコードしないので、測ったコマの速さは映像の速さと違う。区間を出すときの窓やならしは、
	// コマの数で数えるので、測ったコマの速さを使う。
	fps := video.fps
	if scale > 1 {
		fps = measuredRate(times, video.fps)
	}

	material := &Material{
		Fps: fps, Duration: video.duration, Corner: learned.Corner, Box: learned.Box, Times: times, Measures: measures,
		bounds: nil, boundsDuration: 0, offset: 0,
	}
	// 止められたときだけは、止める。
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if sig != nil && sigErr == nil {
		material.bounds, material.boundsDuration = boundaries(sig), sig.duration
		// 今の見分け方の基準 (sig.origin) と、最初のコマの PTS の差でそろえる。
		material.offset = signalOffset(video.firstFrame, sig.origin, times[0])
	}

	return material, nil
}

// learnBands は、学習のキーフレームの上下の帯を読み、縁の向きを積算して返す。scale が 1 より大きければ、コマを scale 分の 1 に縮めてから帯を切り出す。
// キーフレームは、録画の長さを LearnFrames で割った間隔 (LearnMinInterval より詰めない) で取り出す。
func (d *LogoDetector) learnBands(ctx context.Context, input *Input, video *videoInfo, scale int) (*logo.Accumulator, *logo.Accumulator, error) {
	p := d.params
	width, height := video.width/scale, video.height/scale
	bandHeight := logo.Band(height, p)
	interval := max(p.LearnMinInterval.Seconds(), video.duration.Seconds()/float64(p.LearnFrames))
	bands := fmt.Sprintf("split[a][b];[a]crop=%d:%d:0:0:exact=1[t];[b]crop=%d:%d:0:%d:exact=1[u];[t][u]vstack",
		width, bandHeight, width, bandHeight, height-bandHeight)
	read := func(pre []string, filter string) (*logo.Accumulator, *logo.Accumulator, error) {
		top, bottom := logo.NewAccumulator(width, bandHeight), logo.NewAccumulator(width, bandHeight)
		half := width * bandHeight
		_, err := d.frames(ctx, input, pre, filter, 2*half, nil, func(buf []byte) {
			top.Add(&logo.Gray{Width: width, Height: bandHeight, Pix: buf[:half]}, p.EdgeStrength)
			bottom.Add(&logo.Gray{Width: width, Height: bandHeight, Pix: buf[half:]}, p.EdgeStrength)
		})

		return top, bottom, err
	}
	keyframes := []string{skipFrameOption, "nokey"}
	if scale == 1 {
		return read(keyframes, fmt.Sprintf("fps=1/%f,%s,format=gray", interval, bands))
	}
	// 縮める映像 (4K の HEVC) では、キーフレームを全部デコードすると、それだけで録画の長さの 0.2 倍ほどかかる。
	// デコードの前に、使う間隔のキーフレームだけを残す (keyframeThinning)。
	shrink := fmt.Sprintf("format=gray,scale=%d:%d:flags=area", width, height)
	top, bottom, err := read(slices.Concat(keyframes, []string{"-bsf:v", keyframeThinning(interval)}), shrink+","+bands)
	if err == nil || ctx.Err() != nil {
		return top, bottom, err
	}
	// 入力にビットストリームフィルターを掛けられない FFmpeg では、キーフレームを全部デコードして fps で間引く。
	return read(keyframes, fmt.Sprintf("fps=1/%f,%s,%s", interval, shrink, bands))
}

// keyframeThinning は、キーフレームを、前に残したものから interval 秒以上空いたものだけ残し、ほかのパケットを捨てる
// FFmpeg のビットストリームフィルター (noise の drop) を返す (#1505)。捨てたパケットはデコードしない。
// PTS が戻ったら (33 ビットの一周など)、そこから数え直す。PTS のないパケット (式では NaN) は、覚えた PTS を壊さないよう捨てる。
// 式の中の「,」は、フィルターの並びの区切りと見分けるため「\,」と書く。
func keyframeThinning(interval float64) string {
	return fmt.Sprintf(`noise=drop=if(key*not(isnan(pts))*(eq(ld(1)\,0)+gte((pts-ld(0))*tb\,%f)+lt(pts\,ld(0)))\,0*st(1\,1)*st(0\,pts)\,1)`, interval)
}

// cropFilter は、全コマから、ロゴの枠 c (縮めたコマの座標) を切り出す FFmpeg の入力の前の引数とフィルターを返す。
// scale が 1 より大きければ、元のコマから枠に倍率を掛けた範囲を切り出してから、枠の大きさに縮める。倍率が整数で、
// 縮め方 (area) は画素を平均するだけなので、学習で縮めてから切り出したコマと、同じ画素になる。
// 縮める映像では、参照されないコマ (4K の HEVC では半分) をデコードしない (-skip_frame noref)。測るコマは 30 コマ/秒ほどになり、
// 地上波の 1080i とそろう。
func cropFilter(c logo.Box, scale int) ([]string, string) {
	if scale == 1 {
		return []string{}, fmt.Sprintf("crop=%d:%d:%d:%d:exact=1,format=gray", c.W, c.H, c.X, c.Y)
	}

	return []string{skipFrameOption, "noref"}, fmt.Sprintf("crop=%d:%d:%d:%d:exact=1,format=gray,scale=%d:%d:flags=area",
		c.W*scale, c.H*scale, c.X*scale, c.Y*scale, c.W, c.H)
}

// measuredRate は、コマごとの時刻 times (秒) から、測ったコマの速さ (コマ/秒) を返す。映像の欠け (missingGap 以上の空き) は数えない。
// 求められなければ fallback を返す。
func measuredRate(times []float64, fallback float64) float64 {
	var span float64
	n := 0
	for i := 1; i < len(times); i++ {
		if gap := times[i] - times[i-1]; gap > 0 && gap < missingGap.Seconds() {
			span += gap
			n++
		}
	}
	if n == 0 {
		return fallback
	}

	return float64(n) / span
}

// evaluate は、材料から、ロゴの出ていた区間を出し、その外を CM にする。ロゴの出ていた区間が 1 つもなければ、errLogoUnusable を包んで返す。
// 番組の時刻 program が分かれば、番組の尺の窓を置き、窓の外のロゴを前後の番組のものとして捨てる (#1506 の手順 1)。
func (d *LogoDetector) evaluate(m *Material, program *Program) (*logoResult, error) {
	intervals := logo.Intervals(m.Measures, m.Fps, d.params)
	if len(intervals) == 0 {
		return nil, fmt.Errorf("%w: no frame shows the logo", errLogoUnusable)
	}
	missing := missingSections(m.Times, m.Fps)
	duration := time.Duration((m.Times[len(m.Times)-1] - m.Times[0] + 1/m.Fps) * float64(time.Second))
	scope := Section{Start: 0, End: duration}
	if w, ok := programWindow(intervals, m.Times, m.Fps, program); ok {
		intervals = clipIntervals(intervals, m.Times, w.start, w.end)
		if len(intervals) == 0 {
			return nil, fmt.Errorf("%w: no frame shows the logo in the program window", errLogoUnusable)
		}
		scope = Section{Start: seconds(w.start), End: seconds(w.end)}
	}
	sections := dropShortSections(trimMissing(complementSections(intervals, m.Times, m.Fps), missing), duration)
	if m.bounds != nil {
		if shown, ok := m.logoInCMChains(intervals); ok {
			return nil, fmt.Errorf("%w: the logo stays in %.0f%% of the CMs found by the silences", errLogoUnusable, shown*100)
		}
		sections = trimMissing(m.combine(intervals), missing)
	}

	return &logoResult{
		sections:    sections,
		missing:     missing,
		duration:    duration,
		scope:       scope,
		edgeMissing: edgeMissing(m.Times, &videoInfo{width: 0, height: 0, fps: m.Fps, duration: m.Duration, firstFrame: -1}),
		corner:      m.Corner, box: m.Box, measures: m.Measures, intervals: intervals, times: m.Times, fps: m.Fps,
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

// signalOffset は、局ロゴの見分け方のコマの時刻に足すと、今の見分け方の基準 (origin の PTS を 0 にした秒) になる量を返す。
// first は最初のコマの PTS、origin は今の見分け方の時刻の 0 の PTS (どちらも PTS の刻み)、firstTime は最初のコマの、コマの時刻。
// どちらも最初の I フレームなので、ふつうは差がないが、I フレームの探し方を知らない映像では、origin は最初の映像の PES になる。
// コマの時刻の 0 (FFmpeg がパイプの入力の始まりとしたもの) は、ffprobe で読むファイルの始まりと 0.3 秒ほどずれることがあるので、
// 最初のコマの PTS どうしでそろえる。PTS は 33 ビットで一周するので、差は一周を考えて求める。
func signalOffset(first, origin int64, firstTime float64) float64 {
	return float64(ts.PTSDiff(origin, first))/ts.PTSClock - firstTime
}

// maxLogoInCMChains は、無音の境目が 15 秒の倍数で続く並び (今の見分け方が CM とするもの) のうち、ロゴが出ていた秒の割合の上限。
// これを超えれば、局ロゴでないもの (CM の間も出し続ける時刻の表示など) を覚えたとみなし、ロゴを使わずに見分け直す (#1506、#1508)。
// CM の割合の上限 (maxCMRatio) は、CM が多すぎる結果を捨てるが、ロゴが CM の間も出続けて CM を見落とす結果は捨てられない。
const maxLogoInCMChains = 0.8

// logoInCMChains は、無音の境目が 15 秒の倍数で続く並びのうち、ロゴが出ていた秒の割合を返す。並びの長さの合計が、
// 15 秒の CM 2 本分に満たなければ、確かめられないので ok を false にする。割合が上限を超えたときだけ ok を true にする。
func (m *Material) logoInCMChains(intervals []logo.Interval) (float64, bool) {
	shown := m.signalSpans(intervals)
	var total, covered float64
	for _, c := range findChains(m.bounds) {
		a, b := m.bounds[c.first].at, m.bounds[c.last].at
		total += b - a
		for _, s := range shown {
			covered += max(0, min(s.end, b)-max(s.start, a))
		}
	}
	if total < minCMs*cmUnit {
		return 0, false
	}
	ratio := covered / total

	return ratio, ratio > maxLogoInCMChains
}

// signalSpans は、ロゴの出ていた区間を、今の見分け方の基準の秒にする。
func (m *Material) signalSpans(intervals []logo.Interval) []span {
	toSignal := func(frame int) float64 {
		t := m.Times[len(m.Times)-1] + 1/m.Fps
		if frame < len(m.Times) {
			t = m.Times[frame]
		}

		return t + m.offset
	}
	shown := make([]span, 0, len(intervals))
	for _, iv := range intervals {
		shown = append(shown, span{start: toSignal(iv.Start), end: toSignal(iv.End)})
	}

	return shown
}

// combine は、ロゴの出ていた区間と、今の見分け方の境目の候補から、本編と CM の並びを選び、CM の区間を、
// 最初のコマからの時間で返す (#1506)。
func (m *Material) combine(intervals []logo.Interval) []Section {
	// ロゴの区間を、今の見分け方の基準の秒にする。
	shown := m.signalSpans(intervals)
	// CM の区間を、最初のコマからの時間に戻す。最初のコマより前と、最後のコマの後ろは切る。
	origin := m.Times[0] + m.offset
	last := m.Times[len(m.Times)-1] - m.Times[0] + 1/m.Fps
	sections := []Section{}
	for _, s := range combineSections(m.bounds, m.boundsDuration, shown) {
		start, end := max(s.start-origin, 0), min(s.end-origin, last)
		if end > start {
			sections = append(sections, Section{Start: seconds(start), End: seconds(end)})
		}
	}

	return sections
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

// frames は、input の TS を FFmpeg でデコードし、最初の映像に filter を掛けたグレースケールの生のコマを、size バイトずつ fn に渡す。
// pre は、入力の前に付ける引数。コマは間引かず増やさない (-fps_mode passthrough)。
// コマごとの時刻 (秒) を、showinfo の pts_time から読んで返す。コマが落ちたり、時刻が飛んだりしても、コマの番号でなく時刻で区間を出すためである。
// progress を渡せば、FFmpeg の -progress のまとまりと、それまでにファイルから流したバイト数を渡す (#1511)。
func (d *LogoDetector) frames(
	ctx context.Context, input *Input, pre []string, filter string, size int, progress func(*ffprogress.Block, int64), fn func([]byte),
) ([]float64, error) {
	file, err := input.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	args := append([]string{"-nostdin", "-hide_banner", "-nostats", "-progress", "pipe:2", "-loglevel", "info"}, pre...)
	video, _ := input.streams()
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:"+video, "-vf", filter+",showinfo",
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

// showinfoFrame は、showinfo が出すコマの行の、コマの番号と時刻 (秒) の組。
var showinfoFrame = regexp.MustCompile(`n:\s*(\d+)\s+pts:\s*\S+\s+pts_time:(-?[0-9.]+)`)

// parseFrameLog は、FFmpeg の標準エラー出力から、showinfo のコマの時刻と、-progress のまとまりを読む。
//
// FFmpeg は、ログの頭 ([Parsed_showinfo_N @ 0x...]) と本文を別々に書くので、その間に -progress のまとまりが入ることがある。
// そのときは、時刻の入った本文が progress=continue の行の後ろにつながる。行の頭で見分けると、その時刻を落とし、
// コマと時刻の数が合わなくなる (#1800)。そこで、行のどこにあってもコマの番号と時刻の組を拾い、コマの番号で並べる。
// 組を取り除いた残りは、-progress の行やほかのログとして読む。
func parseFrameLog(r io.Reader, onBlock func(*ffprogress.Block)) *frameLog {
	log := &frameLog{times: []float64{}, tail: []string{}}
	byFrame := map[int]float64{}
	parser := ffprogress.NewParser(onBlock, func(line string) {
		log.tail = append(log.tail, line)
		if len(log.tail) > maxStderrLines {
			log.tail = log.tail[1:]
		}
	})
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			matches := showinfoFrame.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				n, nerr := strconv.Atoi(m[1])
				t, terr := strconv.ParseFloat(m[2], 64)
				if nerr == nil && terr == nil {
					byFrame[n] = t
				}
			}
			switch {
			case len(matches) > 0:
				_, _ = parser.Write([]byte(showinfoFrame.ReplaceAllString(line, "")))
			case strings.Contains(line, "Parsed_showinfo"):
				// showinfo がコマごとに出す、ほかの行 (色の情報など)。
			default:
				_, _ = parser.Write([]byte(line))
			}
		}
		// 読み残すと FFmpeg が書き込みで止まるので、最後まで読む。
		if err != nil {
			break
		}
	}
	parser.Flush()
	// コマの番号の順に並べる。番号の範囲を回すと、ログに紛れた大きな番号で長く回るので、拾った番号だけを並べ替える。
	// 抜けた番号があれば、コマと時刻の数が合わなくなり、ロゴを使わずに見分け直す。
	frames := slices.Sorted(maps.Keys(byFrame))
	for _, n := range frames {
		log.times = append(log.times, byFrame[n])
	}

	return log
}

// videoInfo は、録画の最初の映像の大きさと速さと、録画の長さ。
type videoInfo struct {
	width    int
	height   int
	fps      float64
	duration time.Duration
	// firstFrame は、ffprobe で読んだ最初のコマの PTS (90kHz の刻み)。今の見分け方の材料と、時刻の基準をそろえるのに使う (#1506)。
	// 分からなければ -1。
	firstFrame int64
}

// probeVideo は、ffprobe で、input の最初の映像の大きさと速さと、録画の長さと、最初のコマの PTS を読む。
func probeVideo(ctx context.Context, input *Input) (*videoInfo, error) {
	source := probeSource(input)
	video, _ := input.streams()
	out, err := runProbe(exec.CommandContext(ctx, ffprobeCommand, "-v", "error", "-select_streams", video,
		"-show_entries", "stream=width,height,avg_frame_rate:format=duration", "-of", "json", "-f", "mpegts", "-i", source), input)
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
	duration := time.Duration(seconds * float64(time.Second))
	if input.Open != nil {
		// パイプで渡した頭の部分の長さでなく、ファイルの長さにする。
		duration = input.Duration
	}

	return &videoInfo{
		width: s.Width, height: s.Height, fps: fps, duration: duration,
		firstFrame: probeFirstFrame(ctx, input),
	}, nil
}

// probeBytes は、パイプで ffprobe に渡す、ファイルの頭の大きさ。映像の大きさと速さ、最初のコマを読むには足りる。
const probeBytes = 32 << 20

// probeSource は、ffprobe に読ませる入力。Open で渡すファイルは、パイプで渡す。
func probeSource(input *Input) string {
	if input.Open != nil {
		return "pipe:0"
	}

	return input.Path
}

// runProbe は、ffprobe の cmd を動かし、標準出力を返す。Open で渡すファイルは、頭の probeBytes だけを標準入力に流す。
func runProbe(cmd *exec.Cmd, input *Input) ([]byte, error) {
	if input.Open != nil {
		file, err := input.open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		cmd.Stdin = io.LimitReader(file, probeBytes)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ffprobe: %w", err)
	}

	return out, nil
}

// probeFirstFrame は、ffprobe で、path の最初の映像の、最初にデコードできたコマの PTS (90kHz の刻み) を読む。
// 読めなければ (止められた、PTS が N/A など)、-1 を返し、今の見分け方の材料と組み合わせない。
// 頭の 2 秒だけを読む。最初の I フレームより前の PES はデコードできないので、最初のコマは、局ロゴの見分け方の最初のコマと同じになる。
func probeFirstFrame(ctx context.Context, input *Input) int64 {
	source := probeSource(input)
	video, _ := input.streams()
	out, err := runProbe(exec.CommandContext(ctx, ffprobeCommand, "-v", "error", "-select_streams", video, "-read_intervals", "%+2",
		"-show_entries", "frame=pts", "-of", "csv=p=0", "-f", "mpegts", "-i", source), input)
	if err != nil {
		return -1
	}
	line, _, _ := strings.Cut(string(out), "\n")
	pts, err := strconv.ParseInt(strings.Trim(strings.TrimSpace(line), ","), 10, 64)
	if err != nil || pts < 0 {
		return -1
	}

	return pts
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
