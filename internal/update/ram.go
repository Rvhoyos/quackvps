package update

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// Defaults used only if the existing heap range can't be read from disk.
const (
	defaultMinGB = 1
	defaultMaxGB = 4
)

var (
	xmsRE = regexp.MustCompile(`-Xms(\d+)G`)
	xmxRE = regexp.MustCompile(`-Xmx(\d+)G`)
)

// readHeap recovers the heap range (GB) from the existing server so an update
// keeps it rather than silently resetting to a default. NeoForge/Forge store it
// in user_jvm_args.txt; the other loaders inline it in run.sh — we check both.
// A file with an -Xmx but no -Xms mirrors the max onto the min.
func readHeap(dir string) (minGB, maxGB int) {
	for _, name := range []string{"user_jvm_args.txt", "run.sh"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		s := string(data)
		hi, ok := matchGB(xmxRE, s)
		if !ok {
			continue // no ceiling here; try the next file
		}
		lo := hi
		if v, ok := matchGB(xmsRE, s); ok {
			lo = v
		}
		if lo > hi {
			lo = hi
		}
		return lo, hi
	}
	return defaultMinGB, defaultMaxGB
}

// matchGB pulls the first whole-GB value captured by re from s.
func matchGB(re *regexp.Regexp, s string) (int, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	gb, err := strconv.Atoi(m[1])
	if err != nil || gb <= 0 {
		return 0, false
	}
	return gb, true
}
