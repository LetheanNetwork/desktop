// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	"io/fs"

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

type documentFaultMedium struct {
	coreio.Medium
	failRenameOld string
	failRenameNew string
}

func (medium *documentFaultMedium) Rename(oldPath, newPath string) error {
	if oldPath == medium.failRenameOld && newPath == medium.failRenameNew {
		return fs.ErrPermission
	}
	return medium.Medium.Rename(oldPath, newPath)
}
