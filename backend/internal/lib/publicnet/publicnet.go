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

// nonPublicPrefixes は、netip.Addr の IsGlobalUnicast と IsPrivate では除けない、インターネットのグローバルでないアドレスの範囲。
// IPv4 を埋め込む IPv6 の範囲 (NAT64、6to4、IPv4 互換) は、経路によっては LAN の中の IPv4 アドレスに届くので、まとめて除く。
func nonPublicPrefixes() []netip.Prefix {
	return []netip.Prefix{
		// 「このネットワーク」(RFC 1122)。Linux では、0.0.0.0 への接続は自分自身に届く。
		netip.MustParsePrefix("0.0.0.0/8"),
		// CGNAT や Tailscale が使う共有アドレス空間 (RFC 6598)。
		netip.MustParsePrefix("100.64.0.0/10"),
		// IETF のプロトコルの割り当て (RFC 6890)。
		netip.MustParsePrefix("192.0.0.0/24"),
		// 文書用のアドレス (RFC 5737)。
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		// ベンチマーク用のアドレス (RFC 2544)。
		netip.MustParsePrefix("198.18.0.0/15"),
		// 予約済みのアドレス (RFC 1112)。
		netip.MustParsePrefix("240.0.0.0/4"),
		// IPv4 互換の IPv6 アドレス (RFC 4291 で廃止)。
		netip.MustParsePrefix("::/96"),
		// NAT64 (RFC 6052、RFC 8215)。
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		// 破棄用のアドレス (RFC 6666)。
		netip.MustParsePrefix("100::/64"),
		// 文書用のアドレス (RFC 3849)。
		netip.MustParsePrefix("2001:db8::/32"),
		// 6to4 (RFC 3056)。
		netip.MustParsePrefix("2002::/16"),
	}
}

// IsPublic は、addr が、インターネットのグローバルなユニキャストのアドレスかどうかを返す。
// IPv4 射影アドレス (::ffff:0:0/96) は、IPv4 アドレスとして確かめる。
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublicPrefixes() {
		if prefix.Contains(addr) {
			return false
		}
	}

	return true
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
