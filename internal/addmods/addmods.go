// Package addmods installs extra mods into a server that already exists. Update
// only upgrades the jars it finds on disk and restore only swaps the world, so
// this is the flow for putting a new mod on a running server: stop it, download
// the chosen mods and their dependencies into mods/, and start it again. Like
// the other execution packages it never prompts; the wizard or the flag layer
// picks the mods, this installs them.
package addmods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/mods"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Run installs cfg.Mods into cfg.Dir (already validated).
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	if err := checkVersion(cfg); err != nil {
		return err
	}

	// A server set up by hand starts some other way, so it may have no run.sh for
	// the new service to call. An existing one is left alone: it carries whatever
	// flags its owner chose.
	if cfg.AdoptUnit && !minecraft.HasRunScript(cfg.Dir) {
		ui.Step("Writing a launch script")
		if err := minecraft.WriteLaunchScript(ctx, cfg); err != nil {
			return err
		}
	}

	var unit system.Unit
	var owner string
	var err error
	if cfg.Unit == "" {
		// No service to stop and start: the jars go in and the server is left as it
		// was found, which is what the user asked for by skipping it.
		ui.Warn("No service manages %s, so the mods go in but stopping and starting the server stays yours to do.", cfg.Dir)
		owner, err = system.InstanceOwner(unit, cfg.Dir)
	} else {
		unit, owner, err = minecraft.TakeOffline(ctx, cfg)
	}
	if err != nil {
		return err
	}

	ui.Step("Installing mods")
	result, err := mods.Install(ctx, client, cfg.Dir, cfg.Loader, cfg.MCVersion, cfg.Mods)
	if err != nil {
		return undo(ctx, cfg.Dir, unit.Name, result.Added, err)
	}
	if err := system.ChownRecursive(filepath.Join(cfg.Dir, "mods"), owner); err != nil {
		return undo(ctx, cfg.Dir, unit.Name, result.Added, err)
	}

	if unit.Name != "" {
		ui.Step("Starting the server")
		if err := system.StartAndVerify(ctx, unit.Name); err != nil {
			return undo(ctx, cfg.Dir, unit.Name, result.Added, err)
		}
	}
	report(result, unit.Name != "")
	return nil
}

// checkVersion refuses to install mods built for a version this server isn't on,
// which is the one way this flow can hand a working server a set of jars it can't
// load. The world data is the server's own record of the version it last ran, so
// a mismatch means either the wrong version was asked for, or the server has been
// changed and not started since.
func checkVersion(cfg *config.Config) error {
	running, err := minecraft.WorldVersion(cfg.Dir)
	if err != nil {
		return nil // no world yet: nothing to check against, take the version as given
	}
	if running != cfg.MCVersion {
		return fmt.Errorf("this server last ran Minecraft %s, but the mods asked for are %s builds, which it can't load: start it once on %s if you've just changed its version, then add mods",
			running, cfg.MCVersion, cfg.MCVersion)
	}
	return nil
}

// undo removes the jars this run put in mods/ and starts the server back up, so
// a mod that won't download or won't load leaves the server as it was found. The
// original cause is returned, so the run still fails. An empty unit means nothing
// was stopped, so there is nothing to start either.
func undo(ctx context.Context, dir, unit string, added []string, cause error) error {
	ui.Error("Adding mods failed: %v", cause)
	// Read the log before the restart overwrites it: "didn't stay running" on its
	// own leaves the user with nothing to act on, while the server's own words name
	// the mod that wouldn't load.
	for _, reason := range bootFailureReasons(dir) {
		ui.Warn("%s", reason)
	}
	if len(added) > 0 {
		ui.Info("Removing the %d jar(s) this run added.", len(added))
		for _, name := range added {
			if err := os.Remove(filepath.Join(dir, "mods", name)); err != nil && !os.IsNotExist(err) {
				ui.Warn("could not remove %s: %v", name, err)
			}
		}
	}
	if unit == "" {
		return cause
	}
	if err := system.StartAndVerify(ctx, unit); err != nil {
		ui.Warn("the server did not start again either: %v", err)
	} else {
		ui.Info("The server is back up with the mods it had before.")
	}
	return cause
}

// bootFailureReasons pulls the mod-loading errors out of the server's log, the
// same scan the modpack boot-test uses to explain a red run.
func bootFailureReasons(dir string) []string {
	log, err := os.ReadFile(filepath.Join(dir, "logs", "latest.log"))
	if err != nil {
		return nil
	}
	return minecraft.FailureReasons(string(log))
}

func report(result mods.Result, started bool) {
	ui.Step("Mods added")
	if len(result.Skipped) > 0 {
		ui.Info("Already installed, left as they are: %s", strings.Join(result.Skipped, ", "))
	}
	if len(result.Added) == 0 {
		if started {
			ui.Success("Nothing new to install; the server is back up unchanged.")
		} else {
			ui.Success("Nothing new to install; the server is unchanged.")
		}
		return
	}
	ui.Success("%d jar(s) added, dependencies included:", len(result.Added))
	ui.Bullet(result.Added...)
	if !started {
		ui.Info("Start the server yourself to load them.")
	}
}
