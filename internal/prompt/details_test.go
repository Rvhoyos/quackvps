package prompt

import "testing"

func TestSlugFromInput(t *testing.T) {
	cases := map[string]string{
		"cobblemon-fabric":                          "cobblemon-fabric",
		"  cobblemon-fabric  ":                      "cobblemon-fabric",
		"modrinth.com/modpack/cobblemon-fabric":     "cobblemon-fabric",
		"https://modrinth.com/modpack/create_plus":  "create_plus",
		"https://modrinth.com/modpack/create_plus/": "create_plus",
		"": "",
	}
	for in, want := range cases {
		if got := slugFromInput(in); got != want {
			t.Errorf("slugFromInput(%q) = %q, want %q", in, got, want)
		}
	}
}
