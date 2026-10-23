package mirakurun

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// maxSSELineSize は、SSE の 1 行の長さの上限。mirakc のイベントは数十バイトなので、十分に大きい。
const maxSSELineSize = 1 << 20

// SSEReader は、mirakc の /events (Server-Sent Events) を逐次に読む。
//
// コメント行 (":" で始まる行。mirakc は keep-alive に使う) は読み飛ばす。
// 複数の data 行は改行でつなぎ、event 行がなければイベント名を "message" とする (WHATWG の仕様に従う)。
type SSEReader struct {
	scanner *bufio.Scanner
}

// NewSSEReader は r から読む SSEReader を作る。
func NewSSEReader(r io.Reader) *SSEReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), maxSSELineSize)
	scanner.Split(scanSSELines)

	return &SSEReader{scanner: scanner}
}

// Next は次のイベントを返す。
// ストリームがイベントの区切り (空行) で終わったら io.EOF を、イベントの途中で切れたら io.ErrUnexpectedEOF を返す。
func (r *SSEReader) Next() (*Event, error) {
	name := ""
	data := []string{}
	pending := false

	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			if !pending {
				continue
			}

			return newSSEEvent(name, data), nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
			pending = true
		case "data":
			data = append(data, value)
			pending = true
		default:
			// id と retry は使わない。知らない項目は仕様どおり無視する。
		}
	}
	if err := r.scanner.Err(); err != nil {
		if errors.Is(err, errSSETruncated) {
			return nil, io.ErrUnexpectedEOF
		}

		return nil, err
	}
	if pending {
		return nil, io.ErrUnexpectedEOF
	}

	return nil, io.EOF
}

// newSSEEvent は、mirakc のイベント名 (例: epg.programs-updated) を最初の "." で Resource と Type に分ける。
func newSSEEvent(name string, data []string) *Event {
	if name == "" {
		name = "message"
	}
	resource, eventType, _ := strings.Cut(name, ".")

	var raw json.RawMessage
	if len(data) > 0 {
		raw = json.RawMessage(strings.Join(data, "\n"))
	}

	return &Event{Resource: resource, Type: eventType, Data: raw, Time: time.Time{}}
}

// scanSSELines は、SSE の行の区切り (CRLF、LF、CR のどれか) で分ける。
func scanSSELines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 < len(data) {
				if data[i+1] == '\n' {
					return i + 2, data[:i], nil
				}

				return i + 1, data[:i], nil
			}
			if atEOF {
				return i + 1, data[:i], nil
			}

			// CR の次が LF かどうかは、続きを読むまで分からない。
			return 0, nil, nil
		}
	}
	if atEOF && len(data) > 0 {
		// 行の途中で切れた。最後の行として返すと、Next がイベントの途中で切れたことに気づけない。
		return 0, nil, errSSETruncated
	}

	return 0, nil, nil
}

var errSSETruncated = errors.New("sse stream ended in the middle of a line")
