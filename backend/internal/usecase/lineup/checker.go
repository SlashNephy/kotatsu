package lineup

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

// DefaultCheckInterval は、放送の構成とチューナーサーバーの一覧を比べる間隔 (設計書 付録 A)。
// 知らせるのは、同じ version の食い違いを DefaultRecheckInterval あけて 2 回読んだときなので、それより十分短くする。
const DefaultCheckInterval = 10 * time.Minute

// kindTerrestrial は、地上波の知らせの種別。
const kindTerrestrial = "GR"

// systemStatusURL は、知らせを押したときに開く、システムの状態の画面のパス。
const systemStatusURL = "/admin/system"

// ServiceLister は、チューナーサーバーのサービスの一覧を返す。tunerserver.Client が満たす。
type ServiceLister interface {
	Services(ctx context.Context) ([]*tunerserver.Service, error)
}

var _ ServiceLister = tunerserver.Client(nil)

// Checker は、mikan が DB に書いた衛星の構成 (broadcast_lineups) と地上波のネットワークのサービス (terrestrial_lineups) を、
// チューナーサーバーのテレビのサービスと定期的に比べ、食い違いを管理者に知らせる (#1519、設計書 6.2)。neko で動かす。
//
// 比べた状態 (衛星の確かめ済みの version と 1 回目に見た食い違い、地上波の知らせた食い違い) は、プロセスの中だけに持つ。
// 再起動したら、続いている食い違いも、もう一度知らせる (衛星は 2 回読み直したところで)。
type Checker struct {
	pool     *pgxpool.Pool
	services ServiceLister
	clock    clock.Clock
	logger   *slog.Logger
	interval time.Duration
	comparer *Comparer
	// known は、前に比べたチューナーサーバーのテレビのサービスの集合の指紋。変わったら (スキャンし直したら)、比べ直す。
	known string
	// terrestrial は、地上波のネットワークごとの、知らせた食い違い。同じ食い違いを繰り返し知らせない。
	terrestrial map[uint16]string
}

// NewChecker は、Checker を返す。interval が 0 以下なら DefaultCheckInterval を使う。
func NewChecker(pool *pgxpool.Pool, services ServiceLister, clk clock.Clock, logger *slog.Logger, interval time.Duration) *Checker {
	if interval <= 0 {
		interval = DefaultCheckInterval
	}

	return &Checker{
		pool: pool, services: services, clock: clk, logger: logger, interval: interval,
		comparer: NewComparer(clk, DefaultRecheckInterval), known: "", terrestrial: map[uint16]string{},
	}
}

