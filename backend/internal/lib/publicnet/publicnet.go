// Package publicnet は、ユーザーが渡した URL へ接続するときに、LAN の中のアドレスへ接続させないための部品。
// 名前を引いた後のアドレスで確かめるので、DNS の応答を後から変えられても、LAN の中には接続しない。
package publicnet

import (
	"errors"
	"fmt"
	"net/netip"
	"syscall"
)

// ErrNonPublicAddress は、接続先がインターネットのグローバルなアドレスでないことを表す。
var ErrNonPublicAddress = errors.New("the destination must have a public address")

// IsPublic は、addr が、インターネットのグローバルなユニキャストのアドレスかどうかを返す。
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	// CGNAT や Tailscale が使う 100.64.0.0/10 (RFC 6598) は、netip.Addr.IsPrivate が含めないので、別に除く。
	sharedAddressSpace := netip.PrefixFrom(netip.AddrFrom4([4]byte{100, 64, 0, 0}), 10)

	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !sharedAddressSpace.Contains(addr)
}

// Control は、net.Dialer の Control に渡し、グローバルでないアドレスへの接続を ErrNonPublicAddress で断る。
func Control(_, address string, _ syscall.RawConn) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("failed to parse the address %s: %w", address, err)
	}
	if !IsPublic(addrPort.Addr()) {
		return fmt.Errorf("%w: %s", ErrNonPublicAddress, addrPort.Addr())
	}

	return nil
}
