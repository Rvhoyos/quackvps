package web

import (
	"path/filepath"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
)

// bluemap is the live map: a Caddy-fronted service. Two HOCON config files matter
// (keys verified against the BlueMap 5.x that installs for 1.20.1/1.21.1 — a
// future BlueMap major could rename them):
//   - webserver.conf: we set only `port` and leave the bind field alone (UFW
//     blocks the port externally, Caddy reaches it locally).
//   - core.conf: BlueMap refuses to start its webserver until `accept-download`
//     is true — it fetches Minecraft client resources from Mojang on first run.
//     We set it true because the user already accepted the Minecraft EULA during
//     install (BlueMap's own comment ties this flag to that same EULA).
type bluemap struct{}

func (bluemap) Key() string              { return config.PortBlueMap }
func (bluemap) IsWeb() bool              { return true }
func (bluemap) DefaultPort() int         { return 8100 }
func (bluemap) DefaultSubdomain() string { return "map" }
func (bluemap) Proto() string            { return "" }

func (b bluemap) WritePort(dir string, port int) error {
	base := filepath.Join(dir, "config", "bluemap")
	if err := setHOCONKey(filepath.Join(base, "webserver.conf"), keysBlueMapPort, strconv.Itoa(port)); err != nil {
		return err
	}
	return setHOCONKey(filepath.Join(base, "core.conf"), keysBlueMapAccept, "true")
}

func (b bluemap) CaddyBlock(subdomain, domain string, port int) string {
	return caddyBlock(subdomain, domain, port)
}
