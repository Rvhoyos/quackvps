package web

import (
	"path/filepath"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
)

// geyser is Bedrock crossplay: the Geyser mod bridges Bedrock clients to the Java
// server over UDP, so like Simple Voice Chat it's a plain firewall port, never
// proxied. Two edits go into Geyser's own config.yml: the Bedrock listener port
// and auth-type: floodgate, which hands Bedrock authentication to the paired
// Floodgate mod so players don't need a paid Java account. Floodgate itself needs
// no edits (its defaults generate the shared key.pem Geyser reads on the same box).
//
// Geyser writes its config under a loader-specific folder (config/Geyser-Fabric or
// config/Geyser-NeoForge), so the component carries the loader to find it.
type geyser struct{ loader string }

func (geyser) Key() string              { return config.PortGeyser }
func (geyser) IsWeb() bool              { return false }
func (geyser) DefaultPort() int         { return 19132 }
func (geyser) DefaultSubdomain() string { return "" }
func (geyser) Proto() string            { return "udp" }

func (g geyser) WritePort(dir string, port int) error {
	conf := geyserConfigPath(dir, g.loader)
	// port appears under both bedrock and the Java-server section, so scope this
	// edit to bedrock; auth-type is unique, so a flat edit reaches it wherever it
	// sits.
	if err := setYAMLSectionKey(conf, "bedrock", keysGeyserPort, strconv.Itoa(port)); err != nil {
		return err
	}
	return setHOCONKey(conf, keysGeyserAuth, "floodgate")
}

func (geyser) CaddyBlock(string, string, int) string { return "" }

// geyserModDir maps a loader to the folder Geyser's mod creates under config/.
// Crossplay is only offered on Fabric and NeoForge (the only loaders Geyser and
// Floodgate build for), so those are the only cases.
var geyserModDir = map[string]string{
	config.LoaderFabric:   "Geyser-Fabric",
	config.LoaderNeoForge: "Geyser-NeoForge",
}

// geyserConfigPath is the single source of truth for Geyser's config location,
// used both to write it and to wait for it during the warm-up boot.
func geyserConfigPath(dir, loader string) string {
	return filepath.Join(dir, "config", geyserModDir[loader], "config.yml")
}

// GeyserConfigPath exposes the config path to the install package so the warm-up
// boot can wait for Geyser to generate it.
func GeyserConfigPath(dir, loader string) string {
	return geyserConfigPath(dir, loader)
}
