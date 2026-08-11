package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rvhoyos/quackvps/internal/ui"
)

// BackupWorld zips the instance's world/ into backups/world-<timestamp>.zip. The
// filename matches QuackedSMP's own backup format, so a kept backup can also be
// targeted by the (future) restore feature. Returns the backup path.
func BackupWorld(dir string) (string, error) {
	world := filepath.Join(dir, "world")
	if _, err := os.Stat(world); err != nil {
		return "", fmt.Errorf("no world/ to back up in %s: %w", dir, err)
	}

	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backups dir: %w", err)
	}
	stamp := time.Now().Format("20060102-150405")
	dest := filepath.Join(backupDir, "world-"+stamp+".zip")

	if err := zipTree(world, "world", dest); err != nil {
		return "", err
	}
	return dest, nil
}

// zipTree writes the file tree rooted at src into a zip at dest, storing paths
// under prefix (so the archive root is "world/...").
func zipTree(src, prefix, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // zip is a flat list of files; dirs are implied by paths
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(filepath.Join(prefix, rel)))
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

// removeBackup deletes a backup after a successful update.
func removeBackup(path string) error { return os.Remove(path) }

// keepBackup wraps a failure with the manual rollback instructions, since v1 does
// not auto-restore — the kept world zip is the way back.
func keepBackup(path string, cause error) error {
	ui.Error("Update failed: %v", cause)
	ui.Warn("Your world backup is kept at: %s", path)
	ui.Bullet(
		"To roll back: stop the server, remove the current world/, then unzip the backup in place:",
		fmt.Sprintf("unzip %q -d <instance-dir>", path),
	)
	return cause
}
