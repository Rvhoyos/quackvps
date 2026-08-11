package minecraft

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// firstRunTimeout caps the throwaway boot. Without an accepted EULA the server
// generates its files and exits in seconds; the timeout only guards against a
// build that behaves unexpectedly, so we never hang the install.
const firstRunTimeout = 3 * time.Minute

// WriteRunScript writes run.sh (executable) with the given body.
func WriteRunScript(dir, body string) error {
	path := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return fmt.Errorf("write run.sh: %w", err)
	}
	return nil
}

// FirstRunGenerate boots the server once via run.sh so it writes eula.txt and
// server.properties, then returns. With the EULA not yet accepted the server
// exits on its own; if it somehow keeps running we stop it at the timeout. This
// is the loader-agnostic way to get the config files generated.
func FirstRunGenerate(ctx context.Context, dir string) error {
	runCtx, cancel := context.WithTimeout(ctx, firstRunTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "run.sh")
	cmd.Dir = dir
	// A non-zero exit (EULA not accepted) is the expected outcome, so we verify by
	// the generated file, not the exit code — but we keep the output so a genuine
	// failure (e.g. the JVM running out of memory) isn't invisible.
	out, _ := cmd.CombinedOutput()

	if _, err := os.Stat(filepath.Join(dir, "eula.txt")); err != nil {
		return fmt.Errorf("server did not generate eula.txt on first run:\n%s", firstRunDiagnostics(dir, out))
	}
	return nil
}

// firstRunDiagnostics builds a helpful failure message from the server's own
// output plus any JVM crash dump it left behind.
func firstRunDiagnostics(dir string, out []byte) string {
	msg := tailLines(string(out), 15)
	if crash, _ := filepath.Glob(filepath.Join(dir, "hs_err_pid*.log")); len(crash) > 0 {
		msg += fmt.Sprintf("\n(JVM crash dump: %s — often means too much RAM was requested for this box)", crash[0])
	}
	return msg
}

// tailLines returns the last n non-empty lines of s.
func tailLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// AcceptEULA sets eula=true in eula.txt, recording agreement to Mojang's EULA.
func AcceptEULA(dir string) error {
	path := filepath.Join(dir, "eula.txt")
	contents := "# Set by quackvps after the user accepted the Minecraft EULA.\neula=true\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write eula.txt: %w", err)
	}
	return nil
}

// SetServerPort writes the chosen game port into server.properties.
func SetServerPort(dir string, port int) error {
	return SetProp(filepath.Join(dir, "server.properties"), "server-port", strconv.Itoa(port))
}
