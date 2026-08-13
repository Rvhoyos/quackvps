package web

import (
	"fmt"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
)

// defaultGamePort is Minecraft's well-known port. Players reach a server on it
// without typing a port, so a server on it needs no SRV record.
const defaultGamePort = 25565

// DNSRecordGuidance returns the "here are the DNS records to create" block for the
// end-of-run summary, or "" when there's no domain (the SSH-tunnel path creates no
// records). We never set DNS ourselves, so this tells the user exactly what to add:
// an A record per web add-on subdomain, plus the Minecraft join address for this
// instance, with an SRV record only when the game port isn't the default 25565.
func DNSRecordGuidance(cfg *config.Config, publicIP string) string {
	if cfg.Domain == "" {
		return ""
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
	if cfg.ServerPort != defaultGamePort {
		fmt.Fprintf(&b, "    %-4s _minecraft._tcp.%s   priority 1  weight 1  port %d  target %s\n",
			"SRV", join, cfg.ServerPort, join)
		fmt.Fprintf(&b, "    Players join at %s (the SRV record hides the port).\n", join)
	}
	return b.String()
}
