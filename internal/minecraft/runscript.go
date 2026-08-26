package minecraft

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/java"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/system"
	"github.com/rvhoyos/quackvps/internal/ui"
)

// Defaults used only if the existing heap range can't be read from disk.
const (
	defaultMinGB = 1
	defaultMaxGB = 4
)

var (
	xmsRE = regexp.MustCompile(`-Xms(\d+)G`)
	xmxRE = regexp.MustCompile(`-Xmx(\d+)G`)
)

// WriteLaunchScript gives a server the run.sh the systemd unit calls, for the
// flows that meet one without it: a server set up by hand starts some other way,
// so adopting it under systemd means writing the launch script first. It's the
// same script install generates, keeping the heap the server already runs with.
func WriteLaunchScript(ctx context.Context, cfg *config.Config) error {
	// Read the heap before the installer runs: it rewrites user_jvm_args.txt with
	// its own defaults, and this server's range is the one to keep.
	minGB, maxGB := ReadHeap(cfg.Dir)

	major, err := java.Required(ctx, cfg.MCVersion)
	if err != nil {
		return err
	}
	javaPath, err := java.Ensure(ctx, major)
	if err != nil {
		return err
	}
	l, err := loader.For(cfg.Loader, javaPath)
	if err != nil {
		return err
	}

	// Forge and NeoForge launch through argfiles the installer writes under
	// libraries/, and we only ever edit the script it generates rather than
	// reconstruct that path, so the installer has to run for there to be one. It's
	// the same in-place reinstall an update does: it lays down the loader next to
	// the world, mods and configs it never touches.
	if err := ui.Spinner("Reinstalling "+cfg.Loader+" for "+cfg.MCVersion, func() error {
		return l.InstallServer(ctx, cfg.Dir, cfg.MCVersion)
	}); err != nil {
		return err
	}

	body, err := l.RunScript(cfg.Dir, minGB, maxGB)
	if err != nil {
		return err
	}
	if err := WriteRunScript(cfg.Dir, body); err != nil {
		return err
	}

	// The installer runs as us, so what it wrote belongs to root; the server runs
	// as the account that owns its folder.
	owner, err := system.InstanceOwner(system.Unit{}, cfg.Dir)
	if err != nil {
		return err
	}
	return system.ChownRecursive(cfg.Dir, owner)
}

// ReadHeap recovers the heap range (GB) from an existing server so a rewritten
// run.sh keeps it rather than silently resetting to a default. NeoForge/Forge
// store it in user_jvm_args.txt; the other loaders inline it in run.sh, we check
// both. A file with an -Xmx but no -Xms mirrors the max onto the min.
func ReadHeap(dir string) (minGB, maxGB int) {
	for _, name := range []string{"user_jvm_args.txt", runScript} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		s := string(data)
		hi, ok := matchGB(xmxRE, s)
		if !ok {
			continue // no ceiling here; try the next file
		}
		lo := hi
		if v, ok := matchGB(xmsRE, s); ok {
			lo = v
		}
		if lo > hi {
			lo = hi
		}
		return lo, hi
	}
	return defaultMinGB, defaultMaxGB
}

// matchGB pulls the first whole-GB value captured by re from s.
func matchGB(re *regexp.Regexp, s string) (int, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	gb, err := strconv.Atoi(m[1])
	if err != nil || gb <= 0 {
		return 0, false
	}
	return gb, true
}
