package caddy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithImportLine(t *testing.T) {
	t.Run("empty file no email", func(t *testing.T) {
		got, changed := withImportLine("", "")
		if !changed || got != importLine+"\n" {
			t.Errorf("got %q changed=%v", got, changed)
		}
	})

	t.Run("empty file with email adds global block first", func(t *testing.T) {
		got, changed := withImportLine("", "me@example.com")
		if !changed {
			t.Fatal("expected change")
		}
		if !strings.HasPrefix(got, "{\n\temail me@example.com\n}\n") {
			t.Errorf("global block not first:\n%s", got)
		}
		if !strings.Contains(got, importLine) {
			t.Errorf("missing import line:\n%s", got)
		}
	})

	t.Run("existing global block inserts import after it", func(t *testing.T) {
		in := "{\n\temail a@b.com\n}\n\nexample.com {\n\trespond \"hi\"\n}\n"
		got, changed := withImportLine(in, "")
		if !changed {
			t.Fatal("expected change")
		}
		globalEnd := strings.Index(got, "}")
		importIdx := strings.Index(got, importLine)
		siteIdx := strings.Index(got, "example.com {")
		if globalEnd >= importIdx || importIdx >= siteIdx {
			t.Errorf("import not placed after global block, before site:\n%s", got)
		}
	})

	t.Run("no global block prepends import above user site", func(t *testing.T) {
		in := "example.com {\n\trespond \"hi\"\n}\n"
		got, changed := withImportLine(in, "")
		if !changed || strings.Index(got, importLine) > strings.Index(got, "example.com") {
			t.Errorf("import should precede the user site:\n%s", got)
		}
	})

	t.Run("idempotent when import already present", func(t *testing.T) {
		in := importLine + "\n\nexample.com {\n}\n"
		got, changed := withImportLine(in, "me@example.com")
		if changed || got != in {
			t.Errorf("should be a no-op, got changed=%v\n%s", changed, got)
		}
	})

	t.Run("leading comments before global block", func(t *testing.T) {
		in := "# my caddyfile\n\n{\n\temail a@b.com\n}\n"
		got, changed := withImportLine(in, "")
		if !changed {
			t.Fatal("expected change")
		}
		if strings.Index(got, importLine) < strings.Index(got, "}") {
			t.Errorf("import should be after the global block even with leading comments:\n%s", got)
		}
	})
}

func TestUsedSubdomains(t *testing.T) {
	dir := t.TempDir()
	old := InstanceDir
	InstanceDir = dir
	defer func() { InstanceDir = old }()

	// A prior instance holds status + map on this domain.
	os.WriteFile(filepath.Join(dir, "buns.caddy"),
		[]byte("status.example.com {\n\treverse_proxy 127.0.0.1:8126\n}\nmap.example.com {\n\treverse_proxy 127.0.0.1:8101\n}\n"), 0o644)
	// A different domain must not leak into the set.
	os.WriteFile(filepath.Join(dir, "other.caddy"),
		[]byte("panel.other.net {\n\treverse_proxy 127.0.0.1:9000\n}\n"), 0o644)

	used, err := UsedSubdomains("example.com", "survival")
	if err != nil {
		t.Fatal(err)
	}
	if !used["status"] || !used["map"] {
		t.Errorf("expected status+map claimed, got %v", used)
	}
	if used["panel"] {
		t.Errorf("a different domain's label leaked in: %v", used)
	}

	// The current instance's own file is excluded, so a re-run doesn't clash with
	// itself: "creeper" claims "live", but excluding creeper hides it.
	os.WriteFile(filepath.Join(dir, "creeper.caddy"),
		[]byte("live.example.com {\n\treverse_proxy 127.0.0.1:8127\n}\n"), 0o644)
	used, err = UsedSubdomains("example.com", "creeper")
	if err != nil {
		t.Fatal(err)
	}
	if used["live"] {
		t.Errorf("the excluded instance's own label should not be claimed: %v", used)
	}
	if !used["status"] {
		t.Errorf("another instance's label should still be claimed: %v", used)
	}

	// A missing InstanceDir is not an error.
	InstanceDir = filepath.Join(dir, "does-not-exist")
	if used, err := UsedSubdomains("example.com", ""); err != nil || len(used) != 0 {
		t.Errorf("missing dir should give empty set, no error: used=%v err=%v", used, err)
	}
}

func TestSubdomainDefault(t *testing.T) {
	used := map[string]bool{"status": true}
	if got := SubdomainDefault("map", "buns", used); got != "map" {
		t.Errorf("free label should be the base, got %q", got)
	}
	if got := SubdomainDefault("status", "buns", used); got != "status-buns" {
		t.Errorf("taken label should bump to base-instance, got %q", got)
	}
}

func TestGlobalBlockEnd(t *testing.T) {
	if _, ok := globalBlockEnd("example.com {\n}\n"); ok {
		t.Error("site block should not be detected as global")
	}
	if end, ok := globalBlockEnd("{\n\temail a@b\n}\nrest"); !ok || end != len("{\n\temail a@b\n}") {
		t.Errorf("globalBlockEnd = %d,%v", end, ok)
	}
}
