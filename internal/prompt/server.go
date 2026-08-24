package prompt

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/picker"
	"github.com/rvhoyos/quackvps/internal/restore"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
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
		return resolveExisting(ctx, cfg)
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
		Description(fmt.Sprintf("A short name like 'survival'. It becomes the folder %s/<name>, the service, and the screen session. To update or restore an existing server, enter its name here.", parent)).
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
// update it in place, restore a world backup, add mods to it, or cancel so the
// user can pick a different name. Never clobbers.
func resolveExisting(ctx context.Context, cfg *config.Config) error {
	choice := "update"
	field := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s already has a server. What now?", cfg.Dir)).
		Description("Update keeps your world and upgrades the loader/mods. Restore rolls the world back to a saved backup. Add mods installs more mods into the server you have, world and version untouched. Cancel lets you re-run and choose a different name; we never overwrite an existing server.").
		Options(
			huh.NewOption("Update it in place", "update"),
			huh.NewOption("Restore a world backup", "restore"),
			huh.NewOption("Add mods to it", "addmods"),
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
	case "addmods":
		cfg.Mode = config.ModeAddMods
	default:
		cfg.Mode = config.ModeUpdate
	}
	// Every one of these modes stops and starts the server, so they need the
	// service that manages it before anything else is asked.
	return resolveUnit(ctx, cfg)
}

// Answers the service prompts return alongside real unit names: create one for
// this instance, or show every service on the box.
const (
	createUnit  = "\x00create-unit"
	browseUnits = "\x00browse-units"
)

// resolveUnit records which systemd service manages this instance. Servers we
// installed have mc-<instance>.service and are taken as-is; a server set up by
// hand is managed by a unit only its owner can identify, so we ask.
func resolveUnit(ctx context.Context, cfg *config.Config) error {
	name, services, err := system.ResolveUnit(ctx, minecraft.UnitName(cfg.Instance)+".service")
	if err != nil {
		return err
	}
	if name != "" {
		cfg.Unit = name
		return nil
	}
	return askUnit(cfg, services)
}

// askUnit settles which service manages this server. It offers the short answer
// first, the services that point at this folder plus "create one", because that
// covers nearly every server; the box's full service list is one step further in,
// for a unit that names neither its folder nor this instance. Picking an unrelated
// service is the one dangerous answer here, so that path is confirmed separately.
func askUnit(cfg *config.Config, services []system.Unit) error {
	matches := system.UnitsForInstance(services, cfg.Dir)
	for {
		choice, err := askManagingService(cfg.Dir, matches)
		if err != nil {
			return err
		}

		if choice == browseUnits {
			choice, err = askAnyService(cfg.Dir, services)
			if err != nil {
				return err
			}
			if choice != createUnit {
				ok, err := confirmService(cfg.Dir, choice)
				if err != nil {
					return err
				}
				if !ok {
					continue // back to the short list
				}
			}
		}

		if choice == createUnit {
			cfg.Unit = minecraft.UnitName(cfg.Instance) + ".service"
			cfg.AdoptUnit = true
			return nil
		}
		cfg.Unit = choice
		return nil
	}
}

// askManagingService is the first screen: the services that already point at this
// folder, creating one, and the way out to the full list. A server nobody set up
// as a service is the common case, so creating one leads when nothing matches.
func askManagingService(dir string, matches []system.Unit) (string, error) {
	options := make([]huh.Option[string], 0, len(matches)+2)
	for _, u := range matches {
		options = append(options, huh.NewOption(u.Name+" (runs in "+dir+")", u.Name))
	}
	options = append(options,
		huh.NewOption("Create a service for this server", createUnit),
		huh.NewOption("Let me pick from every service on this box", browseUnits),
	)

	choice := options[0].Value
	field := huh.NewSelect[string]().
		Title(fmt.Sprintf("How should we manage the server in %s?", dir)).
		Description(managingDescription(matches)).
		Options(options...).
		Value(&choice)
	return choice, field.Run()
}

func managingDescription(matches []system.Unit) string {
	const why = "We stop and start the server through systemd, so we need to know its service. Creating one is safe on a server that has none: it runs the same run.sh, as the same user, and starts again after a reboot."
	if len(matches) == 0 {
		return "No service on this box names this folder, which usually means the server has none. It can also mean its service starts the server some other way, in which case the last option lets you find it by name. " + why
	}
	return "The service listed first runs out of this folder, so it's almost certainly the one. If your server is run by a service that names the folder nowhere, the last option lists them all. " + why
}

// askAnyService lists every service on the box, for the server whose unit names
// neither its folder nor anything we can match on.
func askAnyService(dir string, services []system.Unit) (string, error) {
	options := make([]huh.Option[string], 0, len(services)+1)
	options = append(options, huh.NewOption("Go back and create a service for this server instead", createUnit))
	for _, u := range services {
		options = append(options, huh.NewOption(u.Name, u.Name))
	}

	choice := options[0].Value
	field := huh.NewSelect[string]().
		Title(fmt.Sprintf("Which service runs the server in %s?", dir)).
		Description("Every service on this box, including ones that have nothing to do with Minecraft. Pick only the one you set up to run this server.").
		Options(options...).
		Value(&choice)
	return choice, field.Run()
}

// confirmService gates a service picked off the full list, since nothing about it
// says it belongs to this server. Choosing wrong stops whatever else that service
// does on this box, so the answer defaults to no.
func confirmService(dir, name string) (bool, error) {
	answer := false
	field := huh.NewConfirm().
		Title(fmt.Sprintf("Stop and start %s for this server?", name)).
		Description(fmt.Sprintf("Nothing about %s points at %s, so we can't tell it's the right one.\n", name, dir) +
			ui.Caution("Only say yes if you set this service up to run this server. If it runs something else, we shut that down instead, and this server keeps running while we work on its files.")).
		Affirmative("Yes, that's the one").
		Negative("No, go back").
		Value(&answer)
	if err := field.Run(); err != nil {
		return false, err
	}
	return answer, nil
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
