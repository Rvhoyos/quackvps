package minecraft

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// firstRunTimeout caps the throwaway boot. Without an accepted EULA the server
// generates its files and exits in seconds; the timeout only guards against a
// build that behaves unexpectedly, so we never hang the install.
const firstRunTimeout = 3 * time.Minute

// FirstRunKind classifies why the throwaway first-run boot produced no eula.txt,
// so the caller (which knows the pack, loader, and heap) can turn it into an
// actionable message.
type FirstRunKind int

const (
	FirstRunCrash   FirstRunKind = iota // the server exited on its own, failing to load
	FirstRunOOM                         // the kernel killed it, out of memory
	FirstRunTimeout                     // it never finished within firstRunTimeout
)

// FirstRunError is returned when the first-run boot doesn't generate eula.txt.
// Kind says why; Tail is the last lines of the server's own output, for display.
type FirstRunError struct {
	Kind FirstRunKind
	Tail string
}

func (e *FirstRunError) Error() string {
	switch e.Kind {
	case FirstRunOOM:
		return "the server ran out of memory during first-launch prep"
	case FirstRunTimeout:
		return "the server didn't finish its first run in time"
	default:
		return "the server crashed while loading"
	}
}

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
	// the generated file, not the exit code, but we keep the output so a genuine
	// failure (e.g. the JVM running out of memory) isn't invisible.
	out, _ := cmd.CombinedOutput()

	if _, err := os.Stat(filepath.Join(dir, "eula.txt")); err != nil {
		return &FirstRunError{Kind: classifyFirstRun(runCtx, cmd), Tail: firstRunDiagnostics(dir, out)}
	}
	return nil
}

// classifyFirstRun works out why the boot produced no eula.txt. A context deadline
// means it never finished; a SIGKILL with the context still live means the kernel
// killed it (out of memory, an OOM-kill leaves no JVM crash dump); anything else is
// the server exiting on its own because it failed to load.
func classifyFirstRun(runCtx context.Context, cmd *exec.Cmd) FirstRunKind {
	if runCtx.Err() == context.DeadlineExceeded {
		return FirstRunTimeout
	}
	if cmd.ProcessState != nil {
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL {
			return FirstRunOOM
		}
	}
	return FirstRunCrash
}

// firstRunDiagnostics builds a helpful failure message from the server's own
// output plus any JVM crash dump it left behind. When the log contains a root
// cause ("Caused by:") it's lifted to the front, since a stack trace usually
// pushes the real reason above the last handful of lines.
func firstRunDiagnostics(dir string, out []byte) string {
	msg := tailLines(string(out), 15)
	if cause := rootCause(string(out)); cause != "" && !strings.Contains(msg, cause) {
		msg = cause + "\n…\n" + msg
	}
	if crash, _ := filepath.Glob(filepath.Join(dir, "hs_err_pid*.log")); len(crash) > 0 {
		msg += fmt.Sprintf("\n(JVM crash dump: %s, often means too much RAM was requested for this box)", crash[0])
	}
	return msg
}

// rootCause returns the first "Caused by:" line in the server output, the line
// that usually names why a modpack refused to load. Empty when there's none.
func rootCause(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "Caused by:") {
			return strings.TrimSpace(line)
		}
	}
	return ""
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

// bootReadyMarker is the line a Minecraft server logs once it has finished
// loading and is accepting players. Reaching it is the definition of a clean boot.
const bootReadyMarker = "Done ("

// BootUntilReady boots the server via run.sh and waits until it logs that it's
// ready, then stops it. Reaching bootReadyMarker means the loader and every mod
// loaded cleanly, so this is how the boot-test CI proves a pack works. Unlike
// FirstRunGenerate, which waits for the server to exit, this waits for it to come
// up. It returns an error carrying the log tail if the server exits first or never
// gets there within timeout.
func BootUntilReady(ctx context.Context, dir string, timeout time.Duration) error {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "run.sh")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	logPath := filepath.Join(dir, "logs", "latest.log")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if logContains(logPath, bootReadyMarker) {
			cancel() // it booted; stop the server
			<-exited // and reap it
			return nil
		}
		select {
		case werr := <-exited:
			// It quit before reaching ready: a crash, an OOM kill, or the deadline.
			return fmt.Errorf("server exited before it finished starting: %v\n%s", werr, logTail(logPath))
		case <-ticker.C:
		}
	}
}

// logContains reports whether the file at path contains marker; a missing file
// (the server hasn't written its log yet) counts as not-yet.
func logContains(path, marker string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), marker)
}

// logTail returns the last lines of a server log for a failure message.
func logTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no server log found)"
	}
	return tailLines(string(data), 20)
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
