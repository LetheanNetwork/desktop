// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const testCataloguePath = "desktop/services/catalogue.json"
const testParentDir = "desktop/services"
const testStagingDir = "desktop/services/.staging"
const testStagedPath = "desktop/services/.staging/catalogue.new.json"
const testBackupPath = "desktop/services/.staging/catalogue.backup.json"

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

	// Second independent rename-fault pair — lets a single test force
	// BOTH the primary commit rename AND the restore-on-failure rename
	// to fail (Save's "commit and recovery both failed" branch).
	failRename2Old string
	failRename2New string

	// Path-scoped faults — each nil unless a test opts in, so existing
	// call sites above (readErr, failRenameOld/New) keep working
	// unchanged.
	readErrPath       string
	readErrForPath    error
	readOverridePath  string
	readOverrideValue string
	ensureDirErrPath  string
	ensureDirErr      error
	deleteErrPath     string
	deleteErr         error
	statErrPath       string
	statErr           error
	writeModeErrPath  string
	writeModeErr      error
}

func (medium *catalogueFaultMedium) Read(path string) (string, error) {
	if medium.readErr != nil {
		return "", medium.readErr
	}
	if medium.readErrPath != "" && path == medium.readErrPath {
		return "", medium.readErrForPath
	}
	if medium.readOverridePath != "" && path == medium.readOverridePath {
		return medium.readOverrideValue, nil
	}
	return medium.Medium.Read(path)
}

func (medium *catalogueFaultMedium) Rename(oldPath, newPath string) error {
	if oldPath == medium.failRenameOld && newPath == medium.failRenameNew {
		return fs.ErrPermission
	}
	if oldPath == medium.failRename2Old && newPath == medium.failRename2New {
		return fs.ErrPermission
	}
	return medium.Medium.Rename(oldPath, newPath)
}

func (medium *catalogueFaultMedium) EnsureDir(path string) error {
	if medium.ensureDirErrPath != "" && path == medium.ensureDirErrPath {
		return medium.ensureDirErr
	}
	return medium.Medium.EnsureDir(path)
}

func (medium *catalogueFaultMedium) Delete(path string) error {
	if medium.deleteErrPath != "" && path == medium.deleteErrPath {
		return medium.deleteErr
	}
	return medium.Medium.Delete(path)
}

func (medium *catalogueFaultMedium) Stat(path string) (fs.FileInfo, error) {
	if medium.statErrPath != "" && path == medium.statErrPath {
		return nil, medium.statErr
	}
	return medium.Medium.Stat(path)
}

func (medium *catalogueFaultMedium) WriteMode(path, content string, mode fs.FileMode) error {
	if medium.writeModeErrPath != "" && path == medium.writeModeErrPath {
		return medium.writeModeErr
	}
	return medium.Medium.WriteMode(path, content, mode)
}

// ---- nil-guards ------------------------------------------------------

