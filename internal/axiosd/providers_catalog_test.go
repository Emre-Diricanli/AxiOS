package axiosd

import (
	"testing"

	"github.com/axios-os/axios/pkg/providers"
)

// TestCatalogCoversRegisteredProfiles guards against drift between the
// profile registry (pkg/providers/builtin.go) and providerCatalog(): a
// profile with env credentials but no catalog entry can never receive a
// key — seedEnvCredentials skips it and the UI never lists it.
func TestCatalogCoversRegisteredProfiles(t *testing.T) {
	catalog := make(map[string]bool)
	for _, p := range providerCatalog() {
		catalog[p.ID] = true
	}

	for _, profile := range providers.List() {
		if len(profile.EnvVars) == 0 {
			continue // local/custom profiles take no API key
		}
		if !catalog[profile.Name] {
			t.Errorf("profile %q accepts credentials via %v but has no providerCatalog() entry; its keys can never be configured",
				profile.Name, profile.EnvVars)
		}
	}
}
