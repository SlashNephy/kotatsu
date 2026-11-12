package logring_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/logring"
)

func TestBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		capacity int
		add      []string
		want     []string
	}{
		{name: "空", capacity: 3, add: nil, want: []string{}},
		{name: "いっぱいになる前は足した順", capacity: 3, add: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "いっぱいになったら古い行から上書きする", capacity: 3, add: []string{"a", "b", "c", "d", "e"}, want: []string{"c", "d", "e"}},
		{name: "ちょうどいっぱい", capacity: 3, add: []string{"a", "b", "c"}, want: []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buffer := logring.NewBuffer(tt.capacity, 0)
			for i, text := range tt.add {
				buffer.Add(logring.Line{Time: int64(i), Level: logring.LevelInfo, Text: text})
			}
			got := []string{}
			for _, line := range buffer.Lines() {
				got = append(got, line.Text)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		maxLength int
		want      string
	}{
		{name: "上限以下はそのまま", text: "abcde", maxLength: 5, want: "abcde"},
		{name: "上限を超えたら切って印を付ける", text: "abcdef", maxLength: 5, want: "abcd…"},
		{name: "日本語も 1 字を 1 単位と数える", text: "あいうえおか", maxLength: 5, want: "あいうえ…"},
		// 絵文字は UTF-16 で 2 単位。半端に切らず、手前で切る。
		{name: "サロゲートペアの間で切らない", text: "abc😀def", maxLength: 5, want: "abc…"},
		{name: "サロゲートペアがちょうど収まる", text: "ab😀de", maxLength: 6, want: "ab😀de"},
		{name: "0 なら切らない", text: "abcdefgh", maxLength: 0, want: "abcdefgh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, logring.Truncate(tt.text, tt.maxLength))
		})
	}
}

func TestBufferTruncatesLines(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(2, 10)
	buffer.Add(logring.Line{Time: 0, Level: logring.LevelInfo, Text: strings.Repeat("x", 100)})
	buffer.Add(logring.Line{Time: 0, Level: logring.LevelInfo, Text: "bad\xffutf8"})

	lines := buffer.Lines()
	require.Len(t, lines, 2)
	assert.Equal(t, strings.Repeat("x", 9)+"…", lines[0].Text)
	assert.Equal(t, "bad�utf8", lines[1].Text)
}

type credentials struct {
	User     string        `json:"user"`
	Password config.Secret `json:"password"`
}

func TestHandler(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name      string
		log       func(logger *slog.Logger)
		wantLevel logring.Level
		wantText  string
	}{
		{
			name:      "本文だけ",
			log:       func(logger *slog.Logger) { logger.Info("started") },
			wantLevel: logring.LevelInfo,
			wantText:  "started",
		},
		{
			name: "属性は標準出力と同じ JSON にする",
			log: func(logger *slog.Logger) {
				logger.Warn("tuner degraded", slog.String("reason", "timeout"), slog.Int("count", 3))
			},
			wantLevel: logring.LevelWarn,
			wantText:  `tuner degraded {"reason":"timeout","count":3}`,
		},
		{
			name: "With の属性とグループも入れる",
			log: func(logger *slog.Logger) {
				logger.With(slog.String("component", "neko")).WithGroup("job").Error("failed", slog.Any("error", errors.New("boom")))
			},
			wantLevel: logring.LevelError,
			wantText:  `failed {"component":"neko","job":{"error":"boom"}}`,
		},
		{
			name: "秘密の値は、標準出力と同じく伏せたまま写す",
			log: func(logger *slog.Logger) {
				logger.Info("config loaded", slog.Any("secret", config.Secret("hunter2-hunter2")),
					slog.Any("credentials", credentials{User: "alice", Password: config.Secret("hunter2-hunter2")}))
			},
			wantLevel: logring.LevelInfo,
			wantText:  `config loaded {"secret":"[REDACTED]","credentials":{"user":"alice","password":"[REDACTED]"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			buffer := logring.NewBuffer(10, 0)
			handler := logring.New(slog.NewJSONHandler(&stdout, &slog.HandlerOptions{
				AddSource: false, Level: slog.LevelInfo,
				ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
					if len(groups) == 0 && attr.Key == slog.TimeKey {
						return slog.Time(slog.TimeKey, at)
					}

					return attr
				},
			}), buffer)
			tt.log(slog.New(handler))

			lines := buffer.Lines()
			require.Len(t, lines, 1)
			assert.Equal(t, tt.wantLevel, lines[0].Level)
			assert.Equal(t, tt.wantText, lines[0].Text)
			assert.NotContains(t, lines[0].Text, "hunter2")
			// 本来の出力は変えない。
			assert.Contains(t, stdout.String(), `"msg":`)
			assert.NotContains(t, stdout.String(), "hunter2")
		})
	}
}

func TestHandlerSkipsDisabledLevels(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(10, 0)
	logger := slog.New(logring.New(slog.NewJSONHandler(&bytes.Buffer{}, nil), buffer))
	logger.Debug("debug is not written by default")
	assert.Empty(t, buffer.Lines())
}

func TestLevelOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level slog.Level
		want  logring.Level
	}{
		{level: slog.LevelDebug, want: logring.LevelInfo},
		{level: slog.LevelInfo, want: logring.LevelInfo},
		{level: slog.LevelWarn, want: logring.LevelWarn},
		{level: slog.LevelError, want: logring.LevelError},
		{level: slog.LevelError + 4, want: logring.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, logring.LevelOf(tt.level))
		})
	}
}

// failingHandler は、書くのに失敗する slog.Handler。
type failingHandler struct{ slog.Handler }

var errWrite = errors.New("write failed")

func (failingHandler) Handle(context.Context, slog.Record) error { return errWrite }

func TestHandlerReturnsInnerError(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(10, 0)
	handler := logring.New(failingHandler{slog.NewJSONHandler(&bytes.Buffer{}, nil)}, buffer)
	err := handler.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0))
	require.ErrorIs(t, err, errWrite)
	// 本来の出力に失敗しても、写しは取る。
	assert.Len(t, buffer.Lines(), 1)
}

func TestInstallOnce(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(10, 0)
	installed := logring.Install(slog.NewJSONHandler(&bytes.Buffer{}, nil), buffer)
	// 2 回目は何もしない。包み直すと、1 行が 2 回写る。
	again := logring.Install(installed, logring.NewBuffer(10, 0))
	assert.Same(t, installed, again)
	assert.Same(t, buffer, logring.BufferOf(again))
	assert.Nil(t, logring.BufferOf(slog.NewJSONHandler(&bytes.Buffer{}, nil)))

	slog.New(again).Info("written once")
	assert.Len(t, buffer.Lines(), 1)
}
