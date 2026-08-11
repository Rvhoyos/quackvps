package caddy

import (
	"fmt"
	"os"
	"strings"
)

// EnsureImportLine makes sure the main Caddyfile pulls in our per-instance files
// via a single import line, adding a global options block with the ACME email if
// the file has none. It's idempotent: once the import line is present, the file
// is left untouched. email may be "" to skip adding a global block.
func EnsureImportLine(email string) error {
	existing, err := os.ReadFile(MainCaddyfile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", MainCaddyfile, err)
	}
	updated, changed := withImportLine(string(existing), email)
	if !changed {
		return nil
	}
	if err := os.WriteFile(MainCaddyfile, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", MainCaddyfile, err)
	}
	return nil
}

// withImportLine returns the Caddyfile content with our import line guaranteed to
// be present (after the global block if there is one), plus whether it changed.
// Kept pure so the placement rules are unit-tested without touching disk.
func withImportLine(content, email string) (string, bool) {
	if strings.Contains(content, importLine) {
		return content, false
	}

	end, hasGlobal := globalBlockEnd(content)
	switch {
	case hasGlobal:
		// Insert the import right after the global options block.
		head, tail := content[:end], content[end:]
		return head + "\n" + importLine + "\n" + strings.TrimLeft(tail, "\n"), true
	case email != "":
		// No global block yet, and we have an email → add one, then the import.
		block := fmt.Sprintf("{\n\temail %s\n}\n\n", email)
		return block + importLine + "\n\n" + strings.TrimLeft(content, "\n"), true
	default:
		// No global block and no email → the import can sit at the very top.
		if content == "" {
			return importLine + "\n", true
		}
		return importLine + "\n\n" + strings.TrimLeft(content, "\n"), true
	}
}

// globalBlockEnd finds the end offset of a leading global options block, if the
// Caddyfile has one. A global block is the only block that opens with "{" with no
// address before it; a site block always names an address first. Returns the
// index just past the block's closing "}".
func globalBlockEnd(content string) (int, bool) {
	i := skipLeadingTrivia(content)
	if i >= len(content) || content[i] != '{' {
		return 0, false
	}
	depth := 0
	for ; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false // unbalanced; treat as no global block
}

// skipLeadingTrivia returns the index of the first meaningful character, skipping
// whitespace and full-line "#" comments at the top of the file.
func skipLeadingTrivia(content string) int {
	i := 0
	for i < len(content) {
		// Skip whitespace.
		for i < len(content) && (content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r') {
			i++
		}
		// Skip a comment line.
		if i < len(content) && content[i] == '#' {
			for i < len(content) && content[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	return i
}
