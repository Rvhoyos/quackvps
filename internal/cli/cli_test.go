package cli

import (
	"context"
	"io"
	"testing"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/restore"
)

func TestValidateMode(t *testing.T) {
	for _, m := range []string{"", "install", "update", "restore", "add-mods"} {
		if err := validateMode(m); err != nil {
			t.Errorf("mode %q should be accepted: %v", m, err)
		}
	}
	if err := validateMode("wipe"); err == nil {
		t.Error("unknown mode should be rejected")
	}
}

func TestResolveBackup(t *testing.T) {
	backups := []restore.Backup{
		{Path: "/srv/mc/backups/world-20260610-161024.zip"},
		{Path: "/srv/mc/backups/world-20260101-000000.zip"},
	}
	tests := []struct {
		name    string
		want    string
		expect  string
		wantErr bool
	}{
		{"empty", "", "", true},
		{"full path", "/srv/mc/backups/world-20260101-000000.zip", "/srv/mc/backups/world-20260101-000000.zip", false},
		{"basename", "world-20260610-161024.zip", "/srv/mc/backups/world-20260610-161024.zip", false},
		{"missing", "world-nope.zip", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBackup(tt.want, backups)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expect {
				t.Errorf("got %q, want %q", got, tt.expect)
			}
		})
	}
}

// TestConfigureInstallMapping checks the flag→Config mapping for install: features
// pull in their mods, and only enabled components carry a port/subdomain. It uses a
// parent that doesn't exist, so the on-disk "already a server" guard passes.
func TestConfigureInstallMapping(t *testing.T) {
	opts := Options{
		Mode:               "install",
		Parent:             "/nonexistent-quackvps-test/mc",
		Instance:           "survival",
		Loader:             config.LoaderNeoForge,
		MCVersion:          "1.21.8",
		HeapMinGB:          2,
		HeapMaxGB:          6,
		ServerPort:         25565,
		BlueMap:            true,
		BlueMapPort:        8100,
		BlueMapSubdomain:   "map",
		Dashboard:          true,
		DashboardPort:      8125,
		DashboardSubdomain: "status",
		Geyser:             true,
		GeyserPort:         19132,
		Domain:             "example.com",
	}
	cfg := config.New()
	cfg.RunAsUser, cfg.RunAsHome = "ubuntu", "/home/ubuntu"
	if err := Configure(context.Background(), cfg, opts); err != nil {
		t.Fatalf("configure: %v", err)
	}

	if cfg.Mode != config.ModeInstall {
		t.Errorf("mode = %v, want install", cfg.Mode)
	}
	if !hasMod(cfg.Mods, "bluemap") || !hasMod(cfg.Mods, "quackedsmp") {
		t.Errorf("mods %v missing bluemap/quackedsmp", cfg.Mods)
	}
	// Crossplay is one checkbox that pulls in both mods.
	if !hasMod(cfg.Mods, "geyser") || !hasMod(cfg.Mods, "floodgate") {
		t.Errorf("mods %v missing geyser/floodgate", cfg.Mods)
	}
	if cfg.Ports[config.PortBlueMap] != 8100 || cfg.Ports[config.PortDashboard] != 8125 {
		t.Errorf("ports = %v", cfg.Ports)
	}
	if cfg.Ports[config.PortGeyser] != 19132 {
		t.Errorf("geyser port = %v, want 19132", cfg.Ports[config.PortGeyser])
	}
	if _, ok := cfg.Ports[config.PortVoiceChat]; ok {
		t.Error("disabled voicechat should carry no port")
	}
	if cfg.Subdomains[config.PortDashboard] != "status" {
		t.Errorf("dashboard subdomain = %q", cfg.Subdomains[config.PortDashboard])
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("mapped config should validate: %v", err)
	}
}

func TestConfigureHardenSSHNeedsKey(t *testing.T) {
	opts := Options{
		Mode:      "install",
		Parent:    "/nonexistent-quackvps-test/mc",
		Instance:  "survival",
		HardenSSH: true, // no --ssh-pubkey
	}
	if err := Configure(context.Background(), config.New(), opts); err == nil {
		t.Fatal("harden without a pubkey should be rejected")
	}
}

func hasMod(mods []string, want string) bool {
	for _, m := range mods {
		if m == want {
			return true
		}
	}
	return false
}

func TestSplitSlugs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"simple-voice-chat", []string{"simple-voice-chat"}},
		{"bluemap,simple-voice-chat", []string{"bluemap", "simple-voice-chat"}},
		{" bluemap , simple-voice-chat ", []string{"bluemap", "simple-voice-chat"}},
		{"bluemap,", []string{"bluemap"}}, // trailing comma isn't an error
		{",,", nil},
	}
	for _, tt := range tests {
		got := splitSlugs(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitSlugs(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitSlugs(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

func TestConfigureAddModsNeedsModsAndVersion(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"no mods", Options{Mode: "add-mods", Parent: "/srv/mc", Instance: "survival", MCVersion: "1.21.8"}},
		{"no version", Options{Mode: "add-mods", Parent: "/srv/mc", Instance: "survival", Mods: []string{"bluemap"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both are caught before anything reads the disk or systemd.
			if err := Configure(context.Background(), config.New(), tt.opts); err == nil {
				t.Error("expected an error naming the missing flag")
			}
		})
	}
}

func TestParseMods(t *testing.T) {
	opts, handled, err := Parse([]string{"--mode", "add-mods", "--mods", "bluemap,simple-voice-chat"}, io.Discard)
	if err != nil || handled {
		t.Fatalf("Parse: err=%v handled=%v", err, handled)
	}
	if len(opts.Mods) != 2 || opts.Mods[0] != "bluemap" || opts.Mods[1] != "simple-voice-chat" {
		t.Errorf("Mods = %v, want [bluemap simple-voice-chat]", opts.Mods)
	}
}
