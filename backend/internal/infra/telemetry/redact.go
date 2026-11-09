package telemetry

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const urlPathKey = attribute.Key("url.path")

// redactedSegment は、url.path の ID らしいセグメントの置き換え先。
const redactedSegment = ":id"

// keptPathSegments は、url.path の先頭から残すセグメントの数。/streams/recordings のように、機能が分かる範囲だけを残す。
const keptPathSegments = 2

// isDroppedAttribute は、視聴者を識別できるので送らない属性かを返す。network.peer.* は、手前にプロキシがなければ視聴者の IP アドレスになる。
func isDroppedAttribute(key attribute.Key) bool {
	switch key {
	case "client.address", "client.port", "network.peer.address", "network.peer.port", "user_agent.original", "user_agent.synthetic.type":
		return true
	default:
		return false
	}
}

// redactingExporter は、外の送信先に渡す直前に、span から視聴者を識別できる属性と、何を再生したかが分かるパスの ID を除く (#1364)。
type redactingExporter struct {
	sdktrace.SpanExporter

	// recorder は、外部との通信の記録 (#1369)。送信先は HTTP クライアントを通らないので、送るたびに数える。
	recorder *netactivity.Recorder
}

// redactedSpan は、属性だけを差し替えた span。
type redactedSpan struct {
	sdktrace.ReadOnlySpan

	attributes []attribute.KeyValue
}

// Attributes は、除いた後の属性を返す。
func (s redactedSpan) Attributes() []attribute.KeyValue {
	return s.attributes
}

// ExportSpans は、属性を除いた span を送る。
func (e redactingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	redacted := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		redacted = append(redacted, redactedSpan{ReadOnlySpan: span, attributes: redactAttributes(span.Attributes())})
	}
	err := e.SpanExporter.ExportSpans(ctx, redacted)
	e.recorder.Add(netactivity.PurposeTelemetryTraces, "", err == nil, netactivity.Totals{Count: 1, SentBytes: 0, ReceivedBytes: 0})
	if err != nil {
		return fmt.Errorf("failed to export spans: %w", err)
	}

	return nil
}

func redactAttributes(attributes []attribute.KeyValue) []attribute.KeyValue {
	redacted := make([]attribute.KeyValue, 0, len(attributes))
	for _, kv := range attributes {
		if isDroppedAttribute(kv.Key) {
			continue
		}
		if kv.Key == urlPathKey {
			kv = urlPathKey.String(redactPath(kv.Value.AsString()))
		}
		redacted = append(redacted, kv)
	}

	return redacted
}

// redactPath は、先頭の keptPathSegments 個のセグメントだけを残し、後ろと、数字を含むセグメントを :id にまとめる。
// 録画やサービスの ID、ユーザー名などから、何を再生したかや誰を見ているかが分からないようにする。
func redactPath(path string) string {
	segments := strings.Split(path, "/")
	kept := 0
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		if kept >= keptPathSegments || strings.ContainsFunc(segment, unicode.IsDigit) {
			segments[i] = redactedSegment
		}
		kept++
	}

	return strings.Join(segments, "/")
}
