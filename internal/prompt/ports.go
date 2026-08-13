package prompt

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/caddy"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/web"
)

// askDomainAndPorts gathers, in the SPEC order, the domain (once, only if a web
// service is enabled), then the game port, then each flagged add-on's port, then
// the subdomains. Port defaults are the next free port after a collision scan.
func askDomainAndPorts(ctx context.Context, cfg *config.Config) error {
	if cfg.AnyWeb() {
		if err := askDomain(cfg); err != nil {
			return err
		}
	}

	used, err := system.CollisionScan(ctx, cfg.Parent)
	if err != nil {
		return err
	}

	if err := askServerPort(cfg, used); err != nil {
		return err
	}
	for _, c := range web.Components(cfg.Features, cfg.Loader) {
		if err := askComponentPort(cfg, c, used); err != nil {
			return err
		}
	}
	return askSubdomains(cfg)
}

// askDomain asks once whether the user has a domain, and if so the ACME email
// used when we need to add Caddy's global block.
func askDomain(cfg *config.Config) error {
	domain := ""
	field := huh.NewInput().
		Title("Domain for your web services? (leave blank if you don't have one)").
		Description("With a domain, Caddy gets real HTTPS at sub.yourdomain. Point that subdomain's DNS at this server first. Blank means we print an SSH tunnel command instead (private, no domain needed).").
		Placeholder("example.com").
		Value(&domain)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.Domain = domain
	if domain != "" {
		return askEmail(cfg)
	}
	return nil
}

func askEmail(cfg *config.Config) error {
	email := ""
	field := huh.NewInput().
		Title("Email for HTTPS certificates? (optional)").
		Description("Caddy gives this to Let's Encrypt when it registers your certificate account, so the CA can reach you if issuing or renewing ever fails. Only added if your Caddyfile has no contact yet; leave blank to skip (certs still work).").
		Validate(config.ValidateEmail).
		Value(&email)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.Email = strings.TrimSpace(email)
	return nil
}

func askServerPort(cfg *config.Config, used map[int]bool) error {
	def := system.NextFree(25565, used)
	port, err := askPort(
		"Minecraft game port",
		"The port players connect to. 25565 is the default; we pick the next free one so servers don't collide.",
		def, used)
	if err != nil {
		return err
	}
	cfg.ServerPort = port
	used[port] = true
	return nil
}

func askComponentPort(cfg *config.Config, c web.Component, used map[int]bool) error {
	def := system.NextFree(c.DefaultPort(), used)
	port, err := askPort(
		fmt.Sprintf("%s port", c.Key()),
		portHint(c),
		def, used)
	if err != nil {
		return err
	}
	cfg.Ports[c.Key()] = port
	used[port] = true
	return nil
}

func portHint(c web.Component) string {
	if c.IsWeb() {
		return "Bound to localhost and reached through Caddy, not exposed directly. The default is collision-checked."
	}
	return "Opened in the firewall for direct connections. The default is collision-checked."
}

// askSubdomains asks a subdomain per web component, but only on the domain path.
// The default is collision-checked against other instances already using this
// domain, so a second server defaults to status-<instance> rather than clashing on
// status.<domain> (which Caddy would later reject as a duplicate site address).
func askSubdomains(cfg *config.Config) error {
	if cfg.Domain == "" {
		return nil
	}
	used, err := caddy.UsedSubdomains(cfg.Domain, cfg.Instance)
	if err != nil {
		return err
	}
	for _, c := range web.Components(cfg.Features, cfg.Loader) {
		if !c.IsWeb() {
			continue
		}
		sub := caddy.SubdomainDefault(c.DefaultSubdomain(), cfg.Instance, used)
		field := huh.NewInput().
			Title(fmt.Sprintf("Subdomain for %s?", c.Key())).
			Description(fmt.Sprintf("Reachable at <sub>.%s. Default %q, but pick anything you like.", cfg.Domain, sub)).
			Value(&sub).
			Validate(validateNotEmpty)
		if err := field.Run(); err != nil {
			return err
		}
		cfg.Subdomains[c.Key()] = sub
		used[sub] = true // so a second web component can't reuse this label
	}
	return nil
}

// askPort is the shared numeric-port prompt. The default is already the next free
// port; the validator also rejects a typed port that's in the used set, so a
// user can't hand-enter a collision.
func askPort(title, why string, def int, used map[int]bool) (int, error) {
	value := strconv.Itoa(def)
	field := huh.NewInput().
		Title(title).
		Description(why).
		Value(&value).
		Validate(func(s string) error {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("enter a port between 1 and 65535")
			}
			// The default equals def and isn't in used yet, so it always passes;
			// only a hand-typed in-use port is rejected.
			if n != def && used[n] {
				return fmt.Errorf("port %d is already in use, pick another", n)
			}
			return nil
		})
	if err := field.Run(); err != nil {
		return 0, err
	}
	return mustAtoi(value), nil
}
