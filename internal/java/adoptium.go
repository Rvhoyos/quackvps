package java

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/rvhoyos/quackvps/internal/system"
)

const adoptiumList = "/etc/apt/sources.list.d/adoptium.list"

// ensureAdoptiumRepo configures Adoptium's apt repository (the source of Temurin
// JDKs) once. It's idempotent: if the source list already exists we do nothing.
func ensureAdoptiumRepo(ctx context.Context) error {
	if _, err := os.Stat(adoptiumList); err == nil {
		return nil
	}
	if err := system.AptInstall(ctx, "wget", "gpg", "apt-transport-https"); err != nil {
		return err
	}
	if err := installAdoptiumKey(ctx); err != nil {
		return err
	}
	return writeAdoptiumSource()
}

// codename reads the distribution codename (e.g. "noble", "bookworm") that the
// Adoptium apt line is keyed on.
func codename() (string, error) {
	out, err := exec.Command("bash", "-c", ". /etc/os-release && echo $VERSION_CODENAME").Output()
	if err != nil {
		return "", fmt.Errorf("read distro codename: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("empty distro codename")
	}
	return name, nil
}

func installAdoptiumKey(ctx context.Context) error {
	const keyURL = "https://packages.adoptium.net/artifactory/api/gpg/key/public"
	const keyPath = "/etc/apt/keyrings/adoptium.asc"
	if err := os.MkdirAll("/etc/apt/keyrings", 0o755); err != nil {
		return fmt.Errorf("create keyrings dir: %w", err)
	}
	cmd := exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf("wget -qO- %q > %q", keyURL, keyPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fetch Adoptium key: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeAdoptiumSource() error {
	code, err := codename()
	if err != nil {
		return err
	}
	line := fmt.Sprintf(
		"deb [signed-by=/etc/apt/keyrings/adoptium.asc] https://packages.adoptium.net/artifactory/deb %s main\n",
		code)
	if err := os.WriteFile(adoptiumList, []byte(line), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", adoptiumList, err)
	}
	return nil
}

// jvmDirRE matches Temurin/OpenJDK directory names and captures the major, e.g.
// "temurin-21-jdk-amd64", "java-21-openjdk-amd64", "jdk-21.0.3+9".
var jvmDirRE = regexp.MustCompile(`(?:temurin-|jdk-|java-)(\d+)`)

// majorFromDirName extracts the Java major from a /usr/lib/jvm directory name.
func majorFromDirName(name string) (int, bool) {
	m := jvmDirRE.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return major, true
}
