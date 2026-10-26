package telemetry_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/SlashNephy/kotatsu/internal/infra/telemetry"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

// テストのレジストリに登録するメトリクスの名前。
const (
	instanceMetric = "kotatsu_kyusu_sessions"
	runtimeMetric  = "go_goroutines"
	clusterMetric  = "kotatsu_recordings"
)

// received は、OTLP の受け手に届いた 1 回分の送信。
type received struct {
	request *colmetricspb.ExportMetricsServiceRequest
	// header は、送信に付いていたヘッダー (gRPC ではメタデータ)。名前は小文字にする。
	header map[string]string
	// encoding は、HTTP の Content-Encoding。gRPC では空。
	encoding string
}

type grpcReceiver struct {
	colmetricspb.UnimplementedMetricsServiceServer

	ch chan received
}

func (r *grpcReceiver) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	header := map[string]string{}
	md, _ := metadata.FromIncomingContext(ctx)
	for name, values := range md {
		header[name] = values[0]
	}
	r.ch <- received{request: req, header: header, encoding: ""}

	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

// startGRPCReceiver は、OTLP/gRPC の受け手を立て、送信先の URL と、届いたものを受け取る channel を返す。
func startGRPCReceiver(t *testing.T) (string, <-chan received) {
	t.Helper()

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	receiver := &grpcReceiver{UnimplementedMetricsServiceServer: colmetricspb.UnimplementedMetricsServiceServer{}, ch: make(chan received, 16)}
	server := grpc.NewServer()
	colmetricspb.RegisterMetricsServiceServer(server, receiver)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return "http://" + listener.Addr().String(), receiver.ch
}

// startHTTPReceiver は、OTLP/HTTP の受け手を立て、送信先のベースの URL と、届いたものを受け取る channel を返す。
func startHTTPReceiver(t *testing.T) (string, <-chan received) {
	t.Helper()

	ch := make(chan received, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics" {
			http.NotFound(w, r)

			return
		}
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)

				return
			}
			body = gz
		}
		raw, err := io.ReadAll(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}
		req := &colmetricspb.ExportMetricsServiceRequest{}
		if err := proto.Unmarshal(raw, req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}
		header := map[string]string{}
		for name := range r.Header {
			header[http.CanonicalHeaderKey(name)] = r.Header.Get(name)
		}
		ch <- received{request: req, header: header, encoding: r.Header.Get("Content-Encoding")}
		w.Header().Set("Content-Type", "application/x-protobuf")
		out, _ := proto.Marshal(&colmetricspb.ExportMetricsServiceResponse{})
		_, _ = io.Copy(w, bytes.NewReader(out))
	}))
	t.Cleanup(server.Close)

	return server.URL, ch
}

// drain は、届いたものをすべて取り出す。shutdown の後に呼ぶので、待たない。
func drain(ch <-chan received) []received {
	out := []received{}
	for {
		select {
		case r := <-ch:
			out = append(out, r)
		default:
			return out
		}
	}
}

// resourceMetrics は、届いたリソースごとの、属性とメトリクスの名前。
type resourceMetrics struct {
	attributes map[string]string
	names      []string
}

func resourcesOf(all []received) []resourceMetrics {
	out := []resourceMetrics{}
	for _, r := range all {
		for _, rm := range r.request.GetResourceMetrics() {
			res := resourceMetrics{attributes: map[string]string{}, names: []string{}}
			for _, kv := range rm.GetResource().GetAttributes() {
				res.attributes[kv.GetKey()] = kv.GetValue().GetStringValue()
			}
			for _, sm := range rm.GetScopeMetrics() {
				for _, m := range sm.GetMetrics() {
					res.names = append(res.names, m.GetName())
				}
			}
			slices.Sort(res.names)
			out = append(out, res)
		}
	}

	return out
}

// names は、届いたメトリクスの名前を、リソースをまたいで返す。
func names(all []received) []string {
	out := []string{}
	for _, res := range resourcesOf(all) {
		out = append(out, res.names...)
	}
	slices.Sort(out)

	return out
}

func newRegistries(t *testing.T) (prometheus.Gatherer, prometheus.Gatherer) {
	t.Helper()

	instance, cluster := prometheus.NewRegistry(), prometheus.NewRegistry()
	instance.MustRegister(
		prometheus.NewGauge(prometheus.GaugeOpts{Name: instanceMetric, Help: "instance"}),
		prometheus.NewGauge(prometheus.GaugeOpts{Name: runtimeMetric, Help: "runtime"}),
	)
	cluster.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: clusterMetric, Help: "cluster"}))

	return instance, cluster
}

