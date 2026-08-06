// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestMediumRuntimeMetadata_RoundTripGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		medium,
		"desktop/files/runtime.json",
	)
	snapshot := RuntimeSnapshot{
		Version: 1,
		Favourites: []Favourite{{
			MountID: "documents",
			Path:    "",
		}},
		Recent: []Recent{{
			MountID:  "documents",
			Path:     "notes/readme.md",
			Name:     "readme.md",
			Kind:     EntryFile,
			OpenedAt: "2026-07-26T12:00:00Z",
		}},
		Trash: []TrashReceipt{},
	}

	core.RequireNoError(t, runtime.Save(snapshot))
	got, err := runtime.Load()

	core.AssertNoError(t, err)
	core.AssertEqual(t, snapshot, got)
	core.AssertTrue(t, medium.IsFile("desktop/files/runtime.json"))
}

func TestMediumRuntimeMetadata_BadUnavailableDoesNotReset(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(
		t,
		base.Write("desktop/files/runtime.json", `{"version":1}`),
	)
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, readErr: fs.ErrPermission},
		"desktop/files/runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertTrue(t, base.IsFile("desktop/files/runtime.json"))
}

func TestMediumRuntimeMetadata_BadMalformedDocument(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(
		t,
		medium.Write("runtime.json", `{"version":1,"recent":`),
	)
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertTrue(t, medium.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_UglyRejectsUnknownVersion(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	err := runtime.Save(RuntimeSnapshot{Version: 2})

	core.AssertError(t, err)
	core.AssertFalse(t, medium.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_UglyRecoversValidBackup(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(
		".lthn-files/staging/runtime/older.backup.json",
		`{"version":1,"favourites":[],"recent":[],"trash":[]}`,
	))
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	got, err := runtime.Load()

	core.AssertNoError(t, err)
	core.AssertEqual(t, 1, got.Version)
	core.AssertTrue(t, medium.IsFile("runtime.json"))
}

func TestService_PreviewRecordsRecentGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})
	snapshot, err := runtime.Load()

	core.RequireTrue(t, result.OK)
	core.AssertNoError(t, err)
	core.AssertEqual(t, "readme.md", snapshot.Recent[0].Path)
	core.AssertEqual(t, EntryFile, snapshot.Recent[0].Kind)
}

// opaqueProviderErr is a cause that is neither fs.ErrNotExist nor
// fs.ErrPermission, so providerFailure maps it to the default
// ErrorProviderUnavailable branch instead of a permission- or
// missing-entry-specific code.
func opaqueProviderErr() error {
	return core.E("test.inject", "provider is unavailable", nil)
}

// sequencedRenameMedium fails only the Nth Rename call (1-indexed), letting
// tests isolate Save's backup-rename, commit-rename, and restore-rename
// steps individually even though they share one Medium method.
type sequencedRenameMedium struct {
	coreio.Medium
	calls  int
	failOn map[int]error
}

func (medium *sequencedRenameMedium) Rename(oldPath, newPath string) error {
	medium.calls++
	if err, ok := medium.failOn[medium.calls]; ok {
		return err
	}
	return medium.Medium.Rename(oldPath, newPath)
}

// pathReadFailureMedium fails Read only for one exact path, leaving reads of
// every other path (such as the absent primary document) untouched.
type pathReadFailureMedium struct {
	coreio.Medium
	failPath string
	err      error
}

func (medium *pathReadFailureMedium) Read(path string) (string, error) {
	if path == medium.failPath {
		return "", medium.err
	}
	return medium.Medium.Read(path)
}

func TestMediumRuntimeMetadata_Save_BadEnsureParentFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, ensureDirErr: opaqueProviderErr()},
		"desktop/runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Save_BadStagingDirFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	// No parent segment, so ensureMediumParent short-circuits and the only
	// EnsureDir call left is the staging directory.
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, ensureDirErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Save_BadStagedWriteFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, writeModeErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
	core.AssertFalse(t, base.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_Save_UglyUnexpectedStatFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, statErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Save_BadBackupRenameFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("runtime.json", `{"version":1}`))
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, renameErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
	content, readErr := base.Read("runtime.json")
	core.AssertNoError(t, readErr)
	core.AssertEqual(t, `{"version":1}`, content)
}

