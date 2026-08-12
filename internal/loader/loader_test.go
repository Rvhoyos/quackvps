package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvhoyos/quackvps/internal/config"
)

func TestNeoforgePrefix(t *testing.T) {
	tests := []struct {
		mc, want string
		wantErr  bool
	}{
		{"1.21.8", "21.8.", false},
		{"1.21.11", "21.11.", false},
		{"1.21", "21.0.", false},
		{"26.1.2", "26.1.", false},
		{"26.1", "26.1.", false},
		{"1.21-rc1", "", true},
	}
	for _, tt := range tests {
		got, err := neoforgePrefix(tt.mc)
		if tt.wantErr {
			if err == nil {
				t.Errorf("neoforgePrefix(%q) = %q, want error", tt.mc, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("neoforgePrefix(%q) = %q,%v want %q", tt.mc, got, err, tt.want)
		}
	}
}

func TestInlineRunScript(t *testing.T) {
	got := inlineRunScript("/usr/lib/jvm/temurin-21/bin/java", "server.jar", 2, 6)
	if !strings.Contains(got, "-Xms2G -Xmx6G") {
		t.Errorf("missing heap flags: %q", got)
	}
	if !strings.Contains(got, "-jar server.jar nogui") {
		t.Errorf("missing launch: %q", got)
	}
	if !strings.HasPrefix(got, "#!/usr/bin/env bash") {
		t.Errorf("missing shebang: %q", got)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name   string
		create string // relative path to touch
		want   string
	}{
		{"vanilla", "server.jar", config.LoaderVanilla},
		{"fabric", "fabric-server-launch.jar", config.LoaderFabric},
		{"quilt", "quilt-server-launch.jar", config.LoaderQuilt},
		{"neoforge", "libraries/net/neoforged/neoforge/21.8.54/x", config.LoaderNeoForge},
		{"forge", "libraries/net/minecraftforge/forge/1.20.1-47.4.10/x", config.LoaderForge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.create)
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte("x"), 0o644)
			got, err := Detect(dir)
			if err != nil || got != tt.want {
				t.Errorf("Detect = %q,%v want %q", got, err, tt.want)
			}
		})
	}

	if _, err := Detect(t.TempDir()); err == nil {
		t.Error("Detect on empty dir should error")
	}
}

func TestForUnknown(t *testing.T) {
	if _, err := For("paper", "/x/java"); err == nil {
		t.Error("For(paper) should error")
	}
}

func TestNeoforgeBuildToMC(t *testing.T) {
	tests := []struct{ build, want string }{
		{"21.8.54", "1.21.8"},
		{"21.11.45", "1.21.11"},
		{"20.4.237", "1.20.4"},
		{"21.0.167", "1.21"}, // trailing .0 dropped — MC is "1.21", not "1.21.0"
		{"26.1.2.30-beta", "26.1.2"},
		{"26.2.0", "26.2"}, // calendar trailing .0 dropped too
		{"garbage", ""},
	}
	for _, tt := range tests {
		if got := neoforgeBuildToMC(tt.build); got != tt.want {
			t.Errorf("neoforgeBuildToMC(%q) = %q, want %q", tt.build, got, tt.want)
		}
	}
}

func TestFilterRangeSplit(t *testing.T) {
	all := []string{"1.19.2", "1.20.1", "1.20.4", "1.21", "1.21.1", "1.21.8", "26.2"}

	forge := filterRange(config.LoaderForge, all)
	for _, v := range forge {
		if v == "1.21" || v == "1.21.1" || v == "1.21.8" || v == "26.2" || v == "1.19.2" {
			t.Errorf("forge range should exclude %s, got %v", v, forge)
		}
	}
	if len(forge) != 2 { // 1.20.1, 1.20.4
		t.Errorf("forge = %v, want just the 1.20.x releases", forge)
	}

	neo := filterRange(config.LoaderNeoForge, all)
	for _, v := range neo {
		if v == "1.20.1" || v == "1.20.4" || v == "1.19.2" {
			t.Errorf("neoforge range should exclude %s, got %v", v, neo)
		}
	}
	if neo[0] != "26.2" { // newest first
		t.Errorf("neoforge newest = %s, want 26.2", neo[0])
	}
}

func TestStripPromoSuffix(t *testing.T) {
	if got := stripPromoSuffix("1.20.1-recommended"); got != "1.20.1" {
		t.Errorf("recommended = %q", got)
	}
	if got := stripPromoSuffix("1.21.1-latest"); got != "1.21.1" {
		t.Errorf("latest = %q", got)
	}
	if got := stripPromoSuffix("1.20.1"); got != "" {
		t.Errorf("no suffix should be empty, got %q", got)
	}
}
