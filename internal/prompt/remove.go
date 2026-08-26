package prompt

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/restore"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// The two halves of a removal, as the checklist returns them.
const (
	removeInfra = "infra"
	removeFiles = "files"
)

// askRemoval settles what a removal takes away, then confirms it twice. It's the
// only destructive answer the wizard collects, so nothing here is a default the
// Enter key can walk into: the checklist starts with both halves ticked because
// that's what "remove this server" means, but the confirm after it defaults to no
// and the name has to be typed out.
func askRemoval(cfg *config.Config) error {
	if err := askWhatToRemove(cfg); err != nil {
		return err
	}
	if cfg.RemoveInfra {
		if err := askUnitFile(cfg); err != nil {
			return err
		}
	}
	warnAboutBackups(cfg)

	ok, err := confirmRemoval(cfg)
	if err != nil || !ok {
		return cancelled(err)
	}
	return confirmInstanceName(cfg)
}

// askWhatToRemove is the checklist. The two halves are independent because they
// answer different questions: taking the server off the box, and throwing the
// world away.
func askWhatToRemove(cfg *config.Config) error {
	selected := []string{removeInfra, removeFiles}
	field := huh.NewMultiSelect[string]().
		Title(fmt.Sprintf("What should we remove for %s?", cfg.Instance)).
		Description(ui.Keys("Space to tick or untick, enter when the list is right.")+"\n"+
			"Keeping the folder leaves the world, the mods and the configs where they are.\n"+
			"Keeping the setup leaves the service, so the server still starts at boot.").
		Options(
			huh.NewOption("Its setup: the service, its firewall ports and its web address", removeInfra).Selected(true),
			huh.NewOption("Its folder: the world, the mods and every config", removeFiles).Selected(true),
		).
		Value(&selected)
	if err := field.Run(); err != nil {
		return err
	}

	for _, s := range selected {
		switch s {
		case removeInfra:
			cfg.RemoveInfra = true
		case removeFiles:
			cfg.RemoveFiles = true
		}
	}
	if !cfg.RemoveInfra && !cfg.RemoveFiles {
		return fmt.Errorf("nothing selected, so nothing was removed")
	}
	return nil
}

// askUnitFile settles what happens to the service file itself. One we wrote is
// ours to delete along with the server; one the user wrote is stopped and taken
// off the boot sequence either way, but the file stays unless they say otherwise.
func askUnitFile(cfg *config.Config) error {
	if cfg.Unit == "" {
		return nil
	}
	if system.OwnUnitFile(cfg.Unit) {
		cfg.RemoveUnitFile = true
		return nil
	}

	answer := false
	field := huh.NewConfirm().
		Title(fmt.Sprintf("Delete %s too?", cfg.Unit)).
		Description("We didn't write this service, so it's yours. Either way it's stopped and won't come back at boot.").
		Affirmative("Yes, delete it").
		Negative("No, just stop it").
		Value(&answer)
	if err := field.Run(); err != nil {
		return err
	}
	cfg.RemoveUnitFile = answer
	return nil
}

// warnAboutBackups says the one thing the folder line can't: the world backups
// QuackedSMP wrote are inside that folder, and nothing copies them out.
func warnAboutBackups(cfg *config.Config) {
	if !cfg.RemoveFiles {
		return
	}
	backups, err := restore.ListBackups(cfg.Dir)
	if err != nil || len(backups) == 0 {
		return
	}
	ui.Warn("%d world backup(s) live in this folder and go with it. Copy them off the box now if you want them.", len(backups))
}

// confirmRemoval is the first gate: everything that's about to go, in one list,
// answered no by default.
func confirmRemoval(cfg *config.Config) (bool, error) {
	answer := false
	field := huh.NewConfirm().
		Title(fmt.Sprintf("Remove %s?", cfg.Instance)).
		Description(ui.Caution("%s", removalSummary(cfg))).
		Affirmative("Yes, remove it").
		Negative("No, stop here").
		Value(&answer)
	if err := field.Run(); err != nil {
		return false, err
	}
	return answer, nil
}

// removalSummary lists what the answers add up to, in the order it happens.
func removalSummary(cfg *config.Config) string {
	var lines []string
	if cfg.Unit != "" {
		if cfg.RemoveInfra && cfg.RemoveUnitFile {
			lines = append(lines, "stop and delete "+cfg.Unit)
		} else if cfg.RemoveInfra {
			lines = append(lines, "stop and disable "+cfg.Unit+", keeping the file")
		} else {
			lines = append(lines, "stop "+cfg.Unit)
		}
	}
	if cfg.RemoveInfra {
		lines = append(lines, "close its firewall ports, except any another server uses")
		lines = append(lines, "drop its web address from the proxy")
	}
	if cfg.RemoveFiles {
		lines = append(lines, "delete "+cfg.Dir+", world included")
	} else {
		lines = append(lines, "keep "+cfg.Dir)
	}
	return "This will " + strings.Join(lines, ", then ") + ". There is no undo."
}

// confirmInstanceName is the last gate. Typing the name is deliberately the only
// way through: no sequence of Enters reaches a deleted world, and getting it wrong
// cancels rather than asking again.
func confirmInstanceName(cfg *config.Config) error {
	var typed string
	field := huh.NewInput().
		Title(fmt.Sprintf("Type %s to confirm", cfg.Instance)).
		Description("Last stop. Anything else cancels and nothing is touched.").
		Value(&typed)
	if err := field.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(typed) != cfg.Instance {
		return fmt.Errorf("that isn't %q, so nothing was removed", cfg.Instance)
	}
	return nil
}

// cancelled turns a "no" at a confirm into the same clear stop as an error.
func cancelled(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("cancelled, nothing was removed")
}
