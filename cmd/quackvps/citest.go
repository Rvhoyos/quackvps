package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rvhoyos/quackvps/internal/modrinth"
	"github.com/rvhoyos/quackvps/internal/packtest"
)

// isCISubcommand reports whether the first argument selects one of the hidden CI
// subcommands the modpack boot-test workflow drives. They're deliberately absent
// from --help: they're for CI, not end users.
func isCISubcommand(name string) bool {
	return name == "ci-pack" || name == "bootcheck"
}

// runCISubcommand dispatches the hidden CI commands. They run before the normal
// flag parse and skip preflight (the root/OS/tool checks), since they run on a
// plain CI runner, not the target VPS.
func runCISubcommand(name string, args []string) error {
	switch name {
	case "ci-pack":
		return runCIPack(args)
	case "bootcheck":
		return runBootcheck(args)
	default:
		return fmt.Errorf("unknown command %q", name)
	}
}

// runCIPack emits, as JSON, the curated pack to boot-test for a given day and the
// versions it can build. The workflow's select job turns this into the matrix.
func runCIPack(args []string) error {
	fs := flag.NewFlagSet("ci-pack", flag.ContinueOnError)
	day := fs.Int("day", 0, "day index selecting the pack (e.g. day-of-year)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := packtest.SelectMatrix(context.Background(), modrinth.New(), *day)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(m)
}

// runBootcheck installs and boots one pack+version, returning an error (non-zero
// exit) if it fails to come up. It's the per-job worker of the boot-test matrix.
func runBootcheck(args []string) error {
	fs := flag.NewFlagSet("bootcheck", flag.ContinueOnError)
	loaderName := fs.String("loader", "", "mod loader")
	mc := fs.String("mc", "", "Minecraft version")
	modpack := fs.String("modpack", "", "Modrinth modpack slug")
	dir := fs.String("dir", "", "install directory")
	javaPath := fs.String("java", "java", "path to the java executable")
	timeout := fs.Duration("timeout", 15*time.Minute, "how long to wait for the server to boot")
	reasonFile := fs.String("reason-file", "", "on failure, write the one-line reason here for the CI report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *loaderName == "" || *mc == "" || *dir == "" {
		return fmt.Errorf("bootcheck needs --loader, --mc and --dir")
	}
	err := packtest.Run(context.Background(), *loaderName, *mc, *modpack, *dir, *javaPath, *timeout)
	if err != nil && *reasonFile != "" {
		// Best-effort: the reason is a report nicety, never a reason to mask the failure.
		_ = os.WriteFile(*reasonFile, []byte(packtest.FailureReason(*dir, err)), 0o644)
	}
	return err
}
