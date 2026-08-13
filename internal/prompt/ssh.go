package prompt

import (
	"context"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// askHardenSSH runs first, since securing the box should precede building on it.
// It's skipped entirely when the box is already key-only. When a user has no key
// yet, it walks them through adding one — the highest-lockout-risk step, so it
// gathers the key here but the actual sshd change happens in execution only after
// the user confirms key login works.
func askHardenSSH(ctx context.Context, cfg *config.Config) error {
	if system.IsHardened(ctx) {
		ui.Info("SSH is already key-only — skipping hardening.")
		return nil
	}

	harden := true
	confirm := huh.NewConfirm().
		Title("Harden SSH to key-only login?").
		Description("Turns off password logins so only your SSH key works — the single biggest security win on a VPS. We never touch root-login settings (those depend on your host).").
		Value(&harden)
	if err := confirm.Run(); err != nil {
		return err
	}
	cfg.HardenSSH = harden
	if !harden {
		return nil
	}

	// If there's no key yet, collect one and install it now — before the verify
	// step, so the user can actually test it. Deferring the append to execution
	// would leave the verify login falling back to a password every time.
	if !system.AuthorizedKeysPresent(cfg.RunAsHome) {
		if err := askPublicKey(cfg); err != nil {
			return err
		}
		if err := system.AppendAuthorizedKey(cfg.RunAsUser, cfg.RunAsHome, cfg.SSHPubKey); err != nil {
			return err
		}
	}

	// The real anti-lockout guard: prove the key works before disabling passwords.
	// The concrete signal is a login with NO password prompt — SSH tries the key
	// before the password, so if you get in without being asked for a password,
	// the key is what let you in (and turning passwords off is safe).
	verified := false
	verify := huh.NewConfirm().
		Title("In a NEW terminal, run `ssh " + cfg.RunAsUser + "@<this-server>`. Did it log you in WITHOUT asking for a password?").
		Description("No password prompt = your key works, so it's safe to turn passwords off. If it asked for a password (or failed), choose No and we'll skip hardening.").
		Affirmative("Yes — no password prompt").
		Negative("No — it asked for a password").
		Value(&verified)
	if err := verify.Run(); err != nil {
		return err
	}
	cfg.SSHVerified = verified
	if !verified {
		cfg.HardenSSH = false
		ui.Warn("Skipping SSH hardening — verify key login and re-run to enable it.")
	}
	return nil
}

func askPublicKey(cfg *config.Config) error {
	var key string
	field := huh.NewText().
		Title("Paste your SSH PUBLIC key (the one-line .pub, starts with ssh-ed25519).").
		Description("On your own machine: `cat ~/.ssh/id_ed25519.pub` and copy the whole line. NEVER paste the private key (the long BEGIN...PRIVATE KEY block).").
		Validate(system.ValidatePublicKey).
		Value(&key)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.SSHPubKey = key
	return nil
}
