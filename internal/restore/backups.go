package restore

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// stampLayout is the timestamp QuackedSMP (and our own update backup) puts in a
// backup filename: world-YYYYMMDD-HHMMSS.zip.
const stampLayout = "20060102-150405"

// Backup is one restorable world archive found in an instance's backups/ folder.
type Backup struct {
	Path  string    // absolute path to the .zip
	When  time.Time // taken from the filename stamp, or the file's mtime as a fallback
	Label string    // human-readable time for the picker, e.g. "2026-06-10 16:10:24"
}

// ListBackups returns the world backups under <dir>/backups, newest first. A
// missing backups/ folder is not an error, it just means there's nothing to
// restore, and the caller reports that.
func ListBackups(dir string) ([]Backup, error) {
	zips, err := filepath.Glob(filepath.Join(dir, "backups", "world-*.zip"))
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var backups []Backup
	for _, path := range zips {
		when := backupTime(path)
		backups = append(backups, Backup{
			Path:  path,
			When:  when,
			Label: when.Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].When.After(backups[j].When) })
	return backups, nil
}

// backupTime reads the timestamp from a world-<stamp>.zip name, falling back to
// the file's modification time when the name doesn't parse.
func backupTime(path string) time.Time {
	name := strings.TrimSuffix(filepath.Base(path), ".zip")
	stamp := strings.TrimPrefix(name, "world-")
	if t, err := time.ParseInLocation(stampLayout, stamp, time.Local); err == nil {
		return t
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// extractZip unpacks a backup zip into destDir. A QuackedSMP backup's root is
// world/ itself, so extracting into the instance dir recreates <destDir>/world/.
// zipRootDir reports the single top-level folder a backup holds, which names the
// world it restores: our own backups and QuackedSMP's are both a zip of one world
// folder, but that folder is called whatever level-name said when it was written.
// The caller moves that same folder aside, so the two always pair up.
func zipRootDir(src string) (string, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return "", fmt.Errorf("open backup %s: %w", src, err)
	}
	defer zr.Close()

	root := ""
	for _, f := range zr.File {
		top, _, nested := strings.Cut(filepath.ToSlash(f.Name), "/")
		if !nested || top == "" {
			return "", fmt.Errorf("backup %s has %q at its root; it should hold one world folder", filepath.Base(src), f.Name)
		}
		if root == "" {
			root = top
		} else if top != root {
			return "", fmt.Errorf("backup %s holds more than one folder (%s, %s); it should hold one world folder", filepath.Base(src), root, top)
		}
	}
	if root == "" {
		return "", fmt.Errorf("backup %s is empty", filepath.Base(src))
	}
	return root, nil
}

func extractZip(src, destDir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open backup %s: %w", src, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if err := extractOne(f, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, destDir string) error {
	dest := filepath.Join(destDir, f.Name)
	if f.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("read %s from backup: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
	if err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("extract %s: %w", f.Name, err)
	}
	// A swallowed close error can leave a truncated file written over the world,
	// so surface it rather than reporting a clean restore.
	if err := out.Close(); err != nil {
		return fmt.Errorf("extract %s: %w", f.Name, err)
	}
	return nil
}
