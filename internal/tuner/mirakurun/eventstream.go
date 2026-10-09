package mirakurun

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// maxEventSize は、1 件のイベントの大きさの上限。番組の詳細を含めても数十 KB に収まる。
const maxEventSize = 4 << 20

// EventReader は、Mirakurun の /api/events/stream を逐次に読む。
//
// このストリームは SSE でも NDJSON でもない。最初に "[" を送り、その後はイベントの JSON と "," を改行で区切って
// 送り続け、配列を閉じない (設計書 3.3)。そこで、配列の要素を 1 件ずつ切り出してから JSON として読む。
// 読み終えたイベントは手元に残らないので、何時間続くストリームでも使うメモリは増えない。
type EventReader struct {
	reader  *bufio.Reader
	started bool
	buf     []byte
}

// NewEventReader は r から読む EventReader を作る。
func NewEventReader(r io.Reader) *EventReader {
	return &EventReader{reader: bufio.NewReader(r), started: false, buf: []byte{}}
}

// mirakurunEvent は、Mirakurun が送るイベントの JSON の形。
type mirakurunEvent struct {
	Resource string          `json:"resource"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Time     int64           `json:"time"`
}

// Next は次のイベントを返す。
// ストリームがイベントの区切りで終わったら io.EOF を、イベントの途中で切れたら io.ErrUnexpectedEOF を返す。
func (r *EventReader) Next() (*Event, error) {
	if !r.started {
		b, err := r.skipSpaces()
		if err != nil {
			return nil, err
		}
		if b != '[' {
			return nil, fmt.Errorf("event stream must start with '[', got %q", b)
		}
		r.started = true
	}

	// 要素の間の空白と "," を読み飛ばす。
	var b byte
	for {
		var err error
		b, err = r.skipSpaces()
		if err != nil {
			return nil, err
		}
		if b != ',' {
			break
		}
	}
	switch b {
	case ']':
		return nil, io.EOF
	case '{':
	default:
		return nil, fmt.Errorf("event must be a JSON object, got %q", b)
	}

	raw, err := r.readObject()
	if err != nil {
		return nil, err
	}
	var event mirakurunEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("failed to decode an event: %w", err)
	}

	return &Event{
		Resource: event.Resource,
		Type:     event.Type,
		Data:     event.Data,
		Time:     time.UnixMilli(event.Time).UTC(),
	}, nil
}

// skipSpaces は、JSON の空白を読み飛ばし、次の 1 バイトを返す。
func (r *EventReader) skipSpaces() (byte, error) {
	for {
		b, err := r.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch b {
		case ' ', '\t', '\r', '\n':
		default:
			return b, nil
		}
	}
}

// readObject は、"{" を読んだ直後から、対応する "}" までを読み、"{" を含めた JSON を返す。
// 文字列の中の括弧と、エスケープされた引用符は数えない。
func (r *EventReader) readObject() ([]byte, error) {
	r.buf = append(r.buf[:0], '{')
	depth := 1
	inString := false
	escaped := false
	for depth > 0 {
		b, err := r.reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}
		if len(r.buf) >= maxEventSize {
			return nil, fmt.Errorf("event exceeds %d bytes", maxEventSize)
		}
		r.buf = append(r.buf, b)

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		case inString:
		case b == '{' || b == '[':
			depth++
		case b == '}' || b == ']':
			depth--
		}
	}

	return r.buf, nil
}
