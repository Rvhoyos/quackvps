package loader

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
)

// forge runs Forge's installer, which — like NeoForge, from which it forked —
// lays down a libraries/ tree and argfiles. Forge is offered for the 1.20.1 era,
// whose big modpack library is Forge-based. 1.17+ Forge uses the argfile launcher
// (our floor is 1.20.1), so the launch shape matches NeoForge exactly.
type forge struct{ javaPath string }

func (forge) Name() string { return config.LoaderForge }

func (f forge) InstallServer(ctx context.Context, dir, mcVersion string) error {
	version, err := forgeVersion(ctx, mcVersion)
	if err != nil {
		return err
	}
	full := mcVersion + "-" + version // e.g. 1.20.1-47.4.10
	url := fmt.Sprintf(
		"https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar",
		full, full)
	jar := filepath.Join(dir, "forge-installer.jar")
	if err := dl.Download(ctx, url, jar); err != nil {
		return fmt.Errorf("download forge installer: %w", err)
	}
	if err := runJavaJar(ctx, f.javaPath, "forge-installer.jar", dir, "--installServer"); err != nil {
		return fmt.Errorf("run forge installer: %w", err)
	}
	return nil
}

// RunScript writes the heap range into user_jvm_args.txt and pins the JDK in the
// run.sh the installer generated.
func (f forge) RunScript(dir string, minGB, maxGB int) (string, error) {
	return argfileRunScript(dir, f.javaPath, minGB, maxGB)
}

// forgeVersion resolves the Forge build for a Minecraft version from Forge's
// promotions, preferring the recommended build and falling back to the latest.
func forgeVersion(ctx context.Context, mcVersion string) (string, error) {
	promos, err := forgePromotions(ctx)
	if err != nil {
		return "", err
	}
	if v, ok := promos[mcVersion+"-recommended"]; ok {
		return v, nil
	}
	if v, ok := promos[mcVersion+"-latest"]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no Forge build for Minecraft %s", mcVersion)
}

const forgePromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"

// forgePromotions returns Forge's promo map, keyed like "1.20.1-recommended".
func forgePromotions(ctx context.Context) (map[string]string, error) {
	var resp struct {
		Promos map[string]string `json:"promos"`
	}
	if err := dl.GetJSON(ctx, forgePromotionsURL, &resp); err != nil {
		return nil, fmt.Errorf("fetch forge promotions: %w", err)
	}
	return resp.Promos, nil
}
