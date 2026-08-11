// Package caddy manages the reverse proxy that fronts the web components. It
// follows a managed-file approach: each instance's site blocks live in their own
// file under quackvps.d/, and the only edit to the user's main Caddyfile is
// ensuring a single import line. That way we never touch a user's hand-written
// blocks, and removing an instance is just deleting its one file.
package caddy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/system"
)

// Paths are package variables so tests can redirect them; in production they are
// the standard Caddy locations.
var (
	MainCaddyfile = "/etc/caddy/Caddyfile"
	InstanceDir   = "/etc/caddy/quackvps.d"
)

const importLine = "import quackvps.d/*.caddy"

// Detect reports whether Caddy is installed and its version string.
func Detect(ctx context.Context) (bool, string) {
	if !system.HasCommand("caddy") {
		return false, ""
	}
	out, err := version(ctx)
	if err != nil {
		return true, ""
	}
	return true, strings.TrimSpace(out)
}

// EnsureInstalled installs Caddy from its official apt repository if it isn't
// already present.
func EnsureInstalled(ctx context.Context) error {
	if installed, _ := Detect(ctx); installed {
		return nil
	}
	return installFromApt(ctx)
}

// WriteInstanceFile writes an instance's site blocks to its own file, overwriting
// wholesale so each run's config fully replaces the last.
func WriteInstanceFile(instance string, blocks []string) error {
	if err := os.MkdirAll(InstanceDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", InstanceDir, err)
	}
	path := filepath.Join(InstanceDir, instance+".caddy")
	contents := strings.Join(blocks, "\n") + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// RemoveInstanceFile deletes an instance's site file — the whole teardown for its
// Caddy config.
func RemoveInstanceFile(instance string) error {
	path := filepath.Join(InstanceDir, instance+".caddy")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// Validate checks the whole Caddy config parses before we reload.
func Validate(ctx context.Context) error {
	return validate(ctx, MainCaddyfile)
}

// Reload validates then reloads Caddy, so a bad config never reaches a running
// proxy. The caller is responsible for restoring the prior instance file on a
// validation failure (see ReloadInstance).
func Reload(ctx context.Context) error {
	if err := Validate(ctx); err != nil {
		return fmt.Errorf("caddy config invalid: %w", err)
	}
	return reload(ctx)
}

// ReloadInstance validates and reloads after writing an instance file; if
// validation fails it rolls that file back to prior (its contents before this
// run, or "" to delete it) so one bad block can't wedge the whole proxy.
func ReloadInstance(ctx context.Context, instance, prior string) error {
	if err := Reload(ctx); err != nil {
		if rbErr := rollbackInstance(instance, prior); rbErr != nil {
			return fmt.Errorf("%w; additionally rollback failed: %v", err, rbErr)
		}
		return err
	}
	return nil
}

func rollbackInstance(instance, prior string) error {
	if prior == "" {
		return RemoveInstanceFile(instance)
	}
	return WriteInstanceFile(instance, []string{strings.TrimRight(prior, "\n")})
}
