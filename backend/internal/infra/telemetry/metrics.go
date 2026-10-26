package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	otelprom "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

const (
	// leaderCheckTimeout は、送るたびに、リーダーかどうかを DB で確かめる時間の上限。
	leaderCheckTimeout = 5 * time.Second
	// flushTimeout は、止めるときに、溜まっている値を送り切るのを待つ時間の上限。
	// exporter は送れるまで送り直し続けるので、受け手が応答しないと、Pod の停止の猶予を使い切ってしまう。
	flushTimeout = 5 * time.Second
)

// MeterOptions は、OTLP でメトリクスを送るのに要るもの。
type MeterOptions struct {
	// Instance は、インスタンスごとの値。各インスタンスが送る。
	Instance prometheus.Gatherer
	// Cluster は、どのインスタンスでも同じになる、クラスター全体の値。IsLeader が true のインスタンスだけが送る。
	Cluster prometheus.Gatherer
	// IsLeader は、このインスタンスがクラスター全体の値を送るかどうか。エラーなら、その回は送らない。
	IsLeader func(ctx context.Context) (bool, error)
	// InstanceID、Roles、Version は、インスタンスの範囲のリソースの属性にする。
	InstanceID uuid.UUID
	Roles      []string
	Version    string
	Logger     *slog.Logger
}

// NewMeterProviders は、Prometheus のレジストリの内容を、OTLP で決まった間隔で送る (設計書 11.3)。
//
// 送る値は、インスタンスの範囲とクラスターの範囲に分け、範囲ごとに MeterProvider を作る。
// push では、インスタンスの数だけ同じ系列が送られるので、クラスター全体の値は、リーダーだけが、インスタンスの ID を付けずに送る。
// リーダーが替わっても、受け手から見て同じ系列のまま続く。
// 送信先が設定されていなければ、何もしない。返す shutdown は、溜まっている値を送り切ってから止める。
func NewMeterProviders(ctx context.Context, cfg *config.Telemetry, opts *MeterOptions) (func(context.Context) error, error) {
	if cfg.OTLPMetricsEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	headers, err := cfg.OTLPMetricsHeaderMap()
	if err != nil {
		return nil, fmt.Errorf("invalid OTLP metrics headers: %w", err)
	}

	scopes := []struct {
		gatherer prometheus.Gatherer
		resource *resource.Resource
	}{
		{
			gatherer: opts.Instance,
			resource: resource.NewSchemaless(
				attribute.String("service.name", serviceName),
				attribute.String("service.version", opts.Version),
				attribute.String("service.instance.id", opts.InstanceID.String()),
				attribute.String("kotatsu.roles", strings.Join(opts.Roles, ",")),
			),
		},
		{
			// クラスターの範囲には、インスタンスの ID も版も付けない。リーダーが替わっても、リリースしても、系列を続けるため。
			gatherer: &leaderGatherer{gatherer: opts.Cluster, isLeader: opts.IsLeader, logger: opts.Logger},
			resource: resource.NewSchemaless(attribute.String("service.name", serviceName)),
		},
	}
	providers := make([]*sdkmetric.MeterProvider, 0, len(scopes))
	for _, scope := range scopes {
		exporter, err := newMetricExporter(ctx, cfg, headers)
		if err != nil {
			return nil, err
		}
		reader := sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(time.Duration(cfg.OTLPMetricsIntervalSeconds)*time.Second),
			sdkmetric.WithProducer(otelprom.NewMetricProducer(otelprom.WithGatherer(
				&excludeGatherer{gatherer: scope.gatherer, patterns: cfg.OTLPMetricsExclude},
			))),
		)
		providers = append(providers, sdkmetric.NewMeterProvider(sdkmetric.WithResource(scope.resource), sdkmetric.WithReader(reader)))
	}

	return func(ctx context.Context) error {
		// 範囲ごとに順に止めると、先の範囲が待つ間に、後の範囲の期限が切れる。同じ期限で並べて止める。
		ctx, cancel := context.WithTimeout(ctx, flushTimeout)
		defer cancel()
		errs := make([]error, len(providers))
		var wg sync.WaitGroup
		for i, provider := range providers {
			wg.Go(func() { errs[i] = provider.Shutdown(ctx) })
		}
		wg.Wait()

		return errors.Join(errs...)
	}, nil
}

// newMetricExporter は、cfg の送り方で、gzip で圧縮して送る exporter を返す。
func newMetricExporter(ctx context.Context, cfg *config.Telemetry, headers map[string]string) (sdkmetric.Exporter, error) {
	if cfg.OTLPMetricsProtocol == config.OTLPProtocolHTTP {
		// トレースと同じく、ベースの URL にメトリクスのパスを付ける。
		endpoint, err := url.JoinPath(cfg.OTLPMetricsEndpoint, "v1", "metrics")
		if err != nil {
			return nil, fmt.Errorf("invalid OTLP metrics endpoint: %w", err)
		}
		exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint), otlpmetrichttp.WithHeaders(headers),
			otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
		if err != nil {
			return nil, fmt.Errorf("failed to create the OTLP/HTTP metric exporter: %w", err)
		}

		return exporter, nil
	}
	// http:// の送信先には、暗号化せずに送る (WithEndpointURL がスキームで決める)。
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.OTLPMetricsEndpoint), otlpmetricgrpc.WithHeaders(headers),
		otlpmetricgrpc.WithCompressor("gzip"))
	if err != nil {
		return nil, fmt.Errorf("failed to create the OTLP/gRPC metric exporter: %w", err)
	}

	return exporter, nil
}

// excludeGatherer は、名前がパターンに一致するメトリクスを落とす。橋渡しで変換する前に落として、変換の手間を省く。
type excludeGatherer struct {
	gatherer prometheus.Gatherer
	patterns []string
}

var _ prometheus.Gatherer = new(excludeGatherer)

func (g *excludeGatherer) Gather() ([]*dto.MetricFamily, error) {
	families, err := g.gatherer.Gather()
	kept := make([]*dto.MetricFamily, 0, len(families))
	for _, family := range families {
		if !g.excluded(family.GetName()) {
			kept = append(kept, family)
		}
	}

	return kept, err
}

func (g *excludeGatherer) excluded(name string) bool {
	for _, pattern := range g.patterns {
		// パターンは設定を読むときに確かめてあるので、エラーにはならない。
		if matched, _ := path.Match(pattern, name); matched {
			return true
		}
	}

	return false
}

// leaderGatherer は、リーダーのときだけ、gatherer の値を返す。
type leaderGatherer struct {
	gatherer prometheus.Gatherer
	isLeader func(ctx context.Context) (bool, error)
	logger   *slog.Logger
}

var _ prometheus.Gatherer = new(leaderGatherer)

func (g *leaderGatherer) Gather() ([]*dto.MetricFamily, error) {
	// Gather には context がないので、時間の上限を付けた context を作る。
	ctx, cancel := context.WithTimeout(context.Background(), leaderCheckTimeout)
	defer cancel()
	leader, err := g.isLeader(ctx)
	if err != nil {
		// リーダーか分からないときは、重複して送らないよう、その回は送らない。
		g.logger.WarnContext(ctx, "failed to check whether this instance is the leader; skipped exporting cluster metrics", slog.Any("error", err))

		return []*dto.MetricFamily{}, nil
	}
	if !leader {
		return []*dto.MetricFamily{}, nil
	}

	return g.gatherer.Gather()
}
