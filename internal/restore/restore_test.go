package restore

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rvhoyos/quackvps/internal/minecraft"
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
		"notes.txt", // ignored: not a zip
	} {
		os.WriteFile(filepath.Join(backups, name), []byte("x"), 0o644)
	}
	// A zip named nothing like QuackedSMP's format is still a backup; it just has
	// to be dated by its mtime instead of by its name.
	handmade := filepath.Join(backups, "before-the-dragon.zip")
	os.WriteFile(handmade, []byte("x"), 0o644)
	os.Chtimes(handmade, time.Time{}, time.Date(2026, 4, 1, 8, 30, 0, 0, time.Local))

	got, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"world-20260610-161024.zip",
		"before-the-dragon.zip",
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
	// The label has to name the file, since the name is all that tells two backups
	// taken the same second apart, and date it, since a name need not carry a date.
	if want := "world-20260610-161024.zip (2026-06-10 16:10:24)"; got[0].Label != want {
		t.Errorf("label = %q, want %q", got[0].Label, want)
	}
	if want := "before-the-dragon.zip (2026-04-01 08:30:00)"; got[1].Label != want {
		t.Errorf("label = %q, want %q", got[1].Label, want)
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

func TestReadArchive(t *testing.T) {
	dir := t.TempDir()

	renamed := filepath.Join(dir, "before-the-dragon.zip")
	writeWorldZip(t, renamed, map[string]string{
		"myworld/level.dat":        string(levelDat("1.21.9", 4554)),
		"myworld/region/r.0.0.mca": "chunk",
	})
	got, err := readArchive(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if got.level != "myworld" {
		t.Errorf("level = %q, want myworld", got.level)
	}
	if got.saved.Version != "1.21.9" || got.saved.DataVersion != 4554 {
		t.Errorf("saved = %+v, want {1.21.9 4554}", got.saved)
	}

	// No readable level.dat costs the version check, not the backup.
	nolevel := filepath.Join(dir, "nolevel.zip")
	writeWorldZip(t, nolevel, map[string]string{"world/region/r.0.0.mca": "chunk"})
	got, err = readArchive(nolevel)
	if err != nil {
		t.Fatal(err)
	}
	if got.level != "world" || got.saved.DataVersion != 0 {
		t.Errorf("readArchive(no level.dat) = %+v, want world with no version", got)
	}

	loose := filepath.Join(dir, "loose.zip")
	writeWorldZip(t, loose, map[string]string{"level.dat": "leveldata"})
	if _, err := readArchive(loose); err == nil {
		t.Error("a backup with files at its root should be rejected")
	}

	two := filepath.Join(dir, "two.zip")
	writeWorldZip(t, two, map[string]string{"world/level.dat": "a", "nether/level.dat": "b"})
	if _, err := readArchive(two); err == nil {
		t.Error("a backup with two folders should be rejected")
	}
}

func TestRefuseNewerWorld(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "world"), 0o755)
	os.WriteFile(filepath.Join(dir, "world", "level.dat"), levelDat("1.21.9", 4554), 0o644)

	tests := []struct {
		name    string
		saved   minecraft.Level
		refused bool
	}{
		{"newer than the server", minecraft.Level{Version: "26.2", DataVersion: 5120}, true},
		{"older than the server", minecraft.Level{Version: "1.21.4", DataVersion: 4189}, false},
		{"the version the server runs", minecraft.Level{Version: "1.21.9", DataVersion: 4554}, false},
		{"a version we could not read", minecraft.Level{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseNewerWorld(dir, tt.saved)
			if tt.refused && err == nil {
				t.Error("expected the restore to be refused")
			}
			if !tt.refused && err != nil {
				t.Errorf("expected the restore to go ahead, got %v", err)
			}
		})
	}
}

// A server that has never generated a world has nothing to compare against, so
// even a backup from a newer version goes in: there is no world to corrupt.
func TestRefuseNewerWorldWithoutAWorld(t *testing.T) {
	newer := minecraft.Level{Version: "26.2", DataVersion: 5120}
	if err := refuseNewerWorld(t.TempDir(), newer); err != nil {
		t.Errorf("expected the restore to go ahead, got %v", err)
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

// levelDat builds a gzipped level.dat naming a version: root > Data >
// {DataVersion, Version > Name}. The NBT reader's own edge cases belong to the
// minecraft package's tests, so this only has to be a file that parses.
func levelDat(version string, dataVersion int32) []byte {
	const (
		tagEnd      = 0
		tagInt      = 3
		tagString   = 8
		tagCompound = 10
	)
	var b bytes.Buffer
	b.WriteByte(tagCompound)
	writeName(&b, "") // the unnamed root every NBT file opens with
	b.WriteByte(tagCompound)
	writeName(&b, "Data")
	b.WriteByte(tagInt)
	writeName(&b, "DataVersion")
	binary.Write(&b, binary.BigEndian, dataVersion)
	b.WriteByte(tagCompound)
	writeName(&b, "Version")
	b.WriteByte(tagString)
	writeName(&b, "Name")
	writeName(&b, version)
	b.WriteByte(tagEnd) // Version
	b.WriteByte(tagEnd) // Data
	b.WriteByte(tagEnd) // root

	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(b.Bytes())
	w.Close()
	return gz.Bytes()
}

// writeName writes NBT's length-prefixed string, used for both names and values.
func writeName(b *bytes.Buffer, s string) {
	binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
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
