package system

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// InstanceOwner resolves who an instance's files must belong to: the user its
// unit runs as, or, when the unit names none, whoever owns the directory today.
// Servers we didn't install often run as their own account, so ownership has to
// come from the server itself rather than from whoever invoked us.
func InstanceOwner(unit Unit, dir string) (string, error) {
	if unit.User != "" {
		return unit.User, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", dir, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("cannot read the owner of %s", dir)
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return "", fmt.Errorf("look up uid %d (owner of %s): %w", stat.Uid, dir, err)
	}
	return u.Username, nil
}

// ChownRecursive gives an entire tree to a user (and their primary group). The
// installer runs as root, so files it creates are root-owned; the server must run
// as the login user, so we hand the instance directory over before starting it.
func ChownRecursive(root, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("look up user %q: %w", username, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		return nil
	})
}
