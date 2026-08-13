// Package web models the add-on components that need a port: the two Caddy-fronted
// web services (the QuackedSMP dashboard and BlueMap) and the two plain firewall
// ports (Votifier and Simple Voice Chat). Each component knows how to write its
// own port into its own config and, if it's a web service, how to render its Caddy
// site block. The install flow drives them uniformly off the feature flags.
package web

import (
	"github.com/rvhoyos/quackvps/internal/config"
)

// Component is one port-owning add-on. IsWeb splits the two worlds cleanly: web
// components produce a Caddy block and are never opened in the firewall; the rest
// produce a firewall rule and never touch Caddy.
type Component interface {
	// Key is the component's identifier, matching its config.Ports key.
	Key() string
	// IsWeb reports whether it's fronted by Caddy (vs. a plain firewall port).
	IsWeb() bool
	// DefaultPort is the suggested port before collision-checking.
	DefaultPort() int
	// DefaultSubdomain is the suggested subdomain label for web components ("" for
	// non-web).
	DefaultSubdomain() string
	// WritePort writes the chosen port into the component's own config under dir.
	WritePort(dir string, port int) error
	// CaddyBlock renders the reverse-proxy site block ("" when !IsWeb).
	CaddyBlock(subdomain, domain string, port int) string
	// Proto is the firewall protocol ("tcp"/"udp") for non-web components ("" when
	// IsWeb).
	Proto() string
}

// Components returns the enabled components in a stable order, derived from the
// feature flags. This is the single registry the install flow iterates. The loader
// is needed by Geyser, whose config lives under a loader-specific folder.
func Components(f config.Features, loader string) []Component {
	var comps []Component
	if f.Dashboard {
		comps = append(comps, dashboard{})
	}
	if f.BlueMap {
		comps = append(comps, bluemap{})
	}
	if f.Votifier {
		comps = append(comps, votifier{})
	}
	if f.VoiceChat {
		comps = append(comps, voicechat{})
	}
	if f.Geyser {
		comps = append(comps, geyser{loader})
	}
	return comps
}
