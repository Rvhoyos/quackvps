package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const unitDir = "/etc/systemd/system"

// WriteUnit writes a unit file into /etc/systemd/system. Call DaemonReload after
// to make systemd pick it up.
func WriteUnit(name, contents string) error {
	path := filepath.Join(unitDir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write unit %s: %w", name, err)
	}
	return nil
}

// UnitExists reports whether systemd knows a unit — used by the "is this folder
// already a server" test. It checks our unit dir first, then asks systemd so
// units defined elsewhere still count.
func UnitExists(name string) bool {
	if _, err := os.Stat(filepath.Join(unitDir, name)); err == nil {
		return true
	}
	return succeeds(context.Background(), "systemctl", "cat", name)
}

func DaemonReload(ctx context.Context) error { return Run(ctx, "systemctl", "daemon-reload") }

// EnableStart enables a unit to start on boot and starts it now.
func EnableStart(ctx context.Context, unit string) error {
	return Run(ctx, "systemctl", "enable", "--now", unit)
}

func Start(ctx context.Context, unit string) error { return Run(ctx, "systemctl", "start", unit) }
func Stop(ctx context.Context, unit string) error  { return Run(ctx, "systemctl", "stop", unit) }

// IsActive reports whether a unit is currently running.
func IsActive(ctx context.Context, unit string) bool {
	return succeeds(ctx, "systemctl", "is-active", "--quiet", unit)
}

// WaitInactive blocks until a unit is no longer active, or the timeout elapses.
// Updates rely on this: never touch a server's files until systemd confirms it
// has fully stopped and the world has saved.
func WaitInactive(ctx context.Context, unit string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !IsActive(ctx, unit) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not stop within %s", unit, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
