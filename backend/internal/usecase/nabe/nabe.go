// Package nabe は、EDCB の NetworkTV の TS を、ほかのロールに配信する役 (nabe) を扱う (#1535、設計書 1.1、3.1)。
//
// EDCB は、同じ局でも NetworkTV ごとにチューナーを使い、1 本の TS を受け取れるのも 2 つまでである。ロールを分けた構成で、
// mikan と tenban がそれぞれ NetworkTV を開くと、同じサービスでもチューナーを 2 本使う。そこで、nabe だけが NetworkTV を開き、
// サービスごとに 1 本の TS を、Mirakurun 互換の API (web/mirakurunapi) でほかのロールに配信する。
//
// ほかのロールは、インスタンスの登録簿で nabe を引き、サービスのストリームだけを nabe から受け取る (Streams)。
// サービスと番組、チューナー、イベント、ロゴは、今までどおり EDCB から直接読む。nabe がいないか、届かないときは、
// 今までどおり自分で NetworkTV を開く。
package nabe

import (
	"cmp"
	"context"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/innertoken"
)

const (
	// Role は、nabe のロールの名前。インスタンスの登録簿で、nabe を検索するのに使う。
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

// Instances は、ロールの動いているインスタンスを取得する。*registry.Registry が満たす。
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

// Streams は、ストリームを nabe から受け取る。動いている nabe がいないか、どの nabe にも届かなければ、local で開く。
//   - nabe が何台いても、同じサービスは同じ nabe から受け取るよう、サービスから nabe の順番を決め、先頭の nabe から受け取る。
//     ばらばらに受け取ると、nabe ごとに NetworkTV の中継を使う。
//   - 先頭の nabe に届かないか、ほかの理由で拒否されたとき (秘密キーが違うなど) は、次の nabe から受け取る (#1850)。
//     次の nabe は、先頭の nabe と同じ NetworkTV の ID に加わるので、チューナーを増やさない。届かなかった nabe は、
//     登録簿から外れるまで (registry.AliveTTL) 後回しにし、つなぎ直すたびに待たされないようにする。
//   - nabe が、チューナーが足りないか、サービスがないとして拒否したときは、そのまま返す。自分で開いても、同じ理由で開けないためである。
//   - 登録簿を引けないとき、どの nabe にも届かないときは、local で開く。録画を止めないことを、チューナーを余分に使わないことより先にする。
type Streams struct {
	instances Instances
	token     *innertoken.Token
	local     StreamOpener
	clock     clock.Clock
	logger    *slog.Logger

	mu sync.Mutex
	// clients は、nabe のアドレスごとのクライアント。
	clients map[string]*mirakurun.Client
	// failed は、届かなかった nabe と、その時刻。
	failed map[uuid.UUID]time.Time
}

// NewStreams は Streams を返す。token は nabe へのリクエストに付ける。
func NewStreams(instances Instances, token *innertoken.Token, local StreamOpener, clk clock.Clock, logger *slog.Logger) *Streams {
	return &Streams{
		instances: instances, token: token, local: local, clock: clk, logger: logger,
		mu: sync.Mutex{}, clients: map[string]*mirakurun.Client{}, failed: map[uuid.UUID]time.Time{},
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
	for _, instance := range s.order(instances, key) {
		client, err := s.client(instance.Address)
		if err != nil {
			s.logger.WarnContext(ctx, "the nabe address is invalid; trying the next one",
				slog.String("instance", instance.Address), slog.Any("error", err))
			s.fail(instance)

			continue
		}
		stream, err := open(client)
		if err == nil || errors.Is(err, tunerserver.ErrTunerUnavailable) || errors.Is(err, tunerserver.ErrNotFound) || ctx.Err() != nil {
			if err == nil {
				s.succeed(instance)
			}

			return stream, err
		}
		s.logger.WarnContext(ctx, "failed to open the stream from nabe; trying the next one",
			slog.String("instance", instance.Address), slog.Any("error", err))
		s.fail(instance)
	}
	if len(instances) > 0 {
		s.logger.WarnContext(ctx, "no nabe could open the stream; opening the NetworkTV by itself")
	}

	return open(s.local)
}

// order は、key から決まった順に nabe を並べる (rendezvous hashing)。nabe が増えたり減ったりしても、ほかの nabe が受け持つサービスの
// 先頭は動かない。届かなかった nabe は、AliveTTL の間、後ろに回す。
func (s *Streams) order(instances []*registry.Instance, key string) []*registry.Instance {
	ranked := rank(instances, key)
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	healthy := make([]*registry.Instance, 0, len(ranked))
	var failed []*registry.Instance
	for _, instance := range ranked {
		if at, ok := s.failed[instance.ID]; ok && now.Sub(at) < registry.AliveTTL {
			failed = append(failed, instance)

			continue
		}
		delete(s.failed, instance.ID)
		healthy = append(healthy, instance)
	}

	return append(healthy, failed...)
}

// fail は、instance に届かなかったことを保存する。
func (s *Streams) fail(instance *registry.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[instance.ID] = s.clock.Now()
}

// succeed は、instance に届いたので、届かなかったことを忘れる。
func (s *Streams) succeed(instance *registry.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failed, instance.ID)
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

// rank は、key から決まった順に nabe を並べる (rendezvous hashing)。スコアの高い順で、同じスコアなら ID の順にする。
func rank(instances []*registry.Instance, key string) []*registry.Instance {
	scores := make(map[uuid.UUID]uint64, len(instances))
	for _, instance := range instances {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(key + "\n" + instance.ID.String()))
		scores[instance.ID] = hash.Sum64()
	}
	ranked := slices.Clone(instances)
	slices.SortStableFunc(ranked, func(a, b *registry.Instance) int {
		if c := cmp.Compare(scores[b.ID], scores[a.ID]); c != 0 {
			return c
		}

		return strings.Compare(a.ID.String(), b.ID.String())
	})

	return ranked
}
