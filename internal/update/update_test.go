package update

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupWorldRenamedLevel(t *testing.T) {
	dir := t.TempDir()
	// server.properties points the level somewhere other than world/, which is all
	// a server needs to have no world/ folder at all.
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("level-name=myworld\nserver-port=25565\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "myworld", "region"), 0o755)
	os.WriteFile(filepath.Join(dir, "myworld", "level.dat"), []byte("data"), 0o644)

	path, err := BackupWorld(dir)
	if err != nil {
		t.Fatalf("renamed level should still back up: %v", err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "myworld/") {
			t.Errorf("archived %q, want everything under myworld/", f.Name)
		}
	}
}

func TestBackupWorld(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "world")
	os.MkdirAll(filepath.Join(world, "region"), 0o755)
	os.WriteFile(filepath.Join(world, "level.dat"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(world, "region", "r.0.0.mca"), []byte("chunk"), 0o644)

	path, err := BackupWorld(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(dir, "backups") {
		t.Errorf("backup not in backups/: %s", path)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Errorf("backup zip missing or empty: %v", err)
	}

	// No world/ → clear error.
	if _, err := BackupWorld(t.TempDir()); err == nil {
		t.Error("expected error when world/ is absent")
	}
}

func TestWipeMods(t *testing.T) {
	dir := t.TempDir()
	mods := filepath.Join(dir, "mods")
	os.MkdirAll(mods, 0o755)
	os.WriteFile(filepath.Join(mods, "a.jar"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(mods, "keep.txt"), []byte("x"), 0o644)

	if err := wipeMods(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mods, "a.jar")); !os.IsNotExist(err) {
		t.Error("jar should have been removed")
	}
	if _, err := os.Stat(filepath.Join(mods, "keep.txt")); err != nil {
		t.Error("non-jar should be left alone")
	}
	// Missing mods/ is not an error.
	if err := wipeMods(t.TempDir()); err != nil {
		t.Errorf("wipeMods on missing dir: %v", err)
	}
}
