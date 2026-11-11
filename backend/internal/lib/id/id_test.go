package id_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

func TestNewIsVersion7(t *testing.T) {
	t.Parallel()

	got := id.New()
	// 版は 7 番目のバイトの上位 4 ビット、変種は 9 番目のバイトの上位 2 ビットにある (RFC 9562 4.1、4.2)。
	assert.Equal(t, byte(7), got[6]>>4)
	assert.Equal(t, byte(0b10), got[8]>>6)
}

func TestNewIsOrderedByGeneration(t *testing.T) {
	t.Parallel()

	// 同じミリ秒の中で生成した ID も含めて、生成した順に並ぶことを確かめる。
	const n = 10000
	prev := id.New()
	for range n {
		next := id.New()
		require.Negative(t, bytes.Compare(prev[:], next[:]), "%s must sort before %s", prev, next)
		prev = next
	}
}
