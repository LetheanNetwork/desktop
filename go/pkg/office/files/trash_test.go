// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestService_TrashRestore_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("notes/idea.md", "ship it"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	trashed := service.Trash(TrashInput{
		MountID: "documents",
		Path:    "notes/idea.md",
	})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	listed := service.ListTrash()
	core.RequireTrue(t, listed.OK)
	trash := listed.Value.(TrashSnapshot)
	core.AssertEqual(t, receiptID, trash.Entries[0].ReceiptID)
	core.AssertTrue(t, trash.Entries[0].Available)

	restored := service.Restore(RestoreInput{ReceiptID: receiptID})
	core.RequireTrue(t, restored.OK)
	content, err := medium.Read("notes/idea.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "ship it", content)

	listed = service.ListTrash()
	core.RequireTrue(t, listed.OK)
	core.AssertEqual(t, 0, len(listed.Value.(TrashSnapshot).Entries))
}

func TestService_Trash_BadUnownedNamespace(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(
		t,
		medium.Write(".lthn-files/unrelated", "foreign data"),
	)
	core.RequireNoError(t, medium.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	result := service.Trash(TrashInput{
		MountID: "documents",
		Path:    "report.md",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
	content, err := medium.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "content", content)
}

func TestService_Trash_BadRuntimeSaveRollsBack(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "content"))
	runtime := &stubRuntimeMetadata{saveErr: fs.ErrPermission}
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	result := service.Trash(TrashInput{
		MountID: "documents",
		Path:    "report.md",
	})

	core.AssertFalse(t, result.OK)
	content, err := medium.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "content", content)
}

func TestService_Restore_BadConflict(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "old"))
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())
	trashed := service.Trash(TrashInput{
		MountID: "documents",
		Path:    "report.md",
	})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID
	core.RequireNoError(t, medium.Write("report.md", "new"))

	result := service.Restore(RestoreInput{ReceiptID: receiptID})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationConflict, operation.Status)
	content, err := medium.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "new", content)
}

func TestService_ListTrash_UglyMarksStaleReceipt(t *core.T) {
	runtime := NewMemoryRuntimeMetadata()
	core.RequireNoError(t, runtime.Save(RuntimeSnapshot{
		Version: 1,
		Trash: []TrashReceipt{{
			ID:           "receipt-1",
			MountID:      "documents",
			OriginalPath: "report.md",
			TrashPath:    ".lthn-files/trash/receipt-1/payload",
			TrashedAt:    "2026-07-26T12:00:00Z",
		}},
	}))
	service := registeredService(t, []Mount{
		memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, runtime)

	result := service.ListTrash()

	core.RequireTrue(t, result.OK)
	entry := result.Value.(TrashSnapshot).Entries[0]
	core.AssertFalse(t, entry.Available)
	core.AssertEqual(t, ErrorMissingEntry, entry.ErrorCode)
}

func TestService_Delete_UglyRequiresRecursiveConfirmation(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("archive/report.md", "content"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	for _, input := range []DeleteInput{
		{MountID: "documents", Path: "archive"},
		{
			MountID:   "documents",
			Path:      "archive",
			Recursive: true,
		},
		{
			MountID:   "documents",
			Path:      "archive",
			Confirmed: true,
		},
	} {
		result := service.Delete(input)
		core.AssertFalse(t, result.OK)
		core.AssertContains(t, result.Error(), string(ErrorInvalidInput))
	}
	core.AssertTrue(t, medium.IsFile("archive/report.md"))
}

func TestService_DeleteTrashReceipt_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "content"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)
	trashed := service.Trash(TrashInput{
		MountID: "documents",
		Path:    "report.md",
	})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	deleted := service.Delete(DeleteInput{
		ReceiptID: receiptID,
		Confirmed: true,
	})

	core.RequireTrue(t, deleted.OK)
	listed := service.ListTrash()
	core.RequireTrue(t, listed.OK)
	core.AssertEqual(t, 0, len(listed.Value.(TrashSnapshot).Entries))
}

func TestService_DeleteDirect_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("archive/report.md", "content"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "archive",
		Recursive: true,
		Confirmed: true,
	})

	core.RequireTrue(t, result.OK)
	core.AssertFalse(t, medium.IsDir("archive"))
	core.AssertFalse(t, medium.IsFile("archive/report.md"))
}

func TestValidateReceiptID_Good(t *core.T) {
	core.AssertNoError(t, validateReceiptID("receipt-1"))
}

