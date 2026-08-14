package catalog

import (
	"context"
	"testing"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/modrinth"
)

// fakeClient reports a build only for slugs listed in withBuild, so we can test
// filtering without the network.
type fakeClient struct {
	withBuild map[string]bool
}

func (f fakeClient) Versions(_ context.Context, slug string, _, _ []string) ([]modrinth.Version, error) {
	if f.withBuild[slug] {
		return []modrinth.Version{{ID: slug + "-v"}}, nil
	}
	return nil, nil
}
func (fakeClient) ResolveMrpack(context.Context, string, string, string) (*modrinth.Mrpack, error) {
	return nil, nil
}
func (fakeClient) IdentifyByHash(context.Context, []string) (map[string]modrinth.Version, error) {
	return nil, nil
}
func (fakeClient) LatestForHashes(context.Context, []string, string, string) (map[string]modrinth.Version, error) {
	return nil, nil
}

func TestModpacksOnlyReturnsBuildable(t *testing.T) {
	// Only two neoforge slugs have a build for this version.
	c := fakeClient{withBuild: map[string]bool{SlugQuackedPack: true, "create_plus": true}}
	offers, err := Modpacks(context.Background(), c, config.LoaderNeoForge, "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 2 {
		t.Fatalf("expected only the 2 buildable packs, got %d: %+v", len(offers), offers)
	}
	if offers[0].Slug != SlugQuackedPack {
		t.Errorf("quackedsmppack should stay first, got %s", offers[0].Slug)
	}
}

func TestModpacksForgeCatalogExists(t *testing.T) {
	// Every forge slug builds → the whole curated forge list comes back.
	all := map[string]bool{}
	for _, p := range featured(config.LoaderForge) {
		all[p.slug] = true
	}
	c := fakeClient{withBuild: all}
	offers, err := Modpacks(context.Background(), c, config.LoaderForge, "1.20.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != len(featured(config.LoaderForge)) || len(offers) < 10 {
		t.Fatalf("forge catalog should be the full curated list (~15), got %d", len(offers))
	}
	if offers[0].Slug != "the-pixelmon-modpack" {
		t.Errorf("forge should lead with the-pixelmon-modpack, got %s", offers[0].Slug)
	}
}

func TestModpacksQuiltReusesFabric(t *testing.T) {
	if len(featured(config.LoaderQuilt)) != len(featured(config.LoaderFabric)) {
		t.Fatal("quilt should reuse the fabric featured list")
	}
}

func TestModpacksVanillaEmpty(t *testing.T) {
	offers, err := Modpacks(context.Background(), fakeClient{}, config.LoaderVanilla, "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Errorf("vanilla should have no modpacks, got %d", len(offers))
	}
}

func TestModpacksSkipsDisabled(t *testing.T) {
	if len(disabledPacks) == 0 {
		t.Skip("no parked packs to exercise")
	}
	d := disabledPacks[0]

	// Give the parked slug a build; it must still be excluded from the offers.
	c := fakeClient{withBuild: map[string]bool{d.slug: true}}
	offers, err := Modpacks(context.Background(), c, d.loader, "1.20.1")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offers {
		if o.Slug == d.slug {
			t.Fatalf("parked pack %q should not be offered", d.slug)
		}
	}
}

func TestAllFeaturedKeepsDisabledInPlace(t *testing.T) {
	if len(disabledPacks) == 0 {
		t.Skip("no parked packs to exercise")
	}
	d := disabledPacks[0]

	var found bool
	for _, p := range AllFeatured() {
		if p.Loader == d.loader && p.Slug == d.slug {
			found = true
			if !p.Disabled {
				t.Errorf("parked pack %q should be marked Disabled in AllFeatured", d.slug)
			}
		}
	}
	if !found {
		t.Fatalf("parked pack %q must stay in AllFeatured to keep the day-index stable", d.slug)
	}
}

func TestModpacksSkipsDelistedSlug(t *testing.T) {
	// No slug has a build → all skipped, no error (resilient to delisted packs).
	offers, err := Modpacks(context.Background(), fakeClient{}, config.LoaderFabric, "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Errorf("expected all skipped, got %d", len(offers))
	}
}
