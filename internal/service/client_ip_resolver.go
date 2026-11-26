package service

import (
	"errors"
	"net/netip"
	"strings"
)

// ClientIPResolver resolves forwarding headers only across an explicitly
// trusted reverse-proxy chain. It is safe to share between request handlers.
type ClientIPResolver struct {
	trustedProxies []netip.Prefix
}

func NewClientIPResolver(trustedProxyCIDRs []string) (*ClientIPResolver, error) {
	resolver := &ClientIPResolver{}
	for _, value := range trustedProxyCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil || prefix.Bits() == 0 {
			return nil, errors.New("invalid trusted proxy CIDR")
		}
		resolver.trustedProxies = append(resolver.trustedProxies, prefix.Masked())
	}
	return resolver, nil
}

func (resolver *ClientIPResolver) Resolve(
	remoteAddress string,
	cfConnectingIP string,
	xRealIP string,
	forwardedFor ...string,
) string {
	peer, ok := parseRemoteAddress(remoteAddress)
	if !ok {
		return ""
	}
	if resolver == nil || !resolver.isTrustedProxy(peer) {
		return peer.String()
	}

	if candidate, err := netip.ParseAddr(strings.TrimSpace(cfConnectingIP)); err == nil {
		return candidate.Unmap().String()
	}
	if chain, valid := parseForwardedFor(forwardedFor); valid && len(chain) > 0 {
		candidate := peer
		for index := len(chain) - 1; index >= 0 && resolver.isTrustedProxy(candidate); index-- {
			candidate = chain[index]
		}
		return candidate.Unmap().String()
	}
	if candidate, err := netip.ParseAddr(strings.TrimSpace(xRealIP)); err == nil {
		return candidate.Unmap().String()
	}
	return peer.String()
}

func (resolver *ClientIPResolver) isTrustedProxy(address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range resolver.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseRemoteAddress(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap(), true
	}
	address, err := netip.ParseAddr(strings.Trim(value, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func parseForwardedFor(values []string) ([]netip.Addr, bool) {
	chain := make([]netip.Addr, 0, len(values))
	for _, header := range values {
		for _, raw := range strings.Split(header, ",") {
			address, err := netip.ParseAddr(strings.TrimSpace(raw))
			if err != nil {
				return nil, false
			}
			chain = append(chain, address.Unmap())
		}
	}
	return chain, true
}
