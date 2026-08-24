// Package mods puts Modrinth mods into an instance's mods/ folder: it resolves a
// slug to the build matching the server's loader and Minecraft version, pulls the
// required dependencies along with it, and skips anything already installed.
// Install and the add-mods flow both go through here, so a mod lands the same way
// whether it was chosen during setup or added to a running server later.
package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Result records what an Install wrote, so callers can report it and undo it.
type Result struct {
	Added   []string // jar filenames written into mods/
	Skipped []string // slugs whose project was already installed
}

// Install downloads each slug's build for loader+mc into dir/mods, along with the
// mod's required dependencies. A project already present (a modpack's bundle, or
// an earlier run) is left alone rather than installed a second time at a
// different version, which would leave two conflicting jars.
func Install(ctx context.Context, client modrinth.Client, dir, loader, mc string, slugs []string) (Result, error) {
	var result Result
	if len(slugs) == 0 {
		return result, nil
	}

	installed, err := installedProjects(ctx, client, dir)
	if err != nil {
		return result, err
	}
	for _, slug := range slugs {
		if err := installOne(ctx, client, dir, loader, mc, slug, installed, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// installOne resolves one slug and downloads it with its dependencies.
func installOne(ctx context.Context, client modrinth.Client, dir, loader, mc, slug string, installed map[string]bool, result *Result) error {
	return ui.Spinner("Installing "+slug, func() error {
		versions, err := client.Versions(ctx, slug, []string{loader}, []string{mc})
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			return fmt.Errorf("%s has no build for %s %s", slug, loader, mc)
		}
		// Already present: the jar itself is left alone, but its dependencies are
		// still resolved, since the mod may need one that isn't installed yet.
		if installed[versions[0].ProjectID] {
			result.Skipped = append(result.Skipped, slug)
		}
		return downloadWithDeps(ctx, client, dir, loader, mc, versions[0], installed, result)
	})
}

// downloadWithDeps downloads v and its required dependencies, skipping any
// project already present and recording each one so later mods dedup too.
func downloadWithDeps(ctx context.Context, client modrinth.Client, dir, loader, mc string, v modrinth.Version, installed map[string]bool, result *Result) error {
	for _, want := range append([]modrinth.Version{v}, resolveDepsBestEffort(ctx, client, loader, mc, v)...) {
		if installed[want.ProjectID] {
			continue
		}
		name, fresh, err := download(ctx, dir, want)
		if err != nil {
			return err
		}
		installed[want.ProjectID] = true
		if fresh {
			result.Added = append(result.Added, name)
		}
	}
	return nil
}

func resolveDepsBestEffort(ctx context.Context, client modrinth.Client, loader, mc string, v modrinth.Version) []modrinth.Version {
	deps, err := modrinth.ResolveRequired(ctx, client, v, []string{loader}, []string{mc})
	if err != nil {
		// Dependency resolution is best-effort; a missing optional dep shouldn't
		// abort the whole install. The mod's own jar still gets downloaded.
		ui.Warn("could not resolve dependencies for %s: %v", v.ProjectID, err)
		return nil
	}
	return deps
}

// download saves a resolved version's primary jar into the instance's mods/
// directory, verifying its SHA-512. It reports the filename and whether that file
// is new: a jar already sitting there under the same name is the same build, and
// a caller undoing its own work must not delete a mod it found in place.
func download(ctx context.Context, dir string, v modrinth.Version) (name string, fresh bool, err error) {
	file, ok := v.Primary()
	if !ok {
		return "", false, fmt.Errorf("version %s has no downloadable file", v.ID)
	}
	dest := filepath.Join(dir, "mods", file.Filename)
	_, statErr := os.Stat(dest)
	if err := dl.DownloadVerify(ctx, file.URL, dest, file.Hashes.SHA512); err != nil {
		return "", false, fmt.Errorf("download %s: %w", file.Filename, err)
	}
	return file.Filename, os.IsNotExist(statErr), nil
}

// installedProjects hashes the jars already in mods/ and asks Modrinth which
// project each belongs to, so a mod dedups against what's already there.
// Best-effort: if identification fails we return an empty set (worst case, a
// duplicate jar) rather than failing the install.
func installedProjects(ctx context.Context, client modrinth.Client, dir string) (map[string]bool, error) {
	jars, err := filepath.Glob(filepath.Join(dir, "mods", "*.jar"))
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	if len(jars) == 0 {
		return ids, nil
	}

	var hashes []string
	for _, jar := range jars {
		sum, err := dl.SHA512File(jar)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, sum)
	}
	found, err := client.IdentifyByHash(ctx, hashes)
	if err != nil {
		ui.Warn("could not identify existing mods for dedup: %v", err)
		return ids, nil
	}
	for _, v := range found {
		ids[v.ProjectID] = true
	}
	return ids, nil
}
