// Package prompt is the interactive wizard. Its single job is to fill a
// config.Config from the user's answers; it performs no install work whatsoever.
// That strict split (wizard produces a Config; execution consumes it) is what
// keeps a future non-interactive flag mode a thin layer rather than a rewrite.
//
// Every question follows the teaching rule: the huh Title is the plain-language
// "what", Description is the "why", and the pre-filled value is the suggested
// default.
package prompt

import (
	"context"
	"fmt"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Run drives the wizard, filling cfg. cfg arrives with the fields main already
// knows (RunAsUser/RunAsHome). pickerStart is the folder the directory picker
// opens at. The caller validates the result before executing.
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client, pickerStart string) (*config.Config, error) {
	if err := askHardenSSH(ctx, cfg); err != nil {
		return nil, err
	}
	if err := askServer(ctx, cfg, pickerStart); err != nil {
		return nil, err
	}

	switch cfg.Mode {
	case config.ModeRestore:
		// Restore: the only question is which world backup to put back.
		return cfg, askBackup(cfg)

	case config.ModeUpdate:
		// Update: the loader is fixed (detected from disk, never re-chosen); only
		// the target Minecraft version is asked.
		if err := detectLoader(cfg); err != nil {
			return nil, err
		}
		return cfg, askMCVersion(ctx, cfg, targetVersion)

	case config.ModeAddMods:
		// Adding mods changes neither the loader nor the version: both describe the
		// server as it already is, and the mods have to match them.
		if err := detectLoader(cfg); err != nil {
			return nil, err
		}
		if cfg.Loader == config.LoaderVanilla {
			return nil, fmt.Errorf("%s runs vanilla Minecraft, which has no mod loader, so a mod put in mods/ would never load", cfg.Dir)
		}
		if err := resolveRunningVersion(ctx, cfg); err != nil {
			return nil, err
		}
		return cfg, askMods(ctx, cfg, client)
	}

	// Install: the full path.
	if err := askLoader(cfg); err != nil {
		return nil, err
	}
	if err := askMCVersion(ctx, cfg, targetVersion); err != nil {
		return nil, err
	}
	if err := askModpackAndFeatures(ctx, cfg, client); err != nil {
		return nil, err
	}
	if err := askRAM(cfg); err != nil {
		return nil, err
	}
	if err := askDomainAndPorts(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveRunningVersion settles which Minecraft version the mods must match.
// The server's own world data names it, so there's nothing to ask: mods that
// don't match the running version don't load, and choosing is not a thing the
// user should be able to get wrong. A server that has never generated a world
// has nothing to read, so that one gets the question.
func resolveRunningVersion(ctx context.Context, cfg *config.Config) error {
	version, err := minecraft.WorldVersion(cfg.Dir)
	if err != nil {
		ui.Info("Couldn't read this server's version from its world data (%v).", err)
		return askMCVersion(ctx, cfg, runningVersion)
	}
	cfg.MCVersion = version
	ui.Info("This server is on Minecraft %s, so we'll install the %s builds of the mods you pick.", version, version)
	return nil
}

// detectLoader reads the loader from the instance on disk, for the modes that
// work on a server that already exists. It's never re-chosen: mods are
// loader-specific, so changing it would invalidate everything installed.
func detectLoader(cfg *config.Config) error {
	detected, err := loader.Detect(cfg.Dir)
	if err != nil {
		return err
	}
	cfg.Loader = detected
	return nil
}
