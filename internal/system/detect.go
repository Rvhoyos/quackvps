package system

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"
)

// CheckOS confirms we're on the Debian/Ubuntu family. The tool's whole toolchain
// (apt, systemd units, ufw, the Caddy apt repo) assumes it, so we fail clearly
// here rather than half-working on, say, RHEL.
func CheckOS() error {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return fmt.Errorf("read /etc/os-release: %w (is this a Debian/Ubuntu system?)", err)
	}
	defer f.Close()

	fields := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if ok {
			fields[key] = strings.Trim(val, `"`)
		}
	}
	family := fields["ID"] + " " + fields["ID_LIKE"]
	if !strings.Contains(family, "debian") && !strings.Contains(family, "ubuntu") {
		return fmt.Errorf("unsupported OS %q: this tool needs a Debian/Ubuntu-family system", fields["ID"])
	}
	return nil
}

// RequireCapabilities checks for the tools we can't install ourselves. ufw is
// intentionally not required here — it's apt-installable, so ufw.EnsureInstalled
// handles a box that lacks it.
func RequireCapabilities() error {
	for _, cmd := range []string{"apt-get", "systemctl"} {
		if !HasCommand(cmd) {
			return fmt.Errorf("required command %q not found (needs a systemd + apt system)", cmd)
		}
	}
	return nil
}

// EnsureRoot fails clearly if we aren't root. Configuring sshd, ufw, systemd, and
// apt all need it.
func EnsureRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this needs root: re-run with sudo (e.g. `sudo ./quackvps`)")
	}
	return nil
}

// LoginUser resolves the human who invoked us — the account the server will run
// as, never root. Under sudo that's $SUDO_USER; otherwise the current user.
func LoginUser() (name, home string, err error) {
	name = os.Getenv("SUDO_USER")
	if name == "" {
		u, err := user.Current()
		if err != nil {
			return "", "", fmt.Errorf("determine current user: %w", err)
		}
		name = u.Username
	}
	if name == "" || name == "root" {
		return "", "", fmt.Errorf("could not find a non-root login user; run with sudo as your normal user, not as root directly")
	}
	u, err := user.Lookup(name)
	if err != nil {
		return "", "", fmt.Errorf("look up user %q: %w", name, err)
	}
	return name, u.HomeDir, nil
}
