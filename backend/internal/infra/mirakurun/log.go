package mirakurun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// maxLogSize は、/api/log から読む本文の上限 (バイト)。超えた分は先頭 (古い方) を捨てる。
// Mirakurun は既定で最後の 1000 行を持つ (maxLogHistory)。1 行 1KB としても収まる大きさにする。
const maxLogSize = 2 << 20

// Status は /api/status の応答のうち、不具合の報告の環境に出すもの (#1521)。mirakc は process を返さないので空になる。
type Status struct {
	Version string        `json:"version"`
	Process StatusProcess `json:"process"`
}

// StatusProcess は、チューナーサーバーのプロセスの環境。
type StatusProcess struct {
	Arch     string `json:"arch"`
	Platform string `json:"platform"`
}

// Status は /api/status を取得する。
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var status Status
	if err := c.getJSON(ctx, "api/status", nil, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// Log は /api/log の本文 (1 行に 1 つのログのテキスト) を返す (#1521)。Mirakurun と Mahiron が持つ。
// 本文が maxLogSize を超えたら、最後の maxLogSize バイトだけを返す。先頭の半端な行は捨てる。
func (c *Client) Log(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, "api/log", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp, nil); err != nil {
		return nil, err
	}

	tail := &tailWriter{buf: nil, limit: maxLogSize, cut: false}
	if _, err := io.Copy(tail, resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read the response of %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	body := tail.bytes()
	if tail.cut {
		if i := bytes.IndexByte(body, '\n'); i >= 0 {
			body = body[i+1:]
		}
	}

	return body, nil
}

// tailWriter は、書かれたものの最後の limit バイトだけを持つ。
type tailWriter struct {
	buf   []byte
	limit int
	cut   bool
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	// 書くたびに詰め直すと遅いので、上限の 2 倍まで溜めてから詰める。
	if len(w.buf) > 2*w.limit {
		w.compact()
	}

	return len(p), nil
}

// bytes は、最後の limit バイトを返す。
func (w *tailWriter) bytes() []byte {
	w.compact()

	return w.buf
}

func (w *tailWriter) compact() {
	if over := len(w.buf) - w.limit; over > 0 {
		w.buf = append(w.buf[:0], w.buf[over:]...)
		w.cut = true
	}
}

var _ io.Writer = new(tailWriter)

// HTTPStatus は、err がチューナーサーバーの 2xx 以外の応答によるものなら、その状態を返す。そうでなければ 0。
func HTTPStatus(err error) int {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return 0
	}

	return apiErr.StatusCode
}
