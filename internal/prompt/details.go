package prompt

import (
	"context"
	"fmt"
	"strconv"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/catalog"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// askLoader picks the mod loader, defaulting to NeoForge (what QuackedSMP and most
// current packs use). Paper is intentionally absent — this tool ships mods.
func askLoader(cfg *config.Config) error {
	loader := config.LoaderNeoForge
	field := huh.NewSelect[string]().
		Title("Which mod loader? (the engine that runs your mods)").
		Description("NeoForge — what QuackedSMP and most current packs use. Fabric — lightweight & fast. Forge — the older standard, best for 1.20.1-era packs. Quilt — Fabric-compatible. Vanilla — no mods.").
		Options(
			huh.NewOption("NeoForge (suggested)", config.LoaderNeoForge),
			huh.NewOption("Fabric", config.LoaderFabric),
			huh.NewOption("Forge (for 1.20.1-era modpacks)", config.LoaderForge),
			huh.NewOption("Quilt", config.LoaderQuilt),
			huh.NewOption("Vanilla (no mods)", config.LoaderVanilla),
		).
		Value(&loader)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.Loader = loader
	return nil
}

// askMCVersion takes the Minecraft version. It offers the versions the chosen
// loader can actually run (so e.g. NeoForge lists only 1.21+ and Forge only
// 1.20.x); if that can't be fetched (offline), it falls back to validated free
// text. Used by both install and update — on update the loader is already known.
func askMCVersion(ctx context.Context, cfg *config.Config) error {
	var releases []string
	err := ui.Spinner("Loading "+cfg.Loader+" versions", func() error {
		var e error
		releases, e = loader.SupportedVersions(ctx, cfg.Loader)
		return e
	})
	if err == nil && len(releases) > 0 {
		return selectMCVersion(cfg, releases)
	}
	return inputMCVersion(cfg)
}

// selectMCVersion presents the fetched release list as a searchable dropdown.
func selectMCVersion(cfg *config.Config, releases []string) error {
	version := releases[0] // newest
	opts := make([]huh.Option[string], len(releases))
	for i, v := range releases {
		opts[i] = huh.NewOption(v, v)
	}
	field := huh.NewSelect[string]().
		Title("Which Minecraft version?").
		Description("Type to filter. The matching Java is installed automatically.").
		Options(opts...).
		Value(&version)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.MCVersion = version
	return nil
}

// inputMCVersion is the offline fallback: free text, validated against the
// supported minimum.
func inputMCVersion(cfg *config.Config) error {
	version := cfg.MCVersion
	if version == "" {
		version = "1.21.8"
	}
	field := huh.NewInput().
		Title("Which Minecraft version?").
		Description(fmt.Sprintf("A release like 1.21.8 or 26.1.2 (minimum %s). The matching Java is installed automatically.", config.MinMCVersion)).
		Value(&version).
		Validate(validateMCVersionInput)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.MCVersion = version
	return nil
}

// askModpackAndFeatures presents the modpack list filtered to the chosen
// loader+version, then the single features screen. QuackedSMP, when chosen, opens
// its two bundled sub-prompts.
func askModpackAndFeatures(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	if err := askModpack(ctx, cfg, client); err != nil {
		return err
	}
	return askFeatures(ctx, cfg, client)
}

func askModpack(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	var offers []catalog.ModpackOffer
	if err := ui.Spinner("Checking curated modpacks", func() error {
		var e error
		offers, e = catalog.Modpacks(ctx, client, cfg.Loader, cfg.MCVersion)
		return e
	}); err != nil {
		return err
	}
	// Vanilla (and any loader with no curated packs) has nothing to offer.
	if len(offers) == 0 {
		cfg.Modpack = ""
		return nil
	}

	options := []huh.Option[string]{huh.NewOption("None — just the loader (add mods below)", "")}
	for _, o := range offers {
		options = append(options, huh.NewOption(o.Title, o.Slug))
	}

	modpack := ""
	field := huh.NewSelect[string]().
		Title("Install a modpack? (a curated bundle of mods)").
		Description("Community packs marked server-ready that have a build for your version. Type to filter; pick None to start bare.\nNote: a pack's server tag is set by its author — if one won't start, try another and report it so we can drop it.").
		Options(options...).
		Value(&modpack)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.Modpack = modpack
	return nil
}

// featureCandidate is an add-on the wizard can offer.
type featureCandidate struct{ slug, value, label string }

var featureCandidates = []featureCandidate{
	{catalog.SlugQuackedSMP, "quackedsmp", "QuackedSMP (web dashboard, claims, votifier, voice hooks)"},
	{catalog.SlugBlueMap, config.PortBlueMap, "BlueMap (live web map)"},
	{catalog.SlugVoiceChat, config.PortVoiceChat, "Simple Voice Chat (proximity voice)"},
}

// askFeatures is the one add-on screen. It offers only the add-ons that have a
// build for the chosen loader+MC (so e.g. QuackedSMP, which needs 1.21.8+, isn't
// even shown on 1.21) — the user can't pick something that would fail. Each
// checkbox sets a flag that schedules its later port/Caddy/firewall work.
func askFeatures(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	var options []huh.Option[string]
	if err := ui.Spinner("Checking available add-ons", func() error {
		for _, c := range featureCandidates {
			if catalog.HasBuild(ctx, client, c.slug, cfg.Loader, cfg.MCVersion) {
				options = append(options, huh.NewOption(c.label, c.value))
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if len(options) == 0 {
		ui.Info("No add-ons are available for %s %s.", cfg.Loader, cfg.MCVersion)
		return nil
	}

	var selected []string
	field := huh.NewMultiSelect[string]().
		Title("Add-ons — space to toggle, enter to confirm").
		Description("Only add-ons with a build for your version are shown. Each sets up its own port/proxy/firewall later.").
		Options(options...).
		Value(&selected)
	if err := field.Run(); err != nil {
		return err
	}

	// BlueMap and Simple Voice Chat are standalone mods, so selecting them means
	// downloading the mod too (the install dedups against a modpack that already
	// bundles them). The dashboard/votifier are part of the quackedsmp mod, so
	// they only set flags.
	for _, s := range selected {
		switch s {
		case config.PortBlueMap:
			cfg.BlueMap = true
			cfg.Mods = append(cfg.Mods, catalog.SlugBlueMap)
		case config.PortVoiceChat:
			cfg.VoiceChat = true
			cfg.Mods = append(cfg.Mods, catalog.SlugVoiceChat)
		case "quackedsmp":
			cfg.Mods = append(cfg.Mods, catalog.SlugQuackedSMP)
			if err := askQuackedSMPSubPrompts(cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

// askQuackedSMPSubPrompts asks the two bundled QuackedSMP features: its web
// dashboard and Votifier v2 support.
func askQuackedSMPSubPrompts(cfg *config.Config) error {
	dashboard, votifier := true, false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Set up the QuackedSMP web dashboard/panel?").
			Description("A browser panel to manage the server and share world-backup downloads. Fronted by Caddy with HTTPS.").
			Value(&dashboard),
		huh.NewConfirm().
			Title("Also set up Votifier v2 support (comes bundled with QuackedSMP)?").
			Description("Lets server-list sites send in-game rewards when players vote. Opens one firewall port; no web page.").
			Value(&votifier),
	))
	if err := form.Run(); err != nil {
		return err
	}
	cfg.Dashboard = dashboard
	cfg.Votifier = votifier
	return nil
}

// askRAM takes the heap size in GB. When the box's total memory is known it
// suggests a safe default and rejects a size that would leave the OS too little
// to run — an oversized heap makes the JVM crash on the first boot.
func askRAM(cfg *config.Config) error {
	total := system.TotalMemoryGB()

	def := 4
	desc := "Modded servers want 4GB+. Too little causes lag and crashes; too much starves other servers on the box."
	if total > 0 {
		def = safeDefaultRAM(total)
		desc = fmt.Sprintf("This box has ~%dGB total — leave at least 1GB for the operating system. Modded servers want 4GB+; too little causes lag and crashes.", total)
	}

	value := strconv.Itoa(def)
	field := huh.NewInput().
		Title("How much RAM for this server? (in GB)").
		Description(desc).
		Value(&value).
		Validate(ramValidator(total))
	if err := field.Run(); err != nil {
		return err
	}
	cfg.RAMGB = mustAtoi(value)
	return nil
}

// safeDefaultRAM suggests a heap that leaves headroom for the OS, capped at the
// usual 4GB default.
func safeDefaultRAM(total int) int {
	d := total - 2
	if d < 1 {
		d = 1
	}
	if d > 4 {
		d = 4
	}
	return d
}

// ramValidator rejects a heap that leaves less than ~1GB for the OS, which would
// otherwise OOM-crash the JVM on first boot.
func ramValidator(total int) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return fmt.Errorf("enter a whole number of at least 1")
		}
		if total > 0 && n > total-1 {
			return fmt.Errorf("this box has only ~%dGB — leave at least 1GB for the OS (max %dGB)", total, total-1)
		}
		return nil
	}
}
