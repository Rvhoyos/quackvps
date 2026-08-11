// Command quackvps sets up a Minecraft server and its web-facing companion
// services on a fresh Debian/Ubuntu VPS. It wires the pieces together: parse
// flags, verify the environment, run the wizard to build a Config, validate it
// once, then execute either an install or an in-place update.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/cli"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/install"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/prompt"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/update"
)

func main() {
	if err := run(); err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
}

func run() error {
	opts, handled, err := cli.Parse(os.Args[1:], os.Stdout)
	if err != nil {
		return err // flag package already printed the details
	}
	if handled {
		return nil
	}

	// Cancel cleanly on Ctrl+C / SIGTERM so a long download doesn't wedge.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := preflight(); err != nil {
		return err
	}

	cfg := config.New()
	user, home, err := system.LoginUser()
	if err != nil {
		return err
	}
	cfg.RunAsUser, cfg.RunAsHome = user, home
	if opts.Dir != "" {
		cfg.RunAsHome = opts.Dir // start the picker here instead of the home dir
	}

	client := modrinth.New()
	if _, err := prompt.Run(ctx, cfg, client); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration is incomplete: %w", err)
	}

	switch cfg.Mode {
	case config.ModeUpdate:
		return update.Run(ctx, cfg, client, confirm)
	default:
		return install.Run(ctx, cfg, client)
	}
}

// preflight verifies the essentials before we ask anything: we need root, a
// supported OS, the required tools, and a real terminal for the wizard.
func preflight() error {
	if !cli.Interactive() {
		return fmt.Errorf("quackvps v1 is interactive; run it in a terminal")
	}
	if err := system.EnsureRoot(); err != nil {
		return err
	}
	if err := system.CheckOS(); err != nil {
		return err
	}
	return system.RequireCapabilities()
}

// confirm is the ConfirmFunc the update flow uses for its one mid-run decision
// (wiping mods/). It lives here so the update package stays free of the prompt
// library.
func confirm(question string) (bool, error) {
	answer := false
	field := huh.NewConfirm().Title(question).Value(&answer)
	if err := field.Run(); err != nil {
		return false, err
	}
	return answer, nil
}
