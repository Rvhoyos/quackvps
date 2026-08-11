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
	f := config.Features{Dashboard: true, BlueMap: true, Votifier: true, VoiceChat: true}
	comps := Components(f)
	if len(comps) != 4 {
		t.Fatalf("expected 4 components, got %d", len(comps))
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
	if !web["dashboard"] || !web["bluemap"] || !net["votifier"] || !net["voicechat"] {
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
