package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvhoyos/quackvps/internal/config"
)

func TestComponentsRegistry(t *testing.T) {
	f := config.Features{Dashboard: true, BlueMap: true, Votifier: true, VoiceChat: true, Geyser: true}
	comps := Components(f, config.LoaderNeoForge)
	if len(comps) != 5 {
		t.Fatalf("expected 5 components, got %d", len(comps))
	}
	// Web vs firewall split must match design.
	web, net := map[string]bool{}, map[string]bool{}
	for _, c := range comps {
		if c.IsWeb() {
			web[c.Key()] = true
			if c.CaddyBlock("sub", "example.com", 8125) == "" {
				t.Errorf("%s is web but produced no Caddy block", c.Key())
			}
			if c.Proto() != "" {
				t.Errorf("%s is web but has a firewall proto", c.Key())
			}
		} else {
			net[c.Key()] = true
			if c.CaddyBlock("sub", "example.com", 1) != "" {
				t.Errorf("%s is not web but produced a Caddy block", c.Key())
			}
			if c.Proto() == "" {
				t.Errorf("%s is a firewall port but has no proto", c.Key())
			}
		}
	}
	if !web["dashboard"] || !web["bluemap"] || !net["votifier"] || !net["voicechat"] || !net["geyser"] {
		t.Errorf("web/net split wrong: web=%v net=%v", web, net)
	}
}

// writeQuackedConfig seeds a quackedsmp.json like the mod generates, so the
// edit-only editors have something to edit.
func writeQuackedConfig(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "config", "quackedsmp.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{
  "dashboard": {"enabled": false, "port": 8125, "server_name": "keep me"},
  "votifier": {"enabled": false, "port": 8192},
  "panel_url": "",
  "voicechat_enable": false
}`), 0o644)
}

func TestDashboardWritePort(t *testing.T) {
	dir := t.TempDir()
	writeQuackedConfig(t, dir)

	if err := (dashboard{}).WritePort(dir, 8130); err != nil {
		t.Fatal(err)
	}
	if err := (votifier{}).WritePort(dir, 8192); err != nil {
		t.Fatal(err)
	}
	if err := SetPanelURL(dir, "https://status.example.com"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "config", "quackedsmp.json"))
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	dash := m["dashboard"].(map[string]any)
	if dash["port"].(float64) != 8130 || dash["enabled"] != true {
		t.Errorf("dashboard section wrong: %v", dash)
	}
	// Editing must preserve sibling keys the mod owns.
	if dash["server_name"] != "keep me" {
		t.Errorf("editing dashboard clobbered a sibling key: %v", dash)
	}
	if m["votifier"].(map[string]any)["port"].(float64) != 8192 {
		t.Errorf("votifier section wrong: %v", m["votifier"])
	}
	if m["panel_url"] != "https://status.example.com" {
		t.Errorf("panel_url wrong: %v", m["panel_url"])
	}
}

func TestWritePortErrorsWhenConfigMissing(t *testing.T) {
	dir := t.TempDir() // no quackedsmp.json / bluemap / voicechat configs
	if err := (dashboard{}).WritePort(dir, 8130); err == nil {
		t.Error("dashboard WritePort should error when quackedsmp.json is absent")
	}
	if err := (bluemap{}).WritePort(dir, 8101); err == nil {
		t.Error("bluemap WritePort should error when its config is absent")
	}
	if err := (voicechat{}).WritePort(dir, 24455); err == nil {
		t.Error("voicechat WritePort should error when its config is absent")
	}
	if err := (geyser{config.LoaderNeoForge}).WritePort(dir, 19133); err == nil {
		t.Error("geyser WritePort should error when its config is absent")
	}
}

func TestSetHOCONKeyEditsAndErrors(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "config", "bluemap")
	os.MkdirAll(base, 0o755)
	os.WriteFile(filepath.Join(base, "webserver.conf"), []byte("enabled: true\nport: 8100\nwebroot: \"web\"\n"), 0o644)
	os.WriteFile(filepath.Join(base, "core.conf"), []byte("accept-download: false\ndata: \"bluemap\"\n"), 0o644)

	if err := (bluemap{}).WritePort(dir, 8101); err != nil {
		t.Fatal(err)
	}
	ws, _ := os.ReadFile(filepath.Join(base, "webserver.conf"))
	if !strings.Contains(string(ws), "port: 8101") || !strings.Contains(string(ws), `webroot: "web"`) {
		t.Errorf("webserver.conf edit wrong:\n%s", ws)
	}
	core, _ := os.ReadFile(filepath.Join(base, "core.conf"))
	if !strings.Contains(string(core), "accept-download: true") || !strings.Contains(string(core), `data: "bluemap"`) {
		t.Errorf("core.conf edit wrong:\n%s", core)
	}

	// A file missing the key entirely → reportable error, not a silent no-op.
	os.WriteFile(filepath.Join(base, "core.conf"), []byte("data: \"bluemap\"\n"), 0o644)
	if err := (bluemap{}).WritePort(dir, 8101); err == nil {
		t.Error("expected error when accept-download key is absent")
	}
}

func TestVoiceChatWritePortAndHost(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config", "voicechat")
	os.MkdirAll(conf, 0o755)
	// The mod generates voice_host empty; port has a default.
	os.WriteFile(filepath.Join(conf, "voicechat-server.properties"),
		[]byte("port=24454\nvoice_host=\nmax_voice_distance=48\n"), 0o644)

	if err := (voicechat{}).WritePort(dir, 24455); err != nil {
		t.Fatal(err)
	}
	if err := SetVoiceHost(dir, "203.0.113.7"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(conf, "voicechat-server.properties"))
	for _, want := range []string{"port=24455", "voice_host=203.0.113.7", "max_voice_distance=48"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("properties missing %q:\n%s", want, got)
		}
	}

	// A file without the voice_host key → reportable error, not a silent no-op.
	os.WriteFile(filepath.Join(conf, "voicechat-server.properties"), []byte("port=24454\n"), 0o644)
	if err := SetVoiceHost(dir, "203.0.113.7"); err == nil {
		t.Error("SetVoiceHost should error when voice_host key is absent")
	}
}

// geyserConfig is a trimmed Geyser config.yml with the two-section `port` that
// makes the section-scoped edit necessary, plus comments the edit must preserve.
const geyserConfig = `bedrock:
  # The IP address that Geyser will bind to.
  address: 0.0.0.0
  # The port Geyser listens on for Bedrock.
  port: 19132
  clone-remote-port: false