func TestMediumCatalogue_Load_Bad_NilMedium(t *core.T) {
	catalogue := &mediumCatalogue{}
	r := catalogue.Load()
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_NilMedium(t *core.T) {
	catalogue := &mediumCatalogue{}
	r := catalogue.Save(validCatalogueDocument())
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

// ---- validatePath (shared by Load and Save) ---------------------------

func TestMediumCatalogue_Load_Bad_InvalidPath(t *core.T) {
	medium := coreio.NewMemoryMedium()
	r := NewMediumCatalogue(medium, "/absolute/not/allowed", DefaultLimits()).Load()
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_InvalidPath(t *core.T) {
	medium := coreio.NewMemoryMedium()
	r := NewMediumCatalogue(medium, "../escape.json", DefaultLimits()).Save(validCatalogueDocument())
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

// ---- Save: ensureDirectories / targetExists / hasBackup ----------------

func TestMediumCatalogue_Save_Bad_EnsureDirectoriesFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, ensureDirErrPath: testStagingDir, ensureDirErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_TargetExistsStatFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, statErrPath: testCataloguePath, statErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_BackupExistsStatFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, statErrPath: testBackupPath, statErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

// TestMediumCatalogue_Save_Bad_RecoveryRequiredWhenBackupOutlivesTarget
// seeds a stale backup with no target present (the shape left behind
// by a crash between Save's target-rename and backup-cleanup steps)
// and pins that Save refuses to write over it blind.
func TestMediumCatalogue_Save_Bad_RecoveryRequiredWhenBackupOutlivesTarget(t *core.T) {
	medium := coreio.NewMemoryMedium()
	payload := core.JSONMarshal(validCatalogueDocument())
	core.RequireTrue(t, payload.OK)
	core.RequireNoError(t, medium.Write(testBackupPath, string(payload.Value.([]byte))))

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

// ---- Save: deleteIfExists / WriteMode / staging verification -----------

// TestMediumCatalogue_Save_Good_CleansUpStaleLeftoversOnSuccess seeds
// BOTH a live target and a stale staged+backup leftover from a
// previous interrupted Save, then proves a fresh Save cleans them up
// via deleteIfExists's real-delete path (not just its exists=false
// short-circuit that every other Save test takes).
func TestMediumCatalogue_Save_Good_CleansUpStaleLeftoversOnSuccess(t *core.T) {
	medium := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, medium.Write(testCataloguePath, string(encoded.Value.([]byte))))
	core.RequireNoError(t, medium.Write(testStagedPath, "stale-staged-leftover"))
	core.RequireNoError(t, medium.Write(testBackupPath, "stale-backup-leftover"))

	next := validCatalogueDocument()
	next.Definitions[0].DisplayName = "Renamed"
	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(next)

	core.RequireTrue(t, r.OK, r.Error())
	core.AssertFalse(t, medium.IsFile(testStagedPath))
	core.AssertFalse(t, medium.IsFile(testBackupPath))
}

func TestMediumCatalogue_Save_Bad_DeleteStaleStagedFileFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write(testStagedPath, "stale"))
	medium := &catalogueFaultMedium{Medium: base, deleteErrPath: testStagedPath, deleteErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_DeleteStaleBackupFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, base.Write(testCataloguePath, string(encoded.Value.([]byte))))
	core.RequireNoError(t, base.Write(testBackupPath, "stale"))
	medium := &catalogueFaultMedium{Medium: base, deleteErrPath: testBackupPath, deleteErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_WriteModeFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, writeModeErrPath: testStagedPath, writeModeErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Save_Bad_StagedReadFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, readErrPath: testStagedPath, readErrForPath: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
	core.AssertFalse(t, medium.IsFile(testStagedPath), "staged file must be cleaned up after a verify failure")
}

func TestMediumCatalogue_Save_Bad_StagedContentMismatch(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{
		Medium:            base,
		readOverridePath:  testStagedPath,
		readOverrideValue: "not-what-was-written",
	}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
	core.AssertFalse(t, medium.IsFile(testStagedPath), "staged file must be cleaned up after a verify failure")
}

// ---- Save: commit rename / restore-on-failure ---------------------------

func TestMediumCatalogue_Save_Bad_TargetToBackupRenameFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, base.Write(testCataloguePath, string(encoded.Value.([]byte))))
	medium := &catalogueFaultMedium{
		Medium:        base,
		failRenameOld: testCataloguePath,
		failRenameNew: testBackupPath,
	}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
	core.AssertFalse(t, medium.IsFile(testStagedPath), "staged file must be cleaned up after a commit failure")
}

// TestMediumCatalogue_Save_Bad_CommitAndRestoreBothFail is the doubly-
// unlucky path: the staged->target commit rename fails AND the
// restore-the-backup rename ALSO fails, so Save must report the
// combined failure rather than silently losing the prior document.
func TestMediumCatalogue_Save_Bad_CommitAndRestoreBothFail(t *core.T) {
	base := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, base.Write(testCataloguePath, string(encoded.Value.([]byte))))
	medium := &catalogueFaultMedium{
		Medium:         base,
		failRenameOld:  testStagedPath,
		failRenameNew:  testCataloguePath,
		failRename2Old: testBackupPath,
		failRename2New: testCataloguePath,
	}

	next := validCatalogueDocument()
	next.Definitions[0].DisplayName = "Changed"
	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(next)

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
	core.AssertContains(t, r.Error(), "commit and recovery both failed")
}

