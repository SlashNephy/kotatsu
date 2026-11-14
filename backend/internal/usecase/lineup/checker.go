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

// systemStatusURL は、知らせを押したときに開く、システムの状態の画面のパス。
const systemStatusURL = "/admin/system"

// ServiceLister は、チューナーサーバーのサービスの一覧を返す。tunerserver.Client が満たす。
type ServiceLister interface {
	Services(ctx context.Context) ([]*tunerserver.Service, error)
}

var _ ServiceLister = tunerserver.Client(nil)

// Checker は、mikan が DB に書いた衛星の構成 (broadcast_lineups) を、チューナーサーバーのテレビのサービスと定期的に比べ、
// 食い違いを管理者に知らせる (#1519、設計書 6.2)。neko で動かす。
//
// 比べた状態 (確かめ済みの version と、1 回目に見た食い違い) は、プロセスの中だけに持つ。再起動したら、続いている食い違いも、
// 2 回読み直したところで、もう一度知らせる。
type Checker struct {
	pool     *pgxpool.Pool
	services ServiceLister
	clock    clock.Clock
	logger   *slog.Logger
	interval time.Duration
	comparer *Comparer
	// known は、前に比べたチューナーサーバーのテレビのサービスの集合の指紋。変わったら (スキャンし直したら)、比べ直す。
	known string
}

// NewChecker は、Checker を返す。interval が 0 以下なら DefaultCheckInterval を使う。
func NewChecker(pool *pgxpool.Pool, services ServiceLister, clk clock.Clock, logger *slog.Logger, interval time.Duration) *Checker {
	if interval <= 0 {
		interval = DefaultCheckInterval
	}

	return &Checker{
		pool: pool, services: services, clock: clk, logger: logger, interval: interval,
		comparer: NewComparer(clk, DefaultRecheckInterval), known: "",
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
	known := registeredTV(services)
	if len(known) == 0 {
		return nil
	}
	if fingerprint := fingerprintOf(known); fingerprint != c.known {
		c.comparer.Forget()
		c.known = fingerprint
	}
	q := sqlc.New(c.pool)
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
		if err := c.notify(ctx, q, change); err != nil {
			return err
		}
	}

	return nil
}

// notify は、食い違い change を、有効な管理者に知らせる。管理者がいなければ知らせない。
func (c *Checker) notify(ctx context.Context, q *sqlc.Queries, change *Change) error {
	admins, err := q.ListActiveAdminIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list the administrators: %w", err)
	}
	if len(admins) == 0 {
		// 宛先を空にすると全員に届くので、管理者がいなければ知らせない。
		return nil
	}
	payload := map[string]any{
		"kind": string(change.Kind), "networkId": int(change.NetworkID),
		"added": change.Added, "removed": change.Removed, "url": systemStatusURL,
	}
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
		if service.Type != serviceTypeTV || service.NetworkID < 0 || service.NetworkID > 0xffff ||
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
