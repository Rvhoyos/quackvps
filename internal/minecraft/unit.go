package minecraft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/system"
)

// UnitName returns the systemd unit name for an instance.
func UnitName(instance string) string { return "mc-" + instance }

// UnitFile builds the systemd unit that runs an instance inside a screen session
// , the shape verified on the reference box. Type=forking because screen -dm
// backgrounds itself; the graceful ExecStop stuffs "stop" into the console so the
// world saves before exit; TimeoutStopSec gives that save time to finish before
// systemd would resort to SIGKILL.
func UnitFile(instance, user, dir string) string {
	return fmt.Sprintf(`[Unit]
Description=Minecraft server (%[1]s), %[4]s
After=network-online.target
Wants=network-online.target

[Service]
Type=forking
User=%[2]s
WorkingDirectory=%[3]s
Restart=always
RestartSec=5s
TimeoutStopSec=120
ExecStart=/usr/bin/screen -S %[1]s -dm bash -lc './run.sh'
ExecStop=/usr/bin/screen -S %[1]s -p 0 -X stuff "say Server stopping...\rstop\r"

[Install]
WantedBy=multi-user.target
`, instance, user, dir, system.UnitMarker)
}

// InstallUnit writes an instance's unit file and reloads systemd so it's known.
// Both a fresh install and adopting an existing server go through here, so the
// two always produce the same service.
func InstallUnit(ctx context.Context, instance, user, dir string) error {
	if err := system.WriteUnit(UnitName(instance)+".service", UnitFile(instance, user, dir)); err != nil {
		return err
	}
	return system.DaemonReload(ctx)
}

// Adopt puts an existing server under systemd so update and restore can manage
// it: it keeps running as whoever owns the directory today, and is enabled so it
// comes back after a reboot like any server we install. It refuses while the
// server is still running outside systemd, because managing it from two places
// would mean editing files under a live server.
func Adopt(ctx context.Context, instance, dir string) error {
	if pid, cmdline, running := system.RunningIn(dir); running {
		return fmt.Errorf("a server is already running in %s (pid %d: %s); stop it first, then re-run so we can manage it with systemd", dir, pid, cmdline)
	}
	owner, err := system.InstanceOwner(system.Unit{}, dir)
	if err != nil {
		return err
	}
	if err := InstallUnit(ctx, instance, owner, dir); err != nil {
		return err
	}
	return system.Enable(ctx, UnitName(instance)+".service")
}

// HasRunScript reports whether an instance has the launch script the unit calls.
// Update writes its own, restore never does, so restore checks before adopting.
func HasRunScript(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, runScript))
	return err == nil
}
