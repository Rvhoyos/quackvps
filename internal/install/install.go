// Package install executes a fresh server install from a validated Config. It is
// pure execution, it never prompts; the wizard already gathered every answer.
// The step order matters and mirrors the spec: secure first, get the server
// running, then wire the web layer and firewall, then start and report.
package install

import (
	"context"
	"errors"
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

// ErrHandled marks a failure whose full explanation has already been printed (for
// example a modpack that wouldn't boot), so main exits without printing it again.
var ErrHandled = errors.New("install failed")

// Run performs the install described by cfg, which must already have passed
// config.Validate.
func Run(ctx context.Context, cfg *config.Config, client modrinth.Client) (err error) {
	// Pre-flight: confirm every mod and the modpack actually have a build for this
	// loader + version BEFORE doing anything irreversible (hardening, JDK install,
	// running the loader installer). The wizard already gates these, but the
	// flag-driven path (v3) doesn't, so this is where the guarantee lives.
	ui.Step("Checking mod availability")
	if err := preflight(ctx, cfg, client); err != nil {
		return err
	}

	// Resolve the public IP once here; DNS checks, the voice_host config, and the
	// summary all read cfg.PublicIP rather than each hitting the network again.
	cfg.PublicIP = system.PublicIP(ctx)

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

	// From here on we write to disk. If any later step fails, roll the whole install
	// back so a broken half-install never lingers, but only remove what this run
	// created; a directory the user already had is left untouched.
	_, statErr := os.Stat(cfg.Dir)
	createdDir := os.IsNotExist(statErr)
	wroteUnit := false
	defer func() {
		if err != nil {
			rollback(cfg, createdDir, wroteUnit)
		}
	}()

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
	// The unit is enabled and started later, after ownership is fixed.
	if err := minecraft.InstallUnit(ctx, cfg.Instance, cfg.RunAsUser, cfg.Dir); err != nil {
		return err
	}
	wroteUnit = true
	// Hand the tree to the login user before the service (running as that user)
	// boots it.
	if err := system.ChownRecursive(cfg.Dir, cfg.RunAsUser); err != nil {
		return err
	}

	// Mods only write their config files on a real boot, so warm the server up
	// once to generate them, then edit them. (Skipped when there's nothing to
	// edit.) writeWebConfigs is edit-only, it never fabricates a config.
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

	printSummary(cfg)
	return nil
}

// rollback removes what a failed install created, its systemd unit and its instance
// directory, so the box is left clean and the name is free to reuse. It touches only
// what this run made (guarded by the flags), never pre-existing data, and runs on a
// fresh context so it still completes even if the install was cancelled mid-way.
func rollback(cfg *config.Config, createdDir, wroteUnit bool) {
	ctx := context.Background()
	if wroteUnit {
		system.RemoveUnit(ctx, minecraft.UnitName(cfg.Instance)+".service")
	}
	if createdDir {
		if err := os.RemoveAll(cfg.Dir); err == nil {
			ui.Info("cleaned up %s", cfg.Dir)
		}
	}
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
