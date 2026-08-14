// Package packtest is the boot-test harness behind the modpack CI. It picks the
// day's curated pack and the versions it can build, and boots a single
// pack+version to confirm it reaches server-ready. It composes the loader,
// modrinth, and minecraft seams but never the system or caddy layer, so it runs
// on a plain CI runner with no systemd, ufw, or reverse proxy.
package packtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rvhoyos/quackvps/internal/catalog"
	"github.com/rvhoyos/quackvps/internal/java"
	"github.com/rvhoyos/quackvps/internal/loader"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/modrinth"
)

// The boot test launches with the modded heap default, which fits a standard 16GB
// CI runner. A pack that can't start in this much is a real failure to surface.
const (
	heapMinGB = 2
	heapMaxGB = 6
)

// Version pairs a buildable MC version with the Java major it needs — one entry
// of the workflow matrix.
type Version struct {
	MC   string `json:"mc"`
	Java int    `json:"java"`
}

// Matrix is one day's boot-test plan: a single curated pack and every MC version
// it has a build for, each tagged with its Java major.
type Matrix struct {
	Loader  string    `json:"loader"`
	Slug    string    `json:"slug"`
	Entries []Version `json:"matrix"`
}

// SelectMatrix picks the pack to test for day (an index into the stable
// AllFeatured order, wrapping) and resolves the versions it can actually build,
// each with the Java major that version needs. One pack per day keeps a run far
// under GitHub's 256-job matrix cap.
func SelectMatrix(ctx context.Context, client modrinth.Client, day int) (Matrix, error) {
	packs := catalog.AllFeatured()
	if len(packs) == 0 {
		return Matrix{}, fmt.Errorf("no curated packs")
	}
	pack := selectPack(packs, day)
	if pack.Disabled {
		// A parked pack holds its slot to keep the day-index stable but boots
		// nothing; an empty matrix trips the workflow's count==0 guard, which
		// skips the boot job for the day.
		return Matrix{Loader: pack.Loader, Slug: pack.Slug, Entries: []Version{}}, nil
	}

	versions, err := loader.SupportedVersions(ctx, pack.Loader)
	if err != nil {
		return Matrix{}, fmt.Errorf("list %s versions: %w", pack.Loader, err)
	}

	// Start Entries non-nil so an empty result marshals to [] rather than null,
	// keeping the JSON the workflow parses well-formed even for a pack with no builds.
	m := Matrix{Loader: pack.Loader, Slug: pack.Slug, Entries: []Version{}}
	for _, v := range versions {
		if !catalog.HasBuild(ctx, client, pack.Slug, pack.Loader, v) {
			continue
		}
		major, err := java.Required(ctx, v)
		if err != nil {
			return Matrix{}, fmt.Errorf("java for %s: %w", v, err)
		}
		m.Entries = append(m.Entries, Version{MC: v, Java: major})
	}
	return m, nil
}

// FailureReason returns a one-line explanation for a failed boot, pulled from the
// server log the run left in dir, falling back to the run error's own first line
// when the log has nothing recognizable. It's what the CI puts in the report's
// reason cell so a red row is actionable without opening the job log.
func FailureReason(dir string, runErr error) string {
	log, _ := os.ReadFile(filepath.Join(dir, "logs", "latest.log"))
	if reasons := minecraft.FailureReasons(string(log)); len(reasons) > 0 {
		// One reason per line in the cell (Markdown renders <br> as a break), so
		// several missing mods read as a list, not a run-on.
		return strings.Join(reasons, "<br>")
	}
	if runErr != nil {
		return firstLine(runErr.Error())
	}
	return "failed"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// selectPack maps a day to a pack by indexing into the fixed list, wrapping so
// any day (including a negative one) lands on a real pack and the cycle repeats.
func selectPack(packs []catalog.Pack, day int) catalog.Pack {
	return packs[((day%len(packs))+len(packs))%len(packs)]
}

// Run installs a loader + modpack into dir and boots it once to confirm the pack
// reaches server-ready. It's the per-job worker of the boot-test matrix: no
// systemd, ufw, or caddy, just the headless install seams plus a plain-process
// boot. A non-nil error means the pack failed to boot, and the CI job goes red.
func Run(ctx context.Context, loaderName, mc, modpack, dir, javaPath string, timeout time.Duration) error {
	l, err := loader.For(loaderName, javaPath)
	if err != nil {
		return err
	}
	if err := l.InstallServer(ctx, dir, mc); err != nil {
		return fmt.Errorf("install %s server: %w", loaderName, err)
	}

	if modpack != "" {
		mp, err := modrinth.New().ResolveMrpack(ctx, modpack, mc, loaderName)
		if err != nil {
			return fmt.Errorf("resolve modpack %s: %w", modpack, err)
		}
		if err := mp.Install(ctx, dir); err != nil {
			return fmt.Errorf("install modpack %s: %w", modpack, err)
		}
	}

	body, err := l.RunScript(dir, heapMinGB, heapMaxGB)
	if err != nil {
		return err
	}
	if err := minecraft.WriteRunScript(dir, body); err != nil {
		return err
	}

	// The universal EULA path, same as the installer: a throwaway boot writes
	// eula.txt, we accept it, then the real boot below is what we actually measure.
	if err := minecraft.FirstRunGenerate(ctx, dir); err != nil {
		return err
	}
	if err := minecraft.AcceptEULA(dir); err != nil {
		return err
	}
	return minecraft.BootUntilReady(ctx, dir, timeout)
}
