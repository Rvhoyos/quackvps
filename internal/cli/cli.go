// Package cli is the non-interactive front end. It parses the command line into an
// Options, and, when --mode is given, fills a config.Config from flags and runs
// every input check before any side effect, so a scripted run fails fast with a
// clear message instead of falling back to a prompt. It's the flag-mode counterpart
// to the interactive wizard in package prompt: both do nothing but produce a Config
// that the execution packages consume.
//
// Without --mode the tool stays interactive and only --help/--version/--dir apply.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Version is the build's version string, overridable at link time.
var Version = "dev"

// Options are the parsed command-line inputs. Mode is empty for an interactive
// run; when set, the remaining fields drive a prompt-free install/update/restore.
type Options struct {
	// Dir pre-seeds the parent folder the interactive picker opens at. It only
	// applies to a wizard run; the non-interactive path uses Parent/Instance.
	Dir string

	Mode     string // "" (wizard), or install / update / restore
	Parent   string
	Instance string

	// Install fields.
	Loader     string
	MCVersion  string
	HeapMinGB  int
	HeapMaxGB  int
	ServerPort int
	Modpack    string

	Dashboard bool
	Votifier  bool
	BlueMap   bool
	VoiceChat bool
	Geyser    bool

	DashboardPort int
	BlueMapPort   int
	VotifierPort  int
	VoiceChatPort int
	GeyserPort    int

	Domain             string
	Email              string
	DashboardSubdomain string
	BlueMapSubdomain   string

	HardenSSH bool
	SSHPubKey string

	// Update field.
	EmptyMods bool

	// Unit names the systemd service that manages an existing server, for update
	// and restore on a server we didn't install.
	Unit string

	// Restore field.
	Backup string
}

// Parse reads args (excluding the program name). It returns handled=true when it
// already satisfied the request itself (--help/--version), in which case the caller
// should exit zero. An unknown --mode is rejected here; the rest of the per-mode
// validation happens in Configure and config.Validate.
func Parse(args []string, out io.Writer) (opts Options, handled bool, err error) {
	fs := flag.NewFlagSet("quackvps", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintf(out, "quackvps, set up a Minecraft server and its web layer on a VPS.\n\n")
		fmt.Fprintf(out, "Run with no flags for the interactive wizard, or pass --mode for a\nprompt-free run (for scripts/CI).\n\n")
		fmt.Fprintf(out, "Usage:\n  sudo ./quackvps [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	showVersion := fs.Bool("version", false, "print the version and exit")
	var o Options
	fs.StringVar(&o.Dir, "dir", "", "parent folder to start the interactive picker in")
	fs.StringVar(&o.Mode, "mode", "", "non-interactive: install, update, or restore")
	fs.StringVar(&o.Parent, "parent", "", "parent folder that holds servers, e.g. /home/ubuntu/mcserver")
	fs.StringVar(&o.Instance, "instance", "", "server name (a single folder name under --parent)")

	fs.StringVar(&o.Loader, "loader", "", "install: fabric, neoforge, forge, quilt, or vanilla")
	fs.StringVar(&o.MCVersion, "mcversion", "", "install/update: target Minecraft version, e.g. 1.21.8")
	fs.IntVar(&o.HeapMinGB, "heap-min", 0, "install: starting JVM heap in GB (-Xms)")
	fs.IntVar(&o.HeapMaxGB, "heap-max", 0, "install: maximum JVM heap in GB (-Xmx)")
	fs.IntVar(&o.ServerPort, "server-port", 0, "install: Minecraft game port")
	fs.StringVar(&o.Modpack, "modpack", "", "install: modpack slug, or empty for none")

	fs.BoolVar(&o.Dashboard, "dashboard", false, "install: QuackedSMP web dashboard")
	fs.BoolVar(&o.Votifier, "votifier", false, "install: QuackedSMP Votifier v2 port")
	fs.BoolVar(&o.BlueMap, "bluemap", false, "install: BlueMap live map")
	fs.BoolVar(&o.VoiceChat, "voicechat", false, "install: Simple Voice Chat")
	fs.BoolVar(&o.Geyser, "geyser", false, "install: Bedrock crossplay (Geyser + Floodgate)")

	fs.IntVar(&o.DashboardPort, "dashboard-port", 0, "install: dashboard port (when --dashboard)")
	fs.IntVar(&o.BlueMapPort, "bluemap-port", 0, "install: BlueMap port (when --bluemap)")
	fs.IntVar(&o.VotifierPort, "votifier-port", 0, "install: Votifier port (when --votifier)")
	fs.IntVar(&o.VoiceChatPort, "voicechat-port", 0, "install: voice chat port (when --voicechat)")
	fs.IntVar(&o.GeyserPort, "geyser-port", 0, "install: Bedrock port (when --geyser)")

	fs.StringVar(&o.Domain, "domain", "", "install: domain for web services (empty prints an ssh tunnel instead)")
	fs.StringVar(&o.Email, "email", "", "install: ACME contact email (only used with --domain)")
	fs.StringVar(&o.DashboardSubdomain, "dashboard-subdomain", "", "install: dashboard subdomain (with --domain)")
	fs.StringVar(&o.BlueMapSubdomain, "bluemap-subdomain", "", "install: BlueMap subdomain (with --domain)")

	fs.BoolVar(&o.HardenSSH, "harden-ssh", false, "install: harden SSH to key-only login (needs --ssh-pubkey)")
	fs.StringVar(&o.SSHPubKey, "ssh-pubkey", "", "install: public key to authorize before hardening")

	fs.BoolVar(&o.EmptyMods, "empty-mods", false, "update: empty the mods folder instead of upgrading it")
	fs.StringVar(&o.Unit, "unit", "", "update/restore: systemd service that manages the server (default mc-<instance>.service)")
	fs.StringVar(&o.Backup, "backup", "", "restore: backup zip to restore (path or filename)")

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
	if err := validateMode(o.Mode); err != nil {
		return Options{}, false, err
	}
	return o, false, nil
}

func validateMode(mode string) error {
	switch mode {
	case "", "install", "update", "restore":
		return nil
	default:
		return fmt.Errorf("unknown --mode %q: use install, update, or restore", mode)
	}
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