func TestValidateReceiptID_Bad(t *core.T) {
	for name, id := range map[string]string{
		"empty":         "",
		"forward slash": "a/b",
		"backslash":     `a\b`,
		"unsafe rune":   "a\x01b",
	} {
		t.Run(name, func(t *core.T) {
			err := validateReceiptID(id)

			core.AssertError(t, err)
			core.AssertContains(t, err.Error(), string(ErrorInvalidInput))
		})
	}
}

// statPathFailureMedium fails Stat only for one exact path, leaving
// every other Stat (such as the trash payload check that precedes it)
// on the real medium.
type statPathFailureMedium struct {
	coreio.Medium
	failPath string
	err      error
}

func (medium *statPathFailureMedium) Stat(path string) (fs.FileInfo, error) {
	if path == medium.failPath {
		return nil, medium.err
	}
	return medium.Medium.Stat(path)
}

func TestService_Trash_BadInputs(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	unknownMount := service.Trash(TrashInput{MountID: "ghost", Path: "report.md"})
	core.AssertFalse(t, unknownMount.OK)

	escape := service.Trash(TrashInput{MountID: "documents", Path: "../escape"})
	core.AssertFalse(t, escape.OK)

	missing := service.Trash(TrashInput{MountID: "documents", Path: "absent.md"})
	core.AssertFalse(t, missing.OK)
}

func TestService_Trash_UglyRejectsLink(t *core.T) {
	source := &symlinkMedium{Medium: coreio.NewMemoryMedium()}
	service := registeredService(t, []Mount{
		memoryMount("documents", source, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	result := service.Trash(TrashInput{MountID: "documents", Path: "escape"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorUnsupportedEntry))
}

func TestService_Trash_BadRuntimeLoad(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "content"))
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)
	runtime.loadErr = fs.ErrPermission

	result := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})

	core.AssertFalse(t, result.OK)
}

func TestService_Trash_UglyReceiptDirFails(t *core.T) {
	broken := &failingMedium{Medium: coreio.NewMemoryMedium()}
	core.RequireNoError(t, broken.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", broken, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())
	broken.ensureDirErr = fs.ErrPermission

	result := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})

	core.AssertFalse(t, result.OK)
	core.AssertTrue(t, broken.IsFile("report.md"))
}

func TestService_Trash_UglyRenameFails(t *core.T) {
	sequenced := &sequencedRenameMedium{
		Medium: coreio.NewMemoryMedium(),
		failOn: map[int]error{1: fs.ErrPermission},
	}
	core.RequireNoError(t, sequenced.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", sequenced, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	result := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})

	core.AssertFalse(t, result.OK)
	core.AssertTrue(t, sequenced.IsFile("report.md"))
}

func TestService_Trash_UglySaveAndRollbackFail(t *core.T) {
	sequenced := &sequencedRenameMedium{
		Medium: coreio.NewMemoryMedium(),
		failOn: map[int]error{2: fs.ErrPermission},
	}
	core.RequireNoError(t, sequenced.Write("report.md", "content"))
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount("documents", sequenced, ReadWriteCapabilities()),
	}, runtime)
	runtime.saveErr = fs.ErrPermission

	result := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationPartial, operation.Status)
	core.AssertEqual(t, ErrorPartialMove, operation.Code)
}

func TestService_ListTrash_BadRuntimeLoad(t *core.T) {
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, runtime)
	runtime.loadErr = fs.ErrPermission

	core.AssertFalse(t, service.ListTrash().OK)
}

func TestService_ListTrash_UglyUnknownMountMarksUnavailable(t *core.T) {
	runtime := NewMemoryRuntimeMetadata()
	core.RequireNoError(t, runtime.Save(RuntimeSnapshot{
		Version: 1,
		Trash: []TrashReceipt{{
			ID:           "receipt-1",
			MountID:      "ghost",
			OriginalPath: "report.md",
			TrashPath:    ".lthn-files/trash/receipt-1/payload",
			TrashedAt:    "2026-07-26T12:00:00Z",
		}},
	}))
	service := registeredService(t, []Mount{
		memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, runtime)

	result := service.ListTrash()

	core.RequireTrue(t, result.OK)
	entry := result.Value.(TrashSnapshot).Entries[0]
	core.AssertFalse(t, entry.Available)
	core.AssertEqual(t, ErrorInvalidMount, entry.ErrorCode)
}

func TestService_ListTrash_BadStatError(t *core.T) {
	broken := &failingMedium{Medium: coreio.NewMemoryMedium()}
	core.RequireNoError(t, broken.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", broken, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())
	trashed := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})
	core.RequireTrue(t, trashed.OK)
	broken.statErr = fs.ErrPermission

	core.AssertFalse(t, service.ListTrash().OK)
}

