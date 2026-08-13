package packtest

import (
	"encoding/json"
	"testing"

	"github.com/rvhoyos/quackvps/internal/catalog"
)

func TestSelectPackWraps(t *testing.T) {
	packs := []catalog.Pack{
		{Slug: "a"}, {Slug: "b"}, {Slug: "c"},
	}
	tests := []struct {
		day  int
		want string
	}{
		{0, "a"},
		{1, "b"},
		{2, "c"},
		{3, "a"}, // wraps back to the start
		{4, "b"},
		{-1, "c"}, // a negative day still lands on a real pack
		{366, "a"},
	}
	for _, tt := range tests {
		if got := selectPack(packs, tt.day).Slug; got != tt.want {
			t.Errorf("selectPack(day=%d) = %q, want %q", tt.day, got, tt.want)
		}
	}
}

func TestAllFeaturedStable(t *testing.T) {
	got := catalog.AllFeatured()
	if len(got) == 0 {
		t.Fatal("AllFeatured returned no packs")
	}
	// Order is fixed: NeoForge first, Fabric last (Forge in between). This guards the
	// day-indexing from silently reshuffling when the curated lists change.
	if got[0].Loader != "neoforge" {
		t.Errorf("first pack loader = %q, want neoforge", got[0].Loader)
	}
	if last := got[len(got)-1].Loader; last != "fabric" {
		t.Errorf("last pack loader = %q, want fabric", last)
	}
	for _, p := range got {
		if p.Slug == "" || p.Loader == "" {
			t.Errorf("pack with empty field: %+v", p)
		}
	}
}

func TestMatrixJSONShape(t *testing.T) {
	m := Matrix{
		Loader:  "neoforge",
		Slug:    "cobblemon-neoforge",
		Entries: []Version{{MC: "1.21.8", Java: 21}},
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// The workflow reads these exact keys, so pin them.
	want := `{"loader":"neoforge","slug":"cobblemon-neoforge","matrix":[{"mc":"1.21.8","java":21}]}`
	if string(out) != want {
		t.Errorf("JSON = %s, want %s", out, want)
	}
}
