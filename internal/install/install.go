// Package install executes a fresh server install from a validated Config. It is
// pure execution — it never prompts; the wizard already gathered every answer.
// The step order matters and mirrors the spec: secure first, get the server
// running, then wire the web layer and firewall, then start and report.
package install

import (
	"context"
	"fmt"
	"os"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/java"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Run performs the install described by cfg, which must already have passed
// config.Validate.
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	// Pre-flight: confirm every mod and the modpack actually have a build for this
	// loader + version BEFORE doing anything irreversible (hardening, JDK install,
	// running the loader installer). The wizard already gates these, but the
	// flag-driven path (v3) doesn't, so this is where the guarantee lives.
	ui.Step("Checking mod availability")
	if err := preflight(ctx, cfg, client); err != nil {
		return err
	}

	if cfg.HardenSSH && cfg.SSHVerified {
		ui.Step("Hardening SSH")
		if err := hardenSSH(ctx, cfg); err != nil {
			return err
		}
	}

	ui.Step("Installing Java")
	javaPath, err := ensureJava(ctx, cfg)
	if err != nil {
		return err
	}

	ui.Step("Installing the server")
	l, err := loader.For(cfg.Loader, javaPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", cfg.Dir, err)
	}
	if err := ui.Spinner("Running the "+cfg.Loader+" installer", func() error {
		return l.InstallServer(ctx, cfg.Dir, cfg.MCVersion)
	}); err != nil {
		return err
	}

	ui.Step("Installing mods")
	if err := installContent(ctx, cfg, client); err != nil {
		return err
	}

	ui.Step("Configuring the server")
	if err := configureServer(ctx, cfg, l); err != nil {
		return err
	}

	ui.Step("Creating the service")
	if err := installService(ctx, cfg); err != nil {
		return err
	}
	// Hand the tree to the login user before the service (running as that user)
	// boots it.
	if err := system.ChownRecursive(cfg.Dir, cfg.RunAsUser); err != nil {
		return err
	}

	// Mods only write their config files on a real boot, so warm the server up
	// once to generate them, then edit them. (Skipped when there's nothing to
	// edit.) writeWebConfigs is edit-only — it never fabricates a config.
	if err := warmUpBoot(ctx, cfg); err != nil {
		return err
	}
	if err := writeWebConfigs(cfg); err != nil {
		return err
	}
	if err := system.ChownRecursive(cfg.Dir, cfg.RunAsUser); err != nil {
		return err
	}

	if cfg.AnyWeb() && cfg.Domain != "" {
		ui.Step("Setting up the reverse proxy")
		if err := configureCaddy(ctx, cfg); err != nil {
			return err
		}
	}

	ui.Step("Configuring the firewall")
	if err := configureFirewall(ctx, cfg); err != nil {
		return err
	}

	ui.Step("Starting the server")
	if err := system.EnableStart(ctx, minecraft.UnitName(cfg.Instance)); err != nil {
		return err
	}

	printSummary(ctx, cfg)
	return nil
}

func ensureJava(ctx context.Context, cfg *config.Config) (string, error) {
	major, err := java.Required(ctx, cfg.MCVersion)
	if err != nil {
		return "", err
	}
	var path string
	err = ui.Spinner(fmt.Sprintf("Ensuring Java %d", major), func() error {
		var e error
		path, e = java.Ensure(ctx, major)
		return e
	})
	return path, err
}
