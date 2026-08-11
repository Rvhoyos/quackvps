package dl

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadVerify(t *testing.T) {
	body := []byte("server jar contents")
	sum := sha512.Sum512(body)
	good := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "nested", "server.jar")

	if err := DownloadVerify(context.Background(), srv.URL, dest, good); err != nil {
		t.Fatalf("DownloadVerify with correct hash: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(body) {
		t.Fatalf("downloaded content wrong: %q err=%v", got, err)
	}

	// A wrong hash must fail and leave no file behind.
	bad := filepath.Join(dir, "bad.jar")
	if err := DownloadVerify(context.Background(), srv.URL, bad, "deadbeef"); err == nil {
		t.Fatal("DownloadVerify with wrong hash should fail")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatalf("failed download should not leave a file: err=%v", err)
	}
}

func TestSHA512File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	body := []byte("hash me")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(body)
	got, err := SHA512File(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != hex.EncodeToString(sum[:]) {
		t.Fatalf("SHA512File = %s, want %s", got, hex.EncodeToString(sum[:]))
	}
}
