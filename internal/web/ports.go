package web

import (
	"path/filepath"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/minecraft"
)

// Port is a firewall port an installed instance uses, read back from the add-on's
// own config. Label names it in plain language for the user.
type Port struct {
	Label  string
	Number int
	Proto  string // "tcp" or "udp"
}

// InstalledPorts reports the ports an existing server's add-ons have open, read
// from the same config keys the installer wrote them to. It's how removing a
// server closes exactly what installing it opened, on a server this tool set up or
// one it didn't. An add-on that isn't installed has no config, and contributes
// nothing.
//
// The Caddy-fronted components (the QuackedSMP dashboard, BlueMap) are absent by
// design: they're reached through the proxy and never opened in the firewall.
func InstalledPorts(dir string) []Port {
	var ports []Port
	if p, ok := voiceChatPort(dir); ok {
		ports = append(ports, Port{Label: "Simple Voice Chat", Number: p, Proto: "udp"})
	}
	if p, ok := votifierPort(dir); ok {
		ports = append(ports, Port{Label: "Votifier", Number: p, Proto: "tcp"})
	}
	if p, ok := geyserPort(dir); ok {
		ports = append(ports, Port{Label: "Bedrock crossplay", Number: p, Proto: "udp"})
	}
	return ports
}

func voiceChatPort(dir string) (int, bool) {
	props, err := minecraft.ReadProps(voiceChatConfig(dir))
	if err != nil {
		return 0, false
	}
	for _, key := range keysVoicePort {
		if port, err := strconv.Atoi(props[key]); err == nil {
			return port, true
		}
	}
	return 0, false
}

// votifierPort reads the vote listener's port out of quackedsmp.json, and only
// when the feature is switched on: the mod ships the block with a default port
// whether or not it's in use, and a port nothing listens on isn't ours to close.
func votifierPort(dir string) (int, bool) {
	m, err := loadJSON(quackedsmpConfig(dir))
	if err != nil {
		return 0, false
	}
	section, err := jsonSection(m, keysVotifier, "quackedsmp.json")
	if err != nil {
		return 0, false
	}
	if key, err := jsonKey(section, keysSectionEnabled, "quackedsmp.json section"); err != nil || section[key] != true {
		return 0, false
	}
	key, err := jsonKey(section, keysSectionPort, "quackedsmp.json section")
	if err != nil {
		return 0, false
	}
	// JSON numbers decode as float64; a port is small enough for that to be exact.
	port, ok := section[key].(float64)
	return int(port), ok
}

// geyserPort reads the Bedrock listener's port. Geyser's config sits under a
// loader-specific folder, and the loader of a server we're only inspecting isn't
// always knowable, so this looks for either.
func geyserPort(dir string) (int, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "config", "Geyser-*", "config.yml"))
	if err != nil || len(matches) == 0 {
		return 0, false
	}
	value, err := readYAMLSectionKey(matches[0], "bedrock", keysGeyserPort)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(value)
	return port, err == nil
}
