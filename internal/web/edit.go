package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rvhoyos/quackvps/internal/minecraft"
)

// A mod's config keys can be renamed between versions, so every key we edit is
// declared as a set of candidate names (current name first, historical aliases
// after). The editors try each candidate against the config the mod actually
// generated; if none match, that's a schema change we can't safely guess through,
// so they return a clear, reportable error rather than fabricate anything. These
// editors never create a config from scratch, the install boots the server once
// so the mod writes its own configs first (see install.warmUpBoot).
var (
	keysBlueMapPort     = []string{"port"}
	keysBlueMapAccept   = []string{"accept-download"}
	keysVoicePort       = []string{"port"}
	keysVoiceHost       = []string{"voice_host"}
	keysDashboard       = []string{"dashboard"}
	keysVotifier        = []string{"votifier"}
	keysSectionPort     = []string{"port"}
	keysSectionEnabled  = []string{"enabled"}
	keysPanelURL        = []string{"panel_url"}
	keysVoicechatEnable = []string{"voicechat_enable"}
	keysGeyserPort      = []string{"port"}
	keysGeyserAuth      = []string{"auth-type"}
)

// quackedsmpConfig is the path to the QuackedSMP mod config within an instance.
func quackedsmpConfig(dir string) string {
	return filepath.Join(dir, "config", "quackedsmp.json")
}

// editJSON reads a JSON object the mod generated, applies mutate, and writes it
// back indented. A missing file is an error (the mod should have created it)
// mutate is where the candidate-key lookups live, so it can also error.
func editJSON(path string, mutate func(m map[string]any) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found, the server didn't generate it; please report this", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if err := mutate(m); err != nil {
		return err
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// jsonKey returns the first candidate present in m, or an error naming what we
// looked for, so a renamed key surfaces instead of silently doing nothing.
func jsonKey(m map[string]any, candidates []string, where string) (string, error) {
	for _, c := range candidates {
		if _, ok := m[c]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("none of %v found in %s, the mod's config format may have changed; please report this", candidates, where)
}

// jsonSection returns the nested object under the first matching candidate key.
func jsonSection(m map[string]any, candidates []string, where string) (map[string]any, error) {
	key, err := jsonKey(m, candidates, where)
	if err != nil {
		return nil, err
	}
	sub, ok := m[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q in %s is not an object as expected; please report this", key, where)
	}
	return sub, nil
}

// SetPanelURL sets the top-level panel_url the mod shows players in-game.
func SetPanelURL(dir, url string) error {
	return editJSON(quackedsmpConfig(dir), func(m map[string]any) error {
		key, err := jsonKey(m, keysPanelURL, "quackedsmp.json")
		if err != nil {
			return err
		}
		m[key] = url
		return nil
	})
}

// SetVoicechatEnable toggles QuackedSMP's Simple Voice Chat integration.
func SetVoicechatEnable(dir string, on bool) error {
	return editJSON(quackedsmpConfig(dir), func(m map[string]any) error {
		key, err := jsonKey(m, keysVoicechatEnable, "quackedsmp.json")
		if err != nil {
			return err
		}
		m[key] = on
		return nil
	})
}

// setSectionPort sets `port` (and `enabled: true`) inside a named section of
// quackedsmp.json, used for both the dashboard and votifier blocks.
func setSectionPort(dir string, sectionKeys []string, port int) error {
	return editJSON(quackedsmpConfig(dir), func(m map[string]any) error {
		section, err := jsonSection(m, sectionKeys, "quackedsmp.json")
		if err != nil {
			return err
		}
		portKey, err := jsonKey(section, keysSectionPort, "quackedsmp.json section")
		if err != nil {
			return err
		}
		section[portKey] = port
		if enabledKey, err := jsonKey(section, keysSectionEnabled, "quackedsmp.json section"); err == nil {
			section[enabledKey] = true
		}
		return nil
	})
}

// setHOCONKey sets the first matching candidate key in a HOCON file to value
// (verbatim; caller quotes if needed), preserving indentation and every other
// line. A missing file, or a file with none of the candidate keys, is an error
// the mod generates these configs first, so either case means the schema moved.
func setHOCONKey(path string, candidates []string, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found, the server didn't generate it; please report this", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, key := range candidates {
		re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(key) + `\s*:\s*.*$`)
		if re.Match(data) {
			updated := re.ReplaceAllString(string(data), "${1}"+key+": "+value)
			if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			return nil
		}
	}
	return fmt.Errorf("none of %v found in %s, the mod's config format may have changed; please report this", candidates, path)
}

// setYAMLSectionKey sets the first matching candidate key that lives inside a
// top-level YAML section (e.g. `port` under `bedrock:`). It edits only within that
// section's indented block, so a key that also appears in another section (Geyser's
// `port` exists under both `bedrock` and `java`) is left untouched elsewhere. Like
// setHOCONKey it's edit-only and comment-preserving: a missing file, a missing
// section, or a missing key is a reportable error, never a fabricated line.
func setYAMLSectionKey(path, section string, candidates []string, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found, the server didn't generate it; please report this", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	header := regexp.MustCompile(`^` + regexp.QuoteMeta(section) + `:\s*$`)
	start := -1
	for i, line := range lines {
		if header.MatchString(line) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("section %q not found in %s, the mod's config format may have changed; please report this", section, path)
	}

	// The section's body is its indented lines; the first line starting at column 0
	// (the next top-level key or comment) ends the block.
	topLevel := regexp.MustCompile(`^\S`)
	for i := start; i < len(lines); i++ {
		if topLevel.MatchString(lines[i]) {
			break
		}
		for _, key := range candidates {
			re := regexp.MustCompile(`^(\s+)` + regexp.QuoteMeta(key) + `\s*:\s*.*$`)
			if re.MatchString(lines[i]) {
				lines[i] = re.ReplaceAllString(lines[i], "${1}"+key+": "+value)
				return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
			}
		}
	}
	return fmt.Errorf("none of %v found in section %q of %s, the mod's config format may have changed; please report this", candidates, section, path)
}

// setPropKey sets the first matching candidate key in an existing .properties
// file to value. Missing file or no matching key → error (edit-only). It reuses
// the minecraft props helpers but guards existence itself, because ReadProps
// treats a missing file as empty.
func setPropKey(path string, candidates []string, value string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found, the server didn't generate it; please report this", path)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	props, err := minecraft.ReadProps(path)
	if err != nil {
		return err
	}
	for _, key := range candidates {
		if _, ok := props[key]; ok {
			props[key] = value
			return minecraft.WriteProps(path, props)
		}
	}
	return fmt.Errorf("none of %v found in %s, the mod's config format may have changed; please report this", candidates, path)
}
