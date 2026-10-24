package id_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

func TestNewIsVersion7(t *testing.T) {
	t.Parallel()

	got := id.New()
	assert.Equal(t, uuid.Version(7), got.Version())
	assert.Equal(t, uuid.RFC4122, got.Variant())
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
