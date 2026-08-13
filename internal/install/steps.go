package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rvhoyos/quackvps/internal/caddy"
	"github.com/rvhoyos/quackvps/internal/catalog"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/web"
)

// hardenSSH appends the user's key (if one was gathered) and writes the key-only
// drop-in, validating before the reload so a bad config can't strand the user.
func hardenSSH(ctx context.Context, cfg *config.Config) error {
	if cfg.SSHPubKey != "" {
		if err := system.AppendAuthorizedKey(cfg.RunAsUser, cfg.RunAsHome, cfg.SSHPubKey); err != nil {
			return err
		}
	}
	if err := system.WriteHardeningDropin(); err != nil {
		return err
	}
	if err := system.ValidateConfig(ctx); err != nil {
		return fmt.Errorf("sshd config invalid, not reloading: %w", err)
	}
	if err := system.ReloadSSH(ctx); err != nil {
		return err
	}
	ui.Success("Password logins disabled (key-only). Root-login settings left untouched.")
	return nil
}

// preflight verifies the modpack and every selected mod resolve for the chosen
// loader + MC version, before any irreversible step. It collects all misses and
// reports them together so the user sees the full picture at once.
func preflight(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	var missing []string
	check := func(slug string) {
		versions, err := client.Versions(ctx, slug, []string{cfg.Loader}, []string{cfg.MCVersion})
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s (lookup failed: %v)", slug, err))
			return
		}
		if len(versions) == 0 {
			missing = append(missing, fmt.Sprintf("%s (no build for %s %s)", slug, cfg.Loader, cfg.MCVersion))
		}
	}

	if cfg.Modpack != "" {
		check(cfg.Modpack)
	}
	for _, slug := range cfg.Mods {
		check(slug)
	}

	if len(missing) > 0 {
		return fmt.Errorf("cannot install, the following aren't available for %s %s:\n  - %s",
			cfg.Loader, cfg.MCVersion, strings.Join(missing, "\n  - "))
	}
	return nil
}

// warmUpTimeout bounds the config-generation boot. A working server reaches the
// point of writing its mod configs well within this; a broken one never does, so
// the timeout is also how we catch a pack that can't start.
const warmUpTimeout = 4 * time.Minute

// warmUpBoot starts the server once so the mods generate their own config files,
// waits until the files we need to edit exist, then stops it. It's skipped when
// there are no mod configs to edit. If the files never appear (a mod crash-loops
// or the pack won't boot), it stops the unit and reports with the log tail
// failing loudly here instead of a false "running" at the end.
func warmUpBoot(ctx context.Context, cfg *config.Config) error {
	wanted := expectedConfigFiles(cfg)
	if len(wanted) == 0 {
		return nil
	}
	unit := minecraft.UnitName(cfg.Instance)

	return ui.Spinner("Booting once to generate mod configs", func() error {
		if err := system.Start(ctx, unit); err != nil {
			return err
		}
		waitErr := system.WaitForFiles(ctx, wanted, warmUpTimeout)
		// Check the unit's Result before stopping it, stopping resets it, and an
		// out-of-memory kill leaves no log to tail otherwise.
		oom := waitErr != nil && system.UnitOOMKilled(ctx, unit)
		// Always stop the warm-up server before returning, success or not.
		_ = system.Stop(ctx, unit)
		_ = system.WaitInactive(ctx, unit, 130*time.Second)
		if oom {
			return fmt.Errorf("the server ran out of memory generating mod configs (%dG max heap on a ~%dGB box), lower the maximum heap or add swap", cfg.HeapMaxGB, system.TotalMemoryGB())
		}
		if waitErr != nil {
			return fmt.Errorf("server didn't finish starting:\n%s\n%w", serverLogTail(cfg.Dir), waitErr)
		}
		return nil
	})
}

// expectedConfigFiles is the set of mod config files writeWebConfigs will edit,
// derived from the chosen features, the exact files the warm-up must wait for.
func expectedConfigFiles(cfg *config.Config) []string {
	var files []string
	if containsSlug(cfg.Mods, catalog.SlugQuackedSMP) {
		files = append(files, filepath.Join(cfg.Dir, "config", "quackedsmp.json"))
	}
	if cfg.BlueMap {
		files = append(files,
			filepath.Join(cfg.Dir, "config", "bluemap", "core.conf"),
			filepath.Join(cfg.Dir, "config", "bluemap", "webserver.conf"))
	}
	if cfg.VoiceChat {
		files = append(files, filepath.Join(cfg.Dir, "config", "voicechat", "voicechat-server.properties"))
	}
	return files
}

// serverLogTail returns the last lines of the server log for failure messages.
func serverLogTail(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "logs", "latest.log"))
	if err != nil {
		return "(no server log found)"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return strings.Join(lines, "\n")
}