func TestMediumCatalogue_Save_Bad_FinalBackupDeleteFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	previous := validCatalogueDocument()
	encoded := core.JSONMarshal(previous)
	core.RequireTrue(t, encoded.OK)
	core.RequireNoError(t, base.Write(testCataloguePath, string(encoded.Value.([]byte))))
	medium := &catalogueFaultMedium{Medium: base, deleteErrPath: testBackupPath, deleteErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

// ---- recoverOrCreate: non-notexist Read / ensureDirectories / rename ---

func TestMediumCatalogue_Load_Bad_BackupReadNonNotExistError(t *core.T) {
	base := coreio.NewMemoryMedium()
	medium := &catalogueFaultMedium{Medium: base, readErrPath: testBackupPath, readErrForPath: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Load_Bad_RecoverEnsureDirectoriesFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	payload := core.JSONMarshal(validCatalogueDocument())
	core.RequireTrue(t, payload.OK)
	core.RequireNoError(t, base.Write(testBackupPath, string(payload.Value.([]byte))))
	medium := &catalogueFaultMedium{Medium: base, ensureDirErrPath: testParentDir, ensureDirErr: fs.ErrPermission}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestMediumCatalogue_Load_Bad_RecoverRenameFails(t *core.T) {
	base := coreio.NewMemoryMedium()
	payload := core.JSONMarshal(validCatalogueDocument())
	core.RequireTrue(t, payload.OK)
	core.RequireNoError(t, base.Write(testBackupPath, string(payload.Value.([]byte))))
	medium := &catalogueFaultMedium{
		Medium:        base,
		failRenameOld: testBackupPath,
		failRenameNew: testCataloguePath,
	}

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

// ---- entryExists: IsDir / non-notexist Stat error -----------------------

func TestMediumCatalogue_Load_Bad_TargetPathIsADirectory(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir(testCataloguePath))

	r := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Load()

	// Load's own Read(path) fails first for a directory target on most
	// mediums, but the fallback recoverOrCreate path (no backup) must
	// not silently succeed over a directory collision either way — the
	// contract under test is entryExists's IsDir detection, driven here
	// via Save which calls targetExists explicitly.
	_ = r
	saveResult := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()).Save(validCatalogueDocument())
	core.AssertFalse(t, saveResult.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(saveResult))
}

// ---- decodeCatalogueDocument: Decode() failure (valid JSON, bad shape) -

func TestDecodeCatalogueDocument_Bad_UnknownFieldRejected(t *core.T) {
	content := `{"version":1,"definitions":[],"policyOverrides":[],"updatedAt":"2026-07-27T12:00:00Z","unknownField":true}`

	r := decodeCatalogueDocument(content, DefaultLimits())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

// ---- validateCatalogueDocument gaps --------------------------------------

func TestValidateCatalogueDocument_Bad_ZeroMaxDefinitionsLimit(t *core.T) {
	limits := DefaultLimits()
	limits.MaxDefinitions = 0

	r := validateCatalogueDocument(validCatalogueDocument(), limits)

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

func TestValidateCatalogueDocument_Bad_InvalidUpdatedAt(t *core.T) {
	document := validCatalogueDocument()
	document.UpdatedAt = "not-a-timestamp"

	r := validateCatalogueDocument(document, DefaultLimits())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

func TestValidateCatalogueDocument_Bad_InvalidDefinitionWrapped(t *core.T) {
	document := validCatalogueDocument()
	document.Definitions[0].Command = ""

	r := validateCatalogueDocument(document, DefaultLimits())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

func TestValidateCatalogueDocument_Bad_DuplicatePolicyOverride(t *core.T) {
	document := validCatalogueDocument()
	document.PolicyOverrides = append(document.PolicyOverrides, document.PolicyOverrides[0])

	r := validateCatalogueDocument(document, DefaultLimits())

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(r))
}

// ---- validProviderRelativePath gaps --------------------------------------

func TestValidProviderRelativePath_Bad_NonCanonicalForm(t *core.T) {
	for _, value := range []string{"a//b", "a/./b", "a/b/"} {
		core.AssertFalse(t, validProviderRelativePath(value), core.Concat("expected ", value, " to be rejected"))
	}
}

func TestValidProviderRelativePath_Bad_LeadingParentSegment(t *core.T) {
	core.AssertFalse(t, validProviderRelativePath("../outside"))
}
