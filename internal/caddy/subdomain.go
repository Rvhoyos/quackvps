package caddy

import (
	"os"
	"path/filepath"
	"strings"
)

// UsedSubdomains returns the set of subdomain labels already claimed on domain by
// other instances' site files. It reads every InstanceDir/*.caddy except the given
// instance's own file, so a re-run doesn't collide with itself. A missing
// InstanceDir means nothing is claimed yet. It's the subdomain counterpart to the
// port collision scan in internal/system.
func UsedSubdomains(domain, excludeInstance string) (map[string]bool, error) {
	used := map[string]bool{}
	files, err := filepath.Glob(filepath.Join(InstanceDir, "*.caddy"))
	if err != nil {
		return nil, err
	}
	suffix := "." + domain
	self := excludeInstance + ".caddy"
	for _, f := range files {
		if filepath.Base(f) == self {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, addr := range siteAddresses(string(data)) {
			if label := strings.TrimSuffix(addr, suffix); label != addr && label != "" {
				used[label] = true
			}
		}
	}
	return used, nil
}

// SubdomainDefault picks a collision-safe default label: the bare base if it's
// free, otherwise base-instance (matching the prod pattern status/status-buns).
func SubdomainDefault(base, instance string, used map[string]bool) string {
	if used[base] {
		return base + "-" + instance
	}
	return base
}

// siteAddresses pulls the address token that opens each site block: the first word
// on any line that ends in "{". Our per-instance files only ever hold plain
// "addr { reverse_proxy ... }" blocks, so this is all the parsing we need.
func siteAddresses(content string) []string {
	var addrs []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "{") {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			addrs = append(addrs, fields[0])
		}
	}
	return addrs
}
