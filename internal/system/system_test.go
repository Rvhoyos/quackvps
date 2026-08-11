package system

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestNextFree(t *testing.T) {
	tests := []struct {
		base int
		used []int
		want int
	}{
		{25565, nil, 25565},
		{25565, []int{25565, 25566}, 25567},
		{8100, []int{8100}, 8101},
		{8125, []int{8126}, 8125},
	}
	for _, tt := range tests {
		set := map[int]bool{}
		addAll(set, tt.used)
		if got := NextFree(tt.base, set); got != tt.want {
			t.Errorf("NextFree(%d, %v) = %d, want %d", tt.base, tt.used, got, tt.want)
		}
	}
}

func TestValidatePublicKey(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(ssh.MarshalAuthorizedKey(sshPub)) // includes trailing newline; trimmed inside

	if err := ValidatePublicKey(valid); err != nil {
		t.Errorf("valid ed25519 key rejected: %v", err)
	}
	bad := map[string]string{
		"empty":       "",
		"private key": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
		"two keys":    valid + "ssh-rsa AAAAB3Nza",
		"garbage":     "not a key",
	}
	for name, in := range bad {
		if err := ValidatePublicKey(in); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestScanPortsInFile(t *testing.T) {
	dir := t.TempDir()
	props := filepath.Join(dir, "server.properties")
	os.WriteFile(props, []byte("level-name=world\nserver-port=25565\n"), 0o644)

	qsmp := filepath.Join(dir, "quackedsmp.json")
	os.WriteFile(qsmp, []byte(`{"dashboard":{"port":8125},"votifier":{"port":8192}}`), 0o644)

	got := append(scanPortsInFile(props), scanPortsInFile(qsmp)...)
	sort.Ints(got)
	want := []int{8125, 8192, 25565}
	if len(got) != len(want) {
		t.Fatalf("scanned ports = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scanned ports = %v, want %v", got, want)
		}
	}
}
