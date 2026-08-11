package caddy

import (
	"context"
	"fmt"
	"os"

	"github.com/rvhoyos/quackvps/internal/system"
)

// version returns `caddy version` output.
func version(ctx context.Context) (string, error) {
	return system.Capture(ctx, "caddy", "version")
}

// validate runs `caddy validate` against the given config.
func validate(ctx context.Context, configPath string) error {
	return system.Run(ctx, "caddy", "validate", "--config", configPath, "--adapter", "caddyfile")
}

// reload reloads the caddy service.
func reload(ctx context.Context) error {
	return system.Run(ctx, "systemctl", "reload", "caddy")
}

// installFromApt configures Caddy's official apt repository and installs it. The
// steps mirror Caddy's documented Debian/Ubuntu install.
func installFromApt(ctx context.Context) error {
	if err := system.AptInstall(ctx, "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl", "gpg"); err != nil {
		return err
	}
	steps := [][]string{
		{"bash", "-c", "curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg"},
		{"bash", "-c", "curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list"},
	}
	for _, s := range steps {
		if err := system.Run(ctx, s[0], s[1:]...); err != nil {
			return fmt.Errorf("configure caddy apt repo: %w", err)
		}
	}
	if err := system.AptInstall(ctx, "caddy"); err != nil {
		return fmt.Errorf("install caddy: %w", err)
	}
	if _, err := os.Stat(MainCaddyfile); err != nil {
		return fmt.Errorf("caddy installed but %s missing: %w", MainCaddyfile, err)
	}
	return nil
}
