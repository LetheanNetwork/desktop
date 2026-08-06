// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	"io/fs"
	"sync"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const testDocumentPath = "desktop/state/test.json"

type testDocumentPayload struct {
	Name string `json:"name"`
}

func newTestDocument(medium coreio.Medium) *document[testDocumentPayload] {
	return newDocument(medium, testDocumentPath, documentOptions[testDocumentPayload]{
		Version:      1,
		MaximumBytes: 4 * 1024,
		Validate: func(payload testDocumentPayload) core.Result {
			if payload.Name == "" {
				return core.Fail(core.E(
					"desktopstate.testDocument",
					"name is required",
					nil,
				))
			}
			return core.Ok(payload)
		},
	})
}

func TestDocument_GoodRoundTripUsesRevisionAndRestrictedMode(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)

	saved := document.Save(0, testDocumentPayload{Name: "first"})
	loaded := document.Load()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	savedEnvelope := saved.Value.(documentEnvelope[testDocumentPayload])
	loadedEnvelope := loaded.Value.(documentEnvelope[testDocumentPayload])
	core.AssertEqual(t, uint64(1), savedEnvelope.Revision)
	core.AssertEqual(t, savedEnvelope, loadedEnvelope)
	info, err := medium.Stat(testDocumentPath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, fs.FileMode(0o600), info.Mode().Perm())

	updated := document.Save(1, testDocumentPayload{Name: "second"})
	core.RequireTrue(t, updated.OK, updated.Error())
	core.AssertEqual(
		t,
		uint64(2),
		updated.Value.(documentEnvelope[testDocumentPayload]).Revision,
	)
}

func TestDocument_GoodMissingReturnsVersionedEmptyEnvelope(t *core.T) {
	medium := coreio.NewMemoryMedium()

	result := newTestDocument(medium).Load()

	core.RequireTrue(t, result.OK, result.Error())
	envelope := result.Value.(documentEnvelope[testDocumentPayload])
	core.AssertEqual(t, 1, envelope.Version)
	core.AssertEqual(t, uint64(0), envelope.Revision)
	core.AssertEqual(t, testDocumentPayload{}, envelope.Payload)
	core.AssertFalse(t, medium.IsFile(testDocumentPath))
}

func TestDocument_BadUnavailableMediumFailsClosed(t *core.T) {
	document := newTestDocument(nil)

	loaded := document.Load()
	saved := document.Save(0, testDocumentPayload{Name: "first"})

	core.AssertFalse(t, loaded.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(loaded))
	core.AssertFalse(t, saved.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(saved))
}

func TestDocument_BadMalformedPrimaryPreservesEvidence(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(testDocumentPath, `{"version":1`))

	result := newTestDocument(medium).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(result))
	content, err := medium.Read(testDocumentPath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, `{"version":1`, content)
}

func TestDocument_BadUnsupportedVersionIsNotOverwritten(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(
		testDocumentPath,
		`{"version":2,"revision":1,"updatedAt":"2026-07-27T12:00:00Z","payload":{"name":"future"}}`,
	))

	result := newTestDocument(medium).Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(result))
	content, err := medium.Read(testDocumentPath)
	core.RequireNoError(t, err)
	core.AssertContains(t, content, `"version":2`)
}

func TestDocument_BadRevisionConflictDoesNotWrite(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)
	core.RequireTrue(
		t,
		document.Save(0, testDocumentPayload{Name: "first"}).OK,
	)
	before, err := medium.Read(testDocumentPath)
	core.RequireNoError(t, err)

	result := document.Save(0, testDocumentPayload{Name: "stale"})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateConflict, ErrorCodeOf(result))
	after, err := medium.Read(testDocumentPath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, before, after)
}

func TestDocument_BadRejectsInvalidOrOversizedPayloadBeforeWrite(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)

	invalid := document.Save(0, testDocumentPayload{})
	oversized := document.Save(
		0,
		testDocumentPayload{Name: core.Repeat("x", 5*1024)},
	)

	core.AssertFalse(t, invalid.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(invalid))
	core.AssertFalse(t, oversized.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(oversized))
	core.AssertFalse(t, medium.IsFile(testDocumentPath))
}

func TestDocument_UglyRecoversOnlyValidBackup(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)
	backupPath := document.backupPath()
	core.RequireNoError(t, medium.EnsureDir(document.stagingDir()))
	core.RequireNoError(t, medium.Write(
		backupPath,
		`{"version":1,"revision":3,"updatedAt":"2026-07-27T12:00:00Z","payload":{"name":"recovered"}}`,
	))

	result := document.Load()

	core.RequireTrue(t, result.OK, result.Error())
	envelope := result.Value.(documentEnvelope[testDocumentPayload])
	core.AssertEqual(t, uint64(3), envelope.Revision)
	core.AssertEqual(t, "recovered", envelope.Payload.Name)
	core.AssertTrue(t, medium.IsFile(testDocumentPath))
	core.AssertFalse(t, medium.IsFile(backupPath))
}

