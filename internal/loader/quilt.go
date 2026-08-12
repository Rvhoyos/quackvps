package loader

import (
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
)

// quilt runs the Quilt installer headlessly to produce a server launch jar.
type quilt struct{ javaPath string }

func (quilt) Name() string { return config.LoaderQuilt }

func (q quilt) InstallServer(ctx context.Context, dir, mcVersion string) error {
	installerVer, err := quiltInstallerVersion(ctx)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(
		"https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/%s/quilt-installer-%s.jar",
		installerVer, installerVer)
	jar := filepath.Join(dir, "quilt-installer.jar")
	if err := dl.Download(ctx, url, jar); err != nil {
		return fmt.Errorf("download quilt installer: %w", err)
	}
	// Downloads the Minecraft server and produces quilt-server-launch.jar. Without
	// --install-dir the installer drops everything in a ./server subfolder, away
	// from mods/ and the run.sh that launches quilt-server-launch.jar from the root.
	if err := runJavaJar(ctx, q.javaPath, "quilt-installer.jar", dir,
		"install", "server", mcVersion, "--download-server", "--install-dir="+dir); err != nil {
		return fmt.Errorf("run quilt installer: %w", err)
	}
	return nil
}

func (q quilt) RunScript(dir string, minGB, maxGB int) (string, error) {
	return inlineRunScript(q.javaPath, "quilt-server-launch.jar", minGB, maxGB), nil
}

// quiltInstallerVersion returns the released installer version from Quilt's maven
// metadata.
func quiltInstallerVersion(ctx context.Context) (string, error) {
	const metaURL = "https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/maven-metadata.xml"
	data, err := dl.GetBytes(ctx, metaURL)
	if err != nil {
		return "", fmt.Errorf("fetch quilt installer metadata: %w", err)
	}
	var meta struct {
		Versioning struct {
			Release string `xml:"release"`
		} `xml:"versioning"`
	}
	if err := xml.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse quilt installer metadata: %w", err)
	}
	if meta.Versioning.Release == "" {
		return "", fmt.Errorf("no released quilt installer version")
	}
	return meta.Versioning.Release, nil
}
