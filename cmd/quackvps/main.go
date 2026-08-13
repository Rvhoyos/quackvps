// Command quackvps sets up a Minecraft server and its web-facing companion
// services on a fresh Debian/Ubuntu VPS. It wires the pieces together: parse
// flags, verify the environment, run the wizard to build a Config, validate it
// once, then execute either an install or an in-place update.
package main

import (
	"context"
	"errors"
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
	"github.com/rvhoyos/quackvps/internal/restore"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/update"
)

func main() {
	if err := run(); err != nil {
		// ErrHandled means the failure was already fully explained; just exit.
		if !errors.Is(err, install.ErrHandled) {
			ui.Error("%v", err)
		}
		os.Exit(1)
	}
}

func run() error {
	// The hidden CI subcommands (used only by the modpack boot-test workflow) run
	// before the normal flag parse and preflight; they aren't a VPS operation.
	if len(os.Args) > 1 && isCISubcommand(os.Args[1]) {
		return runCISubcommand(os.Args[1], os.Args[2:])
	}

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

	nonInteractive := opts.Mode != ""
	if err := preflight(nonInteractive); err != nil {
		return err
	}

	cfg := config.New()
	user, home, err := system.LoginUser()
	if err != nil {
		return err
	}
	cfg.RunAsUser, cfg.RunAsHome = user, home

	client := modrinth.New()

	// Two ways to fill the Config: the flag layer (prompt-free, for scripts) or the
	// interactive wizard. Both only produce a Config; execution is identical.
	if nonInteractive {
		if err := cli.Configure(cfg, opts); err != nil {
			return err
		}
	} else {
		// The picker opens at the user's home by default; --dir only changes that
		// starting folder. It must not touch RunAsHome, where the SSH key lives.
		pickerStart := cfg.RunAsHome
		if opts.Dir != "" {
			pickerStart = opts.Dir
		}
		if _, err := prompt.Run(ctx, cfg, client, pickerStart); err != nil {
			return err
		}
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration is incomplete: %w", err)
	}
	if nonInteractive {
		// The wizard can't offer an unbuildable version/pack; a flag run can, so
		// verify against the sources before touching anything.
		if err := cli.VerifyBuildable(ctx, cfg, client); err != nil {
			return err
		}
	}

	switch cfg.Mode {
	case config.ModeUpdate:
		return update.Run(ctx, cfg, client, updateConfirm(opts))
	case config.ModeRestore:
		return restore.Run(ctx, cfg)
	default:
		return install.Run(ctx, cfg, client)
	}
}

// preflight verifies the essentials before we ask anything: root, a supported OS,
// and the required tools. The interactive wizard additionally needs a real
// terminal; a flag-driven run does not.
func preflight(nonInteractive bool) error {
	if !nonInteractive && !cli.Interactive() {
		return fmt.Errorf("quackvps is interactive without --mode; run it in a terminal or pass --mode")
	}
	if err := system.EnsureRoot(); err != nil {
		return err
	}
	if err := system.CheckOS(); err != nil {
		return err
	}
	return system.RequireCapabilities()
}

// updateConfirm picks how the update's mods-wipe decision is answered: from the
// --empty-mods flag on a non-interactive run, or the huh prompt interactively.
func updateConfirm(opts cli.Options) update.ConfirmFunc {
	if opts.Mode != "" {
		return func(string) (bool, error) { return !opts.EmptyMods, nil }
	}
	return confirm
}

// confirm is the ConfirmFunc the update flow uses for its one mid-run decision
// (wiping mods/). It lives here so the update package stays free of the prompt
// library.
func confirm(question string) (bool, error) {
	// Default to keeping mods, the expected action; the empty-mods path is the
	// deliberate opt-out, never something a stray Enter should trigger.
	answer := true
	field := huh.NewConfirm().
		Title(question).
		Affirmative("Yes, upgrade my mods").
		Negative("No, empty mods folder").
		Value(&answer)
	if err := field.Run(); err != nil {
		return false, err
	}
	return answer, nil
}
