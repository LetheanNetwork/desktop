// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const testCataloguePath = "desktop/services/catalogue.json"

func validCatalogueDocument() CatalogueDocument {
	return CatalogueDocument{
		Version:     CatalogueVersion,
		Definitions: []Definition{validDefinition()},
		PolicyOverrides: []PolicyOverride{{
			ID:                "local-api",
			RestartPolicy:     RestartOnFailure,
			GracePeriodMillis: 10_000,
		}},
		UpdatedAt: "2026-07-27T12:00:00Z",
	}
}

func TestMediumCatalogue_GoodRoundTripUsesProviderRelativePath(t *core.T) {
	medium := coreio.NewMemoryMedium()
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())
	document := validCatalogueDocument()

	saved := catalogue.Save(document)
	loaded := catalogue.Load()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	core.AssertEqual(t, document, loaded.Value.(CatalogueDocument))
	core.AssertTrue(t, medium.IsFile(testCataloguePath))
	info, err := medium.Stat(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, fs.FileMode(0o600), info.Mode().Perm())
}

func TestMediumCatalogue_GoodMissingDocumentReturnsFreshEmptyVersion(t *core.T) {
	medium := coreio.NewMemoryMedium()

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.RequireTrue(t, result.OK, result.Error())
	document := result.Value.(CatalogueDocument)
	core.AssertEqual(t, CatalogueVersion, document.Version)
	core.AssertEqual(t, 0, len(document.Definitions))
	core.AssertEqual(t, 0, len(document.PolicyOverrides))
	core.AssertFalse(t, medium.IsFile(testCataloguePath))
}

func TestMediumCatalogue_BadMalformedDocumentFailsClosed(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(testCataloguePath, `{"version":1`))

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(result))
	content, err := medium.Read(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, `{"version":1`, content)
}

func TestMediumCatalogue_BadUnsupportedVersionFailsClosed(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(
		testCataloguePath,
		`{"version":2,"definitions":[],"policyOverrides":[],"updatedAt":"2026-07-27T12:00:00Z"}`,
	))

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(result))
}

func TestMediumCatalogue_UglyRecoversOnlyAValidatedBackup(t *core.T) {
	medium := coreio.NewMemoryMedium()
	payload := core.JSONMarshal(validCatalogueDocument())
	core.RequireTrue(t, payload.OK)
	core.RequireNoError(t, medium.Write(
		"desktop/services/.staging/catalogue.backup.json",
		string(payload.Value.([]byte)),
	))

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, validCatalogueDocument(), result.Value.(CatalogueDocument))
	core.AssertTrue(t, medium.IsFile(testCataloguePath))
	core.AssertFalse(t, medium.IsFile("desktop/services/.staging/catalogue.backup.json"))
}

func TestMediumCatalogue_UglyInvalidBackupIsPreservedAndFailsClosed(t *core.T) {
	medium := coreio.NewMemoryMedium()
	backupPath := "desktop/services/.staging/catalogue.backup.json"
	core.RequireNoError(t, medium.Write(backupPath, `{"version":1`))

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(result))
	core.AssertTrue(t, medium.IsFile(backupPath))
	core.AssertFalse(t, medium.IsFile(testCataloguePath))
}

func TestMediumCatalogue_UglyCommitFailureRestoresPreviousDocument(t *core.T) {
	base := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	previous.UpdatedAt = "2026-07-27T11:00:00Z"
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, base.Write(testCataloguePath, string(encoded.Value.([]byte))))
	medium := &catalogueFaultMedium{
		Medium:        base,
		failRenameOld: "desktop/services/.staging/catalogue.new.json",
		failRenameNew: testCataloguePath,
	}
	next := validCatalogueDocument()
	next.Definitions[0].DisplayName = "Changed"

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(next)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(result))
	content, err := base.Read(testCataloguePath)
	core.RequireNoError(t, err)
	var restored CatalogueDocument
	core.RequireTrue(t, core.JSONUnmarshalString(content, &restored).OK)
	core.AssertEqual(t, previous, restored)
}

func TestMediumCatalogue_BadProviderFailureDoesNotResetOrOverwriteEvidence(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write(testCataloguePath, `{"version":1}`))
	medium := &catalogueFaultMedium{Medium: base, readErr: fs.ErrPermission}

	result := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(result))
	content, err := base.Read(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, `{"version":1}`, content)
}

func TestMediumCatalogue_BadRejectsDuplicateDefinitionsAndInvalidOverrides(t *core.T) {
	medium := coreio.NewMemoryMedium()
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())

	duplicate := validCatalogueDocument()
	duplicate.Definitions = append(duplicate.Definitions, cloneDefinition(duplicate.Definitions[0]))
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(catalogue.Save(duplicate)))

	invalidPolicy := validCatalogueDocument()
	invalidPolicy.PolicyOverrides[0].RestartPolicy = RestartPolicy("sometimes")
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(catalogue.Save(invalidPolicy)))

	core.AssertFalse(t, medium.IsFile(testCataloguePath))
}

func TestMediumCatalogue_GoodPersistsNoRuntimeOrOutputState(t *core.T) {
	medium := coreio.NewMemoryMedium()
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())

	core.RequireTrue(t, catalogue.Save(validCatalogueDocument()).OK)
	content, err := medium.Read(testCataloguePath)
	core.RequireNoError(t, err)

	for _, forbidden := range []string{
		`"desired"`, `"pid"`, `"processId"`, `"output"`, `"lastError"`,
		`"environment"`, `"credentials"`,
	} {
		core.AssertNotContains(t, content, forbidden)
	}
}

type catalogueFaultMedium struct {
	coreio.Medium
	readErr       error
	failRenameOld string
	failRenameNew string
}

func (medium *catalogueFaultMedium) Read(path string) (string, error) {
	if medium.readErr != nil {
		return "", medium.readErr
	}
	return medium.Medium.Read(path)
}

func (medium *catalogueFaultMedium) Rename(oldPath, newPath string) error {
	if oldPath == medium.failRenameOld && newPath == medium.failRenameNew {
		return fs.ErrPermission
	}
	return medium.Medium.Rename(oldPath, newPath)
}
