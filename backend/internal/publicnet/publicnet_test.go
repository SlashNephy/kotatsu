package publicnet_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/publicnet"
)

// TestIsPublic は、接続してよい、インターネットのアドレスだけを受け付けることを確かめる。
func TestIsPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{addr: "8.8.8.8", want: true},
		{addr: "2001:4860:4860::8888", want: true},
		{addr: "127.0.0.1", want: false},
		{addr: "10.0.0.1", want: false},
		{addr: "172.16.0.1", want: false},
		{addr: "192.168.1.1", want: false},
		{addr: "169.254.169.254", want: false},
		{addr: "100.100.100.100", want: false},
		{addr: "0.0.0.0", want: false},
		{addr: "::1", want: false},
		{addr: "fe80::1", want: false},
		{addr: "fd00::1", want: false},
		{addr: "::ffff:192.168.1.1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, publicnet.IsPublic(netip.MustParseAddr(tt.addr)))
		})
	}
}
