package web

import (
	"path/filepath"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
)

// voicechat is Simple Voice Chat: a plain UDP port opened in the firewall. Its
// port lives in voicechat-server.properties. Unlike the web services, voice is a
// direct public UDP connection (clients hit voice_host straight), so it's never
// proxied — we only set the port and open the firewall.
type voicechat struct{}

func (voicechat) Key() string              { return config.PortVoiceChat }
func (voicechat) IsWeb() bool              { return false }
func (voicechat) DefaultPort() int         { return 24454 }
func (voicechat) DefaultSubdomain() string { return "" }
func (voicechat) Proto() string            { return "udp" }

func (v voicechat) WritePort(dir string, port int) error {
	conf := filepath.Join(dir, "config", "voicechat", "voicechat-server.properties")
	return setPropKey(conf, keysVoicePort, strconv.Itoa(port))
}

func (voicechat) CaddyBlock(string, string, int) string { return "" }
