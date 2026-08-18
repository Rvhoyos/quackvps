package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Unit is what systemd knows about one service: enough to tell whether it manages
// a given instance, and who it runs as. ExecStart is systemd's own verbose form
// ("{ path=/usr/bin/screen ; argv[]=... }"), kept raw because we only ever search
// it for a path or a screen session name.
type Unit struct {
	Name       string // full id, e.g. mc-survival.service
	WorkingDir string
	User       string // empty means the service runs as root
	ExecStart  string
}

// ListServices returns every service unit installed on the box. Servers we didn't
// install are managed by units we can't guess the name of, so update and restore
// offer this list and let the user say which one is theirs.
func ListServices(ctx context.Context) ([]Unit, error) {
	out, err := Capture(ctx, "systemctl", "list-unit-files", "--type=service", "--no-legend", "--plain")
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	names := serviceNames(out)
	if len(names) == 0 {
		return nil, nil
	}

	// One `show` for every unit: it prints a block per unit, which is far cheaper
	// than a systemctl call each.
	args := append([]string{"show", "-p", "Id", "-p", "WorkingDirectory", "-p", "User", "-p", "ExecStart"}, names...)
	shown, err := Capture(ctx, "systemctl", args...)
	if err != nil {
		return nil, fmt.Errorf("read service details: %w", err)
	}
	return parseShow(shown), nil
}

// ShowUnit reads one unit's details, how update and restore learn who a server
// runs as and which screen session it drives.
func ShowUnit(ctx context.Context, name string) (Unit, error) {
	out, err := Capture(ctx, "systemctl", "show", "-p", "Id", "-p", "WorkingDirectory", "-p", "User", "-p", "ExecStart", name)
	if err != nil {
		return Unit{}, fmt.Errorf("read unit %s: %w", name, err)
	}
	units := parseShow(out)
	if len(units) == 0 {
		return Unit{}, fmt.Errorf("systemd knows no unit named %s", name)
	}
	return units[0], nil
}

// serviceNames pulls unit names out of `systemctl list-unit-files` output,
// skipping templates (foo@.service), which are not units you can start.
func serviceNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if name == "" || strings.HasSuffix(name, "@.service") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// parseShow turns `systemctl show` output into Units. Properties come as
// key=value lines, one unit per block, blocks separated by a blank line.
func parseShow(out string) []Unit {
	var units []Unit
	var current Unit

	flush := func() {
		if current.Name != "" {
			units = append(units, current)
		}
		current = Unit{}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Id":
			current.Name = val
		case "WorkingDirectory":
			current.WorkingDir = val
		case "User":
			current.User = val
		case "ExecStart":
			current.ExecStart = val
		}
	}
	flush()
	return units
}

// UnitsForInstance narrows a service list to the ones that look like they manage
// the instance in dir, either because systemd runs them there or because their
// command line names the directory. Order is preserved so callers show them as
// found.
func UnitsForInstance(units []Unit, dir string) []Unit {
	want := filepath.Clean(dir)
	var matches []Unit
	for _, u := range units {
		if filepath.Clean(u.WorkingDir) == want || strings.Contains(u.ExecStart, want) {
			matches = append(matches, u)
		}
	}
	return matches
}

// ResolveUnit answers which unit manages an instance: defaultUnit when the box
// has it (every server we installed), otherwise "" plus every service on the box,
// which callers narrow with UnitsForInstance. The wizard turns those into a
// picker and the CLI turns them into an error, so the lookup lives in one place.
func ResolveUnit(ctx context.Context, defaultUnit string) (name string, services []Unit, err error) {
	if UnitExists(defaultUnit) {
		return defaultUnit, nil, nil
	}
	services, err = ListServices(ctx)
	if err != nil {
		return "", nil, err
	}
	return "", services, nil
}

var screenSessionRE = regexp.MustCompile(`-S\s+(\S+)`)

// ScreenSession returns the screen session name an ExecStart line starts, or ""
// when the service doesn't use screen. Updates use it as a second check that the
// server is really down; a unit that launches java directly simply has none.
func ScreenSession(execStart string) string {
	m := screenSessionRE.FindStringSubmatch(execStart)
	if m == nil {
		return ""
	}
	return m[1]
}

// RunningIn reports a Minecraft process running out of dir, found by its working
// directory rather than any unit, so we can refuse to adopt (or touch) a server
// someone started by hand. It returns the pid and its command line.
func RunningIn(dir string) (pid int, cmdline string, running bool) {
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return 0, "", false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", false
	}
	for _, e := range entries {
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a process
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || cwd != want {
			continue
		}
		cmd := processCmdline(e.Name())
		if strings.Contains(cmd, "java") || strings.Contains(cmd, "run.sh") {
			return id, cmd, true
		}
	}
	return 0, "", false
}

// processCmdline reads a process's command line, whose arguments are NUL
// separated, and returns it as a readable string.
func processCmdline(pid string) string {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
}
