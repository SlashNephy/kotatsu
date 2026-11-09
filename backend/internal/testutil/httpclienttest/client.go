// Package httpclienttest は、テストで使う HTTP のクライアントを作る。
package httpclienttest

import (
	"net/http"
	"testing"
)

// New は、テストごとの Transport を持つクライアントを返す。
//
// httptest.Server の Close は http.DefaultTransport のアイドルの接続を閉じるので、既定のクライアントを並列のテストで共有すると、
// ほかのテストのサーバーが閉じたときに、再利用しかけた接続が切れて、要求が CloseIdleConnections called で失敗することがある。
func New(tb testing.TB) *http.Client {
	tb.Helper()

	transport := new(http.Transport)
	tb.Cleanup(transport.CloseIdleConnections)

	client := new(http.Client)
	client.Transport = transport

	return client
}
