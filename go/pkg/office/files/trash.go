// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"
	"path"

	core "dappco.re/go"
)

func (s *Service) trash(input TrashInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Trash || !s.internalReady[mount.ID] {
		return core.Fail(capabilityFailure("Trash", mount.ID, input.Path))
	}
	relativePath, err := normaliseRelativePath(input.Path, false)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.Path))
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	snapshot, err := s.runtime.Load()
	if err != nil {
		return core.Fail(providerFailure(
			"Trash",
			mount.ID,
			relativePath,
			err,
		))
	}
	info, err := mount.Medium.Stat(relativePath)
	if err != nil {
		return core.Fail(providerFailure(
			"Trash",
			mount.ID,
			relativePath,
			err,
		))
	}
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		return core.Fail(newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			relativePath,
			"Only regular files and directories can be moved to Trash.",
			nil,
		))
	}
	receiptID := core.ID()
	receiptRoot := trustedTrashRoot(receiptID)
	trashPath := core.Join("/", receiptRoot, "payload")
	if err := mount.Medium.EnsureDir(receiptRoot); err != nil {
		return core.Fail(providerFailure(
			"Trash",
			mount.ID,
			relativePath,
			err,
		))
	}
	if err := mount.Medium.Rename(relativePath, trashPath); err != nil {
		_ = deleteOwnedTree(mount, receiptRoot)
		return core.Fail(providerFailure(
			"Trash",
			mount.ID,
			relativePath,
			err,
		))
	}
	trashedAt := core.Now().UTC().Format(core.RFC3339Nano)
	snapshot.Trash = append([]TrashReceipt{{
		ID:           receiptID,
		MountID:      mount.ID,
		OriginalPath: relativePath,
		TrashPath:    trashPath,
		TrashedAt:    trashedAt,
	}}, snapshot.Trash...)
	if err := s.runtime.Save(snapshot); err != nil {
		rollbackErr := mount.Medium.Rename(trashPath, relativePath)
		if rollbackErr != nil {
			core.Error(
				"Files trash rollback failed",
				"mount",
				mount.ID,
				"path",
				relativePath,
				"receipt",
				receiptID,
				"err",
				rollbackErr,
			)
			return core.Ok(FileOperationResult{
				OperationID: receiptID,
				Operation:   "trash",
				Status:      OperationPartial,
				Code:        ErrorPartialMove,
				Source:      FileAddress{MountID: mount.ID, Path: relativePath},
				Affected:    []FileAddress{{MountID: mount.ID, Path: relativePath}},
				Message:     "The item moved, but its Trash record could not be saved.",
				ReceiptID:   receiptID,
			})
		}
		_ = deleteOwnedTree(mount, receiptRoot)
		return core.Fail(providerFailure(
			"TrashMetadata",
			mount.ID,
			relativePath,
			err,
		))
	}
	operationID := core.ID()
	source := FileAddress{MountID: mount.ID, Path: relativePath}
	s.fireEvent("trash", operationID, source)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "trash",
		Status:      OperationCompleted,
		Source:      source,
		Affected:    []FileAddress{source},
		Message:     core.Concat(entryLabel(info), " moved to Trash."),
		ReceiptID:   receiptID,
	})
}

func (s *Service) listTrash() core.Result {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	snapshot, err := s.runtime.Load()
	if err != nil {
		return core.Fail(providerFailure("ListTrash", "", "", err))
	}
	entries := make([]TrashEntry, 0, len(snapshot.Trash))
	for _, receipt := range snapshot.Trash {
		mount, mountErr := s.mount(receipt.MountID)
		if mountErr != nil {
			entries = append(entries, unavailableTrashEntry(
				receipt,
				ErrorInvalidMount,
			))
			continue
		}
		info, statErr := mount.Medium.Stat(receipt.TrashPath)
		if statErr != nil {
			if core.Is(statErr, fs.ErrNotExist) {
				entries = append(entries, unavailableTrashEntry(
					receipt,
					ErrorMissingEntry,
				))
				continue
			}
			return core.Fail(providerFailure(
				"ListTrash",
				mount.ID,
				receipt.OriginalPath,
				statErr,
			))
		}
		entries = append(entries, TrashEntry{
			ReceiptID:    receipt.ID,
			MountID:      receipt.MountID,
			OriginalPath: receipt.OriginalPath,
			Name:         path.Base(receipt.OriginalPath),
			Kind:         entryKindFromInfo(info),
			SizeBytes:    info.Size(),
			TrashedAt:    receipt.TrashedAt,
			Available:    true,
		})
	}
	return core.Ok(TrashSnapshot{
		Entries:     entries,
		RefreshedAt: core.Now().UTC().Format(core.RFC3339Nano),
	})
}

