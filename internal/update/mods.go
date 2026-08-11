package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// identifyMods hashes every jar in mods/ and asks Modrinth for the newest build
// of each compatible with the target version. This works on ANY server because it
// reads the jars on disk, never our own records. Jars Modrinth doesn't recognise
// are returned as "unknown" for the user to migrate by hand.
//
// resolved maps the old jar's filename to its upgraded version; unknown lists old
// filenames with no Modrinth match.
func identifyMods(ctx context.Context, cfg *config.Config, client modrinth.Client) (resolved map[string]modrinth.Version, unknown []string, err error) {
	jars, err := listJars(filepath.Join(cfg.Dir, "mods"))
	if err != nil {
		return nil, nil, err
	}

	hashToJar := map[string]string{}
	var hashes []string
	for _, jar := range jars {
		sum, err := dl.SHA512File(jar)
		if err != nil {
			return nil, nil, err
		}
		hashToJar[sum] = filepath.Base(jar)
		hashes = append(hashes, sum)
	}
	if len(hashes) == 0 {
		return map[string]modrinth.Version{}, nil, nil
	}

	updates, err := client.LatestForHashes(ctx, hashes, cfg.Loader, cfg.MCVersion)
	if err != nil {
		return nil, nil, err
	}

	resolved = map[string]modrinth.Version{}
	for _, sum := range hashes {
		jar := hashToJar[sum]
		if v, ok := updates[sum]; ok {
			resolved[jar] = v
		} else {
			unknown = append(unknown, jar)
		}
	}
	return resolved, unknown, nil
}

// listJars returns the .jar files directly inside a mods directory. A missing
// directory is not an error — it just means nothing to carry over.
func listJars(modsDir string) ([]string, error) {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", modsDir, err)
	}
	var jars []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jar") {
			jars = append(jars, filepath.Join(modsDir, e.Name()))
		}
	}
	return jars, nil
}

// wipeMods empties the mods/ directory before the upgraded builds are downloaded.
func wipeMods(dir string) error {
	modsDir := filepath.Join(dir, "mods")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", modsDir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jar") {
			if err := os.Remove(filepath.Join(modsDir, e.Name())); err != nil {
				return fmt.Errorf("remove %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

// redownloadMods fetches each resolved upgraded build into mods/.
func redownloadMods(ctx context.Context, dir string, resolved map[string]modrinth.Version) error {
	for _, v := range resolved {
		file, ok := v.Primary()
		if !ok {
			continue
		}
		dest := filepath.Join(dir, "mods", file.Filename)
		if err := dl.DownloadVerify(ctx, file.URL, dest, file.Hashes.SHA512); err != nil {
			return fmt.Errorf("download %s: %w", file.Filename, err)
		}
	}
	return nil
}

func reportModPlan(resolved map[string]modrinth.Version, unknown []string) {
	ui.Info("%d mod(s) will be upgraded; %d could not be identified.", len(resolved), len(unknown))
	if len(unknown) > 0 {
		ui.Warn("Not on Modrinth — migrate these by hand if you still need them:")
		ui.Bullet(unknown...)
	}
}

func reportDone(resolved map[string]modrinth.Version, unknown []string) {
	ui.Step("Update complete")
	ui.Success("%d mod(s) upgraded.", len(resolved))
	if len(unknown) > 0 {
		ui.Warn("%d mod(s) were not carried over (not on Modrinth): %s", len(unknown), strings.Join(unknown, ", "))
	}
}
