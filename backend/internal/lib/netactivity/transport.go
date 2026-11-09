package netactivity

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientOptions は、NewClient の設定。
type ClientOptions struct {
	// Transport は、通信に使う RoundTripper。nil なら http.DefaultTransport を使う。
	Transport http.RoundTripper
	Timeout   time.Duration
	// CheckRedirect は、http.Client の CheckRedirect。
	CheckRedirect func(req *http.Request, via []*http.Request) error
}

// NewClient は、目的 purpose の通信を recorder に記録する HTTP クライアントを返す。recorder が nil なら記録しない。
func NewClient(recorder *Recorder, purpose Purpose, opts ClientOptions) *http.Client {
	base := opts.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	return &http.Client{
		Transport:     &transport{base: base, recorder: recorder, purpose: purpose},
		CheckRedirect: opts.CheckRedirect,
		Jar:           nil,
		Timeout:       opts.Timeout,
	}
}

type purposeKey struct{}

// WithPurpose は、1 つのクライアントで複数の目的の通信をするときに、この ctx の通信の目的を上書きする。
func WithPurpose(ctx context.Context, purpose Purpose) context.Context {
	return context.WithValue(ctx, purposeKey{}, purpose)
}

type transport struct {
	base     http.RoundTripper
	recorder *Recorder
	purpose  Purpose
}

var _ http.RoundTripper = new(transport)

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	purpose := t.purpose
	if override, ok := req.Context().Value(purposeKey{}).(Purpose); ok {
		purpose = override
	}
	host := req.URL.Hostname()
	sent := max(req.ContentLength, 0)

	res, err := t.base.RoundTrip(req)
	if err != nil {
		t.recorder.Add(purpose, host, false, Totals{Count: 1, SentBytes: sent, ReceivedBytes: 0})

		return nil, fmt.Errorf("failed to round trip: %w", err)
	}
	success := res.StatusCode < http.StatusBadRequest
	t.recorder.Add(purpose, host, success, Totals{Count: 1, SentBytes: sent, ReceivedBytes: 0})

	count := func(received, written int) {
		t.recorder.Add(purpose, host, success, Totals{Count: 0, SentBytes: int64(written), ReceivedBytes: int64(received)})
	}
	// WebSocket は、Upgrade の応答の Body を読み書きする接続として使う。
	if conn, ok := res.Body.(io.ReadWriteCloser); ok {
		res.Body = &countingConn{ReadWriteCloser: conn, count: count}
	} else if res.Body != nil {
		res.Body = &countingBody{ReadCloser: res.Body, count: count}
	}

	return res, nil
}

type countingBody struct {
	io.ReadCloser

	count func(received, written int)
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.count(n, 0)
	}

	return n, err
}

type countingConn struct {
	io.ReadWriteCloser

	count func(received, written int)
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	if n > 0 {
		c.count(n, 0)
	}

	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Write(p)
	if n > 0 {
		c.count(0, n)
	}

	return n, err
}
