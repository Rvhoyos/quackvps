package minecraft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHeap(t *testing.T) {
	t.Run("range from user_jvm_args.txt", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "user_jvm_args.txt"), []byte("-Xms2G\n-Xmx6G\n"), 0o644)
		if min, max := ReadHeap(dir); min != 2 || max != 6 {
			t.Errorf("readHeap = %d,%d want 2,6", min, max)
		}
	})

	t.Run("from run.sh", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "run.sh"), []byte("exec java -Xms4G -Xmx8G -jar server.jar nogui\n"), 0o644)
		if min, max := ReadHeap(dir); min != 4 || max != 8 {
			t.Errorf("readHeap = %d,%d want 4,8", min, max)
		}
	})

	t.Run("xmx only mirrors onto xms", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "run.sh"), []byte("exec java -Xmx8G -jar server.jar nogui\n"), 0o644)
		if min, max := ReadHeap(dir); min != 8 || max != 8 {
			t.Errorf("readHeap = %d,%d want 8,8", min, max)
		}
	})

	t.Run("defaults when absent", func(t *testing.T) {
		if min, max := ReadHeap(t.TempDir()); min != defaultMinGB || max != defaultMaxGB {
			t.Errorf("readHeap = %d,%d want %d,%d", min, max, defaultMinGB, defaultMaxGB)
		}
	})
}
