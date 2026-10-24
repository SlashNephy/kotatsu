// Package telemetry は、Prometheus 形式のメトリクスと OpenTelemetry のトレースを用意する。
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const serviceName = "kotatsu"

// NewRegistry は、Go のランタイムとプロセスのメトリクスを登録したレジストリを返す。
// 各ロールのメトリクスは、このレジストリに登録する。
func NewRegistry() *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return registry
}

// NewMetricsHandler は、registry の内容を Prometheus 形式で返す /metrics を作る。
func NewMetricsHandler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		Registry: registry,
	}))

	return mux
}

// NewTracerProvider は、OTLP/HTTP でトレースを送る TracerProvider を返す。
// 送信先が設定されていなければ、何も送らない TracerProvider を返す。
// 返す shutdown は、溜まっているトレースを送り切ってから止める。
func NewTracerProvider(ctx context.Context, cfg *config.Telemetry) (trace.TracerProvider, func(context.Context) error, error) {
	if cfg.OTLPEndpoint == "" {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}

	// 設定するのは OTEL_EXPORTER_OTLP_ENDPOINT と同じくベースの URL なので、トレースのパスを付ける。
	// WithEndpointURL は URL のパスをそのまま使い、付け足さない。
	endpoint, err := url.JoinPath(cfg.OTLPEndpoint, "v1", "traces")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid OTLP endpoint: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the OTLP trace exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
	)

	return provider, provider.Shutdown, nil
}
