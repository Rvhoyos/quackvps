package minecraft

import (
	"context"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// TakeOffline brings an existing instance down so its files can be worked on,
// and reports the service that manages it plus the account it runs as. Update,
// restore, and adding mods all start this way: create the service first if the
// user asked us to adopt a hand-made server, then stop it and wait for the world
// to finish saving.
//
// The owner comes from the server itself rather than from whoever invoked us,
// because an instance we didn't install may well run as its own account, and
// everything we write afterwards has to end up belonging to it.
func TakeOffline(ctx context.Context, cfg *config.Config) (system.Unit, string, error) {
	if cfg.AdoptUnit {
		ui.Step("Creating a service for this server")
		if err := Adopt(ctx, cfg.Instance, cfg.Dir); err != nil {
			return system.Unit{}, "", err
		}
		ui.Success("%s now manages %s.", cfg.Unit, cfg.Dir)
	}

	unit, err := system.ShowUnit(ctx, cfg.Unit)
	if err != nil {
		return system.Unit{}, "", err
	}
	owner, err := system.InstanceOwner(unit, cfg.Dir)
	if err != nil {
		return system.Unit{}, "", err
	}

	ui.Step("Stopping the server")
	if err := system.StopAndWait(ctx, unit.Name, owner, system.ScreenSession(unit.ExecStart), system.DefaultStopWait); err != nil {
		return system.Unit{}, "", err
	}
	return unit, owner, nil
}
