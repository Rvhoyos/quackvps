package modrinth

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/dl"
)

// Mrpack is a parsed .mrpack: the server-relevant files to fetch plus the two
// override trees to copy over them.
type Mrpack struct {
	Files           []MrpackFile
	Overrides       map[string][]byte // path → contents from overrides/
	ServerOverrides map[string][]byte // path → contents from server-overrides/
}

// MrpackFile is one entry from modrinth.index.json.
type MrpackFile struct {
	Path      string            `json:"path"`
	Downloads []string          `json:"downloads"`
	Hashes    map[string]string `json:"hashes"` // e.g. "sha512": "..."
	Env       struct {
		Client string `json:"client"`
		Server string `json:"server"`
	} `json:"env"`
}

// serverSupported reports whether a file should be installed on a server. A pack
// that omits env entirely (Server == "") is treated as required — the common case.
func (f MrpackFile) serverSupported() bool {
	return f.Env.Server != "unsupported"
}

// ResolveMrpack finds the modpack version for the chosen loader + MC, downloads
// its .mrpack, and parses it.
func (c *HTTPClient) ResolveMrpack(ctx context.Context, slug, mc, loader string) (*Mrpack, error) {
	versions, err := c.Versions(ctx, slug, []string{loader}, []string{mc})
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("modpack %s has no build for %s %s", slug, loader, mc)
	}
	file, ok := versions[0].Primary()
	if !ok {
		return nil, fmt.Errorf("modpack %s version has no downloadable file", slug)
	}

	tmp, err := os.CreateTemp("", "quackvps-*.mrpack")
	if err != nil {
		return nil, fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := dl.DownloadVerify(ctx, file.URL, tmp.Name(), file.Hashes.SHA512); err != nil {
		return nil, err
	}
	return ParseMrpack(tmp.Name())
}

// ParseMrpack reads a .mrpack (a zip) into memory: the file index plus the two
// override trees. Kept separate from download so it's unit-testable offline.
func ParseMrpack(path string) (*Mrpack, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open mrpack: %w", err)
	}
	defer zr.Close()

	mp := &Mrpack{
		Overrides:       map[string][]byte{},
		ServerOverrides: map[string][]byte{},
	}
	indexFound := false
	for _, f := range zr.File {
		switch {
		case f.Name == "modrinth.index.json":
			if err := decodeIndex(f, mp); err != nil {
				return nil, err
			}
			indexFound = true
		case strings.HasPrefix(f.Name, "overrides/"):
			if err := readOverride(f, "overrides/", mp.Overrides); err != nil {
				return nil, err
			}
		case strings.HasPrefix(f.Name, "server-overrides/"):
			if err := readOverride(f, "server-overrides/", mp.ServerOverrides); err != nil {
				return nil, err
			}
		}
	}
	if !indexFound {
		return nil, fmt.Errorf("mrpack missing modrinth.index.json")
	}
	return mp, nil
}

// Install downloads every server-supported file to its path under dir (verifying
// sha512), then applies overrides, then server-overrides (server wins).
func (mp *Mrpack) Install(ctx context.Context, dir string) error {
	for _, f := range mp.Files {
		if !f.serverSupported() || len(f.Downloads) == 0 {
			continue
		}
		dest := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := dl.DownloadVerify(ctx, f.Downloads[0], dest, f.Hashes["sha512"]); err != nil {
			return fmt.Errorf("download %s: %w", f.Path, err)
		}
	}
	if err := writeTree(dir, mp.Overrides); err != nil {
		return err
	}
	return writeTree(dir, mp.ServerOverrides)
}

func decodeIndex(f *zip.File, mp *Mrpack) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer rc.Close()
	var index struct {
		Files []MrpackFile `json:"files"`
	}
	if err := json.NewDecoder(rc).Decode(&index); err != nil {
		return fmt.Errorf("decode index: %w", err)
	}
	mp.Files = index.Files
	return nil
}

func readOverride(f *zip.File, prefix string, into map[string][]byte) error {
	if strings.HasSuffix(f.Name, "/") {
		return nil // directory entry
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read %s: %w", f.Name, err)
	}
	into[strings.TrimPrefix(f.Name, prefix)] = data
	return nil
}

func writeTree(dir string, tree map[string][]byte) error {
	for rel, data := range tree {
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", rel, err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}
