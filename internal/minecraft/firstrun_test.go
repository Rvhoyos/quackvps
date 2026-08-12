package minecraft

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A first run that never writes eula.txt must be classified so the installer can
// explain it: a server that exits on its own is a crash (with the root cause lifted
// to the front), while one killed by SIGKILL is treated as out of memory.
func TestFirstRunGenerateClassifies(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		want    FirstRunKind
		tailHas string
	}{
		{
			// The cause is buried above 20 stack frames, so it falls outside the
			// last-15-lines tail and must be lifted to the front.
			name:    "crash lifts a buried root cause",
			script:  "#!/usr/bin/env bash\necho 'Caused by: java.lang.IllegalStateException: broken'\nfor i in $(seq 1 20); do echo \"  at com.example.Foo.method($i)\"; done\nexit 1\n",
			want:    FirstRunCrash,
			tailHas: "Caused by: java.lang.IllegalStateException: broken",
		},
		{
			name:   "sigkill reads as out of memory",
			script: "#!/usr/bin/env bash\nkill -9 $$\n",
			want:   FirstRunOOM,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(tc.script), 0o755); err != nil {
				t.Fatal(err)
			}

			var fre *FirstRunError
			if err := FirstRunGenerate(context.Background(), dir); !errors.As(err, &fre) {
				t.Fatalf("want *FirstRunError, got %v", err)
			}
			if fre.Kind != tc.want {
				t.Errorf("Kind = %d, want %d", fre.Kind, tc.want)
			}
			if tc.tailHas != "" && !strings.HasPrefix(fre.Tail, tc.tailHas) {
				t.Errorf("Tail = %q, want it to start with %q", fre.Tail, tc.tailHas)
			}
		})
	}
}
