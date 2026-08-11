package mcver

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.20.5", "1.20.5", 0},
		{"1.21", "1.21.0", 0},
		{"1.20.5", "1.21.11", -1},
		{"1.21.11", "1.20.5", 1},
		{"1.21.11", "26.1", -1}, // calendar scheme is newer than any legacy 1.x
		{"26.1", "1.21.11", 1},
		{"26.1.2", "26.1", 1},
		{"26.1", "26.1.2", -1},
	}
	for _, tt := range tests {
		a, err := Parse(tt.a)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.a, err)
		}
		b, err := Parse(tt.b)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.b, err)
		}
		if got := Compare(a, b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseRejectsSnapshots(t *testing.T) {
	for _, s := range []string{"", "24w14a", "1.21-rc1", "latest"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", s)
		}
	}
}
