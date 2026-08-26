package web

import (
	"path/filepath"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
)

// voicechat is Simple Voice Chat: a plain UDP port opened in the firewall. Its
// port lives in voicechat-server.properties. Unlike the web services, voice is a
// direct public UDP connection (clients hit voice_host straight), so it's never
// proxied, we only set the port and open the firewall.
type voicechat struct{}

func (voicechat) Key() string              { return config.PortVoiceChat }
func (voicechat) IsWeb() bool              { return false }
func (voicechat) DefaultPort() int         { return 24454 }
func (voicechat) DefaultSubdomain() string { return "" }
func (voicechat) Proto() string            { return "udp" }

func (v voicechat) WritePort(dir string, port int) error {
	conf := voiceChatConfig(dir)
	return setPropKey(conf, keysVoicePort, strconv.Itoa(port))
}

// SetVoiceHost sets voice_host to the server's public IP. The mod leaves this
// empty by default and auto-detects the address to hand clients; on a VPS that
// detection can land on an internal address, so clients get an unreachable host
// and can't connect. Writing the public IP overrides that. Voice is a direct UDP
// connection (never proxied), so the raw IP is what clients need.
func SetVoiceHost(dir, ip string) error {
	conf := voiceChatConfig(dir)
	return setPropKey(conf, keysVoiceHost, ip)
}

func (voicechat) CaddyBlock(string, string, int) string { return "" }

// voiceChatConfig is the single source of truth for the mod's config location,
// written on install and read back when a server is removed.
func voiceChatConfig(dir string) string {
	return filepath.Join(dir, "config", "voicechat", "voicechat-server.properties")
}
