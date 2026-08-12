// Package update upgrades an existing server in place: stop it, back up the
// world, upgrade the loader and mods, then start it again — scoped entirely to
// one instance so other servers on the box are untouched. Like install, it never
// prompts; the one unavoidable mid-run decision (wiping mods/) is delegated to a
// ConfirmFunc the caller supplies, so execution stays independent of the prompt
// library and a future flag mode can pass an auto-confirm.
package update

import (
	"context"
	"fmt"
	"time"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/java"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/web"
)

// ConfirmFunc asks the user a yes/no question during execution. The caller wires
// it to a real prompt; automation can pass one that always returns true.
type ConfirmFunc func(question string) (bool, error)

// stopTimeout bounds the graceful shutdown wait. It's deliberately a little
// longer than the unit's TimeoutStopSec (120s) so systemd's own graceful stop —
// letting the world finish saving — has time to complete before we give up.
const stopTimeout = 130 * time.Second

// Run performs the in-place update described by cfg (already validated). client
// resolves the new mod builds; confirm gates the mods/ wipe.
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client, confirm ConfirmFunc) error {
	unit := minecraft.UnitName(cfg.Instance)

	ui.Step("Stopping the server")
	if err := stopAndWait(ctx, cfg, unit); err != nil {
		return err
	}

	ui.Step("Backing up the world")
	backup, err := BackupWorld(cfg.Dir)
	if err != nil {
		return err
	}
	ui.Success("World backed up to %s", backup)

	ui.Step("Identifying current mods")
	resolved, unknown, err := identifyMods(ctx, cfg, client)
	if err != nil {
		return err
	}
	reportModPlan(resolved, unknown)
	if web.BlueMapPresent(cfg.Dir) {
		ui.Warn("BlueMap will be upgraded — its map display settings reset to fresh defaults so the new version loads cleanly. Manual BlueMap map tweaks are lost; your world and rendered map are untouched.")
	}

	keepMods, err := confirm(fmt.Sprintf("Upgrade the %d mod(s) in mods/ to their %s builds? Choose No to update the server to %s with an empty mods/ folder (a fresh start, no mods carried over). Your world is backed up either way.", len(resolved)+len(unknown), cfg.MCVersion, cfg.MCVersion))
	if err != nil {
		return err
	}
	if !keepMods {
		// Declining doesn't abort — the server is already stopped, so we finish the
		// update and bring it back up, just with an empty mods/ folder.
		ui.Info("Updating with an empty mods/ folder — no mods carried over.")
		resolved = map[string]modrinth.Version{}
		unknown = nil
	}

	ui.Step("Upgrading the server")
	if err := upgrade(ctx, cfg, resolved); err != nil {
		return keepBackup(backup, err)
	}

	ui.Step("Starting the server")
	if err := startAndVerify(ctx, unit); err != nil {
		return keepBackup(backup, err)
	}

	// Success: the world migrated cleanly, so the safety-net backup is no longer
	// needed.
	if err := removeBackup(backup); err != nil {
		ui.Warn("could not remove backup %s: %v", backup, err)
	}
	reportDone(resolved, unknown, keepMods)
	return nil
}

// stopAndWait stops the unit and blocks until it's truly down — never edit a
// running server's files. It refuses to force a stuck server.
func stopAndWait(ctx context.Context, cfg *config.Config, unit string) error {
	if err := system.Stop(ctx, unit); err != nil {
		return err
	}
	if err := system.WaitInactive(ctx, unit, stopTimeout); err != nil {
		return fmt.Errorf("%w; not touching files — try again once it's stopped", err)
	}
	if system.ScreenExists(ctx, cfg.RunAsUser, cfg.Instance) {
		return fmt.Errorf("screen session %q still present after stop; aborting to protect the world", cfg.Instance)
	}
	return nil
}

// upgrade wipes mods/, reinstalls the loader for the new version, regenerates
// run.sh (preserving the existing RAM), and re-downloads the resolved mods.
func upgrade(ctx context.Context, cfg *config.Config, resolved map[string]modrinth.Version) error {
	minGB, maxGB := readHeap(cfg.Dir)

	javaPath, err := ensureJava(ctx, cfg)
	if err != nil {
		return err
	}
	l, err := loader.For(cfg.Loader, javaPath)
	if err != nil {
		return err
	}

	if err := wipeMods(cfg.Dir); err != nil {
		return err
	}
	if err := ui.Spinner("Reinstalling "+cfg.Loader+" for "+cfg.MCVersion, func() error {
		return l.InstallServer(ctx, cfg.Dir, cfg.MCVersion)
	}); err != nil {
		return err
	}

	body, err := l.RunScript(cfg.Dir, minGB, maxGB)
	if err != nil {
		return err
	}
	if err := minecraft.WriteRunScript(cfg.Dir, body); err != nil {
		return err
	}

	if err := redownloadMods(ctx, cfg.Dir, resolved); err != nil {
		return err
	}
	if web.BlueMapPresent(cfg.Dir) {
		// The upgraded BlueMap regenerates fresh map configs on the next boot; the
		// old ones may be a schema it now rejects. See web.ResetBlueMapMaps.
		if err := web.ResetBlueMapMaps(cfg.Dir); err != nil {
			return err
		}
	}
	return system.ChownRecursive(cfg.Dir, cfg.RunAsUser)
}

func startAndVerify(ctx context.Context, unit string) error {
	if err := system.Start(ctx, unit); err != nil {
		return err
	}
	// Give it a moment to fail fast (bad jar, port clash) before we call it up.
	time.Sleep(5 * time.Second)
	if !system.IsActive(ctx, unit) {
		return fmt.Errorf("%s did not stay running after start", unit)
	}
	return nil
}

func ensureJava(ctx context.Context, cfg *config.Config) (string, error) {
	major, err := java.Required(ctx, cfg.MCVersion)
	if err != nil {
		return "", err
	}
	return java.Ensure(ctx, major)
}
