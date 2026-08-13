// Package catalog assembles the mod and modpack choices the wizard offers. The
// modpack list is a small, hand-curated set per loader, not a live Modrinth
// search, which is dominated by low-quality "optimization" packs that game the
// metadata. Each curated pack is checked live for a build matching the chosen
// loader + MC version; ones without a match are dropped so only installable
// packs are offered.
package catalog

import (
	"context"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/modrinth"
)

// Modrinth slugs of the projects we pin.
const (
	SlugQuackedSMP  = "quackedsmp"
	SlugQuackedPack = "quackedsmppack"

	// Slugs for the add-on features that are standalone Modrinth mods (unlike the
	// dashboard/votifier, which are part of the quackedsmp mod). Selecting these
	// features means we must also download the mod itself.
	SlugBlueMap   = "bluemap"
	SlugVoiceChat = "simple-voice-chat"
)

// ModpackOffer is a curated modpack the user can install for the chosen loader +
// MC version. Only installable ones are offered.
type ModpackOffer struct {
	Slug  string
	Title string
}

// featuredPack is a curated catalog entry before the version check.
type featuredPack struct {
	slug, title string
}

// featured returns the hand-curated modpacks for a loader, best first (by
// Modrinth follows). The list spans MC versions; Modpacks live-checks each so a
// version only ever shows the packs it can actually run. Vanilla has none (a
// modpack needs a loader). Quilt reuses the Fabric list, since Quilt runs Fabric
// mods.
func featured(loader string) []featuredPack {
	switch loader {
	case config.LoaderNeoForge:
		return []featuredPack{
			{SlugQuackedPack, "Play Quacked SMP"},
			{"the-pixelmon-modpack", "The Pixelmon Modpack"},
			{"create_plus", "Create+"},
			{"cobblemon-neoforge", "Cobblemon Official Modpack"},
			{"cobblemon-x-creating", "Cobblemon x Create"},
			{"better-mc-neoforge-bmc5", "Better MC (BMC5)"},
			{"blockfront-mod-pack", "BlockFront"},
			{"farming-experience", "Farming Experience"},
			{"keralis-create-pack", "Keralis Create Pack"},
			{"create-complete-by-shalz", "Create: Complete"},
			{"create.ultimate", "Create Ultimate"},
			{"create-oneblock", "Create: OneBlock"},
			{"battlearmorytacz", "BattleArmory TACZ"},
			{"reminiscent-create", "Reminiscent Create"},
			{"old-school-minecraft", "Old School Minecraft"},
			{"terrafirmacraftmodpack", "TerraFirmaCraft Enhanced"},
		}
	case config.LoaderForge:
		return []featuredPack{
			{"the-pixelmon-modpack", "The Pixelmon Modpack"},
			{"create-live-5", "Create Live 5"},
			{"better-mc-forge-bmc4", "Better MC (BMC4)"},
			{"create_plus", "Create+"},
			{"the-lost-era", "The Lost Era"},
			{"medieval-mc-forge-mmc4", "Medieval MC (MMC4)"},
			{"dwellers-modpack", "The Dwellers"},
			{"prehistoric-world-modpack", "Prehistoric World"},
			{"parasites-reloaded", "Parasites: Reloaded"},
			{"cave-horror-project-modpack", "Cave Horror Project"},
			{"alaskan-wilderness", "Alaskan Wilderness"},
			{"mc-rebalanced", "MC Rebalanced"},
			{"technical-electrical", "Technical Electrical"},
			{"reminiscence", "Reminiscence"},
			{"osmp", "Origins SMP"},
			{"slimes-adventure", "Slimes Adventure"},
		}
	case config.LoaderFabric, config.LoaderQuilt:
		return []featuredPack{
			{"cobblemon-fabric", "Cobblemon Official Modpack"},
			{"cobbleverse", "COBBLEVERSE"},
			{"aged", "Aged"},
			{"prominence-2-fabric", "Prominence II"},
			{"better-mc-fabric-bmc2", "Better MC (BMC2)"},
			{"homestead", "Homestead"},
			{"harpy-express", "The Last Voyage of the Harpy"},
			{"realisticcraft", "RealisticCraft"},
			{"landscapes-reimagined-genesis", "Landscapes Reimagined"},
			{"sensible-modpack", "Sensible MC"},
			{"ardacraft", "ArdaCraft"},
			{"elysium-days", "Elysium Days"},
			{"better-adventures++", "Better Adventures++"},
			{"jonathans-cobblemon-pack", "Cobblemon Expanded"},
		}
	default:
		return nil
	}
}

// SupportsModpacks reports whether a loader can run modpacks at all. Only Vanilla
// can't (a modpack needs a loader), so the wizard skips the whole modpack step for
// it while still offering manual slug entry on every real loader.
func SupportsModpacks(loader string) bool {
	return len(featured(loader)) > 0
}

// Modpacks returns the curated modpack offers that have a build for the chosen
// loader + MC version. Incompatible ones are dropped rather than shown greyed
// with a 15-pack list per loader, greying most of them is noise; the user only
// wants the ones they can install. A slug that errors (delisted, network) is
// skipped, never fatal, so one bad entry can't break the whole screen.
func Modpacks(ctx context.Context, c modrinth.Client, loader, mc string) ([]ModpackOffer, error) {
	offers := make([]ModpackOffer, 0)
	for _, p := range featured(loader) {
		if HasBuild(ctx, c, p.slug, loader, mc) {
			offers = append(offers, ModpackOffer{Slug: p.slug, Title: p.title})
		}
	}
	return offers, nil
}

// HasBuild reports whether a project has a version for the loader + MC version.
// Errors are treated as "no build" so a single delisted or unreachable slug is
// skipped instead of failing the screen. Used both for modpack filtering and for
// gating add-on mods (e.g. QuackedSMP isn't offered where it has no build).
func HasBuild(ctx context.Context, c modrinth.Client, slug, loader, mc string) bool {
	versions, err := c.Versions(ctx, slug, []string{loader}, []string{mc})
	if err != nil {
		return false
	}
	return len(versions) > 0
}