func TestService_Restore_BadInputs(t *core.T) {
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, runtime)

	invalid := service.Restore(RestoreInput{ReceiptID: "a/b"})
	core.AssertFalse(t, invalid.OK)

	unknown := service.Restore(RestoreInput{ReceiptID: "receipt-9"})
	core.AssertFalse(t, unknown.OK)
	core.AssertContains(t, unknown.Error(), string(ErrorMissingEntry))

	runtime.loadErr = fs.ErrPermission
	core.AssertFalse(t, service.Restore(RestoreInput{ReceiptID: "receipt-9"}).OK)
}

func TestService_Restore_BadMountAndCapability(t *core.T) {
	capabilities := ReadWriteCapabilities()
	capabilities.Restore = false
	runtime := NewMemoryRuntimeMetadata()
	core.RequireNoError(t, runtime.Save(RuntimeSnapshot{
		Version: 1,
		Trash: []TrashReceipt{
			{
				ID:           "receipt-ghost",
				MountID:      "ghost",
				OriginalPath: "report.md",
				TrashPath:    ".lthn-files/trash/receipt-ghost/payload",
				TrashedAt:    "2026-07-26T12:00:00Z",
			},
			{
				ID:           "receipt-caps",
				MountID:      "documents",
				OriginalPath: "report.md",
				TrashPath:    ".lthn-files/trash/receipt-caps/payload",
				TrashedAt:    "2026-07-26T12:00:00Z",
			},
		},
	}))
	service := registeredService(t, []Mount{
		memoryMount("documents", coreio.NewMemoryMedium(), capabilities),
	}, runtime)

	ghost := service.Restore(RestoreInput{ReceiptID: "receipt-ghost"})
	core.AssertFalse(t, ghost.OK)

	denied := service.Restore(RestoreInput{ReceiptID: "receipt-caps"})
	core.AssertFalse(t, denied.OK)
	core.AssertContains(t, denied.Error(), string(ErrorCapabilityDenied))
}

func TestService_Restore_UglyProviderFaults(t *core.T) {
	broken := &failingMedium{Medium: coreio.NewMemoryMedium()}
	core.RequireNoError(t, broken.Write("notes/idea.md", "ship it"))
	service := registeredService(t, []Mount{
		memoryMount("documents", broken, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())
	trashed := service.Trash(TrashInput{MountID: "documents", Path: "notes/idea.md"})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	broken.statErr = fs.ErrPermission
	core.AssertFalse(t, service.Restore(RestoreInput{ReceiptID: receiptID}).OK)
	broken.statErr = nil

	broken.ensureDirErr = fs.ErrPermission
	core.AssertFalse(t, service.Restore(RestoreInput{ReceiptID: receiptID}).OK)
	broken.ensureDirErr = nil

	broken.renameErr = fs.ErrPermission
	core.AssertFalse(t, service.Restore(RestoreInput{ReceiptID: receiptID}).OK)
	broken.renameErr = nil
}

func TestService_Restore_BadOriginalStatError(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("report.md", "content"))
	wrapped := &statPathFailureMedium{
		Medium:   medium,
		failPath: "report.md",
		err:      fs.ErrPermission,
	}
	service := registeredService(t, []Mount{
		memoryMount("documents", wrapped, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())
	// Trash reads the original through Stat before the wrapper arms —
	// so trash the file with the failure path pointed elsewhere, then
	// retarget it at the original path for the restore conflict check.
	wrapped.failPath = "elsewhere"
	trashed := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID
	wrapped.failPath = "report.md"

	core.AssertFalse(t, service.Restore(RestoreInput{ReceiptID: receiptID}).OK)
}

func TestService_Restore_UglySaveFailureArms(t *core.T) {
	runtime := &stubRuntimeMetadata{}
	sequenced := &sequencedRenameMedium{
		Medium: coreio.NewMemoryMedium(),
		failOn: map[int]error{},
	}
	core.RequireNoError(t, sequenced.Write("report.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", sequenced, ReadWriteCapabilities()),
	}, runtime)
	trashed := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	// Save fails, rollback rename succeeds: hard failure.
	runtime.saveErr = fs.ErrPermission
	failed := service.Restore(RestoreInput{ReceiptID: receiptID})
	core.AssertFalse(t, failed.OK)

	// Save fails AND the rollback rename fails: partial result.
	// Renames so far: trash=1, restore=2, rollback=3, restore=4 — the
	// next rollback is call 5.
	sequenced.failOn[5] = fs.ErrPermission
	partial := service.Restore(RestoreInput{ReceiptID: receiptID})
	core.RequireTrue(t, partial.OK)
	operation := partial.Value.(FileOperationResult)
	core.AssertEqual(t, OperationPartial, operation.Status)
	core.AssertEqual(t, ErrorPartialMove, operation.Code)
}

func TestService_DeleteDirect_BadInputs(t *core.T) {
	capabilities := ReadWriteCapabilities()
	capabilities.Delete = false
	service := registeredService(t, []Mount{
		memoryMount(
			"restricted",
			coreio.NewMemoryMedium(),
			capabilities,
		),
	}, NewMemoryRuntimeMetadata())

	unknown := service.Delete(DeleteInput{
		MountID:   "ghost",
		Path:      "report.md",
		Confirmed: true,
	})
	core.AssertFalse(t, unknown.OK)

	denied := service.Delete(DeleteInput{
		MountID:   "restricted",
		Path:      "report.md",
		Confirmed: true,
	})
	core.AssertFalse(t, denied.OK)
	core.AssertContains(t, denied.Error(), string(ErrorCapabilityDenied))
}

func TestService_DeleteDirect_UglyProviderFaults(t *core.T) {
	broken := &failingMedium{Medium: coreio.NewMemoryMedium()}
	core.RequireNoError(t, broken.Write("archive/report.md", "content"))
	core.RequireNoError(t, broken.Write("single.md", "content"))
	service := registeredService(t, []Mount{
		memoryMount("documents", broken, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	escape := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "../escape",
		Confirmed: true,
	})
	core.AssertFalse(t, escape.OK)

	missing := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "absent.md",
		Confirmed: true,
	})
	core.AssertFalse(t, missing.OK)

	broken.listErr = fs.ErrPermission
	listBroken := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "archive",
		Recursive: true,
		Confirmed: true,
	})
	core.AssertFalse(t, listBroken.OK)
	broken.listErr = nil

	broken.deleteErr = fs.ErrPermission
	deleteBroken := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "single.md",
		Confirmed: true,
	})
	core.AssertFalse(t, deleteBroken.OK)
	broken.deleteErr = nil

	broken.deleteAllErr = fs.ErrPermission
	recursiveBroken := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "archive",
		Recursive: true,
		Confirmed: true,
	})
	core.AssertFalse(t, recursiveBroken.OK)
}

