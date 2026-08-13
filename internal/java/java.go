// Package java picks and provisions the JDK a given Minecraft version needs.
// The required major comes from Mojang's own per-version metadata (with a small
// static fallback), and missing majors are installed side-by-side via Temurin
// the system default JDK is never downgraded or replaced, because different
// instances may need different majors and mod loaders are picky about theirs.
package java

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/rvhoyos/quackvps/internal/mcver"
	"github.com/rvhoyos/quackvps/internal/mojang"
	"github.com/rvhoyos/quackvps/internal/system"
)

const jvmDir = "/usr/lib/jvm"

// Required returns the Java major version a Minecraft release needs. It reads
// Mojang's authoritative javaVersion.majorVersion so a future bump needs no code
// change; if the network lookup fails it falls back to the verified table.
func Required(ctx context.Context, mcVersion string) (int, error) {
	if major, err := mojang.JavaMajor(ctx, mcVersion); err == nil {
		return major, nil
	}
	return requiredFromTable(mcVersion)
}

// requiredFromTable is the offline fallback used only when Mojang's metadata
// can't be fetched: Java 25 for the calendar-versioned 26.1+, Java 21 for
// 1.20.5–1.21.x, Java 17 for 1.18–1.20.4. Below 1.18 is out of scope.
func requiredFromTable(mcVersion string) (int, error) {
	v, err := mcver.Parse(mcVersion)
	if err != nil {
		return 0, err
	}
	calendar, _ := mcver.Parse("26.1")
	java21, _ := mcver.Parse("1.20.5")
	java17, _ := mcver.Parse("1.18")
	switch {
	case mcver.AtLeast(v, calendar):
		return 25, nil
	case mcver.AtLeast(v, java21):
		return 21, nil
	case mcver.AtLeast(v, java17):
		return 17, nil
	default:
		return 0, fmt.Errorf("minecraft %s needs Java 16 or older, which is out of scope", mcVersion)
	}
}

// Ensure returns the path to a `java` binary of the required major, installing
// Temurin side-by-side if that major isn't already present.
func Ensure(ctx context.Context, major int) (string, error) {
	installed, err := Installed()
	if err != nil {
		return "", err
	}
	if path, ok := installed[major]; ok {
		return path, nil
	}
	if err := installTemurin(ctx, major); err != nil {
		return "", err
	}
	installed, err = Installed()
	if err != nil {
		return "", err
	}
	path, ok := installed[major]
	if !ok {
		return "", fmt.Errorf("installed Java %d but could not find its binary under %s", major, jvmDir)
	}
	return path, nil
}

// Installed maps each Java major found under /usr/lib/jvm to its `bin/java` path.
// Pinning the systemd unit to an explicit path (not the `java` on PATH) is what
// lets instances run on different majors without touching the box default.
func Installed() (map[int]string, error) {
	entries, err := os.ReadDir(jvmDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", jvmDir, err)
	}

	// Sort so that when several directories expose the same major, we always
	// pick the same one deterministically.
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)

	found := map[int]string{}
	for _, name := range names {
		major, ok := majorFromDirName(name)
		if !ok {
			continue
		}
		if _, taken := found[major]; taken {
			continue
		}
		bin := filepath.Join(jvmDir, name, "bin", "java")
		if _, err := os.Stat(bin); err == nil {
			found[major] = bin
		}
	}
	return found, nil
}

func installTemurin(ctx context.Context, major int) error {
	if err := ensureAdoptiumRepo(ctx); err != nil {
		return err
	}
	pkg := fmt.Sprintf("temurin-%d-jdk", major)
	if err := system.AptInstall(ctx, pkg); err != nil {
		return fmt.Errorf("install %s: %w", pkg, err)
	}
	return nil
}
