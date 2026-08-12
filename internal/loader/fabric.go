package loader

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
)

// fabric installs Fabric's ready-to-run server launcher directly from the Fabric
// meta service — no separate installer step. The launcher downloads the rest on
// first boot, which the install flow triggers headlessly.
type fabric struct{ javaPath string }

func (fabric) Name() string { return config.LoaderFabric }

func (fabric) InstallServer(ctx context.Context, dir, mcVersion string) error {
	loaderVer, err := fabricLatestStable(ctx, "https://meta.fabricmc.net/v2/versions/loader/"+mcVersion)
	if err != nil {
		return fmt.Errorf("resolve fabric loader for %s: %w", mcVersion, err)
	}
	installerVer, err := fabricLatestStable(ctx, "https://meta.fabricmc.net/v2/versions/installer")
	if err != nil {
		return fmt.Errorf("resolve fabric installer: %w", err)
	}

	url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar",
		mcVersion, loaderVer, installerVer)
	dest := filepath.Join(dir, "fabric-server-launch.jar")
	if err := dl.Download(ctx, url, dest); err != nil {
		return fmt.Errorf("download fabric server launcher: %w", err)
	}
	return nil
}

func (f fabric) RunScript(dir string, minGB, maxGB int) (string, error) {
	return inlineRunScript(f.javaPath, "fabric-server-launch.jar", minGB, maxGB), nil
}

// fabricLatestStable returns the first stable version from a Fabric meta list.
// Both the loader-for-mc and installer endpoints return the same {version,
// stable} shape ordered newest-first, so the first stable entry is the pick.
func fabricLatestStable(ctx context.Context, url string) (string, error) {
	var entries []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
		// The loader-for-mc endpoint nests the version under "loader".
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := dl.GetJSON(ctx, url, &entries); err != nil {
		return "", err
	}
	for _, e := range entries {
		switch {
		case e.Loader.Version != "" && e.Loader.Stable:
			return e.Loader.Version, nil
		case e.Loader.Version == "" && e.Stable:
			return e.Version, nil
		}
	}
	return "", fmt.Errorf("no stable version at %s", url)
}
