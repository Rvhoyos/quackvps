package restore

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errBoot = errors.New("simulated boot failure")

func TestListBackups(t *testing.T) {
	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	os.MkdirAll(backups, 0o755)
	// Deliberately out of order on disk; ListBackups must sort newest first.
	for _, name := range []string{
		"world-20260610-161024.zip",
		"world-20260101-090000.zip",
		"world-20260315-120000.zip",
		"notes.txt", // ignored: not a world-*.zip
	} {
		os.WriteFile(filepath.Join(backups, name), []byte("x"), 0o644)
	}

	got, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"world-20260610-161024.zip",
		"world-20260315-120000.zip",
		"world-20260101-090000.zip",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d backups, want %d", len(got), len(want))
	}
	for i, b := range got {
		if filepath.Base(b.Path) != want[i] {
			t.Errorf("backup[%d] = %s, want %s", i, filepath.Base(b.Path), want[i])
		}
	}
	if got[0].Label != "2026-06-10 16:10:24" {
		t.Errorf("label = %q, want 2026-06-10 16:10:24", got[0].Label)
	}
}

func TestListBackupsMissingDir(t *testing.T) {
	got, err := ListBackups(t.TempDir())
	if err != nil {
		t.Fatalf("missing backups/ should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d backups, want 0", len(got))
	}
}

func TestZipRootDir(t *testing.T) {
	dir := t.TempDir()

	renamed := filepath.Join(dir, "world-20260610-161024.zip")
	writeWorldZip(t, renamed, map[string]string{
		"myworld/level.dat":        "leveldata",
		"myworld/region/r.0.0.mca": "chunk",
	})
	root, err := zipRootDir(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if root != "myworld" {
		t.Errorf("root = %q, want myworld", root)
	}

	loose := filepath.Join(dir, "loose.zip")
	writeWorldZip(t, loose, map[string]string{"level.dat": "leveldata"})
	if _, err := zipRootDir(loose); err == nil {
		t.Error("a backup with files at its root should be rejected")
	}

	two := filepath.Join(dir, "two.zip")
	writeWorldZip(t, two, map[string]string{"world/level.dat": "a", "nether/level.dat": "b"})
	if _, err := zipRootDir(two); err == nil {
		t.Error("a backup with two folders should be rejected")
	}
}

func TestExtractZip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "world-20260610-161024.zip")
	writeWorldZip(t, src, map[string]string{
		"world/level.dat":         "leveldata",
		"world/region/r.0.0.mca":  "chunk",
		"world/DIM1/data/foo.dat": "dim",
	})

	dest := t.TempDir()
	if err := extractZip(src, dest); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"world/level.dat":        "leveldata",
		"world/region/r.0.0.mca": "chunk",
	} {
		got, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestMoveAside(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "world")
	os.MkdirAll(world, 0o755)
	os.WriteFile(filepath.Join(world, "level.dat"), []byte("x"), 0o644)

	aside, err := moveAside(world)
	if err != nil {
		t.Fatal(err)
	}
	if aside == "" {
		t.Fatal("expected a move-aside path")
	}
	if _, err := os.Stat(world); !os.IsNotExist(err) {
		t.Error("world/ should be gone after move")
	}
	if _, err := os.Stat(filepath.Join(aside, "level.dat")); err != nil {
		t.Errorf("moved-aside world missing its contents: %v", err)
	}

	// No world/ present → no-op, empty path, no error.
	got, err := moveAside(filepath.Join(t.TempDir(), "world"))
	if err != nil || got != "" {
		t.Errorf("moveAside(absent) = %q, %v; want \"\", nil", got, err)
	}
}

func TestRestoreAsidePutsWorldBack(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "world")
	// Simulate a failed restore: a half-written new world/ and the original aside.
	os.MkdirAll(world, 0o755)
	os.WriteFile(filepath.Join(world, "partial.dat"), []byte("bad"), 0o644)
	aside := world + ".pre-restore-20260610-161024"
	os.MkdirAll(aside, 0o755)
	os.WriteFile(filepath.Join(aside, "level.dat"), []byte("original"), 0o644)

	restoreAside(world, aside, "backup.zip", errBoot)

	if _, err := os.Stat(aside); !os.IsNotExist(err) {
		t.Error("aside should have been renamed back")
	}
	got, err := os.ReadFile(filepath.Join(world, "level.dat"))
	if err != nil || string(got) != "original" {
		t.Errorf("original world not restored: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(world, "partial.dat")); !os.IsNotExist(err) {
		t.Error("partial restored world should have been removed")
	}
}

// writeWorldZip creates a zip at path whose entries are the given path→content map.
func writeWorldZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
