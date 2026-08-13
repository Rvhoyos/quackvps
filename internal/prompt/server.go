package prompt

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/picker"
	"github.com/rvhoyos/quackvps/internal/restore"
	"github.com/rvhoyos/quackvps/internal/system"
)

// askServer picks the parent container, then either takes a name for a new server
// or, if the chosen name already holds one, offers to update it in place. It sets
// Mode, Parent, Instance, and Dir. pickerStart is where the tree browser opens.
func askServer(ctx context.Context, cfg *config.Config, pickerStart string) error {
	parent, err := picker.PickParent(ctx, pickerStart)
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

	if system.InstanceExists(parent, name) {
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

// resolveExisting handles the branch when the folder already holds a server:
// update it in place, restore a world backup, or cancel so the user can pick a
// different name. Never clobbers.
func resolveExisting(cfg *config.Config) error {
	choice := "update"
	field := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s already has a server. What now?", cfg.Dir)).
		Description("Update keeps your world and upgrades the loader/mods. Restore rolls the world back to a saved backup. Cancel lets you re-run and choose a different name; we never overwrite an existing server.").
		Options(
			huh.NewOption("Update it in place", "update"),
			huh.NewOption("Restore a world backup", "restore"),
			huh.NewOption("Cancel", "cancel"),
		).
		Value(&choice)
	if err := field.Run(); err != nil {
		return err
	}
	switch choice {
	case "cancel":
		return fmt.Errorf("cancelled: re-run and choose a different name for a new server")
	case "restore":
		cfg.Mode = config.ModeRestore
	default:
		cfg.Mode = config.ModeUpdate
	}
	return nil
}

// askBackup lists the instance's world backups and lets the user pick one to
// restore, newest first. It only gathers the choice into cfg.Backup; the restore
// itself runs later. No backups means there's nothing to restore, reported clearly.
func askBackup(cfg *config.Config) error {
	backups, err := restore.ListBackups(cfg.Dir)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no world backups found in %s", filepath.Join(cfg.Dir, "backups"))
	}

	options := make([]huh.Option[string], len(backups))
	for i, b := range backups {
		options[i] = huh.NewOption(b.Label, b.Path)
	}

	choice := backups[0].Path
	field := huh.NewSelect[string]().
		Title("Which backup should we restore?").
		Description("Your current world is moved aside first, so a bad restore can be undone. The server restarts on the world from the backup you pick.").
		Options(options...).
		Value(&choice)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.Backup = choice
	return nil
}
