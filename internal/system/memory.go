package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// TotalMemoryGB returns the box's total RAM in whole GB (rounded down), or 0 if
// it can't be read. The RAM prompt uses it to pick a safe default and to reject a
// heap size the box can't actually commit.
func TotalMemoryGB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// Line looks like: "MemTotal:       8123456 kB"
		line := sc.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0
		}
		return kb / (1024 * 1024)
	}
	return 0
}
