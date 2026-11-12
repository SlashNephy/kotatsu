package transcode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// fieldDiffGraph は、窓のフィールドどうしの輝度の差を出すフィルタ (#1509)。主映像に固定する ([0:v:0])。地上波の TS には
// ワンセグ (320×180 の H.264) が残っていて、指定しないとフィルタがそちらに当たることがあるため。
// 1 本目は、上フィールドだけを並べ、前の上フィールドとの差を出す (1 秒に 30 個)。2 本目は、上下のフィールドを交互に並べ、
// 隣との差を出す (1 秒に 60 個)。出力の区別は、フィルタに付けた名前 (metadata@top と metadata@fields) で行う。
// 番号 (Parsed_metadata_N) はフィルタの並びで変わるため。
const fieldDiffGraph = "[0:v:0]split[a][b];" +
	"[a]field=type=top,signalstats,metadata@top=mode=print:key=lavfi.signalstats.YDIF[top];" +
	"[b]separatefields,signalstats,metadata@fields=mode=print:key=lavfi.signalstats.YDIF[fields]"

const (
	// topLogPrefix と fieldsLogPrefix は、metadata のフィルタが標準エラー出力に書く行の頭。
	topLogPrefix    = "[metadata@top @ "
	fieldsLogPrefix = "[metadata@fields @ "
	// ydifKey は、metadata のフィルタが書く、輝度の差の値の頭。
	ydifKey = "lavfi.signalstats.YDIF="
)

var errNoFieldDiff = errors.New("FFmpeg reported no field difference")

// MeasureFields は、TS のファイル path の、offset から length の間の、フィールドどうしの輝度の差を測る (#1509)。
// 入力の前の -ss で、ファイルを直接シークして読む。
func (e *FFmpegEncoder) MeasureFields(ctx context.Context, path string, offset, length time.Duration) (*FieldDiffs, error) {
	start, duration := formatSeconds(offset), formatSeconds(length)
	cmd := exec.CommandContext(ctx, ffmpegCommand, "-hide_banner", "-nostdin", "-nostats", "-loglevel", "info",
		"-ss", start, "-t", duration, "-i", path, "-filter_complex", fieldDiffGraph,
		"-map", "[top]", "-fps_mode", "passthrough", "-f", "null", "-",
		"-map", "[fields]", "-fps_mode", "passthrough", "-f", "null", "-")
	parser := new(fieldDiffParser)
	cmd.Stderr = parser
	if err := e.lowerer.Run(cmd); err != nil {
		return nil, fmt.Errorf("failed to measure the fields: %w: %s", err, parser.tail.String())
	}

	return parser.diffs()
}

// formatSeconds は、FFmpeg の時間の引数を、秒の小数で書く。
func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// fieldDiffParser は、FFmpeg の標準エラー出力から、metadata のフィルタが書いた輝度の差を読む。
// ほかの行は、失敗の説明に添えるため、最後の stderrLimit バイトだけを残す。
type fieldDiffParser struct {
	partial []byte
	top     []float64
	fields  []float64
	tail    bytes.Buffer
}

func (p *fieldDiffParser) Write(b []byte) (int, error) {
	p.partial = append(p.partial, b...)
	for {
		i := bytes.IndexByte(p.partial, '\n')
		if i < 0 {
			break
		}
		p.line(string(bytes.TrimRight(p.partial[:i], "\r")))
		p.partial = p.partial[i+1:]
	}

	return len(b), nil
}

// line は、標準エラー出力の 1 行を読む。値の行 (例: [metadata@top @ 0x55d0] lavfi.signalstats.YDIF=1.25) だけを拾い、
// コマの行 (frame:0 pts:0 pts_time:0) は捨てる。
func (p *fieldDiffParser) line(s string) {
	var dest *[]float64
	switch {
	case strings.HasPrefix(s, topLogPrefix):
		dest = &p.top
	case strings.HasPrefix(s, fieldsLogPrefix):
		dest = &p.fields
	default:
		p.tail.WriteString(s + "\n")
		if over := p.tail.Len() - stderrLimit; over > 0 {
			p.tail.Next(over)
		}

		return
	}
	_, raw, ok := strings.Cut(s, ydifKey)
	if !ok {
		return
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		*dest = append(*dest, v)
	}
}

// diffs は、読んだ差を返す。どちらの列も、先頭の値は前がないので捨てる。
func (p *fieldDiffParser) diffs() (*FieldDiffs, error) {
	if len(p.top) == 0 || len(p.fields) == 0 {
		return nil, fmt.Errorf("%w: %s", errNoFieldDiff, p.tail.String())
	}

	return &FieldDiffs{Top: p.top[1:], Fields: p.fields[1:]}, nil
}
