// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"
	"path"

	core "dappco.re/go"
)

func (s *Service) createDirectory(input CreateDirectoryInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.CreateDirectory {
		return core.Fail(capabilityFailure(
			"CreateDirectory",
			mount.ID,
			input.ParentPath,
		))
	}
	parent, err := normaliseRelativePath(input.ParentPath, true)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.ParentPath))
	}
	if err := validateEntryName(input.Name); err != nil {
		return core.Fail(withAddress(err, mount.ID, parent))
	}
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	if parent != "" {
		info, statErr := mount.Medium.Stat(parent)
		if statErr != nil {
			return core.Fail(providerFailure(
				"CreateDirectory",
				mount.ID,
				parent,
				statErr,
			))
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return core.Fail(newFailure(
				ErrorUnsupportedEntry,
				mount.ID,
				parent,
				"The parent is not a directory.",
				nil,
			))
		}
	}
	destination := FileAddress{
		MountID: mount.ID,
		Path:    joinRelative(parent, input.Name),
	}
	source := FileAddress{MountID: mount.ID, Path: parent}
	if info, statErr := mount.Medium.Stat(destination.Path); statErr == nil {
		return core.Ok(conflictResult(
			"create-directory",
			source,
			destination,
			entryKindFromInfo(info),
		))
	} else if !core.Is(statErr, fs.ErrNotExist) {
		return core.Fail(providerFailure(
			"CreateDirectory",
			mount.ID,
			destination.Path,
			statErr,
		))
	}
	if err := mount.Medium.EnsureDir(destination.Path); err != nil {
		return core.Fail(providerFailure(
			"CreateDirectory",
			mount.ID,
			destination.Path,
			err,
		))
	}
	operationID := core.ID()
	s.fireEvent("create-directory", operationID, destination)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "create-directory",
		Status:      OperationCompleted,
		Source:      source,
		Destination: &destination,
		Affected:    []FileAddress{destination},
		Message:     "Folder created.",
	})
}

func (s *Service) rename(input RenameInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Rename {
		return core.Fail(capabilityFailure("Rename", mount.ID, input.Path))
	}
	if input.Path == "" {
		return core.Fail(newFailure(
			ErrorBoundaryRejected,
			mount.ID,
			"",
			"The mount root cannot be renamed.",
			nil,
		))
	}
	sourcePath, err := normaliseRelativePath(input.Path, false)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.Path))
	}
	if err := validateEntryName(input.Name); err != nil {
		return core.Fail(withAddress(err, mount.ID, sourcePath))
	}
	parent := path.Dir(sourcePath)
	if parent == "." {
		parent = ""
	}
	destination := FileAddress{
		MountID: mount.ID,
		Path:    joinRelative(parent, input.Name),
	}
	source := FileAddress{MountID: mount.ID, Path: sourcePath}
	lock := s.locks[mount.ID]
	lock.Lock()
	defer lock.Unlock()
	sourceInfo, err := mount.Medium.Stat(sourcePath)
	if err != nil {
		return core.Fail(providerFailure(
			"Rename",
			mount.ID,
			sourcePath,
			err,
		))
	}
	if info, statErr := mount.Medium.Stat(destination.Path); statErr == nil {
		return core.Ok(conflictResult(
			"rename",
			source,
			destination,
			entryKindFromInfo(info),
		))
	} else if !core.Is(statErr, fs.ErrNotExist) {
		return core.Fail(providerFailure(
			"Rename",
			mount.ID,
			destination.Path,
			statErr,
		))
	}
	if err := mount.Medium.Rename(sourcePath, destination.Path); err != nil {
		return core.Fail(providerFailure(
			"Rename",
			mount.ID,
			sourcePath,
			err,
		))
	}
	operationID := core.ID()
	s.fireEvent("rename", operationID, source, destination)
	return core.Ok(FileOperationResult{
		OperationID: operationID,
		Operation:   "rename",
		Status:      OperationCompleted,
		Source:      source,
		Destination: &destination,
		Affected:    []FileAddress{source, destination},
		Message:     core.Concat(entryLabel(sourceInfo), " renamed."),
	})
}

func conflictResult(
	operation string,
	source FileAddress,
	destination FileAddress,
	kind EntryKind,
) FileOperationResult {
	operationID := core.ID()
	return FileOperationResult{
		OperationID: operationID,
		Operation:   operation,
		Status:      OperationConflict,
		Code:        ErrorConflict,
		Source:      source,
		Destination: &destination,
		Affected:    []FileAddress{},
		Conflict: &FileConflict{
			Source:      source,
			Destination: destination,
			Kind:        kind,
		},
		Message: "An entry with that name already exists.",
	}
}

func entryKindFromInfo(info fs.FileInfo) EntryKind {
	switch {
	case info == nil:
		return EntryOther
	case info.Mode()&fs.ModeSymlink != 0:
		return EntryLink
	case info.IsDir():
		return EntryDirectory
	case info.Mode().IsRegular():
		return EntryFile
	default:
		return EntryOther
	}
}

func entryLabel(info fs.FileInfo) string {
	if info != nil && info.IsDir() {
		return "Folder"
	}
	return "File"
}
