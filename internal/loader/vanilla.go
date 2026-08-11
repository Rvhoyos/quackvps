package loader

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/mojang"
)

// vanilla installs the plain Mojang server jar — no mods, no installer.
type vanilla struct{ javaPath string }

func (vanilla) Name() string { return config.LoaderVanilla }

func (vanilla) InstallServer(ctx context.Context, dir, mcVersion string) error {
	url, err := mojang.ServerJarURL(ctx, mcVersion)
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, "server.jar")
	if err := dl.Download(ctx, url, dest); err != nil {
		return fmt.Errorf("download vanilla server: %w", err)
	}
	return nil
}

func (v vanilla) RunScript(dir string, ramGB int) (string, error) {
	return inlineRunScript(v.javaPath, "server.jar", ramGB), nil
}
