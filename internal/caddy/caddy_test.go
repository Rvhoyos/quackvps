package caddy

import (
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
		if !(globalEnd < importIdx && importIdx < siteIdx) {
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

func TestGlobalBlockEnd(t *testing.T) {
	if _, ok := globalBlockEnd("example.com {\n}\n"); ok {
		t.Error("site block should not be detected as global")
	}
	if end, ok := globalBlockEnd("{\n\temail a@b\n}\nrest"); !ok || end != len("{\n\temail a@b\n}") {
		t.Errorf("globalBlockEnd = %d,%v", end, ok)
	}
}
