package servicelogo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ErrNoLogo は、サービスのロゴを集めていないことを表す。
var ErrNoLogo = errors.New("no logo has been collected for the service")

// Store は、集めたロゴを読む。
type Store struct {
	db sqlc.DBTX
}

// NewStore は Store を返す。
func NewStore(db sqlc.DBTX) *Store {
	return &Store{db: db}
}

// ServiceLogo は、サービス ID (networkId × 100000 + serviceId) のサービスの、集めたロゴの PNG を返す。
// 種類が複数あれば、HD の大きいロゴを先にする。なければ ErrNoLogo を返す。
func (s *Store) ServiceLogo(ctx context.Context, serviceID int64) ([]byte, error) {
	data, err := sqlc.New(s.db).GetServiceLogo(ctx, serviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoLogo
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the service logo: %w", err)
	}

	return data, nil
}
