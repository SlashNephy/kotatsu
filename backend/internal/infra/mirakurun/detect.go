package mirakurun

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
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
			Kind:         ServerMahiron,
			Version:      version,
			EventsStream: true,
			SSEEvents:    false,
			OnAir:        false,
		}, nil
	}

	sse, err := c.hasSSEEvents(ctx)
	if err != nil {
		return nil, err
	}
	if sse {
		return &ServerInfo{
			Kind:         ServerMirakc,
			Version:      version,
			EventsStream: false,
			SSEEvents:    true,
			OnAir:        true,
		}, nil
	}

	return &ServerInfo{
		Kind:         ServerMirakurun,
		Version:      version,
		EventsStream: true,
		SSEEvents:    false,
		OnAir:        false,
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

// OpenEvents は、サーバーの種類に合ったイベントストリームを開く。
// Mirakurun と Mahiron では /api/events/stream を、mirakc では /events を読み、tunerserver.Event に読み替えて返す。
// 開けなかったときは判定の結果を捨てるので、次に開くときはサーバーの種類から判定し直す。
func (c *Client) OpenEvents(ctx context.Context) (tunerserver.EventStream, error) {
	server, err := c.Server(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := c.openEvents(ctx, server)
	if err != nil {
		c.Invalidate()

		return nil, err
	}

	return stream, nil
}

func (c *Client) openEvents(ctx context.Context, server *ServerInfo) (tunerserver.EventStream, error) {
	path := "api/events/stream"
	if server.SSEEvents {
		path = "events"
	}
	req, err := c.newRequest(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	if server.SSEEvents {
		req.Header.Set("Accept", "text/event-stream")
	}
	body, err := c.openStream(req, 0, nil)
	if err != nil {
		return nil, err
	}
	if server.SSEEvents {
		return &eventStream{reader: NewSSEReader(body), body: body}, nil
	}

	return &eventStream{reader: NewEventReader(body), body: body}, nil
}

// rawEventReader は、イベントを解釈する前の形で読むもの。EventReader と SSEReader が満たす。
type rawEventReader interface {
	Next() (*RawEvent, error)
}

var (
	_ rawEventReader = new(EventReader)
	_ rawEventReader = new(SSEReader)
)

// eventStream は、読んだイベントを tunerserver.Event に読み替えるストリーム。
type eventStream struct {
	reader rawEventReader
	body   io.ReadCloser
}

var _ tunerserver.EventStream = new(eventStream)

// Next は、次の Kotatsu が使うイベントを返す。使わないイベントと、形の分からないイベントは読み飛ばす。
func (s *eventStream) Next() (*tunerserver.Event, error) {
	for {
		raw, err := s.reader.Next()
		if err != nil {
			return nil, err
		}
		if event, ok := toEvent(raw); ok {
			return event, nil
		}
	}
}

func (s *eventStream) Close() error {
	return s.body.Close()
}