func TestMediumRuntimeMetadata_Save_BadCommitRenameFailureNoPriorDocument(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, renameErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
	core.AssertFalse(t, base.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_Save_UglyCommitFailureRestoresBackup(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("runtime.json", `{"version":1}`))
	commitErr := core.E("test", "commit failed", nil)
	medium := &sequencedRenameMedium{
		Medium: base,
		failOn: map[int]error{2: commitErr},
	}
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
	content, readErr := base.Read("runtime.json")
	core.AssertNoError(t, readErr)
	core.AssertEqual(t, `{"version":1}`, content)
}

func TestMediumRuntimeMetadata_Save_UglyCommitAndRestoreBothFail(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("runtime.json", `{"version":1}`))
	commitErr := core.E("test", "commit failed", nil)
	restoreErr := core.E("test", "restore failed", nil)
	medium := &sequencedRenameMedium{
		Medium: base,
		failOn: map[int]error{2: commitErr, 3: restoreErr},
	}
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(
		t,
		err.Error(),
		"commit and recovery both failed",
	)
}

func TestMediumRuntimeMetadata_Save_BadBackupDeleteFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("runtime.json", `{"version":1}`))
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, deleteErr: opaqueProviderErr()},
		"runtime.json",
	)

	err := runtime.Save(RuntimeSnapshot{Version: 1})

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
	// The commit itself succeeded; only the stale backup cleanup failed.
	content, readErr := base.Read("runtime.json")
	core.AssertNoError(t, readErr)
	core.AssertEqual(
		t,
		`{"version":1,"favourites":[],"recent":[],"trash":[]}`,
		content,
	)
}

func TestMediumRuntimeMetadata_Load_BadRecoveryListFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, listErr: opaqueProviderErr()},
		"runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Load_BadRecoveryCandidateReadFailure(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	candidatePath := ".lthn-files/staging/runtime/broken.backup.json"
	core.RequireNoError(
		t,
		base.Write(candidatePath, `{"version":1,"favourites":[],"recent":[],"trash":[]}`),
	)
	medium := &pathReadFailureMedium{
		Medium:   base,
		failPath: candidatePath,
		err:      opaqueProviderErr(),
	}
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Load_UglyOnlyInvalidBackupsFailClosed(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write(
		".lthn-files/staging/runtime/broken.backup.json",
		"not valid json",
	))
	runtime := NewMediumRuntimeMetadata(base, "runtime.json")

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), "no valid backup")
}

func TestMediumRuntimeMetadata_Load_BadRecoveryEnsureParentFailure(
	t *core.T,
) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write(
		".lthn-files/staging/runtime/older.backup.json",
		`{"version":1,"favourites":[],"recent":[],"trash":[]}`,
	))
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, ensureDirErr: opaqueProviderErr()},
		"desktop/runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestMediumRuntimeMetadata_Load_BadRecoveryRenameFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write(
		".lthn-files/staging/runtime/older.backup.json",
		`{"version":1,"favourites":[],"recent":[],"trash":[]}`,
	))
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, renameErr: opaqueProviderErr()},
		"runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorProviderUnavailable))
}

func TestDeleteMediumEntry_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("staged/entry.json", "x"))

	err := deleteMediumEntry(medium, "staged/entry.json")

	core.AssertNoError(t, err)
	core.AssertFalse(t, medium.IsFile("staged/entry.json"))
}

func TestDeleteMediumEntry_GoodMissingEntryIsNotAnError(t *core.T) {
	medium := coreio.NewMemoryMedium()

	err := deleteMediumEntry(medium, "staged/absent.json")

	core.AssertNoError(t, err)
}

func TestDeleteMediumEntry_BadStatFailurePropagates(t *core.T) {
	medium := &failingMedium{
		Medium:  coreio.NewMemoryMedium(),
		statErr: fs.ErrPermission,
	}

	err := deleteMediumEntry(medium, "staged/entry.json")

	core.AssertSame(t, fs.ErrPermission, err)
}

func TestDeleteMediumEntry_BadDeleteFailurePropagates(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("staged/entry.json", "x"))
	medium := &failingMedium{Medium: base, deleteErr: fs.ErrPermission}

	err := deleteMediumEntry(medium, "staged/entry.json")

	core.AssertSame(t, fs.ErrPermission, err)
}

func TestValidateTrustedTrashPath_Good(t *core.T) {
	err := validateTrustedTrashPath(".lthn-files/trash/2026/receipt-1/file.txt")

	core.AssertNoError(t, err)
}

