package loader

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/dl"
)

// neoforge runs NeoForge's installer, which lays down a libraries/ tree and its
// own argfiles. We launch through those argfiles and keep RAM in
// user_jvm_args.txt, the way NeoForge expects.
type neoforge struct{ javaPath string }

func (neoforge) Name() string { return config.LoaderNeoForge }

func (n neoforge) InstallServer(ctx context.Context, dir, mcVersion string) error {
	version, err := neoforgeVersion(ctx, mcVersion)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
		version, version)
	jar := filepath.Join(dir, "neoforge-installer.jar")
	if err := dl.Download(ctx, url, jar); err != nil {
		return fmt.Errorf("download neoforge installer: %w", err)
	}
	if err := runJavaJar(ctx, n.javaPath, "neoforge-installer.jar", dir, "--installServer"); err != nil {
		return fmt.Errorf("run neoforge installer: %w", err)
	}
	return nil
}

// RunScript writes the heap range into user_jvm_args.txt (NeoForge's argfile) and
// pins the JDK in the run.sh the installer generated.
func (n neoforge) RunScript(dir string, minGB, maxGB int) (string, error) {
	return argfileRunScript(dir, n.javaPath, minGB, maxGB)
}

// neoforgeVersion picks the newest NeoForge build for a Minecraft version.
// NeoForge numbers its builds by the MC version with the leading "1." dropped:
// MC 1.21.8 → 21.8.x, MC 26.1.2 (calendar) → 26.1.x.
func neoforgeVersion(ctx context.Context, mcVersion string) (string, error) {
	prefix, err := neoforgePrefix(mcVersion)
	if err != nil {
		return "", err
	}
	var list struct {
		Versions []string `json:"versions"`
	}
	const url = "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge"
	if err := dl.GetJSON(ctx, url, &list); err != nil {
		return "", fmt.Errorf("fetch neoforge versions: %w", err)
	}
	var best string
	for _, v := range list.Versions {
		if strings.HasPrefix(v, prefix) {
			best = v // list is oldest-first, so the last match is newest
		}
	}
	if best == "" {
		return "", fmt.Errorf("no NeoForge build for Minecraft %s (looked for %s*)", mcVersion, prefix)
	}
	return best, nil
}

// neoforgePrefix maps an MC version to the NeoForge version prefix, e.g.
// "1.21.8" → "21.8.", "1.21" → "21.0.", "26.1.2" → "26.1.".
func neoforgePrefix(mcVersion string) (string, error) {
	parts := strings.Split(mcVersion, ".")
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return "", fmt.Errorf("unsupported minecraft version %q", mcVersion)
		}
	}
	switch {
	case parts[0] == "1" && len(parts) >= 3:
		return parts[1] + "." + parts[2] + ".", nil
	case parts[0] == "1" && len(parts) == 2:
		return parts[1] + ".0.", nil
	case parts[0] != "1" && len(parts) >= 2:
		return parts[0] + "." + parts[1] + ".", nil
	default:
		return "", fmt.Errorf("cannot derive NeoForge version from %q", mcVersion)
	}
}

// neoforgeBuildToMC converts a NeoForge build number to its canonical Minecraft
// version, the inverse of neoforgePrefix. Legacy builds carry the MC minor+patch
// after an implied "1." with a trailing build number ("21.8.54" → "1.21.8");
// calendar builds carry the full MC version ("26.1.2.30-beta" → "26.1.2"). A
// trailing ".0" is dropped because Minecraft omits it, build "21.0.167" is MC
// "1.21", not "1.21.0" (which Modrinth/Mojang don't recognise). Returns "" if
// unparseable.
func neoforgeBuildToMC(build string) string {
	parts := strings.Split(build, ".")
	if len(parts) < 2 {
		return ""
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}

	var mc string
	if major >= 26 { // calendar: MC = the version components verbatim
		mc = parts[0] + "." + parts[1]
		if len(parts) >= 3 {
			if _, err := strconv.Atoi(parts[2]); err != nil {
				return ""
			}
			mc += "." + parts[2]
		}
	} else { // legacy: MC = 1.<major>.<minor>, dropping the build number
		mc = "1." + parts[0] + "." + parts[1]
	}
	return strings.TrimSuffix(mc, ".0")
}
