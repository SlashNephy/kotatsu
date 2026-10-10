package jikkyo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/jikkyo"
)

func TestTableLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		overrides map[string]string
		networkID int64
		serviceID int64
		wantID    string
		wantOK    bool
	}{
		{name: "BS は、ネットワーク ID とサービス ID の組で引く", networkID: 4, serviceID: 101, wantID: jkBS, wantOK: true},
		{name: "地上波は、地域のネットワーク ID によらず、サービス ID で引く", networkID: 0x7FE0, serviceID: 0x0400, wantID: jkNHK, wantOK: true},
		{name: "ほかの地域の地上波", networkID: 0x7FD1, serviceID: 0x2800, wantID: jkNHK, wantOK: true},
		{name: "地上波のサブチャンネルは、1 つ前のメインチャンネルに寄せる", networkID: 0x7FE0, serviceID: 0x0401, wantID: jkNHK, wantOK: true},
		{name: "地上波のサブチャンネルは、2 つ前のメインチャンネルにも寄せる", networkID: 0x7FE0, serviceID: 0x0402, wantID: jkNHK, wantOK: true},
		{name: "実況のチャンネルがないと書かれた地上波", networkID: 0x7F80, serviceID: 0x6438, wantID: "", wantOK: false},
		{name: "地上波でないネットワークでは、サービス ID だけでは引かない", networkID: 4, serviceID: 0x0400, wantID: "", wantOK: false},
		{name: "対応表にない BS", networkID: 4, serviceID: 999, wantID: "", wantOK: false},
		{
			name: "設定で足したサービス", overrides: map[string]string{"6:999": "jk200"},
			networkID: 6, serviceID: 999, wantID: "jk200", wantOK: true,
		},
		{
			name: "設定は、同梱した対応表より優先する", overrides: map[string]string{"4:101": "jk103"},
			networkID: 4, serviceID: 101, wantID: "jk103", wantOK: true,
		},
		{
			name: "設定の地上波は、ネットワーク ID まで一致したときだけ使う", overrides: map[string]string{"32736:1024": "jk8"},
			networkID: 0x7FE1, serviceID: 0x0400, wantID: jkNHK, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := jikkyo.NewTable(tt.overrides)
			require.NoError(t, err)
			id, ok := table.Lookup(tt.networkID, tt.serviceID)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestNewTableRejectsInvalidOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		overrides map[string]string
	}{
		{name: "区切りがない", overrides: map[string]string{"4-101": jkBS}},
		{name: "数でない", overrides: map[string]string{"bs:101": jkBS}},
		{name: "チャンネルの ID が jk で始まらない", overrides: map[string]string{"4:101": "101"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := jikkyo.NewTable(tt.overrides)
			assert.Error(t, err)
		})
	}
}
