// Package modrinth is the client for the Modrinth v2 API (no key required):
// resolving mod and modpack versions, parsing .mrpack files, and — for updates —
// identifying existing jars by content hash so the tool works on any server, not
// just ones it installed itself.
package modrinth

import "context"

const baseURL = "https://api.modrinth.com/v2"

// Client is the seam the rest of the tool builds against. New returns the
// concrete *HTTPClient; consumers accept this interface so they can be tested
// against a fake.
type Client interface {
	// Versions returns a project's versions filtered to the given loaders and
	// game versions, newest first.
	Versions(ctx context.Context, slug string, loaders, gameVersions []string) ([]Version, error)

	// ResolveMrpack fetches and parses the .mrpack matching a modpack slug for
	// the chosen loader + MC version.
	ResolveMrpack(ctx context.Context, slug, mc, loader string) (*Mrpack, error)

	// IdentifyByHash maps each SHA-512 to the Modrinth version that file belongs
	// to. Hashes with no match are simply absent from the result.
	IdentifyByHash(ctx context.Context, sha512s []string) (map[string]Version, error)

	// LatestForHashes maps each SHA-512 to the newest version compatible with the
	// target loader + MC version.
	LatestForHashes(ctx context.Context, sha512s []string, loader, mc string) (map[string]Version, error)
}

// HTTPClient talks to the real Modrinth API.
type HTTPClient struct {
	baseURL string
}

// New returns a client pointed at the public Modrinth API.
func New() *HTTPClient { return &HTTPClient{baseURL: baseURL} }

var _ Client = (*HTTPClient)(nil)

// Version is a single published version of a project.
type Version struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	VersionType  string   `json:"version_type"`
	GameVersions []string `json:"game_versions"`
	Loaders      []string `json:"loaders"`
	Files        []File   `json:"files"`
	Dependencies []Dep    `json:"dependencies"`
}

// File is a downloadable artifact within a version.
type File struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Primary  bool   `json:"primary"`
	Hashes   struct {
		SHA1   string `json:"sha1"`
		SHA512 string `json:"sha512"`
	} `json:"hashes"`
}

// Primary returns the version's primary file, or the first file if none is
// flagged primary. ok is false when the version has no files.
func (v Version) Primary() (File, bool) {
	for _, f := range v.Files {
		if f.Primary {
			return f, true
		}
	}
	if len(v.Files) > 0 {
		return v.Files[0], true
	}
	return File{}, false
}

// Dep is a dependency link to another project or version.
type Dep struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Type      string `json:"dependency_type"` // required|optional|incompatible|embedded
}
