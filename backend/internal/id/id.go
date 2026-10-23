// Package id は、Kotatsu 自身のデータの ID を生成する。
//
// ID の型には github.com/google/uuid の UUID をそのまま使う。pgx と sqlc が直接扱えるので、
// DB の uuid 列との変換を書かずに済む。sqlc の生成物でも、uuid 列はこの型になる。
package id

import "github.com/google/uuid"

// New は UUIDv7 の ID を生成する。
//
// UUIDv7 は時刻の順に並ぶので、B-tree のインデックスへの挿入が末尾に偏り、ランダムな UUID より
// 効率よく書き込める。PostgreSQL 18 の uuidv7() に頼らず Go で生成するのは、
// マネージド DB の古いバージョンでも動かすためである (設計書 8.1)。
// 同じプロセスの中では、同じミリ秒に生成した ID も生成した順に並ぶ。プロセスをまたぐと、同じミリ秒の中の順序は保証しない。
func New() uuid.UUID {
	// NewV7 が失敗するのは乱数の生成に失敗したときだけで、続行できない。
	return uuid.Must(uuid.NewV7())
}
