package system

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

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
