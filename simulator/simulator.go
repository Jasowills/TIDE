// Package simulator bundles the FleetSim scenario library for serving paths
// (API, engine). Same YAML the CLI uses — one source of truth.
package simulator

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed scenarios/*.yaml
var scenariosFS embed.FS

// Get returns the raw YAML for a named scenario.
func Get(name string) ([]byte, error) {
	name = strings.TrimSuffix(name, ".yaml")
	raw, err := scenariosFS.ReadFile("scenarios/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("simulator: unknown scenario %q", name)
	}
	return raw, nil
}

// Names lists available scenarios.
func Names() []string {
	entries, err := scenariosFS.ReadDir("scenarios")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(out)
	return out
}
