package aghnet

import (
	"net/netip"
	"strings"
)

// GenerateHostname generates the hostname from ip.  In case of using IPv4 the
// result should be like:
//
//	192-168-10-1
//
// In case of using IPv6, the result is like:
//
//	ff80-f076-0000-0000-0000-0000-0000-0010
//
// ip must be either an IPv4 or an IPv6.
func GenerateHostname(ip netip.Addr) (hostname string) {
	if !ip.IsValid() {
		// TODO(s.chzhen):  Get rid of it.
		panic("aghnet generate hostname: invalid ip")
	}

	ip = ip.Unmap()

	if ip.Is4() {
		hostname = ip.String()

		return strings.Replace(hostname, ".", "-", -1)
	}

	hostname = ip.StringExpanded()

	return strings.Replace(hostname, ":", "-", -1)
}