// Run は、ctx が終わるまで、interval ごとに比べる。比べるのに失敗しても止まらず、ログに出して次の回を待つ。
func (c *Checker) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		if err := c.Check(ctx); err != nil && ctx.Err() == nil {
			c.logger.WarnContext(ctx, "failed to check the broadcast lineups", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Check は、1 回比べ、知らせる食い違いがあれば、有効な管理者に知らせる。
// チューナーサーバーが空の一覧を返したときは、何もしない。起動直後や不調で、空を返すことがあるからである。
func (c *Checker) Check(ctx context.Context) error {
	services, err := c.services.Services(ctx)
	if err != nil {
		return fmt.Errorf("failed to list the services of the tuner server: %w", err)
	}
	if len(services) == 0 {
		return nil
	}
	q := sqlc.New(c.pool)
	if err := c.checkSatellites(ctx, q, registeredTV(services)); err != nil {
		return err
	}

	return c.checkTerrestrial(ctx, q, services)
}

// checkSatellites は、衛星の構成を、チューナーサーバーの衛星のテレビのサービス known と比べる。known がなければ比べない。
func (c *Checker) checkSatellites(ctx context.Context, q *sqlc.Queries, known []Known) error {
	if len(known) == 0 {
		return nil
	}
	if fingerprint := fingerprintOf(known); fingerprint != c.known {
		c.comparer.Forget()
		c.known = fingerprint
	}
	observations, err := Load(ctx, q)
	if err != nil {
		return fmt.Errorf("failed to load the broadcast lineups: %w", err)
	}
	for _, obs := range observations {
		change, ok := c.comparer.Observe(obs, known)
		if !ok {
			continue
		}
		c.logger.InfoContext(ctx, "the broadcast lineup differs from the tuner server",
			slog.String("kind", string(change.Kind)), slog.Int("network_id", int(change.NetworkID)),
			slog.Int("added", change.Added), slog.Int("removed", change.Removed))
		payload := map[string]any{
			"kind": string(change.Kind), "networkId": int(change.NetworkID), "added": change.Added, "removed": change.Removed,
		}
		if err := c.notify(ctx, q, payload); err != nil {
			// 知らせられなかったら、次の回にもう一度知らせる。
			c.comparer.Unconfirm(obs)

			return err
		}
	}

	return nil
}

// checkTerrestrial は、地上波のネットワークのサービスを、チューナーサーバーの同じネットワークのテレビのサービスと比べる (#1519)。
// 違えば覚えて知らせ、同じになれば忘れる。続いている間は、同じ食い違いを繰り返し知らせない。
// チューナーサーバーにないネットワークは比べない (スキャンし直して、チャンネルを消したときなど)。
func (c *Checker) checkTerrestrial(ctx context.Context, q *sqlc.Queries, services []*tunerserver.Service) error {
	observations, err := LoadTerrestrial(ctx, q)
	if err != nil {
		return fmt.Errorf("failed to load the terrestrial lineups: %w", err)
	}
	registered, channels := registeredTerrestrial(services)
	for _, obs := range observations {
		known, ok := registered[obs.NetworkID]
		if !ok {
			delete(c.terrestrial, obs.NetworkID)

			continue
		}
		added, removed := diffServices(obs.Services, known)
		if len(added) == 0 && len(removed) == 0 {
			delete(c.terrestrial, obs.NetworkID)

			continue
		}
		difference := fmt.Sprint(added, removed)
		if c.terrestrial[obs.NetworkID] == difference {
			continue
		}
		c.logger.InfoContext(ctx, "the terrestrial services differ from the tuner server",
			slog.Int("network_id", int(obs.NetworkID)), slog.String("channel", channels[obs.NetworkID]),
			slog.Int("added", len(added)), slog.Int("removed", len(removed)))
		payload := map[string]any{
			"kind": kindTerrestrial, "networkId": int(obs.NetworkID), "channel": channels[obs.NetworkID],
			"added": len(added), "removed": len(removed),
		}
		if err := c.notify(ctx, q, payload); err != nil {
			return err
		}
		c.terrestrial[obs.NetworkID] = difference
	}

	return nil
}

// registeredTerrestrial は、チューナーサーバーの地上波のサービスを、ネットワークごとのテレビのサービスの集合と、
// ネットワークのチャンネルにまとめる。テレビのサービスがなくても、ネットワークがあれば空の集合を返す。
func registeredTerrestrial(services []*tunerserver.Service) (map[uint16]map[uint16]bool, map[uint16]string) {
	registered := map[uint16]map[uint16]bool{}
	channels := map[uint16]string{}
	for _, service := range services {
		if service == nil || service.NetworkID < 0 || service.NetworkID > 0xffff || service.ServiceID < 0 || service.ServiceID > 0xffff {
			continue
		}
		networkID := uint16(service.NetworkID)
		if !isTerrestrial(networkID) {
			continue
		}
		if registered[networkID] == nil {
			registered[networkID] = map[uint16]bool{}
		}
		if service.Type == serviceTypeTV {
			registered[networkID][uint16(service.ServiceID)] = true
		}
		if service.Channel != nil && channels[networkID] == "" {
			channels[networkID] = service.Channel.Channel
		}
	}

	return registered, channels
}

// diffServices は、放送のテレビのサービス broadcast と、チューナーサーバーの known の食い違いを、service_id の昇順で返す。
func diffServices(broadcast []Service, known map[uint16]bool) ([]uint16, []uint16) {
	onAir := map[uint16]bool{}
	for _, service := range broadcast {
		if service.ServiceType == serviceTypeTV {
			onAir[service.ServiceID] = true
		}
	}
	added := []uint16{}
	for id := range onAir {
		if !known[id] {
			added = append(added, id)
		}
	}
	removed := []uint16{}
	for id := range known {
		if !onAir[id] {
			removed = append(removed, id)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)

	return added, removed
}

// notify は、食い違いの材料 payload を、有効な管理者に知らせる。管理者がいなければ知らせない。
func (c *Checker) notify(ctx context.Context, q *sqlc.Queries, payload map[string]any) error {
	admins, err := q.ListActiveAdminIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list the administrators: %w", err)
	}
	if len(admins) == 0 {
		// 宛先を空にすると全員に届くので、管理者がいなければ知らせない。
		return nil
	}
	payload["url"] = systemStatusURL
	err = notification.EmitTo(ctx, c.pool, c.clock, domain.NotificationKindLineupChanged, payload,
		&notification.Audience{UserIDs: admins, ByRule: false})
	if err != nil {
		return fmt.Errorf("failed to notify the lineup change: %w", err)
	}

	return nil
}

// registeredTV は、チューナーサーバーのサービスのうち、衛星のネットワークのテレビのサービスを返す。
func registeredTV(services []*tunerserver.Service) []Known {
	known := make([]Known, 0, len(services))
	for _, service := range services {
		if service == nil || service.Type != serviceTypeTV || service.NetworkID < 0 || service.NetworkID > 0xffff ||
			service.ServiceID < 0 || service.ServiceID > 0xffff {
			continue
		}
		networkID := uint16(service.NetworkID)
		if _, ok := KindOf(networkID); !ok {
			continue
		}
		known = append(known, Known{NetworkID: networkID, ServiceID: uint16(service.ServiceID)})
	}

	return known
}

// fingerprintOf は、サービスの集合を、並び順によらない文字列にする。
func fingerprintOf(known []Known) string {
	keys := make([]string, 0, len(known))
	for _, service := range known {
		keys = append(keys, strconv.Itoa(int(service.NetworkID))+":"+strconv.Itoa(int(service.ServiceID)))
	}
	slices.Sort(keys)

	return strings.Join(keys, ",")
}
