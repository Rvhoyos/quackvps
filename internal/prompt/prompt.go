// Package prompt is the interactive wizard. Its single job is to fill a
// config.Config from the user's answers — it performs no install work whatsoever.
// That strict split (wizard produces a Config; execution consumes it) is what
// keeps a future non-interactive flag mode a thin layer rather than a rewrite.
//
// Every question follows the teaching rule: the huh Title is the plain-language
// "what", Description is the "why", and the pre-filled value is the suggested
// default.
package prompt

import (
	"context"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/modrinth"
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

	if cfg.Mode == config.ModeRestore {
		// Restore: the only question is which world backup to put back.
		return cfg, askBackup(cfg)
	}

	if cfg.Mode == config.ModeUpdate {
		// Update: the loader is fixed (detected from disk, never re-chosen); only
		// the target Minecraft version is asked.
		detected, err := loader.Detect(cfg.Dir)
		if err != nil {
			return nil, err
		}
		cfg.Loader = detected
		return cfg, askMCVersion(ctx, cfg)
	}

	// Install: the full path.
	if err := askLoader(cfg); err != nil {
		return nil, err
	}
	if err := askMCVersion(ctx, cfg); err != nil {
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
