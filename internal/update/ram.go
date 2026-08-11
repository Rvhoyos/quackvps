package update

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// defaultRAMGB is used only if the existing RAM can't be read from disk.
const defaultRAMGB = 4

var xmxRE = regexp.MustCompile(`-Xmx(\d+)G`)

// readRAM recovers the heap size (GB) from the existing server so an update keeps
// it rather than silently resetting to a default. NeoForge stores it in
// user_jvm_args.txt; the other loaders inline it in run.sh — we check both.
func readRAM(dir string) int {
	for _, name := range []string{"user_jvm_args.txt", "run.sh"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if m := xmxRE.FindStringSubmatch(string(data)); m != nil {
			if gb, err := strconv.Atoi(m[1]); err == nil && gb > 0 {
				return gb
			}
		}
	}
	return defaultRAMGB
}
