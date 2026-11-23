// Package db は、スキーマの原本 (schema.sql) と、その版 (schema.sum)、1 回だけのスクリプト (scripts/) をバイナリに埋め込む。
// kotatsu migrate は、schema.sql と DB の差を出して当てる (設計書 11.4、#2040)。
package db

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

// Schema は、スキーマの原本 (schema.sql) の DDL。
//
//go:embed schema.sql
var Schema string

//go:embed schema.sum
var schemaSum string

// Scripts は、データを書き換える 1 回だけのスクリプト。ファイルは scripts/ の下にあり、
// 名前は「版_名前.before.sql」(差を当てる前に流す) か「版_名前.after.sql」(差を当てた後に流す) にする。
//
//go:embed scripts/*.sql
var Scripts embed.FS

// Migrations は goose の形式のマイグレーション。ファイルは migrations/ の下にある。
// kotatsu migrate は使わず、schema.sql と同じスキーマになることをテストで確かめるためだけに残す (#2040)。
//
//go:embed migrations/*.sql
var Migrations embed.FS

// SchemaSum は、schema.sum に書いたスキーマの版と、schema.sql の SHA-256 (16 進)。
// schema.sum は mise run //backend:db:sum で書く。
func SchemaSum() (int64, string) {
	lines := strings.Fields(schemaSum)
	if len(lines) != 2 {
		// 埋め込んだファイルが壊れているのはビルドの誤りで、実行時に回復できない。
		panic(fmt.Sprintf("invalid schema.sum: %q", schemaSum))
	}
	version, err := strconv.ParseInt(lines[0], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid schema version in schema.sum: %v", err))
	}

	return version, lines[1]
}
