package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRAM(t *testing.T) {
	t.Run("from user_jvm_args.txt", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "user_jvm_args.txt"), []byte("-Xms6G\n-Xmx6G\n"), 0o644)
		if got := readRAM(dir); got != 6 {
			t.Errorf("readRAM = %d, want 6", got)
		}
	})

	t.Run("from run.sh", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "run.sh"), []byte("exec java -Xms8G -Xmx8G -jar server.jar nogui\n"), 0o644)
		if got := readRAM(dir); got != 8 {
			t.Errorf("readRAM = %d, want 8", got)
		}
	})

	t.Run("default when absent", func(t *testing.T) {
		if got := readRAM(t.TempDir()); got != defaultRAMGB {
			t.Errorf("readRAM = %d, want %d", got, defaultRAMGB)
		}
	})
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
