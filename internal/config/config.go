// Package config holds the single Config struct that captures every answer for
// one run, plus Validate — the one gate that must pass before any side effect.
//
// The wizard (internal/prompt) and, later, the flag parser both do nothing but
// fill a Config; execution packages consume it. Keeping that boundary strict is
// what lets the non-interactive CLI (v3) be a thin flag layer rather than a
// rewrite, so Config never reaches into prompting or the system.
package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/mcver"
)

// Mode selects which action a run performs.
type Mode int

const (
	ModeInstall Mode = iota // fresh server into <Parent>/<Instance>
	ModeUpdate              // upgrade an existing instance in place
)

func (m Mode) String() string {
	switch m {
	case ModeInstall:
		return "install"
	case ModeUpdate:
		return "update"
	default:
		return "unknown"
	}
}

// Loader names — the canonical values of Config.Loader. Paper is deliberately
// absent: this tool ships mods, not Bukkit plugins. Forge is included for the
// 1.20.1 era, whose large modpack library is Forge-based (NeoForge's earliest
// version is 1.20.2).
const (
	LoaderFabric   = "fabric"
	LoaderNeoForge = "neoforge"
	LoaderForge    = "forge"
	LoaderQuilt    = "quilt"
	LoaderVanilla  = "vanilla"
)

// Port keys — the map keys for Config.Ports and Config.Subdomains. ServerPort is
// a field of its own; Ports holds only the flagged add-ons.
const (
	PortVoiceChat = "voicechat"
	PortVotifier  = "votifier"
	PortDashboard = "dashboard"
	PortBlueMap   = "bluemap"
)

// MinMCVersion is the oldest release we support. It's NeoForge's own floor —
// NeoForge didn't exist before 1.20.1 — and covers the popular modded era
// (1.20.1/1.19.2/1.18.2). Below it, versions need Java 16/8, which we don't wire
// up. We install Java 17 for 1.18–1.20.4 and Java 21/25 for newer.
const MinMCVersion = "1.20.1"

// Features are the add-ons chosen on the one selection screen. Each flag drives
// its own later port/Caddy/firewall work, so we never re-ask "do you want Caddy?".
type Features struct {
	Dashboard bool // QuackedSMP web panel  → web service (Caddy)
	Votifier  bool // QuackedSMP Votifier v2 → network port (UFW TCP)
	BlueMap   bool // live map              → web service (Caddy)
	VoiceChat bool // Simple Voice Chat      → network port (UFW UDP)
}

// Any reports whether any add-on was selected.
func (f Features) Any() bool { return f.Dashboard || f.Votifier || f.BlueMap || f.VoiceChat }

// AnyWeb reports whether any add-on is a web (Caddy-fronted) service. Only these
// need a domain/subdomain; Votifier and VoiceChat are plain UFW ports.
func (f Features) AnyWeb() bool { return f.Dashboard || f.BlueMap }

// Config is the whole run in one struct: filled by the wizard, consumed by
// everything, validated once before any side effect.
type Config struct {
	Mode Mode

	Parent   string // container that holds instances, e.g. /home/ubuntu/mcserver
	Instance string // instance name (a single path element, not a path)
	Dir      string // <Parent>/<Instance>; set by ResolveDir

	Loader    string
	MCVersion string
	HeapMinGB int // -Xms: heap reserved at startup
	HeapMaxGB int // -Xmx: heap ceiling it may grow to

	ServerPort int      // MC game port (UFW TCP)
	Modpack    string   // Modrinth slug, or "" for none
	Mods       []string // extra mod slugs, e.g. quackedsmp

	Features
	Ports      map[string]int    // add-on ports keyed by the Port* consts
	Domain     string            // "" → no domain: services bind localhost + ssh -L
	Email      string            // ACME contact, only used when adding a Caddy global block
	Subdomains map[string]string // web add-on → subdomain label, e.g. "dashboard":"status"

	HardenSSH   bool
	SSHPubKey   string // public key to append when hardening (may be "" if one exists)
	SSHVerified bool   // user confirmed key login works in a fresh terminal

	RunAsUser string // the invoking login user (e.g. ubuntu); never root
	RunAsHome string // that user's home dir
}

// New returns a Config with the safe defaults that don't depend on any answer.
func New() *Config {
	return &Config{
		HeapMinGB:  1,
		HeapMaxGB:  4,
		Ports:      map[string]int{},
		Subdomains: map[string]string{},
	}
}

// ResolveDir sets Dir from Parent and Instance. Call it after both are known.
func (c *Config) ResolveDir() { c.Dir = filepath.Join(c.Parent, c.Instance) }

