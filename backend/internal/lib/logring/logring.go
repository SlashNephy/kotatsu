// Package logring は、プロセスのログの最後の行を、メモリのリングバッファに写して持つ (#1521、設計書 11.3)。
//
// 管理の画面でログを読み、不具合の報告に添えるのに使う。ファイルにも DB にも書かないので、再起動すれば消える。
package logring

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
)

const (
	// DefaultCapacity は、プロセスごとにメモリに持つログの行数 (保持行数、設計書 付録 A)。
	DefaultCapacity = 5000
	// DefaultMaxLineLength は、溜めるときに 1 行を切る長さ (UTF-16 の単位、設計書 付録 A)。
	DefaultMaxLineLength = 2000
	// ellipsis は、切った行の末尾に付ける印。
	ellipsis = "…"
)

// Level は、行の重さ。debug は info として扱う。
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
)

// LevelOf は、slog の重さを、3 段階の重さにする。
func LevelOf(level slog.Level) Level {
	switch {
	case level >= slog.LevelError:
		return LevelError
	case level >= slog.LevelWarn:
		return LevelWarn
	default:
		return LevelInfo
	}
}

// Line は、溜めた 1 行。
type Line struct {
	// Time は、epoch ミリ秒の時刻。
	Time  int64
	Level Level
	Text  string
}

// Buffer は、ログの最後の capacity 行を持つリングバッファ。いっぱいになったら、古い行から上書きする。
type Buffer struct {
	mu            sync.Mutex
	lines         []Line
	next          int
	full          bool
	maxLineLength int
}

// NewBuffer は、capacity 行を持ち、1 行を maxLineLength (UTF-16 の単位) で切るバッファを作る。
func NewBuffer(capacity, maxLineLength int) *Buffer {
	return &Buffer{mu: sync.Mutex{}, lines: make([]Line, max(capacity, 1)), next: 0, full: false, maxLineLength: maxLineLength}
}

// Add は、行を足す。本文は、壊れた UTF-8 を直し、1 行の上限で切る。
func (b *Buffer) Add(line Line) {
	line.Text = Truncate(strings.ToValidUTF8(line.Text, "�"), b.maxLineLength)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines[b.next] = line
	b.next++
	if b.next == len(b.lines) {
		b.next = 0
		b.full = true
	}
}

// Lines は、溜めた行を古い順に返す。
func (b *Buffer) Lines() []Line {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.full {
		return append([]Line{}, b.lines[:b.next]...)
	}
	lines := make([]Line, 0, len(b.lines))
	lines = append(lines, b.lines[b.next:]...)

	return append(lines, b.lines[:b.next]...)
}

// Truncate は、text を UTF-16 の単位で maxLength までに切り、切ったら末尾に「…」を付ける。
// 字の途中 (サロゲートペアの間) では切らない。maxLength が 0 以下なら切らない。
func Truncate(text string, maxLength int) string {
	if maxLength <= 0 {
		return text
	}
	units := 0
	// limit は、「…」(1 単位) を付けても上限に収まる位置。
	limit := -1
	for i, r := range text {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if units+width > maxLength-1 && limit < 0 {
			limit = i
		}
		units += width
		if units > maxLength {
			return text[:limit] + ellipsis
		}
	}

	return text
}

// Handler は、包んだ slog.Handler に書いてから、同じ行を Buffer に写す slog.Handler。
type Handler struct {
	inner slog.Handler
	// mirror は、写しの属性を、標準出力と同じ JSON の形に整える。時刻、重さ、本文は Line に別に持つので出さない。
	mirror slog.Handler
	state  *state
}

var _ slog.Handler = new(Handler)

// state は、Handler とその WithAttrs、WithGroup で作ったものが共有する。
type state struct {
	mu     sync.Mutex
	out    bytes.Buffer
	buffer *Buffer
}

func (s *state) Write(p []byte) (int, error) {
	return s.out.Write(p)
}

// New は、inner に書いた後に、buffer に写す Handler を作る。
func New(inner slog.Handler, buffer *Buffer) *Handler {
	s := &state{mu: sync.Mutex{}, out: bytes.Buffer{}, buffer: buffer}
	mirror := slog.NewJSONHandler(s, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && (attr.Key == slog.TimeKey || attr.Key == slog.LevelKey || attr.Key == slog.MessageKey) {
				return slog.Attr{}
			}

			return attr
		},
	})

	return &Handler{inner: inner, mirror: mirror, state: s}
}

// Install は、buffer に写すよう、handler を包んで返す。
// 写しの仕組みは、プロセスで 1 回だけ組む。handler が既に Handler なら、二重に写さないよう、そのまま返す。
func Install(handler slog.Handler, buffer *Buffer) slog.Handler {
	if _, ok := handler.(*Handler); ok {
		return handler
	}

	return New(handler, buffer)
}

// BufferOf は、handler が写している Buffer を返す。Handler でなければ nil を返す。
func BufferOf(handler slog.Handler) *Buffer {
	if h, ok := handler.(*Handler); ok {
		return h.state.buffer
	}

	return nil
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle は、本来の出力を先に行い、その後に写しを取る。写しに失敗しても、本来の出力の結果だけを返す。
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	err := h.inner.Handle(ctx, record)
	h.copy(ctx, record)

	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{inner: h.inner.WithAttrs(attrs), mirror: h.mirror.WithAttrs(attrs), state: h.state}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name), mirror: h.mirror.WithGroup(name), state: h.state}
}

// copy は、record を 1 行に整えて Buffer に足す。属性は、標準出力と同じ JSON にして、本文の後ろに続ける。
func (h *Handler) copy(ctx context.Context, record slog.Record) {
	defer func() {
		// 写しの失敗で、ログを出した処理を止めない。
		_ = recover()
	}()

	h.state.mu.Lock()
	h.state.out.Reset()
	mirrorErr := h.mirror.Handle(ctx, record)
	attrs := strings.TrimSpace(h.state.out.String())
	h.state.mu.Unlock()

	text := record.Message
	if mirrorErr == nil && attrs != "{}" && attrs != "" {
		text += " " + attrs
	}
	h.state.buffer.Add(Line{Time: record.Time.UnixMilli(), Level: LevelOf(record.Level), Text: text})
}
