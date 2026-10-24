package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

type position struct {
	StartAt time.Time `json:"startAt"`
	ID      string    `json:"id"`
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	want := position{StartAt: testNow(), ID: "0192"}
	cursor, err := graph.EncodeCursor(want)
	require.NoError(t, err)

	var got position
	require.NoError(t, graph.DecodeCursor(cursor, &got))
	assert.Equal(t, want, got)
}

func TestDecodeCursorRejectsInvalidCursors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "not base64", cursor: "!!!"},
		{name: "not JSON", cursor: "bm90IGpzb24"},
		// {"v":2,"p":{}}
		{name: "unknown version", cursor: "eyJ2IjoyLCJwIjp7fX0"},
		// {"v":1,"p":"x"} の p は position にならない。
		{name: "wrong position", cursor: "eyJ2IjoxLCJwIjoieCJ9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got position
			require.ErrorIs(t, graph.DecodeCursor(tt.cursor, &got), graph.ErrInvalidCursor)
		})
	}
}

func TestGlobalID(t *testing.T) {
	t.Parallel()

	typeName, id, err := graph.ParseGlobalID(graph.GlobalID("User", "0192d1f2-0000-7000-8000-000000000000"))
	require.NoError(t, err)
	assert.Equal(t, "User", typeName)
	assert.Equal(t, "0192d1f2-0000-7000-8000-000000000000", id)

	for _, invalid := range []string{"!!!", "VXNlcg" /* "User" だけ */, "OjE" /* ":1" */} {
		_, _, err := graph.ParseGlobalID(invalid)
		require.ErrorIs(t, err, graph.ErrInvalidID, invalid)
	}
}
