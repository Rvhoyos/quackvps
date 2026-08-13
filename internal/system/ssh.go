package system

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

const hardeningDropin = "/etc/ssh/sshd_config.d/10-quackvps.conf"

// EffectiveConfig returns sshd's resolved settings via `sshd -T`, which merges
// the main file, all drop-ins, and defaults. Grepping files would miss drop-ins
// (e.g. Ubuntu's 50-cloud-init.conf), so this is the only honest source.
func EffectiveConfig(ctx context.Context) (map[string]string, error) {
	out, err := Capture(ctx, "sshd", "-T")
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	for _, line := range splitLines(out) {
		key, val, ok := strings.Cut(line, " ")
		if ok {
			cfg[strings.ToLower(key)] = strings.TrimSpace(val)
		}
	}
	return cfg, nil
}

// IsHardened reports whether password and keyboard-interactive auth are already
// off, in which case the hardening step is a no-op.
func IsHardened(ctx context.Context) bool {
	cfg, err := EffectiveConfig(ctx)
	if err != nil {
		return false
	}
	return cfg["passwordauthentication"] == "no" && cfg["kbdinteractiveauthentication"] == "no"
}

// DetectSSHPort returns the port sshd listens on (first if several), defaulting
// to 22. UFW must allow this before enabling.
func DetectSSHPort(ctx context.Context) int {
	cfg, err := EffectiveConfig(ctx)
	if err == nil {
		if p, err := strconv.Atoi(cfg["port"]); err == nil && p > 0 {
			return p
		}
	}
	return 22
}

// AuthorizedKeysPresent reports whether the user already has at least one key,
// so hardening knows whether it must help them add one first.
func AuthorizedKeysPresent(home string) bool {
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		return false
	}
	for _, line := range splitLines(string(data)) {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			return true
		}
	}
	return false
}

// ValidatePublicKey accepts only a single valid public-key line. It explicitly
// rejects a pasted private key, the classic, dangerous mistake, with a clear
// message instead of a parse error.
func ValidatePublicKey(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("no key provided")
	}
	if strings.Contains(s, "PRIVATE KEY") {
		return fmt.Errorf("that looks like your PRIVATE key, never share it; paste the .pub line instead (it starts with ssh-ed25519 or ssh-rsa)")
	}
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("a public key is a single line; this has line breaks")
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s)); err != nil {
		return fmt.Errorf("not a valid SSH public key: %w", err)
	}
	return nil
}

// AppendAuthorizedKey adds a public key to the user's authorized_keys with the
// right ownership and permissions, skipping it if already present.
func AppendAuthorizedKey(username, home, key string) error {
	key = strings.TrimSpace(key)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", sshDir, err)
	}
	akPath := filepath.Join(sshDir, "authorized_keys")

	existing, _ := os.ReadFile(akPath)
	for _, line := range splitLines(string(existing)) {
		if strings.TrimSpace(line) == key {
			return nil // already authorized
		}
	}

	f, err := os.OpenFile(akPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open authorized_keys: %w", err)
	}
	if _, err := f.WriteString(key + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	// We run as root, so the new files would be root-owned and unusable by sshd
	// for this user, fix ownership on the dir and the key file.
	if err := chownToUser(username, sshDir, akPath); err != nil {
		return err
	}
	return os.Chmod(akPath, 0o600)
}

// WriteHardeningDropin writes the key-only auth settings as a low-numbered
// drop-in. PermitRootLogin is deliberately not set: it's provider-dependent
// (Hostinger logs in as root; OVH disables it) and changing it can lock a user
// out of their normal access path.
func WriteHardeningDropin() error {
	contents := "# Written by quackvps: key-only SSH auth.\n" +
		"# PermitRootLogin is intentionally left untouched (provider-dependent).\n" +
		"PasswordAuthentication no\n" +
		"KbdInteractiveAuthentication no\n"
	if err := os.WriteFile(hardeningDropin, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", hardeningDropin, err)
	}
	return nil
}

// ValidateConfig runs `sshd -t` to check the config parses before we reload
// the guard against a typo stranding the user.
func ValidateConfig(ctx context.Context) error { return Run(ctx, "sshd", "-t") }

// ReloadSSH reloads (not restarts) sshd, so the current session stays alive even
// if something is wrong.
func ReloadSSH(ctx context.Context) error { return Run(ctx, "systemctl", "reload", "ssh") }

func chownToUser(username string, paths ...string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("look up user %q: %w", username, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	for _, p := range paths {
		if err := os.Chown(p, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", p, err)
		}
	}
	return nil
}
