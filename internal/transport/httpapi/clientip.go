package httpapi

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// parseTrustedProxies reads IP addresses and CIDR prefixes.
func parseTrustedProxies(items []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(items))
	for _, item := range items {
		if prefix, err := netip.ParsePrefix(item); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("server.trusted_proxies: %q is neither an IP address nor a CIDR prefix", item)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

// clientAddr returns the address of the client that sent r. X-Forwarded-For is
// believed only when the connection comes from a trusted proxy, and only as far
// as the chain of trusted proxies goes: walking it from the right, the first
// untrusted hop is the client. Hops further left are set by the client itself.
func clientAddr(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	client := peer.Addr().Unmap()
	if !isTrusted(client, trusted) {
		return client
	}

	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		client = hop.Unmap()
		if !isTrusted(client, trusted) {
			break
		}
	}
	return client
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
