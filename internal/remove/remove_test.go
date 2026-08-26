package remove

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rvhoyos/quackvps/internal/system"
)

// writeInstance lays out the config files a removal reads its ports from.
func writeInstance(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, contents := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstancePorts(t *testing.T) {
	dir := t.TempDir()
	writeInstance(t, dir, map[string]string{
		"server.properties":                            "level-name=world\nserver-port=25566\n",
		"config/voicechat/voicechat-server.properties": "port=24455\nvoice_host=\n",
		// A web add-on's port must not show up: BlueMap is reached through Caddy and
		// was never opened in the firewall.
		"config/bluemap/webserver.conf": "port: 8101\n",
	})

	ports := instancePorts(dir)
	want := []firewallPort{
		{rule: system.Rule{Port: 25566, Proto: "tcp"}, label: "Minecraft"},
		{rule: system.Rule{Port: 24455, Proto: "udp"}, label: "Simple Voice Chat"},
	}
	if len(ports) != len(want) {
		t.Fatalf("instancePorts = %+v, want %+v", ports, want)
	}
	for i, w := range want {
		if ports[i].rule != w.rule || ports[i].label != w.label {
			t.Errorf("port %d = %+v, want %+v", i, ports[i], w)
		}
	}
}

func TestInstancePortsBareServer(t *testing.T) {
	// A server that has never generated server.properties names no port at all, so
	// there's nothing to close and nothing to guess.
	if ports := instancePorts(t.TempDir()); len(ports) != 0 {
		t.Errorf("instancePorts on an empty dir = %+v, want none", ports)
	}
}

func TestSplitPorts(t *testing.T) {
	game := firewallPort{rule: system.Rule{Port: 25565, Proto: "tcp"}, label: "Minecraft"}
	voice := firewallPort{rule: system.Rule{Port: 24454, Proto: "udp"}, label: "Simple Voice Chat"}
	votifier := firewallPort{rule: system.Rule{Port: 8192, Proto: "tcp"}, label: "Votifier"}

	tests := []struct {
		name        string
		ports       []firewallPort
		open        []system.Rule
		siblings    map[int]string
		wantClosing []int
		wantShared  []int
	}{
		{
			name:        "closes what it opened",
			ports:       []firewallPort{game, voice},
			open:        []system.Rule{game.rule, voice.rule},
			wantClosing: []int{25565, 24454},
		},
		{
			name:        "spares a port another server uses",
			ports:       []firewallPort{game, voice},
			open:        []system.Rule{game.rule, voice.rule},
			siblings:    map[int]string{24454: "creative"},
			wantClosing: []int{25565},
			wantShared:  []int{24454},
		},
		{
			name:        "skips a port ufw never had",
			ports:       []firewallPort{game, votifier},
			open:        []system.Rule{game.rule},
			wantClosing: []int{25565},
		},
		{
			name:  "same number, different protocol is a different rule",
			ports: []firewallPort{voice},
			open:  []system.Rule{{Port: 24454, Proto: "tcp"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closing, shared := splitPorts(tt.ports, tt.open, tt.siblings)
			if got := numbers(closing); !equal(got, tt.wantClosing) {
				t.Errorf("closing = %v, want %v", got, tt.wantClosing)
			}
			if got := numbers(shared); !equal(got, tt.wantShared) {
				t.Errorf("shared = %v, want %v", got, tt.wantShared)
			}
			for _, p := range shared {
				if p.owner == "" {
					t.Errorf("shared port %s should name the server holding it", p.rule)
				}
			}
		})
	}
}

func numbers(ports []firewallPort) []int {
	var out []int
	for _, p := range ports {
		out = append(out, p.rule.Port)
	}
	return out
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
