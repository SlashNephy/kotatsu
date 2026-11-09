//go:build ruleguard

// Package gorules は、golangci-lint の gocritic (ruleguard) で読むルール。
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// outboundHTTPClient は、外部との通信の記録 (#1369、設計書 11.8) を通らない HTTP クライアントを検出する。
// 外部と通信するクライアントは netactivity.NewClient で作る。チューナーサーバーや kyusu の中継などの内部の通信は、
// .golangci.yaml の exclusions でパッケージごとに除く。
func outboundHTTPClient(m dsl.Matcher) {
	m.Import("net/http")
	m.Match(
		`http.Client{$*_}`,
		`new(http.Client)`,
		`http.DefaultClient`,
		`http.DefaultTransport`,
		`http.Get($*_)`,
		`http.Head($*_)`,
		`http.Post($*_)`,
		`http.PostForm($*_)`,
	).Report(`outbound HTTP must go through netactivity.NewClient so that it is recorded`)
}
