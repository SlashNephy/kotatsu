package mirakurun_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

// TestOnAirPrograms は、/api/onair を mirakc からだけ読み、ほかのサーバーには問い合わせないことを確かめる。
// スタブは mirakc 以外の /api/onair に 404 を返すので、問い合わせれば失敗する。
func TestOnAirPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind mirakurun.ServerKind
		want []*tunerserver.OnAirProgram
	}{
		{
			kind: mirakurun.ServerMirakc,
			want: []*tunerserver.OnAirProgram{{
				ServiceID: 3273601024,
				Current: &tunerserver.Program{
					ID: 327360102401405, EventID: 1405, ServiceID: 1024, NetworkID: 32736,
					StartAt: 1791702600000, Duration: 1, IsFree: true, Name: "中継",
				},
				Next: nil,
			}},
		},
		{kind: mirakurun.ServerMirakurun, want: []*tunerserver.OnAirProgram{}},
		{kind: mirakurun.ServerMahiron, want: []*tunerserver.OnAirProgram{}},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			_, server := newStubServer(t, tt.kind)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			got, err := client.OnAirPrograms(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