func TestService_DeleteDirect_UglyRejectsLink(t *core.T) {
	source := &symlinkMedium{Medium: coreio.NewMemoryMedium()}
	service := registeredService(t, []Mount{
		memoryMount("documents", source, ReadWriteCapabilities()),
	}, NewMemoryRuntimeMetadata())

	result := service.Delete(DeleteInput{
		MountID:   "documents",
		Path:      "escape",
		Confirmed: true,
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorUnsupportedEntry))
}

func TestService_DeleteReceipt_BadInputs(t *core.T) {
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, runtime)

	invalid := service.Delete(DeleteInput{ReceiptID: "a/b", Confirmed: true})
	core.AssertFalse(t, invalid.OK)

	unknown := service.Delete(DeleteInput{ReceiptID: "receipt-9", Confirmed: true})
	core.AssertFalse(t, unknown.OK)

	runtime.loadErr = fs.ErrPermission
	core.AssertFalse(
		t,
		service.Delete(DeleteInput{ReceiptID: "receipt-9", Confirmed: true}).OK,
	)
}

func TestService_DeleteReceipt_UglyProviderFaults(t *core.T) {
	broken := &failingMedium{Medium: coreio.NewMemoryMedium()}
	core.RequireNoError(t, broken.Write("report.md", "content"))
	runtime := &stubRuntimeMetadata{}
	service := registeredService(t, []Mount{
		memoryMount("documents", broken, ReadWriteCapabilities()),
	}, runtime)
	trashed := service.Trash(TrashInput{MountID: "documents", Path: "report.md"})
	core.RequireTrue(t, trashed.OK)
	receiptID := trashed.Value.(FileOperationResult).ReceiptID

	broken.statErr = fs.ErrPermission
	core.AssertFalse(
		t,
		service.Delete(DeleteInput{ReceiptID: receiptID, Confirmed: true}).OK,
	)
	broken.statErr = nil

	broken.deleteErr = fs.ErrPermission
	core.AssertFalse(
		t,
		service.Delete(DeleteInput{ReceiptID: receiptID, Confirmed: true}).OK,
	)
	broken.deleteErr = nil

	// Payload deletion succeeds but the receipt cannot be removed
	// from the runtime snapshot: partial result, stale record.
	runtime.saveErr = fs.ErrPermission
	partial := service.Delete(DeleteInput{ReceiptID: receiptID, Confirmed: true})
	core.RequireTrue(t, partial.OK)
	operation := partial.Value.(FileOperationResult)
	core.AssertEqual(t, OperationPartial, operation.Status)
	core.AssertEqual(t, ErrorPartialMove, operation.Code)
}
