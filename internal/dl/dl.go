// Package dl handles the two things the rest of the tool needs from the network:
// small JSON API calls (Mojang, Modrinth, NeoForge) and file downloads verified
// against a published hash. Everything takes a context so long transfers stay
// cancellable.
package dl

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// userAgent identifies the tool to the APIs we call. Modrinth asks callers to
// send a descriptive User-Agent with a contact, so we point at the repo.
const userAgent = "quackvps/0.1 (github.com/rvhoyos/quackvps)"

var client = &http.Client{Timeout: 5 * time.Minute}

// GetJSON fetches url and decodes the JSON body into out.
func GetJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	return doJSON(req, out)
}

// PostJSON sends body as JSON to url and decodes the JSON response into out.
// out may be nil when the response body isn't needed.
func PostJSON(ctx context.Context, url string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return doJSON(req, out)
}

// GetBytes fetches url and returns the raw body — for the non-JSON endpoints we
// hit, like Quilt's XML maven metadata.
func GetBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get %s: unexpected status %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func doJSON(req *http.Request, out any) error {
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: unexpected status %s", req.Method, req.URL, resp.Status)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Download saves url to dest, creating parent directories as needed. It writes to
// a temp file and renames on success so a failed transfer never leaves a
// half-written file in place.
func Download(ctx context.Context, url, dest string) error {
	return download(ctx, url, dest, "", nil)
}

// DownloadVerify is Download plus a SHA-512 check: the transfer is rejected (and
// the temp file removed) if the hash doesn't match sha512hex.
func DownloadVerify(ctx context.Context, url, dest, sha512hex string) error {
	return download(ctx, url, dest, sha512hex, sha512.New())
}

func download(ctx context.Context, url, dest, wantHex string, h hash.Hash) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("get %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".quackvps-dl-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once we've renamed it away

	var w io.Writer = tmp
	if h != nil {
		w = io.MultiWriter(tmp, h)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if h != nil {
		got := hex.EncodeToString(h.Sum(nil))
		if !equalHex(got, wantHex) {
			return fmt.Errorf("hash mismatch for %s: got %s, want %s", dest, got, wantHex)
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("finalize %s: %w", dest, err)
	}
	return nil
}

// SHA1File returns the lowercase hex SHA-1 of a file. Used to identify existing
// mod jars against Modrinth on update.
func SHA1File(path string) (string, error) { return hashFile(path, sha1.New()) }

// SHA512File returns the lowercase hex SHA-512 of a file.
func SHA512File(path string) (string, error) { return hashFile(path, sha512.New()) }

func hashFile(path string, h hash.Hash) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func equalHex(a, b string) bool {
	// Case-insensitive compare; published hashes vary in casing.
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'F' {
		return c + ('a' - 'A')
	}
	return c
}
