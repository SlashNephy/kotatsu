package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// fieldFrameRate は、フィールドごとに 1 コマにして出すときのコマ数 (59.94)。
	fieldFrameRate = 60000.0 / 1001
	// fieldBitrateNumerator と fieldBitrateDenominator は、60 コマで出すときに、画質のビットレートに掛ける比。
	// kyusu の 60fps の画質 (1080p60、720p60) と同じく、30fps のおよそ 1.4 倍にする。
	fieldBitrateNumerator   = 7
	fieldBitrateDenominator = 5
)

// frameRatePlan は、変換で出すコマ数の決め方 (#1509)。
type frameRatePlan struct {
	// fields は、インターレースをフィールドごとに 1 コマにして、59.94 コマで出すかどうか。
	fields bool
	// rate は、録画のファイルの記録に残す 1 秒あたりのコマ数。分からなければ 0 (記録しない)。
	rate int32
	// sourceRate は、最初のセグメントの映像のフレームレート。分からなければ 0。進み具合の見込みに使う。
	sourceRate float64
}

// outputRate は、変換した MP4 の映像のフレームレートの見込みを返す。分からなければ 0 を返す。
func (p *frameRatePlan) outputRate() float64 {
	if p.fields {
		return fieldFrameRate
	}

	return p.sourceRate
}

// recordedRate は、録画のファイルの記録に残すコマ数を返す。分からなければ nil を返す。
func (p *frameRatePlan) recordedRate() *int32 {
	if p.rate <= 0 {
		return nil
	}

	return new(p.rate)
}

// planFrameRate は、録画設定の出すコマ数 setting と、録画のセグメント segments から、変換で出すコマ数を決める (#1509)。
//   - 60 なら、フィールドごとに 1 コマにする。30 なら、コマごとに解除する。
//   - 自動なら、最初のセグメントの映像の形式 (#1310) がプログレッシブであれば、フィールドを倍にする話にならないので、
//     測らずに元のコマ数のまま出す (720p の局、BS4K の 59.94p)。インターレースか、形式が分からなければ、録画全体に散らした窓で
//     フィールドどうしの差を測り、多数決で決める。
//
// コマごとに解除するとき、プログレッシブの映像は、bwdif がそのまま通すので、元のコマ数のまま出る。
func (w *Worker) planFrameRate(ctx context.Context, encoder Encoder, logger *slog.Logger, setting string, segments []sqlc.RecordingFile) (*frameRatePlan, error) {
	format, rate := w.sourceFormat(ctx, encoder, logger, &segments[0])
	progressive := format != nil && !format.Interlaced
	plan := &frameRatePlan{fields: false, rate: 30, sourceRate: rate}
	if progressive {
		plan.rate = int32(math.Round(rate))
	}
	switch setting {
	case domain.TranscodeFrameRate60:
		plan.fields, plan.rate = true, 60
	case domain.TranscodeFrameRate30:
	default:
		if progressive {
			logger.InfoContext(ctx, "the video is progressive; keeping its frame rate", slog.Float64("frame_rate", rate))

			return plan, nil
		}
		fields, err := w.measureFieldOutput(ctx, encoder, logger, segments)
		if err != nil {
			return nil, err
		}
		if fields {
			plan.fields, plan.rate = true, 60
		}
	}

	return plan, nil
}

// sourceFormat は、最初のセグメント first の映像の形式と、フレームレートを返す。映像の形式がなければ nil を返す。
// HEVC は、映像の形式にフレームレートを読んでいない (0) ので、ffprobe で補う。分からなければ 0 を返す。
func (w *Worker) sourceFormat(ctx context.Context, encoder Encoder, logger *slog.Logger, first *sqlc.RecordingFile) (*ts.VideoFormat, float64) {
	var format *ts.VideoFormat
	if len(first.Video) > 0 {
		format = new(ts.VideoFormat)
		if err := json.Unmarshal(first.Video, format); err != nil {
			format = nil
		}
	}
	var rate float64
	if format != nil {
		rate = format.FrameRate
	}
	if rate <= 0 {
		if path, err := w.store.Path(first.PoolID, first.Path); err == nil {
			if rate, err = encoder.ProbeFrameRate(ctx, path); err != nil {
				logger.WarnContext(ctx, "failed to probe the frame rate", slog.Any("error", err))
			}
		}
	}

	return format, max(rate, 0)
}

