package ts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

func TestIsH264ISliceHeader(t *testing.T) {
	t.Parallel()

	// header は、スライスのヘッダーの先頭 2 バイト。first_mb_in_slice と slice_type を指数ゴロム符号で並べ、残りは 0 で埋める。
	tests := []struct {
		name   string
		header uint16
		want   bool
	}{
		{name: "I (slice_type 2)", header: 0b1_011_000000000000, want: true},
		{name: "I for the whole picture (slice_type 7)", header: 0b1_0001000_00000000, want: true},
		{name: "P (slice_type 0)", header: 0b1_1_00000000000000, want: false},
		{name: "B (slice_type 1)", header: 0b1_010_000000000000, want: false},
		{name: "SI (slice_type 4)", header: 0b1_00101_0000000000, want: false},
		{name: "P for the whole picture (slice_type 5)", header: 0b1_00110_0000000000, want: false},
		{name: "B for the whole picture (slice_type 6)", header: 0b1_00111_0000000000, want: false},
		{name: "SI for the whole picture (slice_type 9)", header: 0b1_0001010_00000000, want: false},
		{name: "not the first slice of the picture", header: 0b010_011_0000000000, want: false},
		{name: "slice_type does not fit in two bytes", header: 0b1_000000000000000, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ts.IsH264ISliceHeader(tt.header))
		})
	}
}