// TestNewMeterProviders は、Prometheus のレジストリの内容を、OTLP で送ることを確かめる (#962)。
func TestNewMeterProviders(t *testing.T) {
	t.Parallel()

	leader := func(context.Context) (bool, error) { return true, nil }
	follower := func(context.Context) (bool, error) { return false, nil }
	broken := func(context.Context) (bool, error) { return false, errors.New("database is down") }
	tests := []struct {
		name     string
		protocol string
		headers  string
		exclude  []string
		isLeader func(context.Context) (bool, error)
		check    func(t *testing.T, all []received)
	}{
		{
			name: "gRPC で届く", protocol: config.OTLPProtocolGRPC, headers: "Mackerel-Api-Key=test", isLeader: leader,
			check: func(t *testing.T, all []received) {
				t.Helper()
				require.NotEmpty(t, all)
				for _, r := range all {
					assert.Equal(t, "test", r.header["mackerel-api-key"])
				}
			},
		},
		{
			name: "HTTP で届く", protocol: config.OTLPProtocolHTTP, headers: "Mackerel-Api-Key=test", isLeader: leader,
			check: func(t *testing.T, all []received) {
				t.Helper()
				require.NotEmpty(t, all)
				for _, r := range all {
					assert.Equal(t, "gzip", r.encoding)
					assert.Equal(t, "test", r.header["Mackerel-Api-Key"])
				}
			},
		},
		{
			name: "リーダーは両方の範囲を送る", protocol: config.OTLPProtocolGRPC, isLeader: leader,
			check: func(t *testing.T, all []received) {
				t.Helper()
				resources := resourcesOf(all)
				require.Len(t, resources, 2)
				for _, res := range resources {
					switch {
					case slices.Contains(res.names, clusterMetric):
						// クラスターの範囲には、インスタンスも版も付けない。リーダーが替わっても系列が続くようにする。
						assert.Equal(t, map[string]string{"service.name": "kotatsu"}, res.attributes)
						assert.Equal(t, []string{clusterMetric}, res.names)
					default:
						assert.Equal(t, "kotatsu", res.attributes["service.name"])
						assert.Equal(t, "v1.2.3", res.attributes["service.version"])
						assert.NotEmpty(t, res.attributes["service.instance.id"])
						assert.Equal(t, "neko,tenban", res.attributes["kotatsu.roles"])
						assert.Equal(t, []string{runtimeMetric, instanceMetric}, res.names)
					}
				}
			},
		},
		{
			name: "リーダーでなければクラスターを送らない", protocol: config.OTLPProtocolGRPC, isLeader: follower,
			check: func(t *testing.T, all []received) {
				t.Helper()
				assert.Equal(t, []string{runtimeMetric, instanceMetric}, names(all))
			},
		},
		{
			name: "リーダーの判定に失敗したらクラスターを送らない", protocol: config.OTLPProtocolGRPC, isLeader: broken,
			check: func(t *testing.T, all []received) {
				t.Helper()
				assert.Equal(t, []string{runtimeMetric, instanceMetric}, names(all))
			},
		},
		{
			name: "除外", protocol: config.OTLPProtocolGRPC, exclude: []string{"go_*"}, isLeader: leader,
			check: func(t *testing.T, all []received) {
				t.Helper()
				assert.Equal(t, []string{instanceMetric, clusterMetric}, names(all))
			},
		},
		{
			name: "すべて除外しても失敗しない", protocol: config.OTLPProtocolGRPC, exclude: []string{"*"}, isLeader: leader,
			check: func(t *testing.T, all []received) {
				t.Helper()
				assert.Empty(t, names(all))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var endpoint string
			var ch <-chan received
			if tt.protocol == config.OTLPProtocolHTTP {
				endpoint, ch = startHTTPReceiver(t)
			} else {
				endpoint, ch = startGRPCReceiver(t)
			}
			instance, cluster := newRegistries(t)
			cfg := &config.Telemetry{
				OTLPEndpoint: "", OTLPMetricsEndpoint: endpoint, OTLPMetricsProtocol: tt.protocol, OTLPMetricsHeaders: config.Secret(tt.headers),
				OTLPMetricsIntervalSeconds: 3600, OTLPMetricsExclude: tt.exclude,
			}
			shutdown, err := telemetry.NewMeterProviders(t.Context(), cfg, &telemetry.MeterOptions{
				Instance: instance, Cluster: cluster, IsLeader: tt.isLeader,
				InstanceID: id.New(), Roles: []string{"neko", "tenban"}, Version: "v1.2.3", Logger: slog.New(slog.DiscardHandler),
			})
			require.NoError(t, err)
			// 止めるときに、溜まっている値を送り切る。
			require.NoError(t, shutdown(t.Context()))
			tt.check(t, drain(ch))
		})
	}
}

// TestNewMeterProvidersDisabled は、送信先を設定していなければ、何も送らないことを確かめる。
func TestNewMeterProvidersDisabled(t *testing.T) {
	t.Parallel()

	_, ch := startGRPCReceiver(t)
	instance, cluster := newRegistries(t)
	shutdown, err := telemetry.NewMeterProviders(t.Context(), &config.Telemetry{
		OTLPEndpoint: "", OTLPMetricsEndpoint: "", OTLPMetricsProtocol: config.OTLPProtocolGRPC, OTLPMetricsHeaders: "",
		OTLPMetricsIntervalSeconds: 60, OTLPMetricsExclude: []string{},
	}, &telemetry.MeterOptions{
		Instance: instance, Cluster: cluster, IsLeader: func(context.Context) (bool, error) { return true, nil },
		InstanceID: id.New(), Roles: []string{}, Version: "", Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	require.NoError(t, shutdown(t.Context()))
	assert.Empty(t, drain(ch))
}

// TestNewMeterProvidersWithoutReceiver は、受け手につながらなくても作れることを確かめる。送れなくても、ロールを止めない。
func TestNewMeterProvidersWithoutReceiver(t *testing.T) {
	t.Parallel()

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	instance, cluster := newRegistries(t)
	shutdown, err := telemetry.NewMeterProviders(t.Context(), &config.Telemetry{
		OTLPEndpoint: "", OTLPMetricsEndpoint: endpoint, OTLPMetricsProtocol: config.OTLPProtocolHTTP, OTLPMetricsHeaders: "",
		OTLPMetricsIntervalSeconds: 60, OTLPMetricsExclude: []string{},
	}, &telemetry.MeterOptions{
		Instance: instance, Cluster: cluster, IsLeader: func(context.Context) (bool, error) { return true, nil },
		InstanceID: id.New(), Roles: []string{}, Version: "", Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	// 止めるときの送り残しは送れないので、エラーになる。呼び出し側はログに出すだけにする。
	// exporter は既定で送り直し続けるので、止めるのに時間の上限を付ける。
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	assert.Error(t, shutdown(ctx))
}
