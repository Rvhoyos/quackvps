// Package cli parses the command line and decides how the program runs. In v1
// the tool is interactive: the only flags are the universal --help and --version,
// plus --dir to pre-seed the parent folder for lightly-scripted runs. The full
// flag-driven, prompt-free mode is a later version; this package is the seam it
// will grow into, which is why parsing is kept separate from the wizard and the
// execution packages.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Version is the build's version string, overridable at link time.
var Version = "dev"

// Options are the parsed command-line inputs.
type Options struct {
	// Dir optionally pre-seeds the parent folder, skipping the tree picker's
	// starting guess. Empty means "let the wizard ask".
	Dir string
}

// Parse reads args (excluding the program name). It returns the options, or
// handled=true when it already satisfied the request itself (--help/--version),
// in which case the caller should exit zero without doing anything else.
func Parse(args []string, out io.Writer) (opts Options, handled bool, err error) {
	fs := flag.NewFlagSet("quackvps", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintf(out, "quackvps — set up a Minecraft server and its web layer on a VPS.\n\n")
		fmt.Fprintf(out, "Usage:\n  sudo ./quackvps [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	showVersion := fs.Bool("version", false, "print the version and exit")
	dir := fs.String("dir", "", "parent folder to start the picker in (e.g. /home/ubuntu/mcserver)")

	if err := fs.Parse(args); err != nil {
		// --help is a satisfied request, not a failure: usage was already printed.
		if err == flag.ErrHelp {
			return Options{}, true, nil
		}
		return Options{}, false, err
	}
	if *showVersion {
		fmt.Fprintf(out, "quackvps %s\n", Version)
		return Options{}, true, nil
	}
	return Options{Dir: *dir}, false, nil
}

// Interactive reports whether we're attached to a terminal. The wizard needs one;
// without it we fail clearly rather than hang on a prompt.
func Interactive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
