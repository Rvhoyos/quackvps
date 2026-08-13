package modrinth

import "context"

// ResolveRequired walks a version's required dependencies (recursively) and
// returns the newest compatible version of each, deduplicated by project. It's a
// free function over the Client interface so it composes with any client, real
// or fake. Optional/incompatible/embedded deps are ignored, only "required" is
// forced.
func ResolveRequired(ctx context.Context, c Client, root Version, loaders, gameVersions []string) ([]Version, error) {
	resolved := map[string]Version{}
	seen := map[string]bool{root.ProjectID: true}

	queue := []Version{root}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]

		for _, dep := range v.Dependencies {
			if dep.Type != "required" || dep.ProjectID == "" || seen[dep.ProjectID] {
				continue
			}
			seen[dep.ProjectID] = true

			versions, err := c.Versions(ctx, dep.ProjectID, loaders, gameVersions)
			if err != nil {
				return nil, err
			}
			if len(versions) == 0 {
				continue // no compatible build; surfaced elsewhere as unresolved
			}
			best := versions[0]
			resolved[dep.ProjectID] = best
			queue = append(queue, best)
		}
	}

	out := make([]Version, 0, len(resolved))
	for _, v := range resolved {
		out = append(out, v)
	}
	return out, nil
}
