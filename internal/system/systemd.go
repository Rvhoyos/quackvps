package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// UnitExists reports whether systemd knows a unit, used by the "is this folder
// already a server" test. It checks our unit dir first, then asks systemd so
// units defined elsewhere still count.
func UnitExists(name string) bool {
	if _, err := os.Stat(filepath.Join(unitDir, name)); err == nil {
		return true
	}
	return succeeds(context.Background(), "systemctl", "cat", name)
}

// InstanceExists reports whether <parent>/<name> already holds a Minecraft server:
// the directory is present AND either its systemd unit exists or it contains a
// launch jar / run.sh. It's the guard that keeps a fresh install from clobbering an
// existing, or half-built, server.
func InstanceExists(parent, name string) bool {
	dir := filepath.Join(parent, name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if UnitExists("mc-" + name + ".service") {
		return true
	}
	for _, marker := range []string{
		"run.sh",
		"server.jar",
		"fabric-server-launch.jar",
		"quilt-server-launch.jar",
		filepath.Join("libraries", "net", "neoforged", "neoforge"),
	} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func DaemonReload(ctx context.Context) error { return Run(ctx, "systemctl", "daemon-reload") }

// EnableStart enables a unit to start on boot and starts it now.
func EnableStart(ctx context.Context, unit string) error {
	return Run(ctx, "systemctl", "enable", "--now", unit)
}

// Enable makes a unit start on boot without starting it now, for a server that
// isn't ready to run yet.
func Enable(ctx context.Context, unit string) error {
	return Run(ctx, "systemctl", "enable", unit)
}

func Start(ctx context.Context, unit string) error { return Run(ctx, "systemctl", "start", unit) }
func Stop(ctx context.Context, unit string) error  { return Run(ctx, "systemctl", "stop", unit) }

// DefaultStopWait bounds a graceful shutdown. It's a little longer than the unit's
// TimeoutStopSec (120s) so systemd's own graceful stop, letting the world finish
// saving, has time to complete before we give up.
const DefaultStopWait = 130 * time.Second

// StopAndWait stops a unit and blocks until it's truly down, so callers never touch
// a running server's files. It also confirms the screen session is gone, and refuses
// to force a stuck server. Both update and restore rely on this. A unit that runs
// its server directly rather than through screen passes an empty session, which
// simply skips that second check.
func StopAndWait(ctx context.Context, unit, user, session string, timeout time.Duration) error {
	if err := Stop(ctx, unit); err != nil {
		return err
	}
	if err := WaitInactive(ctx, unit, timeout); err != nil {
		return fmt.Errorf("%w; not touching files, try again once it's stopped", err)
	}
	if session != "" && ScreenExists(ctx, user, session) {
		return fmt.Errorf("screen session %q still present after stop; aborting to protect the world", session)
	}
	return nil
}

// StartAndVerify starts a unit and confirms it's still running a moment later, so a
// server that dies immediately (bad jar, port clash) is reported as a failure rather
// than a success.
func StartAndVerify(ctx context.Context, unit string) error {
	if err := Start(ctx, unit); err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	if !IsActive(ctx, unit) {
		return fmt.Errorf("%s did not stay running after start", unit)
	}
	return nil
}

// UnitOOMKilled reports whether systemd recorded the unit's last run ending in an
// out-of-memory kill. An OOM-kill leaves no JVM crash dump, so this is how we tell a
// warm-up boot that died on memory apart from one that merely crashed.
func UnitOOMKilled(ctx context.Context, unit string) bool {
	out, err := Capture(ctx, "systemctl", "show", unit, "-p", "Result", "--value")
	return err == nil && strings.TrimSpace(out) == "oom-kill"
}

// RemoveUnit stops and disables a unit, deletes its file, and reloads systemd.
// It's best-effort, used to roll back a failed install, so each step's error is
// ignored; the goal is simply to leave nothing behind.
func RemoveUnit(ctx context.Context, unit string) {
	_ = Run(ctx, "systemctl", "disable", "--now", unit)
	_ = os.Remove(filepath.Join(unitDir, unit))
	_ = DaemonReload(ctx)
}

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
