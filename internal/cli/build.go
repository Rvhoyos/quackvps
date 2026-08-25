package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/caddy"
	"github.com/rvhoyos/quackvps/internal/catalog"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/restore"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/web"
)

// Configure fills cfg from the non-interactive flags and runs every local check
// (filesystem state, and for update/restore reading the loader/backup off disk).
// It's the flag-mode counterpart to prompt.Run: on return cfg is ready for the
// offline config.Validate, then the online VerifyBuildable. Any problem is a clear
// error and a non-zero exit, a scripted run never falls back to a prompt.
func Configure(ctx context.Context, cfg *config.Config, opts Options) error {
	switch opts.Mode {
	case "install":
		cfg.Mode = config.ModeInstall
	case "update":
		cfg.Mode = config.ModeUpdate
	case "restore":
		cfg.Mode = config.ModeRestore
	case "add-mods":
		cfg.Mode = config.ModeAddMods
	default:
		return fmt.Errorf("unknown --mode %q", opts.Mode)
	}

	cfg.Parent = opts.Parent
	cfg.Instance = opts.Instance
	cfg.ResolveDir()

	switch cfg.Mode {
	case config.ModeInstall:
		return configureInstall(cfg, opts)
	case config.ModeUpdate:
		return configureUpdate(ctx, cfg, opts)
	case config.ModeAddMods:
		return configureAddMods(ctx, cfg, opts)
	default:
		return configureRestore(ctx, cfg, opts)
	}
}

func configureInstall(cfg *config.Config, opts Options) error {
	if system.InstanceExists(cfg.Parent, cfg.Instance) {
		return fmt.Errorf("%s already holds a server, use --mode update or --mode restore, or pick another --instance", cfg.Dir)
	}

	cfg.Loader = opts.Loader
	cfg.MCVersion = opts.MCVersion
	cfg.HeapMinGB = opts.HeapMinGB
	cfg.HeapMaxGB = opts.HeapMaxGB
	cfg.ServerPort = opts.ServerPort
	cfg.Modpack = opts.Modpack
	cfg.Domain = opts.Domain
	cfg.Email = opts.Email
	cfg.Features = config.Features{
		Dashboard: opts.Dashboard,
		Votifier:  opts.Votifier,
		BlueMap:   opts.BlueMap,
		VoiceChat: opts.VoiceChat,
		Geyser:    opts.Geyser,
	}

	// Feature-derived mods, mirroring the wizard's askFeatures: BlueMap and Simple
	// Voice Chat are standalone mods; the dashboard/votifier live in the quackedsmp
	// mod, so either one pulls it in.
	if cfg.BlueMap {
		cfg.Mods = append(cfg.Mods, catalog.SlugBlueMap)
	}
	if cfg.VoiceChat {
		cfg.Mods = append(cfg.Mods, catalog.SlugVoiceChat)
	}
	if cfg.Geyser {
		cfg.Mods = append(cfg.Mods, catalog.SlugGeyser, catalog.SlugFloodgate)
	}
	if cfg.Dashboard || cfg.Votifier {
		cfg.Mods = append(cfg.Mods, catalog.SlugQuackedSMP)
	}

	// Ports and subdomains, keyed exactly as the install flow reads them. Only the
	// enabled components' values are carried over; a missing port for an enabled
	// component is left unset so Validate reports it plainly.
	ports := map[string]int{
		config.PortDashboard: opts.DashboardPort,
		config.PortBlueMap:   opts.BlueMapPort,
		config.PortVotifier:  opts.VotifierPort,
		config.PortVoiceChat: opts.VoiceChatPort,
		config.PortGeyser:    opts.GeyserPort,
	}
	subs := map[string]string{
		config.PortDashboard: opts.DashboardSubdomain,
		config.PortBlueMap:   opts.BlueMapSubdomain,
	}
	// An omitted --*-subdomain falls back to the same collision-safe default the
	// wizard prefills: the bare label, bumped to <label>-<instance> if another
	// instance already claimed it on this domain.
	var used map[string]bool
	if cfg.Domain != "" {
		u, err := caddy.UsedSubdomains(cfg.Domain, cfg.Instance)
		if err != nil {
			return err
		}
		used = u
	}
	for _, c := range web.Components(cfg.Features, cfg.Loader) {
		if p := ports[c.Key()]; p != 0 {
			cfg.Ports[c.Key()] = p
		}
		if !c.IsWeb() || cfg.Domain == "" {
			continue
		}
		sub := subs[c.Key()]
		if sub == "" {
			sub = caddy.SubdomainDefault(c.DefaultSubdomain(), cfg.Instance, used)
		}
		cfg.Subdomains[c.Key()] = sub
		used[sub] = true
	}

	if opts.HardenSSH {
		if opts.SSHPubKey == "" {
			return fmt.Errorf("--harden-ssh needs --ssh-pubkey, or you could lock yourself out")
		}
		// The flag is the script author's assertion they can already log in by key;
		// that's the confirmation the wizard gets interactively.
		cfg.HardenSSH = true
		cfg.SSHVerified = true
		cfg.SSHPubKey = opts.SSHPubKey
	}
	return nil
}

func configureUpdate(ctx context.Context, cfg *config.Config, opts Options) error {
	if err := resolveUnit(ctx, cfg, opts); err != nil {
		return err
	}
	// The loader is fixed by what's on disk, never a flag, mods are loader-specific
	// and the update's hash lookup only returns same-loader builds.
	return resolveLoaderAndVersion(cfg, opts)
}

