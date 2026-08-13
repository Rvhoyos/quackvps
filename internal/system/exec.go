// Package system wraps the host-level operations the installer performs on a
// Debian/Ubuntu box: capability detection, apt, systemd, screen, ufw, sshd
// hardening, and port scanning. Every function is a thin, explicit shell over a
// real command, it holds no policy and never prompts.
package system

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a command and discards its output, returning a wrapped error that
// includes the command's own stderr so failures are diagnosable. Sibling packages
// (caddy, install, update) use it so all shell-outs share one error format.
func Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Capture executes a command and returns its stdout.
func Capture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// succeeds reports whether a command exits zero, ignoring its output. Useful for
// yes/no probes like "is this unit active".
func succeeds(ctx context.Context, name string, args ...string) bool {
	return exec.CommandContext(ctx, name, args...).Run() == nil
}

// HasCommand reports whether an executable is on PATH.
func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
