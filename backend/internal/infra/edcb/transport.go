package edcb

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// send は、コマンドを送り、応答の戻り値と本文を返す。接続は 1 コマンドごとに張り直す。
// ctx が終わるか、timeout が過ぎたら、接続を閉じて読むのをやめる。timeout が 0 なら掛けない。
func (c *Client) send(ctx context.Context, cmd uint32, body []byte, timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	conn, err := c.dial(ctx, "tcp", c.address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to EpgTimerSrv: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := writeRequest(conn, cmd, body); err != nil {
		return nil, contextError(ctx, fmt.Errorf("failed to send command %d: %w", cmd, err))
	}
	status, size, err := readHeader(conn)
	if err != nil {
		return nil, contextError(ctx, fmt.Errorf("failed to read the response to command %d: %w", cmd, err))
	}
	if size > maxResponseSize {
		return nil, fmt.Errorf("%w: the response to command %d is %d bytes", ErrMalformed, cmd, size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, contextError(ctx, fmt.Errorf("failed to read the response to command %d: %w", cmd, err))
	}
	if status != statusSuccess {
		return nil, &StatusError{Command: cmd, Status: status}
	}

	return payload, nil
}

// contextError は、ctx が終わって接続を閉じたために失敗したなら、ctx の理由を返す。
func contextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w (%w)", ctxErr, err)
	}

	return err
}

func writeRequest(w io.Writer, cmd uint32, body []byte) error {
	buf := make([]byte, 0, 8+len(body))
	buf = binary.LittleEndian.AppendUint32(buf, cmd)
	buf = binary.LittleEndian.AppendUint32(buf, size(len(body)))
	buf = append(buf, body...)
	_, err := w.Write(buf)

	return err
}

func readHeader(r io.Reader) (uint32, uint32, error) {
	head := make([]byte, 8)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, 0, err
	}

	return binary.LittleEndian.Uint32(head), binary.LittleEndian.Uint32(head[4:]), nil
}

// decode は、本文を読み、形の誤りがあればエラーを返す。新しい版の EpgTimerSrv が末尾に足した項目は、読まずに捨てる。
func decode[T any](cmd uint32, payload []byte, read func(*decoder) T) (T, error) {
	d := newDecoder(payload)
	v := read(d)
	if d.err != nil {
		var zero T

		return zero, fmt.Errorf("failed to read the response to command %d: %w", cmd, d.err)
	}

	return v, nil
}

// versioned は、「2」の付くコマンドの応答の先頭の版を読み、続きを read で読む。
// 求めた版と違っても、読めるところまで読む (EpgTimerSrv は、要求した版で書く)。
func versioned[T any](read func(*decoder) T) func(*decoder) T {
	return func(d *decoder) T {
		_ = d.u16()

		return read(d)
	}
}
