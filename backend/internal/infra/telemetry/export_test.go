package telemetry

import sdktrace "go.opentelemetry.io/otel/sdk/trace"

// NewRedactingExporter は、テストのために redactingExporter を作る。
func NewRedactingExporter(exporter sdktrace.SpanExporter) sdktrace.SpanExporter {
	return redactingExporter{SpanExporter: exporter, recorder: nil}
}
