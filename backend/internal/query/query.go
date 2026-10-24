// Package query は、副作用のないデータ読み取りを表現する。
//
// レイヤの依存関係は [github.com/SlashNephy/kotatsu/internal/domain] の package doc を参照。
//
// # query の制約
//
//   - DB にはプール越しにだけ触れる。トランザクションを受け取ってはならない。トランザクションの中の読み取り
//     (行のロック、書いた直後の読み直し、確かめてから書く処理) は usecase が持つ。
//   - 状態遷移を伴う副作用を持ってはならない。書き込みは usecase の責務。
//   - sqlc の行型を返してはならない。domain の型に直して返す。行が見つからないときの pgx.ErrNoRows は、
//     そのまま返す。
package query

import "github.com/jackc/pgx/v5/pgxpool"

// Queries は、読み取りの問い合わせ。
type Queries struct {
	pool *pgxpool.Pool
}

// New は Queries を作る。
func New(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}
