// SPDX-Licence-Identifier: EUPL-1.2

package models

import (
	"strings"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestCatalogue_List_GoodReturnsOpaquePathSafeEntries(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("lem/models/gemma-4-e2b"))
	core.RequireNoError(t, medium.Write(
		"lem/models/gemma-4-e2b/.sha256",
		"digest  weights.safetensors\n",
	))
	core.RequireNoError(t, medium.Write(
		"lem/models/gemma-4-e2b/weights.safetensors",
		"weights",
	))
	core.RequireNoError(t, medium.EnsureDir("lem/models/unverified"))

	catalogue := NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	)
	result := catalogue.List()

	core.RequireTrue(t, result.OK, result.Error())
	entries := result.Value.([]CatalogueEntry)
	core.RequireTrue(t, len(entries) == 2)
	core.AssertEqual(t, "gemma-4-e2b", entries[0].DisplayName)
	core.AssertTrue(t, entries[0].Loadable)
	core.AssertEqual(t, "snapshot", entries[0].Format)
	core.AssertEqual(t, "unverified", entries[1].DisplayName)
	core.AssertFalse(t, entries[1].Loadable)
	for _, entry := range entries {
		core.AssertTrue(t, strings.HasPrefix(entry.ID, "model-"))
		core.AssertNotContains(t, core.Sprintf("%+v", entry), "/trusted/")
		core.AssertNotContains(t, core.Sprintf("%+v", entry), "lem/models")
	}
}

func TestCatalogue_Resolve_GoodReturnsTrustedInternalReference(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("lem/models/gemma-4-e2b"))
	core.RequireNoError(t, medium.Write(
		"lem/models/gemma-4-e2b/.sha256",
		"digest  weights.safetensors\n",
	))
	catalogue := NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	)
	listed := catalogue.List()
	core.RequireTrue(t, listed.OK, listed.Error())
	id := listed.Value.([]CatalogueEntry)[0].ID

	resolved := catalogue.Resolve(id)

	core.RequireTrue(t, resolved.OK, resolved.Error())
	reference := resolved.Value.(Reference)
	core.AssertEqual(t, id, reference.ID)
	core.AssertEqual(t, "gemma-4-e2b", reference.DisplayName)
	core.AssertEqual(t, "lem/models/gemma-4-e2b", reference.RelativePath)
	core.AssertEqual(
		t,
		"/trusted/Lethean/lem/models/gemma-4-e2b",
		reference.NativePath,
	)
	core.AssertEqual(t, "snapshot", reference.Format)
}

func TestCatalogue_Resolve_BadRejectsUnknownAndUnverifiedModels(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("lem/models/unverified"))
	catalogue := NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	)
	listed := catalogue.List()
	core.RequireTrue(t, listed.OK, listed.Error())
	unverifiedID := listed.Value.([]CatalogueEntry)[0].ID

	unknown := catalogue.Resolve("model-0000000000000000")
	core.AssertFalse(t, unknown.OK)
	core.AssertEqual(t, CatalogueModelNotFound, CatalogueErrorCodeOf(unknown))

	unverified := catalogue.Resolve(unverifiedID)
	core.AssertFalse(t, unverified.OK)
	core.AssertEqual(t, CatalogueModelNotLoadable, CatalogueErrorCodeOf(unverified))

	traversal := catalogue.Resolve("../gemma")
	core.AssertFalse(t, traversal.OK)
	core.AssertEqual(t, CatalogueModelNotFound, CatalogueErrorCodeOf(traversal))
}

func TestCatalogue_List_UglyFailsClosedWithoutMediumOrRoots(t *core.T) {
	var missing *Catalogue
	result := missing.List()
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, CatalogueUnavailable, CatalogueErrorCodeOf(result))

	medium := coreio.NewMemoryMedium()
	noProviderRoot := NewCatalogue(medium, "", "/trusted/models").List()
	core.AssertFalse(t, noProviderRoot.OK)
	core.AssertEqual(t, CatalogueUnavailable, CatalogueErrorCodeOf(noProviderRoot))

	noNativeRoot := NewCatalogue(medium, "lem/models", "").List()
	core.AssertFalse(t, noNativeRoot.OK)
	core.AssertEqual(t, CatalogueUnavailable, CatalogueErrorCodeOf(noNativeRoot))

	missingPrefix := NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	).List()
	core.AssertFalse(t, missingPrefix.OK)
	core.AssertEqual(t, CatalogueUnavailable, CatalogueErrorCodeOf(missingPrefix))
}

func TestCatalogue_List_UglyRejectsMoreThanBoundedEntries(t *core.T) {
	medium := coreio.NewMemoryMedium()
	for index := 0; index < maxCatalogueEntries+1; index++ {
		core.RequireNoError(t, medium.EnsureDir(
			core.Sprintf("lem/models/model-%03d", index),
		))
	}
	catalogue := NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	)

	result := catalogue.List()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, CatalogueInvalid, CatalogueErrorCodeOf(result))
}
