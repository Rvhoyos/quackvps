package caddy

import "net"

// DNSResolvesTo reports whether host currently resolves to publicIP. The install
// flow only warns on a mismatch and continues, because Caddy retries ACME on its
// own once DNS propagates — we never block an install on DNS.
func DNSResolvesTo(host, publicIP string) bool {
	addrs, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if a == publicIP {
			return true
		}
	}
	return false
}
