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
	return versions, nil
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
