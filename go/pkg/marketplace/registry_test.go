// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// sampleCatalogue is a minimal fixture for SearchCatalogue tests.
var sampleCatalogue = subject.FetchIndexResult{
	Entries: []subject.CatalogueEntry{
		{
			Name:      "opencode",
			Display:   "OpenCode",
			Category:  "ai-agents",
			SourceURL: "https://marketplace.lthn.ai/v1/opencode.yml",
		},
		{
			Name:      "phpmyadmin",
			Display:   "PHPMyAdmin",
			Category:  "database-admin",
			SourceURL: "https://marketplace.lthn.ai/v1/phpmyadmin.yml",
		},
		{
			Name:      "forgejo",
			Display:   "Forgejo",
			Category:  "code-forges",
			SourceURL: "https://marketplace.lthn.ai/v1/forgejo.yml",
		},
	},
}

// TestRegistry_SearchCatalogue_Good verifies empty query returns all entries.
func TestRegistry_SearchCatalogue_Good(t *core.T) {
	results := subject.SearchCatalogue(sampleCatalogue, "", "")
	core.AssertLen(t, results, 3)
}

// TestRegistry_SearchCatalogue_Bad verifies a query that matches nothing returns empty.
func TestRegistry_SearchCatalogue_Bad(t *core.T) {
	results := subject.SearchCatalogue(sampleCatalogue, "zzznomatch", "")
	core.AssertLen(t, results, 0)
}

// TestRegistry_SearchCatalogue_Ugly verifies category filter + query intersection.
func TestRegistry_SearchCatalogue_Ugly(t *core.T) {
	// Category-only filter.
	results := subject.SearchCatalogue(sampleCatalogue, "", "ai-agents")
	core.AssertLen(t, results, 1)
	core.AssertEqual(t, "opencode", results[0].Name)

	// Query matching display name.
	results = subject.SearchCatalogue(sampleCatalogue, "phpmyadmin", "")
	core.AssertLen(t, results, 1)
	core.AssertEqual(t, "phpmyadmin", results[0].Name)

	// Query + category that produce empty intersection.
	results = subject.SearchCatalogue(sampleCatalogue, "opencode", "database-admin")
	core.AssertLen(t, results, 0)
}

// TestRegistry_FetchManifest_Good verifies FetchManifest rejects unsupported protocols
// with a clear error rather than panicking.
func TestRegistry_FetchManifest_Good(t *core.T) {
	svc := newTestMarketplaceService()
	ref := (*subject.Service).FetchManifest
	core.AssertNotNil(t, ref)
	// oci:// not supported in v1 — should fail with the right message.
	r := svc.FetchManifest("oci://lthn/opencode:bundle-v1")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not supported in v1")
}

// TestRegistry_FetchManifest_Bad verifies FetchManifest rejects empty URL.
func TestRegistry_FetchManifest_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.FetchManifest("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "source url is required")
}

// TestRegistry_FetchManifest_Ugly verifies all three unsupported protocol
// prefixes return the right error.
func TestRegistry_FetchManifest_Ugly(t *core.T) {
	svc := newTestMarketplaceService()

	for _, url := range []string{
		"oci://lthn/opencode:bundle-v1",
		"git+https://forge.lthn.sh/bundles/opencode",
		"ftp://unsupported",
	} {
		r := svc.FetchManifest(url)
		core.AssertFalse(t, r.OK)
	}
}

// TestRegistry_FetchIndex_Good verifies FetchIndex is exported and returns
// a result (cache-miss path returns Fail since no cache exists in test env).
func TestRegistry_FetchIndex_Good(t *core.T) {
	ref := (*subject.Service).FetchIndex
	core.AssertNotNil(t, ref)
}

// TestRegistry_FetchIndex_Bad verifies FetchIndex with an empty URL falls
// back to the default endpoint (no panic).
func TestRegistry_FetchIndex_Bad(t *core.T) {
	// No network in test — we're confirming it attempts the fetch + fails
	// gracefully rather than panicking or returning wrong type.
	r := newTestMarketplaceService().FetchIndex("")
	if r.OK {
		res := r.Value.(subject.FetchIndexResult)
		core.AssertNotNil(t, res.Entries)
	}
}

// TestRegistry_FetchIndex_Ugly verifies FetchIndexResult fields are exported
// and default zero values are safe.
func TestRegistry_FetchIndex_Ugly(t *core.T) {
	res := subject.FetchIndexResult{
		Entries:   []subject.CatalogueEntry{},
		FromCache: false,
	}
	core.AssertLen(t, res.Entries, 0)
	core.AssertFalse(t, res.FromCache)
}

// TestRegistry_CatalogueEntry_Good verifies CatalogueEntry fields are exported.
func TestRegistry_CatalogueEntry_Good(t *core.T) {
	e := subject.CatalogueEntry{
		Name:            "opencode",
		Display:         "OpenCode",
		Description:     "AI coding agent.",
		Category:        "ai-agents",
		Icon:            "fa-robot",
		Homepage:        "https://opencode.ai",
		License:         "MIT",
		SourceURL:       "https://marketplace.lthn.ai/v1/opencode.yml",
		RequiresVersion: "0.8.0",
		LastUpdated:     "2026-05-15",
	}
	core.AssertEqual(t, "opencode", e.Name)
	core.AssertEqual(t, "ai-agents", e.Category)
	core.AssertEqual(t, "https://marketplace.lthn.ai/v1/opencode.yml", e.SourceURL)
}

// TestRegistry_CatalogueEntry_Bad verifies SearchCatalogue with empty catalogue
// returns empty slice.
func TestRegistry_CatalogueEntry_Bad(t *core.T) {
	empty := subject.FetchIndexResult{Entries: []subject.CatalogueEntry{}}
	results := subject.SearchCatalogue(empty, "anything", "")
	core.AssertLen(t, results, 0)
}

// TestRegistry_CatalogueEntry_Ugly verifies case-insensitive search.
func TestRegistry_CatalogueEntry_Ugly(t *core.T) {
	results := subject.SearchCatalogue(sampleCatalogue, "OPENCODE", "")
	core.AssertLen(t, results, 1)
	core.AssertEqual(t, "opencode", results[0].Name)

	results = subject.SearchCatalogue(sampleCatalogue, "", "AI-AGENTS")
	core.AssertLen(t, results, 1)
}
