// Package nabe は、EDCB の NetworkTV の TS を、ほかのロールに配る役 (nabe) を扱う (#1535、設計書 1.1、3.1)。
//
// EDCB は、同じ局でも NetworkTV ごとにチューナーを使い、1 本の TS を受け取れるのも 2 つまでである。ロールを分けた構成で、
// mikan と tenban がそれぞれ NetworkTV を開くと、同じサービスでもチューナーを 2 本使う。そこで、nabe だけが NetworkTV を開き、
// サービスごとに 1 本の TS を、Mirakurun 互換の API (mirakurunapi) でほかのロールに配る。
//
// ほかのロールは、インスタンスの登録簿で nabe を引き、サービスのストリームだけを nabe から受け取る (Streams)。
// サービスと番組、チューナー、イベント、ロゴは、今までどおり EDCB から直接読む。nabe がいないか、届かないときは、
// 今までどおり自分で NetworkTV を開く。
package nabe

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurunapi"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/innertoken"
)

const (
	// Role は、nabe のロールの名前。インスタンスの登録簿で、nabe を引くのに使う。
	Role = "nabe"
	// PathPrefix は、nabe の Mirakurun 互換の API のパスの前置き。インスタンスのサーバーに載せる。
	PathPrefix = "/internal/nabe/"
	// derivePurpose は、アプリの秘密キーから、ほかのロールが nabe に送るトークンを導くときの用途。
	derivePurpose = "kotatsu/nabe/v1"
)

// NewToken は、アプリの秘密キー secret から、nabe へのリクエストを認可するトークンを導く。
// 秘密キーがなければ innertoken.ErrNoKey を返す。
func NewToken(secret string) (*innertoken.Token, error) {
	return innertoken.New(secret, derivePurpose)
}

// Handler は、source を Mirakurun 互換の API として、PathPrefix の下に公開するハンドラーを返す。token を付けたリクエストだけ受け付ける。
func Handler(source tunerserver.Client, token *innertoken.Token, version string, logger *slog.Logger) http.Handler {
	return token.Middleware(http.StripPrefix(strings.TrimSuffix(PathPrefix, "/"), mirakurunapi.NewServer(source, version, logger)))
}

// Instances は、ロールの動いているインスタンスを引く。*registry.Registry が満たす。
type Instances interface {
	ListAlive(ctx context.Context, role string) ([]*registry.Instance, error)
}

var _ Instances = new(registry.Registry)

// StreamOpener は、サービスとチャンネルのストリームを開く。
type StreamOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error)
	OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error)
}

var (
	_ StreamOpener = new(mirakurun.Client)
	_ StreamOpener = new(Streams)
)

// Streams は、ストリームを nabe から受け取る。動いている nabe がいないか、届かなければ、local で開く。
//   - nabe が何台いても、同じサービスは同じ nabe から受け取るよう、サービスから nabe を決める。nabe ごとに NetworkTV を開くので、
//     ばらばらに受け取ると、同じサービスにチューナーを何本も使う。
//   - nabe が、チューナーが足りないか、サービスがないと断ったときは、そのまま返す。自分で開いても、同じ理由で開けないためである。
//   - 登録簿を引けないとき、nabe に届かないとき、ほかの理由で断られたとき (秘密キーが違うなど) は、
//     local で開く。録画を止めないことを、チューナーを余分に使わないことより先にする。
type Streams struct {
	instances Instances
	token     *innertoken.Token
	local     StreamOpener
	clock     clock.Clock
	logger    *slog.Logger

	mu sync.Mutex
	// clients は、nabe のアドレスごとのクライアント。
	clients map[string]*mirakurun.Client
}

// NewStreams は Streams を返す。token は nabe へのリクエストに付ける。
func NewStreams(instances Instances, token *innertoken.Token, local StreamOpener, clk clock.Clock, logger *slog.Logger) *Streams {
	return &Streams{
		instances: instances, token: token, local: local, clock: clk, logger: logger,
		mu: sync.Mutex{}, clients: map[string]*mirakurun.Client{},
	}
}

// OpenServiceStream は、サービスのストリームを、nabe から受け取る。nabe がいないか届かなければ、自分で開く。
func (s *Streams) OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return s.open(ctx, strconv.FormatInt(serviceID, 10), func(opener StreamOpener) (io.ReadCloser, error) {
		return opener.OpenServiceStream(ctx, serviceID, opts)
	})
}

// OpenChannelStream は、チャンネルのストリームを、nabe から受け取る。nabe がいないか届かなければ、自分で開く。
func (s *Streams) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return s.open(ctx, channelType+"/"+channel, func(opener StreamOpener) (io.ReadCloser, error) {
		return opener.OpenChannelStream(ctx, channelType, channel, opts)
	})
}

func (s *Streams) open(ctx context.Context, key string, open func(StreamOpener) (io.ReadCloser, error)) (io.ReadCloser, error) {
	instances, err := s.instances.ListAlive(ctx, Role)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.logger.WarnContext(ctx, "failed to list nabe instances; opening the NetworkTV by itself", slog.Any("error", err))

		return open(s.local)
	}
	if len(instances) == 0 {
		return open(s.local)
	}
	instance := pick(instances, key)
	client, err := s.client(instance.Address)
	if err != nil {
		s.logger.WarnContext(ctx, "the nabe address is invalid; opening the NetworkTV by itself",
			slog.String("instance", instance.Address), slog.Any("error", err))

		return open(s.local)
	}
	stream, err := open(client)
	if err == nil || errors.Is(err, tunerserver.ErrTunerUnavailable) || errors.Is(err, tunerserver.ErrNotFound) || ctx.Err() != nil {
		return stream, err
	}
	s.logger.WarnContext(ctx, "failed to open the stream from nabe; opening the NetworkTV by itself",
		slog.String("instance", instance.Address), slog.Any("error", err))

	return open(s.local)
}

// client は、address の nabe のクライアントを返す。
func (s *Streams) client(address string) (*mirakurun.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, ok := s.clients[address]; ok {
		return client, nil
	}
	client, err := mirakurun.New(&mirakurun.Options{
		URL: "http://" + address + PathPrefix, Username: "", Password: "", UserAgent: "", HTTPClient: nil,
		RequestTimeout: 0, DetectInterval: 0, StreamIdleTimeout: 0, StreamOpenTimeout: 0, Clock: s.clock, Authorize: s.token.Authorize,
	})
	if err != nil {
		return nil, err
	}
	s.clients[address] = client

	return client, nil
}

// pick は、key から決まった nabe を返す (rendezvous hashing)。nabe が増えたり減ったりしても、ほかの nabe が受け持つサービスは動かない。
func pick(instances []*registry.Instance, key string) *registry.Instance {
	var best *registry.Instance
	var bestScore uint64
	for _, instance := range instances {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(key + "\n" + instance.ID.String()))
		if score := hash.Sum64(); best == nil || score > bestScore || (score == bestScore && instance.ID.String() < best.ID.String()) {
			best, bestScore = instance, score
		}
	}

	return best
}
