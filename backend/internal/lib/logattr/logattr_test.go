package logattr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
)

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "改行のない値はそのまま", value: "/internal/streams/services/400101", want: "/internal/streams/services/400101"},
		{name: "LF をエスケープする", value: "a\nlevel=ERROR msg=forged", want: `a\nlevel=ERROR msg=forged`},
		{name: "CR と LF をエスケープする", value: "a\r\nb\rc", want: `a\r\nb\rc`},
		{name: "空の値", value: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attr := logattr.String("path", tt.value)
			assert.Equal(t, "path", attr.Key)
			assert.Equal(t, tt.want, attr.Value.String())
		})
	}
}

func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want any
	}{
		{name: "改行のない文言はそのまま", err: errors.New("not found"), want: "not found"},
		{
			name: "包んだエラーの文言の改行もエスケープする",
			err:  fmt.Errorf("failed to read %q: %w", "x", errors.New("bad\nlevel=ERROR")),
			want: `failed to read "x": bad\nlevel=ERROR`,
		},
		{name: "nil は nil のまま", err: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attr := logattr.Error(tt.err)
			assert.Equal(t, "error", attr.Key)
			assert.Equal(t, tt.want, attr.Value.Any())
		})
	}
}
