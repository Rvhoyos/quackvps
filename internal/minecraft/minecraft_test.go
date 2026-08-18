package minecraft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPropsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")

	if err := SetProp(path, "server-port", "25566"); err != nil {
		t.Fatal(err)
	}
	if err := SetProp(path, "level-name", "world"); err != nil {
		t.Fatal(err)
	}
	// Overwriting an existing key must not duplicate it.
	if err := SetProp(path, "server-port", "25567"); err != nil {
		t.Fatal(err)
	}

	props, err := ReadProps(path)
	if err != nil {
		t.Fatal(err)
	}
	if props["server-port"] != "25567" {
		t.Errorf("server-port = %q, want 25567", props["server-port"])
	}
	if props["level-name"] != "world" {
		t.Errorf("level-name = %q, want world", props["level-name"])
	}
}

func TestReadPropsMissingFile(t *testing.T) {
	props, err := ReadProps(filepath.Join(t.TempDir(), "nope.properties"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected empty map, got %v", props)
	}
}

func TestUnitFile(t *testing.T) {
	unit := UnitFile("survival", "ubuntu", "/home/ubuntu/mcserver/survival")
	for _, want := range []string{
		"Type=forking",
		"User=ubuntu",
		"WorkingDirectory=/home/ubuntu/mcserver/survival",
		"TimeoutStopSec=120",
		"screen -S survival -dm bash -lc './run.sh'",
		`stuff "say Server stopping...\rstop\r"`,
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestLevelName(t *testing.T) {
	dir := t.TempDir()
	if got := LevelName(dir); got != "world" {
		t.Errorf("no server.properties: got %q, want world", got)
	}

	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("server-port=25565\nlevel-name=myworld\n"), 0o644)
	if got := LevelName(dir); got != "myworld" {
		t.Errorf("got %q, want myworld", got)
	}
}
