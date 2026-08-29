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

	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// stampLayout is the timestamp QuackedSMP (and our own update backup) puts in a
// backup filename: world-YYYYMMDD-HHMMSS.zip. It is how a backup is dated and
// ordered, never what qualifies a file as one.
const stampLayout = "20060102-150405"

// Backup is one restorable world archive found in an instance's backups/ folder.
type Backup struct {
	Path  string    // absolute path to the .zip
	When  time.Time // taken from the filename stamp, or the file's mtime as a fallback
	Label string    // what the picker shows: "world-20260610-161024.zip (2026-06-10 16:10:24)"
}

// ListBackups returns every zip under <dir>/backups, newest first. Nothing but
// the folder makes a file a backup: a world zip renamed by hand, or copied in
// from another box, restores exactly like one QuackedSMP just wrote. A missing
// backups/ folder is not an error, it just means there is nothing to restore, and
// the caller reports that.
func ListBackups(dir string) ([]Backup, error) {
	zips, err := filepath.Glob(filepath.Join(dir, "backups", "*.zip"))
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var backups []Backup
	for _, path := range zips {
		when := backupTime(path)
		backups = append(backups, Backup{
			Path:  path,
			When:  when,
			Label: fmt.Sprintf("%s (%s)", filepath.Base(path), when.Format("2006-01-02 15:04:05")),
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

// archive is what one read of a backup zip tells us about the world inside it.
type archive struct {
	level string          // the single top-level folder, which names the world it holds
	saved minecraft.Level // what that folder's level.dat says, zero when it has none we can read
}

// readArchive reads a backup's shape and its world's version in one pass. The
// folder matters because our own backups and QuackedSMP's are both a zip of one
// world folder, but that folder is called whatever level-name said when it was
// written, so the caller learns from the backup itself which folder to move
// aside and the two always pair up. The version matters because a world only
// migrates forward.
//
// A zip whose level.dat is missing or unreadable is still a backup: what's lost
// is the version check, not the restore.
func readArchive(src string) (archive, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return archive{}, fmt.Errorf("open backup %s: %w", src, err)
	}
	defer zr.Close()

	var a archive
	var level *zip.File
	for _, f := range zr.File {
		top, rest, nested := strings.Cut(filepath.ToSlash(f.Name), "/")
		if !nested || top == "" {
			return archive{}, fmt.Errorf("backup %s has %q at its root; it should hold one world folder", filepath.Base(src), f.Name)
		}
		if a.level == "" {
			a.level = top
		} else if top != a.level {
			return archive{}, fmt.Errorf("backup %s holds more than one folder (%s, %s); it should hold one world folder", filepath.Base(src), a.level, top)
		}
		if rest == "level.dat" {
			level = f
		}
	}
	if a.level == "" {
		return archive{}, fmt.Errorf("backup %s is empty", filepath.Base(src))
	}
	if level == nil {
		ui.Warn("%s holds no level.dat, so its Minecraft version can't be checked against this server's world.", filepath.Base(src))
		return a, nil
	}
	saved, err := readZippedLevel(level)
	if err != nil {
		ui.Warn("Could not read a Minecraft version out of %s: %v", filepath.Base(src), err)
		return a, nil
	}
	a.saved = saved
	return a, nil
}

// readZippedLevel decodes a level.dat straight out of the archive, no unpacking.
func readZippedLevel(f *zip.File) (minecraft.Level, error) {
	rc, err := f.Open()
	if err != nil {
		return minecraft.Level{}, fmt.Errorf("read %s from backup: %w", f.Name, err)
	}
	defer rc.Close()
	return minecraft.ReadLevel(rc)
}

// extractZip unpacks a backup zip into destDir. A backup's root is the world
// folder itself, so extracting into the instance dir recreates <destDir>/world/.
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