// measureFieldOutput は、録画全体に散らした窓で、フィールドどうしの差を測り、60 コマで出すかどうかを多数決で決める (#1509)。
// 窓ごとの rep、wr、分類はログに出し、閾値を詰め直す材料にする。FFmpeg が失敗した窓は、測れなかった窓として数える。
func (w *Worker) measureFieldOutput(ctx context.Context, encoder Encoder, logger *slog.Logger, segments []sqlc.RecordingFile) (bool, error) {
	params := w.cadence
	spans := segmentSpans(segments)
	var total time.Duration
	for _, span := range spans {
		total += span.duration
	}
	results := []*WindowResult{}
	for _, at := range WindowPositions(total, params.Window, params.Windows) {
		segment, offset := locateWindow(spans, at, params.Window)
		result := &WindowResult{Rep: 0, RepSeconds: 0, WR: 0, WRSeconds: 0, Cadence: CadenceUnknown}
		path, err := w.store.Path(segment.file.PoolID, segment.file.Path)
		if err == nil {
			var diffs *FieldDiffs
			diffs, err = encoder.MeasureFields(ctx, path, offset, params.Window)
			if err == nil {
				result = params.Classify(diffs)
			}
		}
		if ctx.Err() != nil {
			return false, fmt.Errorf("failed to measure the fields: %w", ctx.Err())
		}
		attrs := []any{
			slog.Duration("position", at), slog.String("segment", segment.file.ID.String()), slog.Duration("offset", offset),
			slog.Float64("rep", result.Rep), slog.Int("rep_seconds", result.RepSeconds),
			slog.Float64("wr", result.WR), slog.Int("wr_seconds", result.WRSeconds), slog.String("cadence", string(result.Cadence)),
		}
		if err != nil {
			logger.WarnContext(ctx, "failed to measure a window of the fields", append(attrs, slog.Any("error", err))...)
		} else {
			logger.InfoContext(ctx, "measured a window of the fields", attrs...)
		}
		results = append(results, result)
	}
	fields := FieldOutput(results)
	logger.InfoContext(ctx, "chose the frame rate of the transcoding", slog.Int("windows", len(results)), slog.Bool("fields", fields))

	return fields, nil
}

// segmentSpan は、録画のセグメントと、その長さ。
type segmentSpan struct {
	file     *sqlc.RecordingFile
	duration time.Duration
}

// segmentSpans は、セグメントの長さを、記録した始まりと終わりから求める。終わりのないセグメントは 0 にする。
func segmentSpans(segments []sqlc.RecordingFile) []segmentSpan {
	spans := make([]segmentSpan, 0, len(segments))
	for i := range segments {
		var d time.Duration
		if segments[i].EndedAt != nil {
			d = max(segments[i].EndedAt.Sub(segments[i].StartedAt), 0)
		}
		spans = append(spans, segmentSpan{file: &segments[i], duration: d})
	}

	return spans
}

// locateWindow は、セグメントを並べた録画の位置 at に当たるセグメントと、その中の位置を返す。
// 窓がセグメントの終わりをはみ出すときは、セグメントの中に収まるよう前にずらす。
func locateWindow(spans []segmentSpan, at, window time.Duration) (*segmentSpan, time.Duration) {
	for i := range spans {
		span := &spans[i]
		if at < span.duration || i == len(spans)-1 {
			return span, max(min(at, span.duration-window), 0)
		}
		at -= span.duration
	}

	return nil, 0
}
