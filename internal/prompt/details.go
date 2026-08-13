package prompt

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/catalog"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// askLoader picks the mod loader, defaulting to NeoForge (what QuackedSMP and most
// current packs use). Paper is intentionally absent; this tool ships mods.
func askLoader(cfg *config.Config) error {
	loader := config.LoaderNeoForge
	field := huh.NewSelect[string]().
		Title("Which mod loader? (the engine that runs your mods)").
		Description("NeoForge: what QuackedSMP and most current packs use. Fabric: lightweight & fast. Forge: the older standard, best for 1.20.1-era packs. Quilt: Fabric-compatible. Vanilla: no mods.").
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
// text. Used by both install and update; on update the loader is already known.
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

// manualSlug is the sentinel Select value for the "type your own slug" option. A
// null byte can't appear in a Modrinth slug, so it never collides with a real one.
const manualSlug = "\x00manual"

func askModpack(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	// Vanilla has no loader for a modpack to target.
	if !catalog.SupportsModpacks(cfg.Loader) {
		cfg.Modpack = ""
		return nil
	}

	var offers []catalog.ModpackOffer
	if err := ui.Spinner("Checking curated modpacks", func() error {
		var e error
		offers, e = catalog.Modpacks(ctx, client, cfg.Loader, cfg.MCVersion)
		return e
	}); err != nil {
		return err
	}

	options := []huh.Option[string]{huh.NewOption("None: just the loader (add mods below)", "")}
	for _, o := range offers {
		options = append(options, huh.NewOption(o.Title, o.Slug))
	}
	options = append(options, huh.NewOption("Enter a Modrinth slug manually…", manualSlug))

	choice := ""
	field := huh.NewSelect[string]().
		Title("Install a modpack? (a curated bundle of mods)").
		Description("Community packs marked server-ready that have a build for your version. Type to filter; pick None to start bare, or enter any Modrinth slug yourself.\nNote: a pack's server tag is set by its author. If one won't start, try another and report it so we can drop it.").
		Options(options...).
		Value(&choice)
	if err := field.Run(); err != nil {
		return err
	}
	if choice == manualSlug {
		return askManualSlug(ctx, cfg, client)
	}
	cfg.Modpack = choice
	return nil
}

// askManualSlug installs any Modrinth modpack by slug, not just the curated picks.
// The slug is checked for a build matching the loader + MC version; a miss warns
// and re-prompts rather than failing silently or aborting the wizard. Blank = none.
func askManualSlug(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	for {
		slug := ""
		field := huh.NewInput().
			Title("Modrinth modpack slug").
			Description("The name from the pack's URL, e.g. modrinth.com/modpack/cobblemon-fabric. Leave blank for none.").
			Placeholder("cobblemon-fabric").
			Value(&slug)
		if err := field.Run(); err != nil {
			return err
		}
		slug = slugFromInput(slug)
		if slug == "" {
			cfg.Modpack = ""
			return nil
		}

		ok := false
		if err := ui.Spinner("Checking the slug on Modrinth", func() error {
			ok = catalog.HasBuild(ctx, client, slug, cfg.Loader, cfg.MCVersion)
			return nil
		}); err != nil {
			return err
		}
		if ok {
			cfg.Modpack = slug
			return nil
		}
		ui.Warn("no %s modpack %q with a build for Minecraft %s. Check the slug on the pack's Modrinth page, or leave blank for none", cfg.Loader, slug, cfg.MCVersion)
	}
}

// slugFromInput extracts the bare slug, tolerating a pasted URL or path by taking
// the last path segment (modrinth.com/modpack/<slug> → <slug>).
func slugFromInput(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// featureCandidate is an add-on the wizard can offer. It's shown only when every
// slug it needs has a build for the chosen loader+MC, so a multi-mod feature like
// crossplay (Geyser + Floodgate) appears only when both are installable.
type featureCandidate struct {
	slugs []string
	value string
	label string
}

var featureCandidates = []featureCandidate{
	{[]string{catalog.SlugQuackedSMP}, "quackedsmp", "QuackedSMP (web dashboard, claims, votifier, voice hooks)"},
	{[]string{catalog.SlugBlueMap}, config.PortBlueMap, "BlueMap (live web map)"},
	{[]string{catalog.SlugVoiceChat}, config.PortVoiceChat, "Simple Voice Chat (proximity voice)"},
	{[]string{catalog.SlugGeyser, catalog.SlugFloodgate}, config.PortGeyser, "Bedrock crossplay (Geyser + Floodgate: let Bedrock players join)"},
}

// askFeatures is the one add-on screen. It offers only the add-ons that have a
// build for the chosen loader+MC (so e.g. QuackedSMP, which needs 1.21.8+, isn't
// even shown on 1.21); the user can't pick something that would fail. Each
// checkbox sets a flag that schedules its later port/Caddy/firewall work.
func askFeatures(ctx context.Context, cfg *config.Config, client modrinth.Client) error {
	var options []huh.Option[string]
	if err := ui.Spinner("Checking available add-ons", func() error {
		for _, c := range featureCandidates {
			if !allHaveBuilds(ctx, client, c.slugs, cfg.Loader, cfg.MCVersion) {
				continue
			}
			label := c.label
			// Crossplay lets Bedrock players in, but they can't load a Java
			// modpack's client mods, so flag it (not blocked yet) when a pack is set.
			if c.value == config.PortGeyser && cfg.Modpack != "" {
				label += " (not compatible with modpacks)"
			}
			options = append(options, huh.NewOption(label, c.value))
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
		Title("Which add-ons do you want?").
		Description("Space to toggle, enter to confirm. Only add-ons with a build for your version are shown. Each sets up its own port/proxy/firewall later.").
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
		case config.PortGeyser:
			cfg.Geyser = true
			cfg.Mods = append(cfg.Mods, catalog.SlugGeyser, catalog.SlugFloodgate)
		case "quackedsmp":
			cfg.Mods = append(cfg.Mods, catalog.SlugQuackedSMP)
			if err := askQuackedSMPSubPrompts(cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

// allHaveBuilds reports whether every slug has a build for the loader+MC, so a
// feature that needs more than one mod is offered only when all of them install.
func allHaveBuilds(ctx context.Context, client modrinth.Client, slugs []string, loader, mc string) bool {
	for _, slug := range slugs {
		if !catalog.HasBuild(ctx, client, slug, loader, mc) {
			return false
		}
	}
	return true
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

// askRAM takes the JVM heap as two values: the starting heap (-Xms) and the
// maximum heap (-Xmx), which together form a range. Defaults are recommended by
// whether the server is modded, and clamped so the suggestion always fits the box.
func askRAM(cfg *config.Config) error {
	total := system.TotalMemoryGB()
	modded := cfg.Loader != config.LoaderVanilla
	minDef, maxDef := heapDefaults(modded, total)

	minGB, err := askHeap(
		"Starting heap: RAM reserved at launch (-Xms), in GB",
		"How much RAM the server grabs at startup and always holds. Keep it below the maximum so an idle server doesn't tie up memory, handy when you run several on one box.",
		minDef, heapValidator(1, total))
	if err != nil {
		return err
	}

	maxGB, err := askHeap(
		"Maximum heap: the ceiling it can grow to (-Xmx), in GB",
		"The most RAM this server may use. Add ~1GB per 5 players; heavy modpacks want 8-12GB. For a big pack or high player counts, set this equal to the starting heap.",
		maxDef, heapValidator(minGB, total))
	if err != nil {
		return err
	}

	cfg.HeapMinGB, cfg.HeapMaxGB = minGB, maxGB
	return nil
}

// askHeap is the shared GB-input prompt for the two heap values, mirroring
// askPort: pre-filled with the suggested default, parsed after validation.
func askHeap(title, why string, def int, validate func(string) error) (int, error) {
	value := strconv.Itoa(def)
	field := huh.NewInput().
		Title(title).
		Description(why).
		Value(&value).
		Validate(validate)
	if err := field.Run(); err != nil {
		return 0, err
	}
	return mustAtoi(value), nil
}

// heapDefaults suggests the (min, max) heap in GB: a light range for a vanilla
// server, a larger one for a modded server (matching the reference box's 2G/6G).
// The max is clamped down when the box is small so the default always fits, and
// the min never exceeds the max.
func heapDefaults(modded bool, total int) (minGB, maxGB int) {
	minGB, maxGB = 1, 3
	if modded {
		minGB, maxGB = 2, 6
	}
	if total > 0 {
		ceiling := total - 1
		if ceiling < 1 {
			ceiling = 1
		}
		if maxGB > ceiling {
			maxGB = ceiling
		}
		if minGB > maxGB {
			minGB = maxGB
		}
	}
	return minGB, maxGB
}

// heapValidator rejects a heap below floor (1 for -Xms, the chosen -Xms for
// -Xmx), and one larger than the box can honor while leaving ~1GB for the OS.
func heapValidator(floor, total int) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < floor {
			if floor <= 1 {
				return fmt.Errorf("enter a whole number of at least 1")
			}
			return fmt.Errorf("enter a whole number of at least %d (the starting heap)", floor)
		}
		if total > 0 && n > total-1 {
			return fmt.Errorf("this box has only ~%dGB, leave at least 1GB for the OS (max %dGB)", total, total-1)
		}
		return nil
	}
}
