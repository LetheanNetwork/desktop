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