// configureAddMods fills the mods to install into an existing server. The loader
// and the version describe that server rather than choosing anything, so the
// loader is read off disk and --mcversion has to be the version it runs: a mod
// built for another one would be downloaded and never load.
func configureAddMods(ctx context.Context, cfg *config.Config, opts Options) error {
	if len(opts.Mods) == 0 {
		return fmt.Errorf("add-mods needs --mods with at least one Modrinth slug, e.g. --mods simple-voice-chat")
	}
	if err := resolveUnit(ctx, cfg, opts); err != nil {
		return err
	}
	if err := resolveLoaderAndVersion(cfg, opts); err != nil {
		return err
	}
	// The server's world data names the version it runs, so --mcversion is only
	// needed for a server that has never generated a world.
	if cfg.MCVersion == "" {
		version, err := minecraft.WorldVersion(cfg.Dir)
		if err != nil {
			return fmt.Errorf("add-mods needs --mcversion: couldn't read the version from this server's world data (%w)", err)
		}
		cfg.MCVersion = version
	}
	cfg.Mods = opts.Mods
	return nil
}

// resolveLoaderAndVersion reads the loader from the instance on disk and takes
// the version from the flags, the pair the modes that work on an existing server
// share.
func resolveLoaderAndVersion(cfg *config.Config, opts Options) error {
	detected, err := loader.Detect(cfg.Dir)
	if err != nil {
		return fmt.Errorf("detect loader at %s: %w", cfg.Dir, err)
	}
	cfg.Loader = detected
	cfg.MCVersion = opts.MCVersion
	return nil
}

func configureRestore(ctx context.Context, cfg *config.Config, opts Options) error {
	if err := resolveUnit(ctx, cfg, opts); err != nil {
		return err
	}
	backups, err := restore.ListBackups(cfg.Dir)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no world backups in %s", filepath.Join(cfg.Dir, "backups"))
	}
	path, err := resolveBackup(opts.Backup, backups)
	if err != nil {
		return err
	}
	cfg.Backup = path
	return nil
}

// resolveUnit records the service update and restore will stop and start: --unit
// when given, otherwise our own mc-<instance>.service. A scripted run never
// guesses and never prompts, so when neither exists it fails, naming the services
// that do point at this folder.
func resolveUnit(ctx context.Context, cfg *config.Config, opts Options) error {
	if opts.Unit != "" {
		if !system.UnitExists(opts.Unit) {
			return fmt.Errorf("systemd knows no service %q", opts.Unit)
		}
		cfg.Unit = opts.Unit
		return nil
	}

	name, services, err := system.ResolveUnit(ctx, minecraft.UnitName(cfg.Instance)+".service")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("no service manages %s: pass --unit with the one that runs it%s", cfg.Dir, unitHint(services, cfg.Dir))
	}
	cfg.Unit = name
	return nil
}

// unitHint names the services that already point at the instance, so the error
// says what to pass instead of leaving the user to hunt.
func unitHint(services []system.Unit, dir string) string {
	matches := system.UnitsForInstance(services, dir)
	if len(matches) == 0 {
		return " (the interactive wizard can also create one for a server that has none)"
	}
	names := make([]string, len(matches))
	for i, u := range matches {
		names[i] = u.Name
	}
	return ", e.g. " + strings.Join(names, ", ")
}

// resolveBackup matches the --backup value against the instance's backups by full
// path or by filename, and lists what's available when it can't.
func resolveBackup(want string, backups []restore.Backup) (string, error) {
	if want == "" {
		return "", fmt.Errorf("restore needs --backup; available: %s", backupNames(backups))
	}
	for _, b := range backups {
		if b.Path == want || filepath.Base(b.Path) == want {
			return b.Path, nil
		}
	}
	return "", fmt.Errorf("backup %q not found; available: %s", want, backupNames(backups))
}

func backupNames(backups []restore.Backup) string {
	names := make([]string, len(backups))
	for i, b := range backups {
		names[i] = filepath.Base(b.Path)
	}
	return strings.Join(names, ", ")
}

// VerifyBuildable confirms, over the network, that the requested version (and any
// modpack or mods asked for) actually have a build for the chosen loader
// , the guarantee the wizard gets by only offering real choices. It runs after the
// offline config.Validate. A fetch failure is a warning, not a hard stop, so a
// transient network problem doesn't block an otherwise valid run.
func VerifyBuildable(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	if cfg.Mode == config.ModeRestore {
		return nil // restore only swaps the world; no loader/version to check
	}

	releases, err := loader.SupportedVersions(ctx, cfg.Loader)
	switch {
	case err != nil:
		ui.Warn("couldn't fetch %s versions to verify %s (%v), continuing", cfg.Loader, cfg.MCVersion, err)
	case !contains(releases, cfg.MCVersion):
		return fmt.Errorf("%s has no build for Minecraft %s, run without --mode to browse the list", cfg.Loader, cfg.MCVersion)
	}

	if cfg.Mode == config.ModeUpdate {
		return nil // update reuses on-disk mods; nothing else to verify
	}

	if cfg.Modpack != "" {
		if catalog.Disabled(cfg.Loader, cfg.Modpack, cfg.MCVersion) {
			return fmt.Errorf("modpack %q is parked for Minecraft %s: our boot test found it crashes on start there. Pick another version or another pack", cfg.Modpack, cfg.MCVersion)
		}
		if !catalog.HasBuild(ctx, client, cfg.Modpack, cfg.Loader, cfg.MCVersion) {
			return fmt.Errorf("modpack %q has no %s build for Minecraft %s", cfg.Modpack, cfg.Loader, cfg.MCVersion)
		}
	}
	for _, slug := range cfg.Mods {
		if !catalog.HasBuild(ctx, client, slug, cfg.Loader, cfg.MCVersion) {
			return fmt.Errorf("mod %q has no %s build for Minecraft %s", slug, cfg.Loader, cfg.MCVersion)
		}
	}
	return nil
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