java:
  # The IP address of the Java server.
  address: 127.0.0.1
  port: 25565
  # floodgate, online, or offline.
  auth-type: online
`

func TestGeyserWritePort(t *testing.T) {
	dir := t.TempDir()
	conf := GeyserConfigPath(dir, config.LoaderFabric)
	os.MkdirAll(filepath.Dir(conf), 0o755)
	os.WriteFile(conf, []byte(geyserConfig), 0o644)

	if err := (geyser{config.LoaderFabric}).WritePort(dir, 19133); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, conf))
	if !strings.Contains(got, "  port: 19133") {
		t.Errorf("bedrock port not set:\n%s", got)
	}
	// The Java-server port shares the key name but must be left untouched.
	if !strings.Contains(got, "  port: 25565") {
		t.Errorf("java port was clobbered:\n%s", got)
	}
	if !strings.Contains(got, "auth-type: floodgate") {
		t.Errorf("auth-type not switched to floodgate:\n%s", got)
	}
	// Comments and untouched keys survive.
	for _, keep := range []string{"# The port Geyser listens on for Bedrock.", "clone-remote-port: false", "address: 0.0.0.0"} {
		if !strings.Contains(got, keep) {
			t.Errorf("edit lost %q:\n%s", keep, got)
		}
	}
}

func TestSetYAMLSectionKeyErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	os.WriteFile(path, []byte(geyserConfig), 0o644)

	// Missing section → reportable error.
	if err := setYAMLSectionKey(path, "nope", []string{"port"}, "1"); err == nil {
		t.Error("expected error for a missing section")
	}
	// Section present but key absent within it → reportable error (not a silent
	// no-op, and not reaching into another section).
	if err := setYAMLSectionKey(path, "bedrock", []string{"auth-type"}, "x"); err == nil {
		t.Error("expected error when the key is absent from the section")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBlueMapPresent(t *testing.T) {
	dir := t.TempDir()
	if BlueMapPresent(dir) {
		t.Error("BlueMapPresent should be false with no config/bluemap")
	}
	os.MkdirAll(filepath.Join(dir, "config", "bluemap"), 0o755)
	if !BlueMapPresent(dir) {
		t.Error("BlueMapPresent should be true once config/bluemap exists")
	}
}

func TestResetBlueMapMaps(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "config", "bluemap")
	maps := filepath.Join(base, "maps")
	os.MkdirAll(maps, 0o755)
	// The files an update must reset...
	for _, m := range []string{"world.conf", "world_the_nether.conf", "world_the_end.conf"} {
		os.WriteFile(filepath.Join(maps, m), []byte("max-y: 90\n"), 0o644)
	}
	// ...and the ones it must leave alone (they carry our edits).
	os.WriteFile(filepath.Join(base, "core.conf"), []byte("accept-download: true\n"), 0o644)
	os.WriteFile(filepath.Join(base, "webserver.conf"), []byte("port: 8100\n"), 0o644)

	if err := ResetBlueMapMaps(dir); err != nil {
		t.Fatal(err)
	}
	// The maps folder itself must be gone: BlueMap only regenerates defaults when it
	// is absent, so an empty-but-present folder would leave the map broken.
	if _, err := os.Stat(maps); !os.IsNotExist(err) {
		t.Errorf("maps dir still present after reset (err=%v); BlueMap won't regenerate", err)
	}
	for _, keep := range []string{"core.conf", "webserver.conf"} {
		if _, err := os.Stat(filepath.Join(base, keep)); err != nil {
			t.Errorf("ResetBlueMapMaps removed %s, which it must not touch", keep)
		}
	}

	// A server without BlueMap's maps dir is not an error.
	if err := ResetBlueMapMaps(t.TempDir()); err != nil {
		t.Errorf("ResetBlueMapMaps on an absent maps dir should be a no-op, got %v", err)
	}
}

func TestAccessSummary(t *testing.T) {
	cfg := config.New()
	cfg.Features = config.Features{Dashboard: true, BlueMap: true, VoiceChat: true}
	cfg.Ports = map[string]int{"dashboard": 8125, "bluemap": 8100, "voicechat": 24454}

	// Domain path: HTTPS URLs, no ssh -L, no voice port.
	cfg.Domain = "example.com"
	cfg.Subdomains = map[string]string{"dashboard": "status", "bluemap": "map"}
	got := AccessSummary(cfg, "ubuntu@vps")
	if !strings.Contains(got, "https://status.example.com") || !strings.Contains(got, "https://map.example.com") {
		t.Errorf("domain summary missing URLs:\n%s", got)
	}
	if strings.Contains(got, "24454") {
		t.Errorf("voice port should not appear in web summary:\n%s", got)
	}

	// No-domain path: one combined ssh -L with both web ports.
	cfg.Domain = ""
	got = AccessSummary(cfg, "ubuntu@vps")
	if !strings.Contains(got, "-L 8125:localhost:8125") || !strings.Contains(got, "-L 8100:localhost:8100") {
		t.Errorf("tunnel command missing forwards:\n%s", got)
	}
	if strings.Count(got, "ssh ") != 1 {
		t.Errorf("expected exactly one ssh command:\n%s", got)
	}
}

func TestDNSRecordGuidance(t *testing.T) {
	cfg := config.New()
	cfg.Instance = "survival"
	cfg.Features = config.Features{Dashboard: true, BlueMap: true}
	cfg.Subdomains = map[string]string{"dashboard": "status", "bluemap": "map"}

	// No domain → the light teaching note: a placeholder record with the real IP,
	// not the full per-subdomain record list.
	got := DNSRecordGuidance(cfg, "1.2.3.4")
	if !strings.Contains(got, "<your-domain>") || !strings.Contains(got, "1.2.3.4") {
		t.Errorf("no-domain guidance should show a placeholder record with the IP:\n%s", got)
	}
	if strings.Contains(got, "records to create") || strings.Contains(got, "status.") {
		t.Errorf("no-domain path should not print the full domain record list:\n%s", got)
	}

	cfg.Domain = "example.com"

	// Non-standard game port → SRV record present.
	cfg.ServerPort = 25567
	got = DNSRecordGuidance(cfg, "1.2.3.4")
	for _, want := range []string{
		"1.2.3.4",
		"A    status.example.com",
		"A    map.example.com",
		"A    survival.example.com",
		"SRV  _minecraft._tcp.survival.example.com",
		"port 25567",
		"target survival.example.com",
		// The join A record must not be Cloudflare-proxied, or the game port is blocked.
		"DNS only (grey cloud)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("guidance missing %q:\n%s", want, got)
		}
	}

	// Standard game port → the join A record stays, but no SRV record.
	cfg.ServerPort = 25565
	got = DNSRecordGuidance(cfg, "1.2.3.4")
	if !strings.Contains(got, "A    survival.example.com") {
		t.Errorf("standard-port guidance missing the join A record:\n%s", got)
	}
	if strings.Contains(got, "SRV") || strings.Contains(got, "_minecraft._tcp") {
		t.Errorf("standard port should get no SRV record:\n%s", got)
	}
}

func TestDNSRecordGuidanceBedrock(t *testing.T) {
	cfg := config.New()
	cfg.Instance = "survival"
	cfg.Domain = "example.com"
	cfg.ServerPort = 25565 // default Java port, so no Java SRV muddies the check
	cfg.Geyser = true
	cfg.Ports = map[string]int{config.PortGeyser: 19132}

	got := DNSRecordGuidance(cfg, "1.2.3.4")
	// dns.go only lists records to create: the one join A record serves both editions.
	if !strings.Contains(got, "serves both Java and Bedrock") {
		t.Errorf("guidance should note the A record serves Bedrock too:\n%s", got)
	}
	// Bedrock never gets a record of its own; connection fields live elsewhere now.
	if strings.Contains(got, "Server Address:") || strings.Contains(got, "_minecraft._udp") {
		t.Errorf("dns.go should not carry Bedrock connect fields or a Bedrock SRV:\n%s", got)
	}
}

func TestNoDomainGuidance(t *testing.T) {
	cfg := config.New()
	cfg.Instance = "survival"
	cfg.ServerPort = 25565 // default → no SRV tip
	cfg.Geyser = true      // → the Bedrock line

	got := DNSRecordGuidance(cfg, "2.25.105.33")
	t.Logf("\n%s", got) // eyeball the rendered copy
	for _, want := range []string{"No domain", "A    <your-domain>   ->   2.25.105.33", "Bedrock players put <your-domain>"} {
		if !strings.Contains(got, want) {
			t.Errorf("no-domain guidance missing %q:\n%s", want, got)
		}
	}
	// Default port and this is a teaching note, not a record list.
	if strings.Contains(got, "SRV") || strings.Contains(got, "records to create") {
		t.Errorf("default-port no-domain note should stay minimal:\n%s", got)
	}

	// Without crossplay, no Bedrock line.
	cfg.Geyser = false
	if got := DNSRecordGuidance(cfg, "2.25.105.33"); strings.Contains(got, "Bedrock") {
		t.Errorf("no-geyser note should not mention Bedrock:\n%s", got)
	}
}

func TestBedrockConnectGuidance(t *testing.T) {
	// Default port: the note still tells players a 19132 server skips the port field.
	got := BedrockConnectGuidance("play.example.com", 19132)
	for _, want := range []string{"Server Address: play.example.com", "Port:", "19132", "several crossplay servers"} {
		if !strings.Contains(got, want) {
			t.Errorf("guidance missing %q:\n%s", want, got)
		}
	}

	// Non-default port (a bumped second instance): the actual port is shown. It may
	// mention SRV for Java, but must never suggest a Bedrock SRV record.
	got = BedrockConnectGuidance("1.2.3.4", 19133)
	if !strings.Contains(got, "Port:           19133") {
		t.Errorf("guidance missing the chosen port:\n%s", got)
	}
	if !strings.Contains(got, "Java players") {
		t.Errorf("guidance should scope the port rule to Bedrock and clear Java:\n%s", got)
	}
	if strings.Contains(got, "_minecraft") {
		t.Errorf("bedrock guidance must never suggest a Bedrock SRV record:\n%s", got)
	}
}
