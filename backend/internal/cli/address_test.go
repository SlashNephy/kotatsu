package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
)

func TestAdvertiseAddress(t *testing.T) {
	t.Parallel()

	hostname, err := os.Hostname()
	require.NoError(t, err)

	tests := []struct {
		listen        string
		advertiseHost string
		want          string
		wantErr       bool
	}{
		{listen: ":8080", want: hostname + ":8080"},
		// Kubernetes では、全アドレスで待ち受け、Pod の IP を載せる。
		{listen: ":8080", advertiseHost: "10.244.0.8", want: "10.244.0.8:8080"},
		{listen: "0.0.0.0:8080", advertiseHost: "fd00::8", want: "[fd00::8]:8080"},
		{listen: "0.0.0.0:8080", want: hostname + ":8080"},
		{listen: "[::]:8080", want: hostname + ":8080"},
		{listen: "10.0.0.1:8080", want: "10.0.0.1:8080"},
		{listen: "[fd00::1]:8080", want: "[fd00::1]:8080"},
		{listen: "kotatsu.local:8080", want: "kotatsu.local:8080"},
		{listen: "8080", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.listen+"/"+tt.advertiseHost, func(t *testing.T) {
			t.Parallel()

			got, err := cli.AdvertiseAddress(tt.listen, tt.advertiseHost)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
