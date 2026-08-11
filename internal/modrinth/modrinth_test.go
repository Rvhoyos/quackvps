package modrinth

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONArray(t *testing.T) {
	if got := jsonArray([]string{"fabric"}); got != `["fabric"]` {
		t.Errorf("jsonArray single = %s", got)
	}
	if got := jsonArray([]string{"1.21.8", "1.21.9"}); got != `["1.21.8","1.21.9"]` {
		t.Errorf("jsonArray multi = %s", got)
	}
}

func TestPrimaryFile(t *testing.T) {
	v := Version{Files: []File{
		{Filename: "sources.jar", Primary: false},
		{Filename: "mod.jar", Primary: true},
	}}
	f, ok := v.Primary()
	if !ok || f.Filename != "mod.jar" {
		t.Errorf("Primary() = %q,%v want mod.jar,true", f.Filename, ok)
	}

	// Falls back to the first file when none is flagged primary.
	v2 := Version{Files: []File{{Filename: "only.jar"}}}
	if f, ok := v2.Primary(); !ok || f.Filename != "only.jar" {
		t.Errorf("Primary() fallback = %q,%v", f.Filename, ok)
	}

	if _, ok := (Version{}).Primary(); ok {
		t.Error("Primary() on empty version should be false")
	}
}

func TestParseMrpack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.mrpack")
	writeMrpack(t, path, map[string]string{
		"modrinth.index.json":         `{"files":[{"path":"mods/a.jar","downloads":["http://x/a.jar"],"hashes":{"sha512":"abc"},"env":{"client":"required","server":"required"}}]}`,
		"overrides/config/foo.txt":    "foo",
		"server-overrides/server.txt": "srv",
	})

	mp, err := ParseMrpack(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(mp.Files) != 1 || mp.Files[0].Path != "mods/a.jar" {
		t.Fatalf("files = %+v", mp.Files)
	}
	if !mp.Files[0].serverSupported() {
		t.Error("server:required file should be server-supported")
	}
	if string(mp.Overrides["config/foo.txt"]) != "foo" {
		t.Errorf("overrides = %v", mp.Overrides)
	}
	if string(mp.ServerOverrides["server.txt"]) != "srv" {
		t.Errorf("server-overrides = %v", mp.ServerOverrides)
	}
}

func TestParseMrpackMissingIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.mrpack")
	writeMrpack(t, path, map[string]string{"overrides/x.txt": "x"})
	if _, err := ParseMrpack(path); err == nil {
		t.Error("expected error for mrpack without index")
	}
}

func writeMrpack(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
