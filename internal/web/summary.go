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

// webComponents returns the enabled web components in a stable order.
func webComponents(cfg *config.Config) []Component {
	var web []Component
	for _, c := range Components(cfg.Features) {
		if c.IsWeb() {
			web = append(web, c)
		}
	}
	sort.Slice(web, func(i, j int) bool { return web[i].Key() < web[j].Key() })
	return web
}