// installContent installs the modpack (if chosen) and any individual mods,
// resolving required dependencies for each mod.
func installContent(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	if cfg.Modpack != "" {
		if err := ui.Spinner("Installing modpack "+cfg.Modpack, func() error {
			mp, err := client.ResolveMrpack(ctx, cfg.Modpack, cfg.MCVersion, cfg.Loader)
			if err != nil {
				return err
			}
			return mp.Install(ctx, cfg.Dir)
		}); err != nil {
			return err
		}
	}

	// Identify what a modpack already dropped in mods/ so a selected add-on mod
	// (e.g. Simple Voice Chat) isn't installed a second time at a different
	// version, which would leave two conflicting jars.
	installed, err := installedProjects(ctx, client, cfg.Dir)
	if err != nil {
		return err
	}
	for _, slug := range cfg.Mods {
		if err := installMod(ctx, cfg, client, slug, installed); err != nil {
			return err
		}
	}
	return nil
}

// installMod downloads a mod's primary file plus its required dependencies into
// mods/, skipping anything a modpack already provides (tracked by project ID).
func installMod(ctx context.Context, cfg *config.Config, client modrinth.Client, slug string, installed map[string]bool) error {
	return ui.Spinner("Installing "+slug, func() error {
		versions, err := client.Versions(ctx, slug, []string{cfg.Loader}, []string{cfg.MCVersion})
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			return fmt.Errorf("%s has no build for %s %s", slug, cfg.Loader, cfg.MCVersion)
		}
		return downloadWithDeps(ctx, cfg, client, versions[0], installed)
	})
}

// downloadWithDeps downloads v and its required dependencies, skipping any
// project already present and recording each one so later mods dedup too.
func downloadWithDeps(ctx context.Context, cfg *config.Config, client modrinth.Client, v modrinth.Version, installed map[string]bool) error {
	for _, want := range append([]modrinth.Version{v}, resolveDepsBestEffort(ctx, cfg, client, v)...) {
		if installed[want.ProjectID] {
			continue
		}
		if err := downloadMod(ctx, cfg.Dir, want); err != nil {
			return err
		}
		installed[want.ProjectID] = true
	}
	return nil
}

func resolveDepsBestEffort(ctx context.Context, cfg *config.Config, client modrinth.Client, v modrinth.Version) []modrinth.Version {
	deps, err := modrinth.ResolveRequired(ctx, client, v, []string{cfg.Loader}, []string{cfg.MCVersion})
	if err != nil {
		// Dependency resolution is best-effort; a missing optional dep shouldn't
		// abort the whole install. The mod's own jar still gets downloaded.
		ui.Warn("could not resolve dependencies for %s: %v", v.ProjectID, err)
		return nil
	}
	return deps
}

// installedProjects hashes the jars already in mods/ and asks Modrinth which
// project each belongs to, so add-on mods can dedup against a modpack's bundle.
// Best-effort: if identification fails we return an empty set (worst case, a
// duplicate jar) rather than failing the install.
func installedProjects(ctx context.Context, client modrinth.Client, dir string) (map[string]bool, error) {
	jars, err := filepath.Glob(filepath.Join(dir, "mods", "*.jar"))
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	if len(jars) == 0 {
		return ids, nil
	}

	var hashes []string
	for _, jar := range jars {
		sum, err := dl.SHA512File(jar)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, sum)
	}
	found, err := client.IdentifyByHash(ctx, hashes)
	if err != nil {
		ui.Warn("could not identify existing mods for dedup: %v", err)
		return ids, nil
	}
	for _, v := range found {
		ids[v.ProjectID] = true
	}
	return ids, nil
}

// configureServer writes run.sh, boots once to generate configs, accepts the
// EULA, and sets the game port, the loader-agnostic first-run-then-configure
// path.
func configureServer(ctx context.Context, cfg *config.Config, l loader.Loader) error {
	body, err := l.RunScript(cfg.Dir, cfg.HeapMinGB, cfg.HeapMaxGB)
	if err != nil {
		return err
	}
	if err := minecraft.WriteRunScript(cfg.Dir, body); err != nil {
		return err
	}
	if err := ui.Spinner("Generating server files (first run)", func() error {
		return minecraft.FirstRunGenerate(ctx, cfg.Dir)
	}); err != nil {
		return explainFirstRunFailure(cfg, err)
	}
	if err := minecraft.AcceptEULA(cfg.Dir); err != nil {
		return err
	}
	return minecraft.SetServerPort(cfg.Dir, cfg.ServerPort)
}

// explainFirstRunFailure turns a failed first-run boot into an actionable message
// and returns ErrHandled so it's printed exactly once. A crash with a modpack
// selected is the one case worth reporting upstream, the pack won't start, so we
// name it and ask for a report; an out-of-memory kill is the box's fault, not the
// pack's, so it never asks for one.
func explainFirstRunFailure(cfg *config.Config, err error) error {
	var fre *minecraft.FirstRunError
	if !errors.As(err, &fre) {
		return err
	}
	switch fre.Kind {
	case minecraft.FirstRunOOM:
		ui.Warn("The server ran out of memory on first launch (%dG max heap on a ~%dGB box). Lower this server's maximum heap, or add swap to the box.", cfg.HeapMaxGB, system.TotalMemoryGB())
	case minecraft.FirstRunTimeout:
		ui.Warn("The server didn't finish its first run in time, it may be hung.")
		ui.Info("%s", fre.Tail)
	default:
		if cfg.Modpack != "" {
			ui.Warn("The modpack %q failed to start on %s %s, please report it so we can drop it from the list.", cfg.Modpack, cfg.Loader, cfg.MCVersion)
		} else {
			ui.Warn("The server crashed on first launch.")
		}
		ui.Info("%s", fre.Tail)
	}
	return ErrHandled
}

