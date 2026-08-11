package loader

import (
	"context"
	"fmt"
	"sort"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/mcver"
	"github.com/rvhoyos/quackvps/internal/mojang"
)

// forgeNeoSplit is the Minecraft version where the modding community split: 1.20.x
// and older is the Forge era, 1.21+ is NeoForge. We enforce it as a clean, non-
// overlapping divide so each loader only offers versions where its ecosystem
// (mods + modpacks) actually lives, even though both technically publish builds
// across the line.
const forgeNeoSplit = "1.21"

// SupportedVersions returns the Minecraft releases a loader can run, newest first,
// filtered to that loader's supported range so the wizard never offers a version
// with no real ecosystem (e.g. Forge at 26.2, or NeoForge at 1.20.1).
func SupportedVersions(ctx context.Context, name string) ([]string, error) {
	var versions []string
	var err error
	switch name {
	case config.LoaderFabric, config.LoaderQuilt:
		versions, err = fabricGameVersions(ctx) // Quilt targets the same MC set
	case config.LoaderNeoForge:
		versions, err = neoforgeGameVersions(ctx)
	case config.LoaderForge:
		versions, err = forgeGameVersions(ctx)
	case config.LoaderVanilla:
		versions, err = mojang.ReleaseVersions(ctx, config.MinMCVersion)
	default:
		return nil, fmt.Errorf("unknown loader %q", name)
	}
	if err != nil {
		return nil, err
	}
	return filterRange(name, versions), nil
}

func fabricGameVersions(ctx context.Context) ([]string, error) {
	var entries []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := dl.GetJSON(ctx, "https://meta.fabricmc.net/v2/versions/game", &entries); err != nil {
		return nil, fmt.Errorf("fetch fabric game versions: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.Stable {
			out = append(out, e.Version)
		}
	}
	return out, nil
}

// neoforgeGameVersions derives the distinct MC versions NeoForge builds for from
// its maven version list (build "21.8.x" → MC "1.21.8").
func neoforgeGameVersions(ctx context.Context) ([]string, error) {
	var list struct {
		Versions []string `json:"versions"`
	}
	const url = "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge"
	if err := dl.GetJSON(ctx, url, &list); err != nil {
		return nil, fmt.Errorf("fetch neoforge versions: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range list.Versions {
		mc := neoforgeBuildToMC(v)
		if mc != "" && !seen[mc] {
			seen[mc] = true
			out = append(out, mc)
		}
	}
	return out, nil
}

// forgeGameVersions returns the distinct MC versions Forge promotes, from the
// keys of the promotions map ("1.20.1-recommended" → "1.20.1").
func forgeGameVersions(ctx context.Context) ([]string, error) {
	promos, err := forgePromotions(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for key := range promos {
		mc := stripPromoSuffix(key)
		if mc != "" && !seen[mc] {
			seen[mc] = true
			out = append(out, mc)
		}
	}
	return out, nil
}

// filterRange keeps parseable releases within the loader's supported range,
// sorted newest first. All loaders honour the global floor (MinMCVersion); Forge
// and NeoForge additionally split at forgeNeoSplit so they never overlap.
func filterRange(loader string, versions []string) []string {
	floor, _ := mcver.Parse(config.MinMCVersion)
	split, _ := mcver.Parse(forgeNeoSplit)

	inRange := func(v mcver.Version) bool {
		if !mcver.AtLeast(v, floor) {
			return false
		}
		switch loader {
		case config.LoaderForge:
			return !mcver.AtLeast(v, split) // strictly below 1.21 (the 1.20.x era)
		case config.LoaderNeoForge:
			return mcver.AtLeast(v, split) // 1.21 and up
		default:
			return true
		}
	}

	parsed := make([]mcver.Version, 0, len(versions))
	for _, s := range versions {
		v, err := mcver.Parse(s)
		if err != nil {
			continue
		}
		if inRange(v) {
			parsed = append(parsed, v)
		}
	}
	sort.Slice(parsed, func(i, j int) bool { return mcver.Compare(parsed[i], parsed[j]) > 0 })

	out := make([]string, len(parsed))
	for i, v := range parsed {
		out[i] = v.String()
	}
	return out
}

// stripPromoSuffix turns a Forge promo key into its MC version, or "" if it isn't
// a "<mc>-recommended|latest" key.
func stripPromoSuffix(key string) string {
	for _, suffix := range []string{"-recommended", "-latest"} {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			return key[:len(key)-len(suffix)]
		}
	}
	return ""
}