func (s *Service) restore(input RestoreInput) core.Result {
	if err := validateReceiptID(input.ReceiptID); err != nil {
		return core.Fail(err)
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	snapshot, err := s.runtime.Load()
	if err != nil {
		return core.Fail(providerFailure("Restore", "", "", err))
	}
	receipt, index, err := findReceipt(snapshot, input.ReceiptID)
	if err != nil {
		return core.Fail(err)
	}
	mount, err := s.mount(receipt.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Restore || !s.internalReady[mount.ID] {
		return core.Fail(capabilityFailure(
			"Restore",
			mount.ID,
			receipt.OriginalPath,
		))
	}
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	info, err := mount.Medium.Stat(receipt.TrashPath)
	if err != nil {
		return core.Fail(providerFailure(
			"Restore",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	address := FileAddress{
		MountID: mount.ID,
		Path:    receipt.OriginalPath,
	}
	if existing, statErr := mount.Medium.Stat(
		receipt.OriginalPath,
	); statErr == nil {
		return core.Ok(conflictResult(
			"restore",
			address,
			address,
			entryKindFromInfo(existing),
		))
	} else if !core.Is(statErr, fs.ErrNotExist) {
		return core.Fail(providerFailure(
			"Restore",
			mount.ID,
			receipt.OriginalPath,
			statErr,
		))
	}
	if err := ensureMediumParent(
		mount.Medium,
		receipt.OriginalPath,
	); err != nil {
		return core.Fail(providerFailure(
			"Restore",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	if err := mount.Medium.Rename(
		receipt.TrashPath,
		receipt.OriginalPath,
	); err != nil {
		return core.Fail(providerFailure(
			"Restore",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	snapshot.Trash = removeReceipt(snapshot.Trash, index)
	if err := s.runtime.Save(snapshot); err != nil {
		rollbackErr := mount.Medium.Rename(
			receipt.OriginalPath,
			receipt.TrashPath,
		)
		if rollbackErr != nil {
			core.Error(
				"Files restore rollback failed",
				"mount",
				mount.ID,
				"path",
				receipt.OriginalPath,
				"receipt",
				receipt.ID,
				"err",
				rollbackErr,
			)
			return core.Ok(FileOperationResult{
				OperationID: core.ID(),
				Operation:   "restore",
				Status:      OperationPartial,
				Code:        ErrorPartialMove,
				Source:      address,
				Affected:    []FileAddress{address},
				Message:     "The item restored, but its Trash record could not be updated.",
				ReceiptID:   receipt.ID,
			})
		}
		return core.Fail(providerFailure(
			"RestoreMetadata",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	_ = deleteOwnedTree(mount, trustedTrashRoot(receipt.ID))
	operationID := core.ID()
	s.fireEvent("restore", operationID, address)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "restore",
		Status:      OperationCompleted,
		Source:      address,
		Affected:    []FileAddress{address},
		Message:     core.Concat(entryLabel(info), " restored."),
		ReceiptID:   receipt.ID,
	})
}

func (s *Service) delete(input DeleteInput) core.Result {
	directTarget := input.MountID != "" || input.Path != ""
	receiptTarget := input.ReceiptID != ""
	if directTarget == receiptTarget || !input.Confirmed {
		return core.Fail(newFailure(
			ErrorInvalidInput,
			input.MountID,
			input.Path,
			"Permanent deletion requires one target and explicit confirmation.",
			nil,
		))
	}
	if receiptTarget {
		return s.deleteReceipt(input)
	}
	return s.deleteDirect(input)
}

func (s *Service) deleteDirect(input DeleteInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Delete {
		return core.Fail(capabilityFailure("Delete", mount.ID, input.Path))
	}
	relativePath, err := normaliseRelativePath(input.Path, false)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.Path))
	}
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	info, err := mount.Medium.Stat(relativePath)
	if err != nil {
		return core.Fail(providerFailure(
			"Delete",
			mount.ID,
			relativePath,
			err,
		))
	}
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		return core.Fail(newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			relativePath,
			"Only regular files and directories can be deleted.",
			nil,
		))
	}
	if kind == EntryDirectory {
		entries, listErr := mount.Medium.List(relativePath)
		if listErr != nil {
			return core.Fail(providerFailure(
				"Delete",
				mount.ID,
				relativePath,
				listErr,
			))
		}
		if len(entries) > 0 && !input.Recursive {
			return core.Fail(newFailure(
				ErrorInvalidInput,
				mount.ID,
				relativePath,
				"Deleting a non-empty folder requires recursive confirmation.",
				nil,
			))
		}
		if len(entries) > 0 {
			err = mount.Medium.DeleteAll(relativePath)
		} else {
			err = mount.Medium.Delete(relativePath)
		}
	} else {
		err = mount.Medium.Delete(relativePath)
	}
	if err != nil {
		return core.Fail(providerFailure(
			"Delete",
			mount.ID,
			relativePath,
			err,
		))
	}
	operationID := core.ID()
	address := FileAddress{MountID: mount.ID, Path: relativePath}
	s.fireEvent("delete", operationID, address)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "delete",
		Status:      OperationCompleted,
		Source:      address,
		Affected:    []FileAddress{address},
		Message:     core.Concat(entryLabel(info), " permanently deleted."),
	})
}

func (s *Service) deleteReceipt(input DeleteInput) core.Result {
	if err := validateReceiptID(input.ReceiptID); err != nil {
		return core.Fail(err)
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	snapshot, err := s.runtime.Load()
	if err != nil {
		return core.Fail(providerFailure("Delete", "", "", err))
	}
	receipt, index, err := findReceipt(snapshot, input.ReceiptID)
	if err != nil {
		return core.Fail(err)
	}
	mount, err := s.mount(receipt.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Delete || !s.internalReady[mount.ID] {
		return core.Fail(capabilityFailure(
			"Delete",
			mount.ID,
			receipt.OriginalPath,
		))
	}
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	info, err := mount.Medium.Stat(receipt.TrashPath)
	if err != nil {
		return core.Fail(providerFailure(
			"Delete",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	if info.IsDir() {
		err = mount.Medium.DeleteAll(receipt.TrashPath)
	} else {
		err = mount.Medium.Delete(receipt.TrashPath)
	}
	if err != nil {
		return core.Fail(providerFailure(
			"Delete",
			mount.ID,
			receipt.OriginalPath,
			err,
		))
	}
	snapshot.Trash = removeReceipt(snapshot.Trash, index)
	if err := s.runtime.Save(snapshot); err != nil {
		return core.Ok(FileOperationResult{
			OperationID: core.ID(),
			Operation:   "delete",
			Status:      OperationPartial,
			Code:        ErrorPartialMove,
			Source: FileAddress{
				MountID: mount.ID,
				Path:    receipt.OriginalPath,
			},
			Affected: []FileAddress{{
				MountID: mount.ID,
				Path:    receipt.OriginalPath,
			}},
			Message:   "The item was deleted, but its Trash record remains stale.",
			ReceiptID: receipt.ID,
		})
	}
	_ = deleteOwnedTree(mount, trustedTrashRoot(receipt.ID))
	operationID := core.ID()
	address := FileAddress{
		MountID: mount.ID,
		Path:    receipt.OriginalPath,
	}
	s.fireEvent("delete", operationID, address)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "delete",
		Status:      OperationCompleted,
		Source:      address,
		Affected:    []FileAddress{address},
		Message:     "Item permanently deleted.",
		ReceiptID:   receipt.ID,
	})
}

func trustedTrashRoot(receiptID string) string {
	return core.Join("/", internalNamespace, "trash", receiptID)
}

func validateReceiptID(receiptID string) error {
	if receiptID == "" ||
		core.Contains(receiptID, "/") ||
		core.Contains(receiptID, "\\") ||
		hasUnsafePathRune(receiptID) {
		return newFailure(
			ErrorInvalidInput,
			"",
			"",
			"Trash receipt ID is invalid.",
			nil,
		)
	}
	return nil
}

func findReceipt(
	snapshot RuntimeSnapshot,
	receiptID string,
) (TrashReceipt, int, error) {
	for index, receipt := range snapshot.Trash {
		if receipt.ID == receiptID {
			return receipt, index, nil
		}
	}
	return TrashReceipt{}, -1, newFailure(
		ErrorMissingEntry,
		"",
		"",
		"Trash receipt is no longer available.",
		nil,
	)
}

func removeReceipt(
	receipts []TrashReceipt,
	index int,
) []TrashReceipt {
	rows := make([]TrashReceipt, 0, len(receipts)-1)
	rows = append(rows, receipts[:index]...)
	rows = append(rows, receipts[index+1:]...)
	return rows
}

func unavailableTrashEntry(
	receipt TrashReceipt,
	code ErrorCode,
) TrashEntry {
	return TrashEntry{
		ReceiptID:    receipt.ID,
		MountID:      receipt.MountID,
		OriginalPath: receipt.OriginalPath,
		Name:         path.Base(receipt.OriginalPath),
		TrashedAt:    receipt.TrashedAt,
		Available:    false,
		ErrorCode:    code,
	}
}

func deleteOwnedTree(mount Mount, relativePath string) error {
	err := mount.Medium.DeleteAll(relativePath)
	if core.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