func TestDocument_UglyInvalidBackupIsPreserved(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)
	backupPath := document.backupPath()
	core.RequireNoError(t, medium.EnsureDir(document.stagingDir()))
	core.RequireNoError(t, medium.Write(backupPath, `{"version":1`))

	result := document.Load()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(result))
	core.AssertTrue(t, medium.IsFile(backupPath))
	core.AssertFalse(t, medium.IsFile(testDocumentPath))
}

func TestDocument_UglyCommitFailureRestoresPreviousDocument(t *core.T) {
	base := coreio.NewMemoryMedium()
	document := newTestDocument(base)
	first := document.Save(0, testDocumentPayload{Name: "previous"})
	core.RequireTrue(t, first.OK, first.Error())
	before, err := base.Read(testDocumentPath)
	core.RequireNoError(t, err)

	failing := &documentFaultMedium{
		Medium:        base,
		failRenameOld: document.stagedPath(),
		failRenameNew: testDocumentPath,
	}
	result := newTestDocument(failing).Save(
		1,
		testDocumentPayload{Name: "replacement"},
	)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
	after, err := base.Read(testDocumentPath)
	core.RequireNoError(t, err)
	core.AssertEqual(t, before, after)
}

// TestDocument_UglyConcurrentSavesEnforceRevisionGuard proves the revision
// guard that the cross-window session bridge depends on: two windows racing
// a Save against the same loaded revision must not both win. Exactly one
// commits and the loser observes ErrorStateConflict, never a corrupted or
// silently-overwritten document. Run with -race to validate the document's
// mutex actually serialises the staged-write/rename sequence.
func TestDocument_UglyConcurrentSavesEnforceRevisionGuard(t *core.T) {
	medium := coreio.NewMemoryMedium()
	document := newTestDocument(medium)
	core.RequireTrue(
		t,
		document.Save(0, testDocumentPayload{Name: "first"}).OK,
	)

	var wg sync.WaitGroup
	results := make([]core.Result, 2)
	names := []string{"window-a", "window-b"}
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = document.Save(
				1,
				testDocumentPayload{Name: names[index]},
			)
		}(index)
	}
	wg.Wait()

	wins, conflicts := 0, 0
	for _, result := range results {
		switch {
		case result.OK:
			wins++
		case ErrorCodeOf(result) == ErrorStateConflict:
			conflicts++
		}
	}
	core.AssertEqual(t, 1, wins)
	core.AssertEqual(t, 1, conflicts)

	loaded := document.Load()
	core.RequireTrue(t, loaded.OK, loaded.Error())
	core.AssertEqual(
		t,
		uint64(2),
		loaded.Value.(documentEnvelope[testDocumentPayload]).Revision,
	)
}

func TestDocument_BadCommitAndRecoveryBothFail(t *core.T) {
	base := coreio.NewMemoryMedium()
	document := newTestDocument(base)
	first := document.Save(0, testDocumentPayload{Name: "previous"})
	core.RequireTrue(t, first.OK, first.Error())

	failing := &documentFaultMedium{
		Medium:         base,
		failRenameOld:  document.stagedPath(),
		failRenameNew:  testDocumentPath,
		failRenameOld2: document.backupPath(),
		failRenameNew2: testDocumentPath,
	}
	result := newTestDocument(failing).Save(
		1,
		testDocumentPayload{Name: "replacement"},
	)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "commit and recovery failed")
}

func TestDocument_BadStagedWriteFailure(t *core.T) {
	failing := &documentFaultMedium{
		Medium:       coreio.NewMemoryMedium(),
		writeModeErr: fs.ErrPermission,
	}
	document := newTestDocument(failing)

	result := document.Save(0, testDocumentPayload{Name: "first"})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
}

func TestDocument_BadBackupDeleteFailureBeforeRename(t *core.T) {
	base := coreio.NewMemoryMedium()
	document := newTestDocument(base)
	core.RequireTrue(
		t,
		document.Save(0, testDocumentPayload{Name: "first"}).OK,
	)
	// Simulate a stale backup left behind by a previous crash.
	core.RequireNoError(t, base.Write(document.backupPath(), "stale"))

	failing := &documentFaultMedium{
		Medium:    base,
		deleteErr: fs.ErrPermission,
	}
	result := newTestDocument(failing).Save(
		1,
		testDocumentPayload{Name: "second"},
	)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
}

type documentFaultMedium struct {
	coreio.Medium
	failRenameOld  string
	failRenameNew  string
	failRenameOld2 string
	failRenameNew2 string
	writeModeErr   error
	deleteErr      error
}

func (medium *documentFaultMedium) Rename(oldPath, newPath string) error {
	if oldPath == medium.failRenameOld && newPath == medium.failRenameNew {
		return fs.ErrPermission
	}
	if oldPath == medium.failRenameOld2 && newPath == medium.failRenameNew2 {
		return fs.ErrPermission
	}
	return medium.Medium.Rename(oldPath, newPath)
}

func (medium *documentFaultMedium) WriteMode(
	path string,
	content string,
	mode fs.FileMode,
) error {
	if medium.writeModeErr != nil {
		return medium.writeModeErr
	}
	return medium.Medium.WriteMode(path, content, mode)
}

func (medium *documentFaultMedium) Delete(path string) error {
	if medium.deleteErr != nil {
		return medium.deleteErr
	}
	return medium.Medium.Delete(path)
}