var validLoaders = map[string]bool{
	LoaderFabric:   true,
	LoaderNeoForge: true,
	LoaderForge:    true,
	LoaderQuilt:    true,
	LoaderVanilla:  true,
}

// Validate checks every invariant the execution packages rely on. It is the
// single gate: nothing with a side effect should run on a Config that fails it.
//
// The checks split by mode. Both flows need a valid target (parent, instance,
// dir, loader, MC version) and a non-root run-as user. Only install configures
// RAM, ports, features, and the web layer from the wizard — on update those are
// read from the existing server on disk, so they aren't validated here.
func (c *Config) Validate() error {
	if c.Mode != ModeInstall && c.Mode != ModeUpdate {
		return fmt.Errorf("mode not set")
	}

	if !filepath.IsAbs(c.Parent) {
		return fmt.Errorf("parent folder must be an absolute path, got %q", c.Parent)
	}
	if c.Instance == "" || strings.ContainsRune(c.Instance, filepath.Separator) || c.Instance == "." || c.Instance == ".." {
		return fmt.Errorf("instance name must be a single folder name, got %q", c.Instance)
	}
	if want := filepath.Join(c.Parent, c.Instance); c.Dir != want {
		return fmt.Errorf("dir %q does not match parent/instance %q (call ResolveDir)", c.Dir, want)
	}

	if !validLoaders[c.Loader] {
		return fmt.Errorf("unknown loader %q", c.Loader)
	}
	if err := validateMCVersion(c.MCVersion); err != nil {
		return err
	}
	if c.RunAsUser == "" || c.RunAsUser == "root" {
		return fmt.Errorf("run-as user must be a non-root login user, got %q", c.RunAsUser)
	}

	if c.Mode == ModeInstall {
		return c.validateInstall()
	}
	return nil
}

// validateInstall checks the fields only a fresh install configures.
func (c *Config) validateInstall() error {
	if c.HeapMinGB < 1 {
		return fmt.Errorf("starting heap (-Xms) must be at least 1 GB, got %d", c.HeapMinGB)
	}
	if c.HeapMaxGB < c.HeapMinGB {
		return fmt.Errorf("maximum heap (-Xmx) %d GB must be at least the starting heap %d GB", c.HeapMaxGB, c.HeapMinGB)
	}
	if err := validatePort("server", c.ServerPort); err != nil {
		return err
	}
	if err := c.validateFeaturePorts(); err != nil {
		return err
	}
	if err := c.validateUniquePorts(); err != nil {
		return err
	}
	return c.validateSubdomains()
}

func validateMCVersion(s string) error {
	v, err := mcver.Parse(s)
	if err != nil {
		return err
	}
	min, _ := mcver.Parse(MinMCVersion)
	if !mcver.AtLeast(v, min) {
		return fmt.Errorf("minecraft %s is below the supported minimum %s", s, MinMCVersion)
	}
	return nil
}

// featurePortKey maps each enabled feature to the Ports key it must carry.
func (c *Config) featurePortKey() map[string]bool {
	need := map[string]bool{}
	if c.Dashboard {
		need[PortDashboard] = true
	}
	if c.Votifier {
		need[PortVotifier] = true
	}
	if c.BlueMap {
		need[PortBlueMap] = true
	}
	if c.VoiceChat {
		need[PortVoiceChat] = true
	}
	return need
}

func (c *Config) validateFeaturePorts() error {
	for key := range c.featurePortKey() {
		port, ok := c.Ports[key]
		if !ok {
			return fmt.Errorf("feature %q is enabled but has no port", key)
		}
		if err := validatePort(key, port); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateUniquePorts() error {
	seen := map[int]string{c.ServerPort: "server"}
	for key := range c.featurePortKey() {
		port := c.Ports[key]
		if other, clash := seen[port]; clash {
			return fmt.Errorf("port %d used by both %q and %q", port, other, key)
		}
		seen[port] = key
	}
	return nil
}

func (c *Config) validateSubdomains() error {
	for _, key := range []string{PortDashboard, PortBlueMap} {
		enabled := (key == PortDashboard && c.Dashboard) || (key == PortBlueMap && c.BlueMap)
		if !enabled {
			continue
		}
		// A web service only needs a subdomain when we're on the domain path.
		if c.Domain == "" {
			continue
		}
		if c.Subdomains[key] == "" {
			return fmt.Errorf("web service %q needs a subdomain when a domain is set", key)
		}
	}
	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s port %d out of range 1-65535", name, port)
	}
	return nil
}
