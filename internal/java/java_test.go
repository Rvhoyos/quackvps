package java

import "testing"

func TestRequiredFromTable(t *testing.T) {
	tests := []struct {
		mc      string
		want    int
		wantErr bool
	}{
		{"1.20.5", 21, false},
		{"1.21.8", 21, false},
		{"1.21.11", 21, false},
		{"26.1", 25, false},
		{"26.1.2", 25, false},
		{"1.20.1", 17, false}, // NeoForge floor → Java 17
		{"1.18.2", 17, false},
		{"1.16.5", 0, true}, // Java 8, out of scope
	}
	for _, tt := range tests {
		got, err := requiredFromTable(tt.mc)
		if tt.wantErr {
			if err == nil {
				t.Errorf("requiredFromTable(%q) = %d, want error", tt.mc, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("requiredFromTable(%q): %v", tt.mc, err)
		}
		if got != tt.want {
			t.Errorf("requiredFromTable(%q) = %d, want %d", tt.mc, got, tt.want)
		}
	}
}

func TestMajorFromDirName(t *testing.T) {
	tests := []struct {
		name  string
		major int
		ok    bool
	}{
		{"temurin-21-jdk-amd64", 21, true},
		{"temurin-25-jdk-arm64", 25, true},
		{"java-21-openjdk-amd64", 21, true},
		{"jdk-21.0.3+9", 21, true},
		{"some-unrelated-dir", 0, false},
	}
	for _, tt := range tests {
		got, ok := majorFromDirName(tt.name)
		if ok != tt.ok || (ok && got != tt.major) {
			t.Errorf("majorFromDirName(%q) = %d,%v want %d,%v", tt.name, got, ok, tt.major, tt.ok)
		}
	}
}
