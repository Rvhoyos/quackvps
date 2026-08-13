// Package restore puts a QuackedSMP world backup back into an existing instance.
// QuackedSMP writes <dir>/backups/world-<stamp>.zip (a plain zip whose root is
// world/); restoring one stops the server, moves the current world aside, unzips
// the chosen backup in its place, and starts back up, rolling the old world back
// if the server won't boot. Like update, it's scoped to one instance and never
// prompts: the wizard picks the backup, this package executes.
package restore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Run restores cfg.Backup into cfg.Dir (both already validated).
func Run(ctx context.Context, cfg *config.Config) error {
	unit := minecraft.UnitName(cfg.Instance)

	ui.Step("Stopping the server")
	if err := system.StopAndWait(ctx, unit, cfg.RunAsUser, cfg.Instance, system.DefaultStopWait); err != nil {
		return err
	}

	world := filepath.Join(cfg.Dir, "world")
	aside, err := moveAside(world)
	if err != nil {
		return err
	}
	if aside != "" {
		ui.Info("Current world moved to %s while the backup is restored.", filepath.Base(aside))
	}

	ui.Step("Restoring the backup")
	if err := extractZip(cfg.Backup, cfg.Dir); err != nil {
		return restoreAside(world, aside, cfg.Backup, err)
	}
	if err := system.ChownRecursive(cfg.Dir, cfg.RunAsUser); err != nil {
		return restoreAside(world, aside, cfg.Backup, err)
	}

	ui.Step("Starting the server")
	if err := system.StartAndVerify(ctx, unit); err != nil {
		return restoreAside(world, aside, cfg.Backup, err)
	}

	// Booted clean, so the pre-restore world is no longer needed as a safety net.
	if aside != "" {
		if err := os.RemoveAll(aside); err != nil {
			ui.Warn("could not remove the pre-restore world %s: %v", aside, err)
		}
	}
	ui.Step("Restore complete")
	ui.Success("Restored %s.", filepath.Base(cfg.Backup))
	return nil
}

// moveAside renames the instance's world/ out of the way so the backup can take
// its place while keeping the old one recoverable. It returns the new path, or ""
// when there was no world/ to move (an unusual but harmless case, we just unzip).
func moveAside(world string) (string, error) {
	if _, err := os.Stat(world); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("check world/: %w", err)
	}
	aside := world + ".pre-restore-" + time.Now().Format(stampLayout)
	if err := os.Rename(world, aside); err != nil {
		return "", fmt.Errorf("move current world aside: %w", err)
	}
	return aside, nil
}

// restoreAside undoes a failed restore: it removes the partly-written world/ and
// moves the pre-restore world back, so a broken restore leaves the server exactly
// as it was. The backup is kept and its path reported. The original cause is
// returned so the caller still fails.
func restoreAside(world, aside, backup string, cause error) error {
	ui.Error("Restore failed: %v", cause)
	_ = os.RemoveAll(world)
	if aside != "" {
		if err := os.Rename(aside, world); err != nil {
			ui.Error("could not move your original world back from %s: %v", aside, err)
			ui.Warn("Your original world is still at %s, move it back to %s by hand.", aside, world)
			return cause
		}
		ui.Info("Your original world has been put back; the server is unchanged.")
	}
	ui.Warn("The backup is untouched at %s if you want to try again.", backup)
	return cause
}
