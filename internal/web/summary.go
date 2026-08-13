package web

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
)

// AccessSummary builds the single end-of-run block telling the user how to reach
// their web services. It returns the text so the caller controls printing (and so
// it's testable). Two shapes, per the domain choice made up front:
//
//   - domain path: one HTTPS URL per web service.
//   - no-domain path: one combined `ssh -L` command forwarding every web port,
//     then the localhost URLs to open after running it.
//
// Only web components appear, voice/votifier are plain firewall ports with
// nothing to browse to.
func AccessSummary(cfg *config.Config, sshUserHost string) string {
	web := webComponents(cfg)
	if len(web) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Web services:\n")

	if cfg.Domain != "" {
		for _, c := range web {
			sub := cfg.Subdomains[c.Key()]
			fmt.Fprintf(&b, "  https://%s.%s\n", sub, cfg.Domain)
		}
		return b.String()
	}

	// No domain: one tunnel command forwarding all web ports, then the URLs.
	var forwards []string
	for _, c := range web {
		port := cfg.Ports[c.Key()]
		forwards = append(forwards, fmt.Sprintf("-L %d:localhost:%d", port, port))
	}
	b.WriteString("  These bind to localhost only. In a NEW terminal on your own machine, run:\n")
	fmt.Fprintf(&b, "    ssh %s %s\n", strings.Join(forwards, " "), sshUserHost)
	b.WriteString("  Then open:\n")
	for _, c := range web {
		fmt.Fprintf(&b, "    http://localhost:%d\n", cfg.Ports[c.Key()])
	}
	return b.String()
}

// BedrockConnectGuidance explains how Bedrock (crossplay) players connect, for the
// end-of-run summary. Unlike Java, Bedrock can't carry a port in DNS (it ignores
// SRV records), so the address and port always go in separate fields of the Add
// Server screen. Every rule here is Bedrock-only, Java players are unaffected (they
// still get the SRV record); the note is static because it holds on every path.
// host is the domain when one is set, otherwise the raw IP.
func BedrockConnectGuidance(host string, port int) string {
	var b strings.Builder
	b.WriteString("Bedrock players (crossplay): in Add Server, fill the address and port in\n")
	b.WriteString("separate fields, Bedrock can't put the port in a DNS record.\n")
	fmt.Fprintf(&b, "    Server Address: %s\n", host)
	fmt.Fprintf(&b, "    Port:           %d\n", port)
	b.WriteString("  Only a server on port 19132 (Bedrock's default) lets Bedrock players skip\n")
	b.WriteString("  the port field. You can run several crossplay servers, each on its own port,\n")
	b.WriteString("  and Bedrock players type that port for any server not on 19132. Java players\n")
	b.WriteString("  are unaffected, they can still use an SRV record to hide the Java port.\n")
	return b.String()
}

// webComponents returns the enabled web components in a stable order.
func webComponents(cfg *config.Config) []Component {
	var web []Component
	for _, c := range Components(cfg.Features, cfg.Loader) {
		if c.IsWeb() {
			web = append(web, c)
		}
	}
	sort.Slice(web, func(i, j int) bool { return web[i].Key() < web[j].Key() })
	return web
}
