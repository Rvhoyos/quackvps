// Package mcver parses and compares Minecraft release versions across both the
// legacy "1.x.y" scheme and the newer calendar "YY.N" scheme. A component-wise
// numeric comparison orders the two schemes correctly on its own, because a
// calendar year like 26 sorts above the legacy leading 1 (so 26.1 > 1.21.11).
package mcver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed release version, held as its dotted numeric components.
type Version struct {
	parts []int
	raw   string
}

func (v Version) String() string { return v.raw }

// Parse reads a release version such as "1.21.8" or "26.1.2". Snapshots and
// pre-releases (anything non-numeric, e.g. "24w14a" or "1.21-rc1") are rejected:
// v1 supports releases only.
func Parse(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("empty minecraft version")
	}
	fields := strings.Split(s, ".")
	parts := make([]int, len(fields))
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return Version{}, fmt.Errorf("unsupported minecraft version %q (releases only, e.g. 1.21.8 or 26.1.2)", s)
		}
		parts[i] = n
	}
	return Version{parts: parts, raw: s}, nil
}

// Compare returns -1, 0, or 1 for a<b, a==b, a>b. Shorter versions are padded
// with zeros so "1.21" and "1.21.0" compare equal.
func Compare(a, b Version) int {
	n := len(a.parts)
	if len(b.parts) > n {
		n = len(b.parts)
	}
	for i := 0; i < n; i++ {
		x, y := a.at(i), b.at(i)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

func (v Version) at(i int) int {
	if i < len(v.parts) {
		return v.parts[i]
	}
	return 0
}

// AtLeast reports whether a is the same as or newer than b.
func AtLeast(a, b Version) bool { return Compare(a, b) >= 0 }
