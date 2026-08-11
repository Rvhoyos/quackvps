package system

import (
	"context"
	"fmt"
	"os"
	"time"
)

// WaitForFiles blocks until every path exists, or the timeout elapses. The
// install uses it during the warm-up boot to know the mods have generated their
// configs before it stops the server to edit them.
func WaitForFiles(ctx context.Context, paths []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if allExist(paths) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for: %v", timeout, missing(paths))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func allExist(paths []string) bool {
	return len(missing(paths)) == 0
}

func missing(paths []string) []string {
	var out []string
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			out = append(out, p)
		}
	}
	return out
}
