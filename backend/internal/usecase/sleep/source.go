package sleep

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// DBSource は、DB から録画と後処理のジョブと予約を読み、このプロセスの視聴の数を足して、Activity を返す。
type DBSource struct {
	pool *pgxpool.Pool
	// encodeKinds は、エンコードとして数える River のジョブの種類。
	encodeKinds []string
	// viewers は、このプロセスが流しているライブの配信と、ライブのトランスコードのセッションの数を返す。
	viewers func() int64
}

var _ Source = new(DBSource)

// NewDBSource は DBSource を返す。
func NewDBSource(pool *pgxpool.Pool, encodeKinds []string, viewers func() int64) *DBSource {
	return &DBSource{pool: pool, encodeKinds: encodeKinds, viewers: viewers}
}

// Activity は、now の時点の Activity を返す。
func (s *DBSource) Activity(ctx context.Context, now time.Time) (*Activity, error) {
	q := sqlc.New(s.pool)
	row, err := q.GetSleepActivity(ctx, now)
	if err != nil {
		return nil, err
	}
	next, err := q.ListNextReservationStart(ctx, now)
	if err != nil {
		return nil, err
	}
	// River のテーブルは sqlc のスキーマにないので、直接読む。
	var encodings int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE state = 'running' AND kind = ANY($1::text[])`,
		s.encodeKinds).Scan(&encodings); err != nil {
		return nil, err
	}
	activity := &Activity{
		Recordings: row.Recordings + row.Timeshifts, Encodings: encodings, Viewers: s.viewers(), Airing: row.Airing, NextStart: time.Time{},
	}
	if len(next) > 0 {
		activity.NextStart = next[0]
	}

	return activity, nil
}