// writeWebConfigs writes each add-on's port into its own config, points Simple
// Voice Chat at the box's public IP, and sets the two cross-config QuackedSMP
// fields (panel_url, voicechat_enable).
func writeWebConfigs(cfg *config.Config) error {
	for _, c := range web.Components(cfg.Features) {
		if err := c.WritePort(cfg.Dir, cfg.Ports[c.Key()]); err != nil {
			return err
		}
	}

	// Pin voice_host to the public IP so clients get a reachable address instead
	// of the mod's auto-detected one (which is wrong on many VPSes). Skipped if we
	// can't determine the IP, leaving the mod's default.
	if cfg.VoiceChat && cfg.PublicIP != "" {
		if err := web.SetVoiceHost(cfg.Dir, cfg.PublicIP); err != nil {
			return err
		}
	}

	quacked := containsSlug(cfg.Mods, catalog.SlugQuackedSMP)
	if quacked && cfg.Dashboard && cfg.Domain != "" {
		url := fmt.Sprintf("https://%s.%s", cfg.Subdomains[config.PortDashboard], cfg.Domain)
		if err := web.SetPanelURL(cfg.Dir, url); err != nil {
			return err
		}
	}
	if quacked && cfg.VoiceChat {
		if err := web.SetVoicechatEnable(cfg.Dir, true); err != nil {
			return err
		}
	}
	return nil
}

// installService writes the systemd unit and reloads the daemon so the unit is
// known. The unit is enabled and started later, after ownership is fixed.
func installService(ctx context.Context, cfg *config.Config) error {
	unit := minecraft.UnitName(cfg.Instance) + ".service"
	if err := system.WriteUnit(unit, minecraft.UnitFile(cfg)); err != nil {
		return err
	}
	return system.DaemonReload(ctx)
}

// configureCaddy installs Caddy if needed, writes this instance's site file,
// ensures the import line, warns on bad DNS, and reloads with rollback.
func configureCaddy(ctx context.Context, cfg *config.Config) error {
	if err := caddy.EnsureInstalled(ctx); err != nil {
		return err
	}

	var blocks []string
	for _, c := range web.Components(cfg.Features) {
		if !c.IsWeb() {
			continue
		}
		sub := cfg.Subdomains[c.Key()]
		blocks = append(blocks, c.CaddyBlock(sub, cfg.Domain, cfg.Ports[c.Key()]))
		checkDNS(cfg.PublicIP, fmt.Sprintf("%s.%s", sub, cfg.Domain))
	}

	if err := caddy.WriteInstanceFile(cfg.Instance, blocks); err != nil {
		return err
	}
	if err := caddy.EnsureImportLine(cfg.Email); err != nil {
		return err
	}
	// Fresh instance file, so a validation failure rolls back to "no file".
	return caddy.ReloadInstance(ctx, cfg.Instance, "")
}

func checkDNS(ip, host string) {
	if ip == "" {
		return // can't determine our IP; skip the check
	}
	if !caddy.DNSResolvesTo(host, ip) {
		ui.Warn("%s doesn't resolve to this server (%s) yet. Point its DNS here; Caddy will keep retrying the certificate.", host, ip)
	}
}

// configureFirewall opens SSH first (so enabling never drops us), then the game
// port, then 80/443 when a domain is used, then the flagged network ports.
func configureFirewall(ctx context.Context, cfg *config.Config) error {
	if err := system.Firewall.EnsureInstalled(ctx); err != nil {
		return err
	}
	if err := system.Firewall.AllowSSHFirst(ctx, system.DetectSSHPort(ctx)); err != nil {
		return err
	}
	if err := system.Firewall.Allow(ctx, cfg.ServerPort, "tcp"); err != nil {
		return err
	}
	// With a domain, Caddy serves the web add-ons publicly and needs 80/443 open:
	// 443 for HTTPS, 80 for the HTTP->HTTPS redirect and the ACME http-01 challenge.
	// Without a domain the add-ons stay on localhost (reached via ssh -L), so these
	// stay closed.
	if cfg.Domain != "" {
		if err := system.Firewall.Allow(ctx, 80, "tcp"); err != nil {
			return err
		}
		if err := system.Firewall.Allow(ctx, 443, "tcp"); err != nil {
			return err
		}
	}
	for _, c := range web.Components(cfg.Features) {
		if c.IsWeb() {
			continue // web ports stay internal, reached via Caddy
		}
		if err := system.Firewall.Allow(ctx, cfg.Ports[c.Key()], c.Proto()); err != nil {
			return err
		}
	}
	return system.Firewall.Enable(ctx)
}

func containsSlug(slugs []string, want string) bool {
	for _, s := range slugs {
		if s == want {
			return true
		}
	}
	return false
}
