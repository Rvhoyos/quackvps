// Package remove takes one server off the box. It is the mirror image of install,
// and reaches exactly as far as one instance: the service that runs it, the
// firewall ports it opened, its reverse-proxy entry, and its folder. Caddy, Java,
// ufw itself and the ports other servers share are never touched, so removing one
// server on a box that hosts three leaves the other two running.
//
// What goes is decided before this package is reached (the wizard or the flags fill
// RemoveInfra/RemoveFiles); nothing here prompts.
package remove

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rvhoyos/quackvps/internal/caddy"
	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/web"
)

// killGrace is how long a server started outside systemd gets to shut down after
// SIGTERM. It matches the unit's own TimeoutStopSec, since it's the same work:
// saving the world before the JVM exits.
const killGrace = 120 * time.Second

// Run removes the server described by cfg (already validated).
//
// Taking the server offline is the one fatal step: everything else deletes things,
// and deleting files under a live server means it writes them back. After that the
// steps are independent, so a failure is collected and reported rather than
// abandoning the run half-done; the run still exits non-zero.
func Run(ctx context.Context, cfg *config.Config) error {
	if err := takeDown(ctx, cfg); err != nil {
		return err
	}

	var done, kept, problems []string
	fail := func(what string, err error) {
		ui.Error("%v", err)
		problems = append(problems, what)
	}

	if cfg.RemoveInfra {
		// Read the ports before anything is deleted: they live in the configs the
		// folder step is about to take away.
		closed, shared := portsToClose(ctx, cfg)
		for _, p := range closed {
			if err := system.Firewall.Delete(ctx, p.rule); err != nil {
				fail("firewall rule "+p.rule.String(), err)
				continue
			}
			done = append(done, fmt.Sprintf("closed %s (%s)", p.rule, p.label))
		}
		for _, p := range shared {
			kept = append(kept, fmt.Sprintf("%s left open, %s uses it too", p.rule, p.owner))
		}

		removed, err := removeCaddy(ctx, cfg.Instance)
		switch {
		case err != nil:
			fail("the Caddy entry", err)
		case removed:
			done = append(done, "removed its reverse-proxy entry")
		}

		if cfg.Unit != "" {
			if err := removeUnit(ctx, cfg); err != nil {
				fail(cfg.Unit, err)
			} else if cfg.RemoveUnitFile {
				done = append(done, "removed "+cfg.Unit)
			} else {
				done = append(done, "stopped and disabled "+cfg.Unit)
				kept = append(kept, cfg.Unit+" is still on disk, you wrote it")
			}
		}
	}

	if cfg.RemoveFiles {
		if err := os.RemoveAll(cfg.Dir); err != nil {
			fail(cfg.Dir, err)
		} else {
			done = append(done, "deleted "+cfg.Dir)
		}
	} else {
		kept = append(kept, cfg.Dir+" and everything in it")
		if cfg.RemoveInfra && cfg.Unit != "" {
			kept = append(kept, "start it again by hand, its service is gone")
		}
	}

	report(cfg, done, kept)
	if len(problems) > 0 {
		return fmt.Errorf("could not remove: %s", strings.Join(problems, ", "))
	}
	return nil
}

// takeDown stops the server before a single file is touched. A managed server gets
// systemd's own graceful stop, which saves the world; one started by hand answers
// to no unit, so it's signalled directly. run.sh and the JVM it launched are two
// processes, so this repeats until the folder has none left.
func takeDown(ctx context.Context, cfg *config.Config) error {
	if cfg.Unit != "" {
		unit, err := system.ShowUnit(ctx, cfg.Unit)
		if err != nil {
			return err
		}
		ui.Step("Stopping the server")
		// The user here is only for the screen-session check, so the unit's own
		// account is enough and a folder that's already gone doesn't matter.
		if err := system.StopAndWait(ctx, unit.Name, unit.User, system.ScreenSession(unit.ExecStart), system.DefaultStopWait); err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		pid, cmdline, running := system.RunningIn(cfg.Dir)
		if !running {
			return nil
		}
		ui.Warn("Still running in %s: pid %d (%s). Shutting it down before removing anything.", cfg.Dir, pid, cmdline)
		if err := system.StopProcess(ctx, pid, killGrace); err != nil {
			return err
		}
	}
	return fmt.Errorf("something keeps starting a server in %s, stop it and then run this again", cfg.Dir)
}

