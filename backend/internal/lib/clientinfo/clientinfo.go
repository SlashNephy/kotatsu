// Package clientinfo は、HTTP のリクエストの送信元 (IP アドレス、HTTPS かどうか、接続したホスト) を、context で受け渡す。
//
// web/httpserver のアクセスの制限が、信頼済みのプロキシが伝えた値を踏まえて context に入れ、ほかの層が読む。
package clientinfo

import (
	"context"
	"net/netip"
)

// Client は、リクエストの送信元。信頼済みのプロキシを経由したときは、プロキシが伝えた値にする。
type Client struct {
	// IP は送信元の IP アドレス。
	IP netip.Addr
	// Secure は、送信元が HTTPS でつないでいるかどうか。
	Secure bool
	// Host は、送信元が接続したホスト名 (Origin の検査に使う)。
	Host string
}

// Origin は、送信元が接続したオリジン (scheme://host) を返す。
func (c *Client) Origin() string {
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}

	return scheme + "://" + c.Host
}

type clientKey struct{}

// With は、送信元 client を入れた context を返す。
func With(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, clientKey{}, client)
}

// From は、web/httpserver の AccessPolicy.Middleware が context に入れた送信元を返す。入っていなければ nil を返す。
func From(ctx context.Context) *Client {
	client, _ := ctx.Value(clientKey{}).(*Client)

	return client
}
