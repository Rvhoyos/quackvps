// Package loader installs a Minecraft server for a given mod loader and produces
// the run.sh that launches it. One small interface, five implementations
// (Fabric, Quilt, NeoForge, Forge, Vanilla). Each install is headless; RAM is
// applied the way each loader expects — inline java flags for most, a
// user_jvm_args.txt argfile for NeoForge and Forge.
package loader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rvhoyos/quackvps/internal/config"
)

// Loader installs and launches one mod loader's server.
type Loader interface {
	// Name is the loader's config identifier (e.g. "fabric").
	Name() string

	// InstallServer provisions a server for mcVersion into dir, running whatever
	// headless installer the loader needs.
	InstallServer(ctx context.Context, dir, mcVersion string) error

	// RunScript returns the body of run.sh for the given heap range (min = -Xms,
	// max = -Xmx, in GB). NeoForge and Forge also write their user_jvm_args.txt
	// into dir as a side effect.
	RunScript(dir string, minGB, maxGB int) (string, error)
}

// For returns the loader implementation for a config loader name. javaPath is the
// pinned JDK the installers and the generated run.sh should use, so an instance
// always launches on the Java major its Minecraft version needs.
func For(name, javaPath string) (Loader, error) {
	switch name {
	case config.LoaderFabric:
		return fabric{javaPath}, nil
	case config.LoaderQuilt:
		return quilt{javaPath}, nil
	case config.LoaderNeoForge:
		return neoforge{javaPath}, nil
	case config.LoaderForge:
		return forge{javaPath}, nil
	case config.LoaderVanilla:
		return vanilla{javaPath}, nil
	default:
		return nil, fmt.Errorf("unknown loader %q", name)
	}
}

// Detect infers the loader already installed in dir, by the launch artifacts each
// one leaves behind. Used on update, where the loader is fixed and must never be
// re-chosen. It relies only on on-disk evidence so it works on any server, not
// just ones we installed.
func Detect(dir string) (string, error) {
	switch {
	case exists(filepath.Join(dir, "libraries", "net", "neoforged", "neoforge")):
		return config.LoaderNeoForge, nil
	case exists(filepath.Join(dir, "libraries", "net", "minecraftforge", "forge")):
		return config.LoaderForge, nil
	case exists(filepath.Join(dir, "quilt-server-launch.jar")):
		return config.LoaderQuilt, nil
	case exists(filepath.Join(dir, "fabric-server-launch.jar")):
		return config.LoaderFabric, nil
	case exists(filepath.Join(dir, "server.jar")):
		return config.LoaderVanilla, nil
	default:
		return "", fmt.Errorf("could not identify a mod loader in %s", dir)
	}
}

// heapArgs is the single place that spells the JVM heap flags, so the inline
// run.sh and the NeoForge/Forge argfile can't drift apart. min is -Xms (heap
// reserved at startup), max is -Xmx (the ceiling), both in whole GB.
func heapArgs(minGB, maxGB int) (xms, xmx string) {
	return fmt.Sprintf("-Xms%dG", minGB), fmt.Sprintf("-Xmx%dG", maxGB)
}

// inlineRunScript is the run.sh used by loaders whose heap is set with plain java
// flags (Fabric, Quilt, Vanilla); NeoForge and Forge use a user_jvm_args.txt
// argfile instead. jar is the launch jar's filename.
func inlineRunScript(javaPath, jar string, minGB, maxGB int) string {
	xms, xmx := heapArgs(minGB, maxGB)
	return fmt.Sprintf("#!/usr/bin/env bash\nexec %s %s %s -jar %s nogui \"$@\"\n",
		javaPath, xms, xmx, jar)
}

// writeUserJVMArgs writes the heap range into dir/user_jvm_args.txt, one arg per
// line — the argfile format NeoForge and Forge read via @user_jvm_args.txt. It's
// the one owner of that file, shared by both loaders.
func writeUserJVMArgs(dir string, minGB, maxGB int) error {
	xms, xmx := heapArgs(minGB, maxGB)
	contents := xms + "\n" + xmx + "\n"
	if err := os.WriteFile(filepath.Join(dir, "user_jvm_args.txt"), []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write user_jvm_args.txt: %w", err)
	}
	return nil
}

// runJavaJar runs a downloaded installer jar with args, in workDir.
func runJavaJar(ctx context.Context, javaPath, jar, workDir string, args ...string) error {
	full := append([]string{"-jar", jar}, args...)
	cmd := exec.CommandContext(ctx, javaPath, full...)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run %s: %w: %s", jar, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
