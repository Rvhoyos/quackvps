package web

import (
	"fmt"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
)

// defaultGamePort is Minecraft's well-known port. Players reach a server on it
// without typing a port, so a server on it needs no SRV record.
const defaultGamePort = 25565

// DNSRecordGuidance returns the DNS block for the end-of-run summary. With a domain
// it's the full "records to create" list; without one it's a shorter, teaching note
// showing the single record a domain would need (the installer doubles as a tutorial,
// so it explains the option rather than staying silent). We never set DNS ourselves.
func DNSRecordGuidance(cfg *config.Config, publicIP string) string {
	if cfg.Domain == "" {
		return noDomainGuidance(cfg, publicIP)
	}

	var b strings.Builder
	b.WriteString("DNS records to create (we don't set these for you):\n\n")
	fmt.Fprintf(&b, "  Your server's IP: %s\n", publicIP)

	if web := webComponents(cfg); len(web) > 0 {
		b.WriteString("\n  Web add-ons:\n")
		for _, c := range web {
			fmt.Fprintf(&b, "    %-4s %s.%s   ->   %s\n", "A", cfg.Subdomains[c.Key()], cfg.Domain, publicIP)
		}
		b.WriteString("    Keep the Cloudflare proxy on. If Caddy can't get the HTTPS certificate,\n")
		b.WriteString("    switch the record to DNS-only, run `sudo systemctl reload caddy`, then\n")
		b.WriteString("    turn the proxy back on (orange cloud).\n")
	}

	join := cfg.Instance + "." + cfg.Domain
	b.WriteString("\n  Minecraft join address:\n")
	fmt.Fprintf(&b, "    %-4s %s   ->   %s\n", "A", join, publicIP)
	if cfg.Geyser {
		// The same A record serves Bedrock (crossplay); it needs no record of its
		// own. The connection how-to lives in BedrockConnectGuidance.
		fmt.Fprintf(&b, "    This A record serves both Java and Bedrock (crossplay).\n")
	}
	if cfg.ServerPort != defaultGamePort {
		fmt.Fprintf(&b, "    %-4s _minecraft._tcp.%s   priority 1  weight 1  port %d  target %s\n",
			"SRV", join, cfg.ServerPort, join)
		fmt.Fprintf(&b, "    Java players join at %s (the SRV record hides the port).\n", join)
	}
	return b.String()
}

// noDomainGuidance is the lighter DNS note for an install with no domain: the server
// already works by the IP shown above, so this only teaches the single record a
// domain would add, with the domain as a placeholder. It stays short on purpose, a
// user who skipped a domain doesn't need the full per-subdomain record list.
func noDomainGuidance(cfg *config.Config, publicIP string) string {
	var b strings.Builder
	b.WriteString("No domain: players connect with the IP above. To use a name instead, get a\n")
	b.WriteString("domain and add one record:\n")
	fmt.Fprintf(&b, "    %-4s %s   ->   %s\n", "A", "<your-domain>", publicIP)
	b.WriteString("  Java players then join at <your-domain>.\n")
	if cfg.Geyser {
		b.WriteString("  Bedrock players put <your-domain> in the address field (the port is still typed).\n")
	}
	if cfg.ServerPort != defaultGamePort {
		b.WriteString("  A nonstandard Java port can be hidden with an SRV record.\n")
	}
	return b.String()
}
