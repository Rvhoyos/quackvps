// Package minecraft writes the files that make an installed server bootable and
// managed: eula.txt, server.properties, the uniform run.sh, and the systemd unit
// that runs it inside a screen session. It follows the universal EULA path — boot
// once headless to generate the files, then configure them — so it behaves the
// same across every loader.
package minecraft

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadProps parses a Java .properties file (key=value, # comments) into a map.
// A missing file yields an empty map, not an error, so callers can read-modify-
// write a file the server hasn't generated yet.
func ReadProps(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	props := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if ok {
			props[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	return props, nil
}

// SetProp reads a .properties file, sets one key, and writes it back, preserving
// the other entries. Used for server-port and any single-key edits.
func SetProp(path, key, value string) error {
	props, err := ReadProps(path)
	if err != nil {
		return err
	}
	props[key] = value
	return WriteProps(path, props)
}

// WriteProps writes a .properties map back to disk with stable key ordering so
// diffs stay readable across runs.
func WriteProps(path string, props map[string]string) error {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, props[k])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