// firewallPort is one port this server has open: the ufw rule that opened it, and
// what it's for in plain language.
type firewallPort struct {
	rule  system.Rule
	label string
	owner string // the other instance holding this port, when it's shared
}

// portsToClose splits the server's own network ports into the ones we close and
// the ones we leave. A port comes from the server's own config, the same key the
// installer wrote it to, so this works on a server we set up and on one we didn't.
// Two things spare a port: another instance under the same parent using it (closing
// it would take that server off the network), and ufw not having it open in the
// first place. The Caddy-fronted ports never appear here at all, they were never in
// the firewall.
func portsToClose(ctx context.Context, cfg *config.Config) (closing, shared []firewallPort) {
	rules, err := system.Firewall.Rules(ctx)
	if err != nil {
		ui.Warn("Couldn't read the firewall's rules (%v), leaving them alone.", err)
		return nil, nil
	}
	return splitPorts(instancePorts(cfg.Dir), rules, system.SiblingPorts(cfg.Parent, cfg.Instance))
}

// instancePorts is every network port the server has configured: the game port,
// plus each add-on that owns a firewall port.
func instancePorts(dir string) []firewallPort {
	var ports []firewallPort
	if port, ok := minecraft.ServerPort(dir); ok {
		ports = append(ports, firewallPort{rule: system.Rule{Port: port, Proto: "tcp"}, label: "Minecraft"})
	}
	for _, p := range web.InstalledPorts(dir) {
		ports = append(ports, firewallPort{rule: system.Rule{Port: p.Number, Proto: p.Proto}, label: p.Label})
	}
	return ports
}

// splitPorts is the decision portsToClose makes, kept separate from the box so the
// rules are testable: open is what ufw has, siblings maps a port to the other
// instance that also uses it.
func splitPorts(ports []firewallPort, open []system.Rule, siblings map[int]string) (closing, shared []firewallPort) {
	isOpen := map[system.Rule]bool{}
	for _, r := range open {
		isOpen[r] = true
	}
	for _, p := range ports {
		if !isOpen[p.rule] {
			continue // never opened, or already closed: nothing to do
		}
		if owner, ok := siblings[p.rule.Port]; ok {
			p.owner = owner
			shared = append(shared, p)
			continue
		}
		closing = append(closing, p)
	}
	return closing, shared
}

// removeCaddy deletes the instance's site file and reloads the proxy, and reports
// whether there was one. A box with no Caddy has nothing to do here. The import
// line and the directory stay: they belong to every instance, not this one.
func removeCaddy(ctx context.Context, instance string) (bool, error) {
	if installed, _ := caddy.Detect(ctx); !installed {
		return false, nil
	}
	had, err := caddy.HasInstanceFile(instance)
	if err != nil || !had {
		return false, err
	}
	if err := caddy.RemoveInstanceFile(instance); err != nil {
		return false, err
	}
	return true, caddy.Reload(ctx)
}

// removeUnit retires the service. One we wrote goes entirely; one the user wrote is
// stopped and taken off the boot sequence but left on disk, since the file is
// theirs even though the server it started is gone.
func removeUnit(ctx context.Context, cfg *config.Config) error {
	if cfg.RemoveUnitFile {
		system.RemoveUnit(ctx, cfg.Unit)
		return nil
	}
	return system.DisableNow(ctx, cfg.Unit)
}

func report(cfg *config.Config, done, kept []string) {
	ui.Step("Removed " + cfg.Instance)
	if len(done) > 0 {
		ui.Bullet(done...)
	}
	if len(kept) > 0 {
		ui.Info("Left alone:")
		ui.Bullet(kept...)
	}
	switch {
	case cfg.RemoveInfra && cfg.RemoveFiles:
		ui.Success("%s is gone from this box.", cfg.Instance)
	case cfg.RemoveInfra:
		ui.Success("%s no longer runs on this box. Its files are still in %s.", cfg.Instance, cfg.Dir)
	default:
		ui.Success("%s's files are gone.", cfg.Instance)
	}
}
