package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AptInstall installs packages non-interactively. It runs `apt-get update` first
// so a fresh box with a stale index can still resolve the packages.
func AptInstall(ctx context.Context, pkgs ...string) error {
	if err := aptGet(ctx, "update"); err != nil {
		return err
	}
	return aptGet(ctx, append([]string{"install", "-y"}, pkgs...)...)
}

func aptGet(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "apt-get", args...)
	// DEBIAN_FRONTEND keeps apt from trying to open dialogs on a fresh VPS.
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
