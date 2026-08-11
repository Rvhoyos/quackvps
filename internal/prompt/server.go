package prompt

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/picker"
)

// askServer picks the parent container, then either takes a name for a new server
// or, if the chosen name already holds one, offers to update it in place. It sets
// Mode, Parent, Instance, and Dir.
func askServer(ctx context.Context, cfg *config.Config) error {
	parent, err := picker.PickParent(ctx, cfg.RunAsHome)
	if err != nil {
		return err
	}
	cfg.Parent = parent

	name, err := askInstanceName(parent)
	if err != nil {
		return err
	}
	cfg.Instance = name
	cfg.ResolveDir()

	if isExistingInstance(parent, name) {
		return resolveExisting(cfg)
	}
	cfg.Mode = config.ModeInstall
	return nil
}

// askInstanceName takes the server's name, defaulting to nothing and validating
// it's a single folder name.
func askInstanceName(parent string) (string, error) {
	var name string
	field := huh.NewInput().
		Title("Name this server").
		Description(fmt.Sprintf("A short name like 'survival'. It becomes the folder %s/<name>, the service, and the screen session.", parent)).
		Placeholder("survival").
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("name cannot be empty")
			}
			if filepath.Base(s) != s || s == "." || s == ".." {
				return fmt.Errorf("use a plain name, not a path")
			}
			return nil
		}).
		Value(&name)
	if err := field.Run(); err != nil {
		return "", err
	}
	return name, nil
}

// resolveExisting handles the two-choice branch when the folder already holds a
// server: update it in place, or abort so the user can pick a different name.
// Never clobbers.
func resolveExisting(cfg *config.Config) error {
	choice := "update"
	field := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s already has a server. What now?", cfg.Dir)).
		Description("Update keeps your world and upgrades the loader/mods. Cancel lets you re-run and choose a different name — we never overwrite an existing server.").
		Options(
			huh.NewOption("Update it in place", "update"),
			huh.NewOption("Cancel", "cancel"),
		).
		Value(&choice)
	if err := field.Run(); err != nil {
		return err
	}
	if choice == "cancel" {
		return fmt.Errorf("cancelled: re-run and choose a different name for a new server")
	}
	cfg.Mode = config.ModeUpdate
	return nil
}
