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
	"os"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
)

// Run drives the wizard, filling cfg. cfg arrives with the fields main already
// knows (RunAsUser/RunAsHome). The caller validates the result before executing.
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client) (*config.Config, error) {
	if err := askHardenSSH(ctx, cfg); err != nil {
		return nil, err
	}
	if err := askServer(ctx, cfg); err != nil {
		return nil, err
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

// isExistingInstance implements the decided test for "this folder already holds a
// server": the directory exists AND either its systemd unit is present or it
// contains a launch jar / run.sh. Only then does the server step offer update.
func isExistingInstance(parent, name string) bool {
	dir := filepath.Join(parent, name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if system.UnitExists("mc-" + name + ".service") {
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
