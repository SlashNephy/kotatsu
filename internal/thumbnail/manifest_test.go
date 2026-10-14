package thumbnail_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

func TestManifestFrame(t *testing.T) {
	t.Parallel()

	// times は、n 個のコマの時刻。
	times := func(n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(i * 10)
		}

		return s
	}

	tests := []struct {
		name   string
		count  int
		index  int
		want   thumbnail.Frame
		wantOK bool
	}{
		{
			name: "1 行目のコマ", count: 25, index: 3,
			want:   thumbnail.Frame{X: 480, Y: 0, Width: 160, Height: 90, SheetWidth: 1600, SheetHeight: 270},
			wantOK: true,
		},
		{
			name: "行をまたいだコマ", count: 25, index: 24,
			want:   thumbnail.Frame{X: 640, Y: 180, Width: 160, Height: 90, SheetWidth: 1600, SheetHeight: 270},
			wantOK: true,
		},
		{
			name: "コマが 1 行に満たなければ、画像の幅はその数のコマの幅", count: 4, index: 2,
			want:   thumbnail.Frame{X: 320, Y: 0, Width: 160, Height: 90, SheetWidth: 640, SheetHeight: 90},
			wantOK: true,
		},
		{name: "コマの数を超える", count: 4, index: 4, want: thumbnail.Frame{}, wantOK: false},
		{name: "負の番号", count: 4, index: -1, want: thumbnail.Frame{}, wantOK: false},
		{name: "コマがない", count: 0, index: 0, want: thumbnail.Frame{}, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &thumbnail.Manifest{Width: 160, Height: 90, Columns: 10, Times: times(tt.count)}
			got, ok := m.Frame(tt.index)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManifestFrameBefore(t *testing.T) {
	t.Parallel()

	m := &thumbnail.Manifest{Width: 160, Height: 90, Columns: 10, Times: []float64{2, 60, 120}}
	tests := []struct {
		name      string
		position  time.Duration
		want      int
		wantFound bool
	}{
		{name: "最初のコマより前", position: time.Second, want: 0, wantFound: false},
		{name: "コマの時刻ちょうど", position: 60 * time.Second, want: 1, wantFound: true},
		{name: "コマの間なら、前のコマ", position: 119 * time.Second, want: 1, wantFound: true},
		{name: "最後のコマより後", position: time.Hour, want: 2, wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := m.FrameBefore(tt.position)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
