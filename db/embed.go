// Package db は、Atlas で生成したマイグレーションをバイナリに埋め込む。
// マイグレーションは db/schema.sql から mise run db:diff で生成し、手では書かない。
package db

import "embed"

// Migrations は goose の形式のマイグレーション。ファイルは migrations/ の下にある。
//
//go:embed migrations/*.sql
var Migrations embed.FS
