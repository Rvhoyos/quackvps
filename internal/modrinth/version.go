package modrinth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/rvhoyos/quackvps/internal/dl"
)

// Versions returns a project's versions filtered by loader and game version.
func (c *HTTPClient) Versions(ctx context.Context, slug string, loaders, gameVersions []string) ([]Version, error) {
	q := url.Values{}
	if len(loaders) > 0 {
		q.Set("loaders", jsonArray(loaders))
	}
	if len(gameVersions) > 0 {
		q.Set("game_versions", jsonArray(gameVersions))
	}
	endpoint := fmt.Sprintf("%s/project/%s/version?%s", c.baseURL, url.PathEscape(slug), q.Encode())

	var versions []Version
	if err := dl.GetJSON(ctx, endpoint, &versions); err != nil {
		return nil, fmt.Errorf("list versions for %s: %w", slug, err)
	}
	return releasesOnly(slug, versions), nil
}

// prereleaseChannels are projects whose stable channel on Modrinth is "beta", not
// "release". Geyser only ever ships beta builds, that's its normal release model,
// not an unstable snapshot, so filtering to release would hide it entirely. Its
// config schema is stable across those builds, so keeping them is safe. (The slug
// is a literal because modrinth can't import catalog; it mirrors catalog.SlugGeyser.)
var prereleaseChannels = map[string]bool{
	"geyser": true,
}

// releasesOnly drops beta and alpha builds. A mod's version_type is author-set,
// and a pre-release often carries a config schema the stable editors don't
// understand (e.g. QuackedSMP's beta builds below 1.21.11 lack the dashboard key),
// so offering them means installing something that can't be configured. Modrinth's
// version endpoint has no version_type filter, so we do it here. Projects in
// prereleaseChannels are exempt, their beta channel is their stable one.
func releasesOnly(slug string, versions []Version) []Version {
	if prereleaseChannels[slug] {
		return versions
	}
	kept := versions[:0]
	for _, v := range versions {
		if v.VersionType == "release" {
			kept = append(kept, v)
		}
	}
	return kept
}

// IdentifyByHash asks Modrinth which version each SHA-512 belongs to
// (POST /version_files). Unknown hashes are absent from the map.
func (c *HTTPClient) IdentifyByHash(ctx context.Context, sha512s []string) (map[string]Version, error) {
	body := map[string]any{"hashes": sha512s, "algorithm": "sha512"}
	result := map[string]Version{}
	if err := dl.PostJSON(ctx, c.baseURL+"/version_files", body, &result); err != nil {
		return nil, fmt.Errorf("identify by hash: %w", err)
	}
	return result, nil
}

// LatestForHashes returns, per SHA-512, the newest version compatible with the
// target loader + MC version (POST /version_files/update).
func (c *HTTPClient) LatestForHashes(ctx context.Context, sha512s []string, loader, mc string) (map[string]Version, error) {
	body := map[string]any{
		"hashes":        sha512s,
		"algorithm":     "sha512",
		"loaders":       []string{loader},
		"game_versions": []string{mc},
	}
	result := map[string]Version{}
	if err := dl.PostJSON(ctx, c.baseURL+"/version_files/update", body, &result); err != nil {
		return nil, fmt.Errorf("resolve latest by hash: %w", err)
	}
	return result, nil
}

// jsonArray renders a string slice as a JSON array literal, the shape Modrinth's
// list-valued query params expect, e.g. ["fabric"].
func jsonArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = `"` + s + `"`
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
