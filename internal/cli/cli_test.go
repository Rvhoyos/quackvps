package cli

import (
	"testing"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/restore"
)

func TestValidateMode(t *testing.T) {
	for _, m := range []string{"", "install", "update", "restore"} {
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
		Domain:             "example.com",
	}
	cfg := config.New()
	cfg.RunAsUser, cfg.RunAsHome = "ubuntu", "/home/ubuntu"
	if err := Configure(cfg, opts); err != nil {
		t.Fatalf("configure: %v", err)
	}

	if cfg.Mode != config.ModeInstall {
		t.Errorf("mode = %v, want install", cfg.Mode)
	}
	if !hasMod(cfg.Mods, "bluemap") || !hasMod(cfg.Mods, "quackedsmp") {
		t.Errorf("mods %v missing bluemap/quackedsmp", cfg.Mods)
	}
	if cfg.Ports[config.PortBlueMap] != 8100 || cfg.Ports[config.PortDashboard] != 8125 {
		t.Errorf("ports = %v", cfg.Ports)
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
	if err := Configure(config.New(), opts); err == nil {
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
