package graph_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

// TestNetworkActivityPurposes は、外部との通信の目的を足したときに、GraphQL の NetworkActivityPurpose にも足したことを確認する。
// 足し忘れると、記録した目的を返すときに空の文字列になり、画面が壊れる (#1902 で Google Drive を足したときに起きかけた)。
func TestNetworkActivityPurposes(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	var got struct {
		Type struct {
			EnumValues []struct {
				Name string `json:"name"`
			} `json:"enumValues"`
		} `json:"__type"`
	}
	require.Empty(t, f.query(t, `{ __type(name: "NetworkActivityPurpose") { enumValues { name } } }`, nil, &got))
	names := make([]string, 0, len(got.Type.EnumValues))
	for _, value := range got.Type.EnumValues {
		names = append(names, value.Name)
	}
	purposes := make([]string, 0, len(netactivity.PurposeValues()))
	for _, purpose := range netactivity.PurposeValues() {
		purposes = append(purposes, string(purpose))
	}
	slices.Sort(names)
	slices.Sort(purposes)
	assert.Equal(t, purposes, names)
}