func TestValidateTrustedTrashPath_Bad(t *core.T) {
	for name, path := range map[string]string{
		"outside namespace": "documents/file.txt",
		"empty component":   ".lthn-files/trash//file.txt",
		"dot component":     ".lthn-files/trash/./file.txt",
		"dotdot component":  ".lthn-files/trash/../file.txt",
		"unsafe rune":       ".lthn-files/trash/a‮b",
	} {
		t.Run(name, func(t *core.T) {
			err := validateTrustedTrashPath(path)

			core.AssertError(t, err)
			core.AssertContains(t, err.Error(), string(ErrorBoundaryRejected))
		})
	}
}

func TestValidateAndNormaliseRuntimeSnapshot_Bad(t *core.T) {
	tests := map[string]RuntimeSnapshot{
		"bad favourite mount": {
			Version:    1,
			Favourites: []Favourite{{MountID: "", Path: ""}},
		},
		"bad favourite path": {
			Version:    1,
			Favourites: []Favourite{{MountID: "documents", Path: "../x"}},
		},
		"bad recent mount": {
			Version: 1,
			Recent:  []Recent{{MountID: "", Path: "a.txt"}},
		},
		"bad recent path": {
			Version: 1,
			Recent: []Recent{{
				MountID: "documents",
				Path:    "../a.txt",
			}},
		},
		"bad recent kind": {
			Version: 1,
			Recent: []Recent{{
				MountID:  "documents",
				Path:     "a.txt",
				Kind:     "bogus",
				OpenedAt: "2026-07-26T12:00:00Z",
			}},
		},
		"bad recent timestamp": {
			Version: 1,
			Recent: []Recent{{
				MountID:  "documents",
				Path:     "a.txt",
				Kind:     EntryFile,
				OpenedAt: "not-a-time",
			}},
		},
		"trash missing id": {
			Version: 1,
			Trash: []TrashReceipt{{
				MountID:      "documents",
				OriginalPath: "a.txt",
				TrashPath:    ".lthn-files/trash/1/a.txt",
			}},
		},
		"trash bad mount": {
			Version: 1,
			Trash: []TrashReceipt{{
				ID:           "r1",
				MountID:      "",
				OriginalPath: "a.txt",
				TrashPath:    ".lthn-files/trash/1/a.txt",
			}},
		},
		"trash bad original path": {
			Version: 1,
			Trash: []TrashReceipt{{
				ID:           "r1",
				MountID:      "documents",
				OriginalPath: "../a.txt",
				TrashPath:    ".lthn-files/trash/1/a.txt",
			}},
		},
		"trash bad trash path": {
			Version: 1,
			Trash: []TrashReceipt{{
				ID:           "r1",
				MountID:      "documents",
				OriginalPath: "a.txt",
				TrashPath:    "documents/a.txt",
			}},
		},
	}

	for name, snapshot := range tests {
		t.Run(name, func(t *core.T) {
			_, err := validateAndNormaliseRuntimeSnapshot(snapshot)

			core.AssertError(t, err)
		})
	}
}

func TestValidateAndNormaliseRuntimeSnapshot_GoodDedupesAndOrdersRecent(
	t *core.T,
) {
	snapshot := RuntimeSnapshot{
		Version: 1,
		Recent: []Recent{
			{
				MountID:  "documents",
				Path:     "old.txt",
				Kind:     EntryFile,
				OpenedAt: "2026-07-25T12:00:00Z",
			},
			{
				MountID:  "documents",
				Path:     "new.txt",
				Kind:     EntryFile,
				OpenedAt: "2026-07-26T12:00:00Z",
			},
			{
				MountID:  "documents",
				Path:     "new.txt",
				Kind:     EntryFile,
				OpenedAt: "2026-07-24T12:00:00Z",
			},
		},
	}

	got, err := validateAndNormaliseRuntimeSnapshot(snapshot)

	core.AssertNoError(t, err)
	core.RequireTrue(t, len(got.Recent) == 2)
	core.AssertEqual(t, "new.txt", got.Recent[0].Path)
	core.AssertEqual(t, "old.txt", got.Recent[1].Path)
}

func TestService_PreviewRecordsRecent_BadSaveFailure(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	runtime := &stubRuntimeMetadata{saveErr: fs.ErrPermission}
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
}
