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

// TestCatalogueFailure_Error_Good covers CatalogueFailure.Error() on a
// real instance — the example/happy-path tests only ever inspect the
// code via CatalogueErrorCodeOf, so the Error() string accessor itself
// (part of the `error` interface contract) had never been called
// directly.
func TestCatalogueFailure_Error_Good(t *core.T) {
	failure := &CatalogueFailure{Code: CatalogueUnavailable, Message: "unavailable"}
	core.AssertEqual(t, "unavailable", failure.Error())
}

// TestCatalogueFailure_Error_NilReceiver_Bad — Error() on a nil
// *CatalogueFailure must return "" rather than panicking, matching
// the nil-safe convention CoreGO error wrappers use throughout.
func TestCatalogueFailure_Error_NilReceiver_Bad(t *core.T) {
	var failure *CatalogueFailure
	core.AssertEqual(t, "", failure.Error())
}

func TestCatalogueFailure_Unwrap_Good(t *core.T) {
	cause := core.NewError("root cause")
	failure := &CatalogueFailure{Code: CatalogueUnavailable, Cause: cause}
	core.AssertEqual(t, cause, failure.Unwrap())
}

func TestCatalogueFailure_Unwrap_NilReceiver_Bad(t *core.T) {
	var failure *CatalogueFailure
	core.AssertTrue(t, failure.Unwrap() == nil)
}

// TestCatalogueErrorCodeOf_OKResult_Good — an OK result has no error
// to classify; CodeOf must short-circuit to "" without inspecting
// result.Err().
func TestCatalogueErrorCodeOf_OKResult_Good(t *core.T) {
	core.AssertEqual(t, CatalogueErrorCode(""), CatalogueErrorCodeOf(core.Ok(nil)))
}

// TestCatalogueErrorCodeOf_ForeignError_Bad — a Fail Result wrapping
// an error that is NOT a *CatalogueFailure must classify as "" (the
// core.As branch misses) rather than panicking on a bad type assert.
func TestCatalogueErrorCodeOf_ForeignError_Bad(t *core.T) {
	foreign := core.Fail(core.NewError("some unrelated failure"))
	core.AssertEqual(t, CatalogueErrorCode(""), CatalogueErrorCodeOf(foreign))
}

// TestValidModelName_ControlCharacters_Bad — a name containing a raw
// control byte (below 0x20) or DEL (0x7f) must be rejected even
// though it contains none of the other banned characters.
func TestValidModelName_ControlCharacters_Bad(t *core.T) {
	core.AssertFalse(t, validModelName("model\x01name"))
	core.AssertFalse(t, validModelName("model\x7fname"))
}

// TestValidModelName_TooLong_Bad — a name over maxModelNameBytes must
// be rejected.
func TestValidModelName_TooLong_Bad(t *core.T) {
	long := strings.Repeat("a", maxModelNameBytes+1)
	core.AssertFalse(t, validModelName(long))
}

func TestValidModelName_Valid_Good(t *core.T) {
	core.AssertTrue(t, validModelName("gemma-4-e2b"))
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
