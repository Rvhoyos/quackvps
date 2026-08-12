package web

import (
	"fmt"
	"os"
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

// BlueMapPresent reports whether BlueMap is installed in an instance, by the
// presence of its config directory — so an update knows it will upgrade BlueMap
// without relying on our own records or a hardcoded Modrinth id.
func BlueMapPresent(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "config", "bluemap"))
	return err == nil && info.IsDir()
}

// ResetBlueMapMaps deletes the map configs under config/bluemap/maps so an upgraded
// BlueMap regenerates fresh defaults on its next boot. BlueMap only writes a default
// when the file is absent and refuses to auto-migrate an outdated one, so a version
// jump that changes the map-config schema needs the old files gone. It touches only
// the map configs — never core.conf/webserver.conf, which carry our port and
// accept-download edits. A missing maps directory is not an error.
func ResetBlueMapMaps(dir string) error {
	confs, err := filepath.Glob(filepath.Join(dir, "config", "bluemap", "maps", "*.conf"))
	if err != nil {
		return fmt.Errorf("list bluemap map configs: %w", err)
	}
	for _, conf := range confs {
		if err := os.Remove(conf); err != nil {
			return fmt.Errorf("reset bluemap map config %s: %w", filepath.Base(conf), err)
		}
	}
	return nil
}
