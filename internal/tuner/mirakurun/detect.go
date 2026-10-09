package mirakurun

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
)

// ServerKind はチューナーサーバーの種類。
type ServerKind string

const (
	ServerMirakurun ServerKind = "mirakurun"
	ServerMirakc    ServerKind = "mirakc"
	ServerMahiron   ServerKind = "mahiron"
)

// mahironServerName は、Mahiron が /api/version の server で返す名前。
const mahironServerName = "mahiron"

// ServerInfo は、判定したチューナーサーバーの種類と、使える追加機能 (設計書 3.1)。
type ServerInfo struct {
	Kind    ServerKind
	Version *Version
	// EventsStream は、/api/events/stream (Mirakurun 形式のイベント) を使えるかどうか。Mirakurun と Mahiron が持つ。
	EventsStream bool
	// SSEEvents は、/events (SSE) を使えるかどうか。mirakc が持つ。
	SSEEvents bool
	// OnAir は、/api/onair を使えるかどうか。mirakc が持つ。
	OnAir bool
	// DataBroadcast は、データ放送の API を使えるかどうか。Mahiron が持つ。
	DataBroadcast bool
}

// Server は、チューナーサーバーの種類と追加機能を返す。
//
// 判定の結果は DetectInterval の間だけ使い回し、過ぎたら判定し直す。起動時に一度だけ判定して使い続けることはしない。
// EPGStation は API の定義を起動時に取得して持ち続けたため、古い定義のまま固まることがあった (設計書 3.1)。
func (c *Client) Server(ctx context.Context) (*ServerInfo, error) {
	c.mu.Lock()
	if c.server != nil && c.clock.Now().Sub(c.detectedAt) < c.detectInterval {
		server := c.server
		c.mu.Unlock()

		return server, nil
	}
	c.mu.Unlock()

	server, err := c.Detect(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.server = server
	c.detectedAt = c.clock.Now()
	c.mu.Unlock()

	return server, nil
}

// Invalidate は、判定の結果を捨てる。次の Server の呼び出しで判定し直す。
// チューナーサーバーに再接続するときに呼ぶ。サーバーが入れ替わっているかもしれないからである。
func (c *Client) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.server = nil
	c.detectedAt = c.clock.Now()
}

// Detect は、チューナーサーバーの種類と追加機能を判定する。結果は使い回さない。
//
//   - /api/version の server が "mahiron" なら Mahiron
//   - /events が SSE を返せば mirakc
//   - どちらでもなければ Mirakurun
func (c *Client) Detect(ctx context.Context) (*ServerInfo, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return nil, err
	}

	if version.Server == mahironServerName {
		return &ServerInfo{
			Kind:          ServerMahiron,
			Version:       version,
			EventsStream:  true,
			SSEEvents:     false,
			OnAir:         false,
			DataBroadcast: true,
		}, nil
	}

	sse, err := c.hasSSEEvents(ctx)
	if err != nil {
		return nil, err
	}
	if sse {
		return &ServerInfo{
			Kind:          ServerMirakc,
			Version:       version,
			EventsStream:  false,
			SSEEvents:     true,
			OnAir:         true,
			DataBroadcast: false,
		}, nil
	}

	return &ServerInfo{
		Kind:          ServerMirakurun,
		Version:       version,
		EventsStream:  true,
		SSEEvents:     false,
		OnAir:         false,
		DataBroadcast: false,
	}, nil
}

// hasSSEEvents は、/events が SSE を返すかどうかを調べる。
// 状態が 200 でも、SPA のフォールバックで HTML を返すサーバーがありうるので、Content-Type も確かめる。
// 本文は読まずに接続を閉じる。
func (c *Client) hasSSEEvents(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, "events", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))

	return err == nil && mediaType == "text/event-stream", nil
}

// EventStream は、チューナーサーバーのイベントを順に読む。読み終えたら必ず Close を呼ぶ。
type EventStream interface {
	// Next は次のイベントを返す。ストリームが区切りで終わったら io.EOF を、途中で切れたら io.ErrUnexpectedEOF を返す。
	Next() (*Event, error)
	io.Closer
}

// EventsQuery は、Mirakurun 形式のイベントストリームの絞り込みの条件。空の項目では絞り込まない。
// mirakc の /events には絞り込みの機能がないので、mirakc では使わない。
type EventsQuery struct {
	// Resource は program、service、tuner のどれか。
	Resource string
	// Type は create、update、remove のどれか。
	Type string
}

// OpenEvents は、サーバーの種類に合ったイベントストリームを開く。
// Mirakurun と Mahiron では /api/events/stream を、mirakc では /events を読む。
// 開けなかったときは判定の結果を捨てるので、次に開くときはサーバーの種類から判定し直す。
func (c *Client) OpenEvents(ctx context.Context, query *EventsQuery) (EventStream, error) {
	server, err := c.Server(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := c.openEvents(ctx, server, query)
	if err != nil {
		c.Invalidate()

		return nil, err
	}

	return stream, nil
}

func (c *Client) openEvents(ctx context.Context, server *ServerInfo, query *EventsQuery) (EventStream, error) {
	if server.SSEEvents {
		req, err := c.newRequest(ctx, "events", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/event-stream")
		body, err := c.openStream(req)
		if err != nil {
			return nil, err
		}

		return &sseStream{SSEReader: NewSSEReader(body), body: body}, nil
	}

	values := url.Values{}
	if query != nil {
		if query.Resource != "" {
			values.Set("resource", query.Resource)
		}
		if query.Type != "" {
			values.Set("type", query.Type)
		}
	}
	req, err := c.newRequest(ctx, "api/events/stream", values)
	if err != nil {
		return nil, err
	}
	body, err := c.openStream(req)
	if err != nil {
		return nil, err
	}

	return &mirakurunStream{EventReader: NewEventReader(body), body: body}, nil
}

type mirakurunStream struct {
	*EventReader

	body io.ReadCloser
}

var _ EventStream = new(mirakurunStream)

func (s *mirakurunStream) Close() error {
	return s.body.Close()
}

type sseStream struct {
	*SSEReader

	body io.ReadCloser
}

var _ EventStream = new(sseStream)

func (s *sseStream) Close() error {
	return s.body.Close()
}
