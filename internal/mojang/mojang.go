// Package mojang is the single point of access to Mojang's version metadata: the
// version manifest and the per-version JSON. It backs the JDK version gate, the
// vanilla server-jar download, and the wizard's version list, so those three
// callers don't each re-implement the same fetch.
package mojang

import (
	"context"
	"fmt"

	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/mcver"
)

const manifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type manifest struct {
	Versions []entry `json:"versions"`
}

type entry struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "release" | "snapshot" | ...
	URL  string `json:"url"`
}

// versionMeta is the slice of a per-version JSON we care about.
type versionMeta struct {
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
	Downloads struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
}

// ReleaseVersions returns released Minecraft versions at or above min, newest
// first — the list the wizard offers so users pick rather than type a version.
func ReleaseVersions(ctx context.Context, min string) ([]string, error) {
	m, err := fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	floor, err := mcver.Parse(min)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, e := range m.Versions {
		if e.Type != "release" {
			continue
		}
		v, err := mcver.Parse(e.ID)
		if err != nil {
			continue // skip anything that isn't a plain release number
		}
		if mcver.AtLeast(v, floor) {
			out = append(out, e.ID)
		}
	}
	return out, nil
}

// JavaMajor returns the Java major a version requires, from its own metadata.
func JavaMajor(ctx context.Context, version string) (int, error) {
	meta, err := fetchVersionMeta(ctx, version)
	if err != nil {
		return 0, err
	}
	if meta.JavaVersion.MajorVersion == 0 {
		return 0, fmt.Errorf("no javaVersion in %s metadata", version)
	}
	return meta.JavaVersion.MajorVersion, nil
}

// ServerJarURL returns the vanilla server jar download URL for a version.
func ServerJarURL(ctx context.Context, version string) (string, error) {
	meta, err := fetchVersionMeta(ctx, version)
	if err != nil {
		return "", err
	}
	if meta.Downloads.Server.URL == "" {
		return "", fmt.Errorf("minecraft %s has no server download", version)
	}
	return meta.Downloads.Server.URL, nil
}

func fetchManifest(ctx context.Context) (*manifest, error) {
	var m manifest
	if err := dl.GetJSON(ctx, manifestURL, &m); err != nil {
		return nil, fmt.Errorf("fetch version manifest: %w", err)
	}
	return &m, nil
}

func fetchVersionMeta(ctx context.Context, version string) (*versionMeta, error) {
	m, err := fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	var metaURL string
	for _, e := range m.Versions {
		if e.ID == version {
			metaURL = e.URL
			break
		}
	}
	if metaURL == "" {
		return nil, fmt.Errorf("minecraft %s not found in Mojang manifest", version)
	}
	var meta versionMeta
	if err := dl.GetJSON(ctx, metaURL, &meta); err != nil {
		return nil, fmt.Errorf("fetch %s metadata: %w", version, err)
	}
	return &meta, nil
}
