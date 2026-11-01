package publicnet_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
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
		// IPv4
		{addr: "93.184.215.14", want: true},
		{addr: "100.63.255.255", want: true},
		{addr: "100.128.0.0", want: true},
		{addr: "0.1.2.3", want: false},
		{addr: "127.255.255.254", want: false},
		{addr: "192.0.0.8", want: false},
		{addr: "192.0.2.1", want: false},
		{addr: "198.18.0.1", want: false},
		{addr: "198.51.100.1", want: false},
		{addr: "203.0.113.1", want: false},
		{addr: "224.0.0.1", want: false},
		{addr: "239.255.255.250", want: false},
		{addr: "240.0.0.1", want: false},
		{addr: "255.255.255.255", want: false},
		// IPv6
		{addr: "2606:4700:4700::1111", want: true},
		{addr: "::", want: false},
		{addr: "ff02::1", want: false},
		{addr: "fc00::1", want: false},
		{addr: "100::1", want: false},
		{addr: "2001:db8::1", want: false},
		// IPv4 を埋め込んだ IPv6
		{addr: "::ffff:8.8.8.8", want: true},
		{addr: "::ffff:127.0.0.1", want: false},
		{addr: "::ffff:100.64.0.1", want: false},
		{addr: "::10.0.0.1", want: false},
		{addr: "64:ff9b::a00:1", want: false},
		{addr: "64:ff9b:1::1", want: false},
		{addr: "2002:a00:1::1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, publicnet.IsPublic(netip.MustParseAddr(tt.addr)))
		})
	}
}
