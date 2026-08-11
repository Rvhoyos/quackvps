package install

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rvhoyos/quackvps/internal/dl"
	"github.com/rvhoyos/quackvps/internal/modrinth"
)

// downloadMod saves a resolved version's primary jar into the instance's mods/
// directory, verifying its SHA-512.
func downloadMod(ctx context.Context, dir string, v modrinth.Version) error {
	file, ok := v.Primary()
	if !ok {
		return fmt.Errorf("version %s has no downloadable file", v.ID)
	}
	dest := filepath.Join(dir, "mods", file.Filename)
	if err := dl.DownloadVerify(ctx, file.URL, dest, file.Hashes.SHA512); err != nil {
		return fmt.Errorf("download %s: %w", file.Filename, err)
	}
	return nil
}
