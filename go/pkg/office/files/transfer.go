// SPDX-License-Identifier: EUPL-1.2

package files

import (
	goio "io"
	"io/fs"
	"sort"

	core "dappco.re/go"
)

const (
	internalOwnerPath     = ".lthn-files/owner.json"
	internalOwnerDocument = `{"owner":"ai.lthn.desktop.files","version":1}`
	transferBufferBytes   = 128 * 1024
)

type transferEntry struct {
	SourcePath      string
	DestinationPath string
	Kind            EntryKind
	Mode            fs.FileMode
	SizeBytes       int64
	Depth           int
}

type transferManifest struct {
	entries    []transferEntry
	sourceKind EntryKind
	totalBytes int64
}

func (s *Service) initialiseInternalNamespace(mount Mount) {
	if !mount.Capabilities.CopyTo &&
		!mount.Capabilities.Trash &&
		!mount.Capabilities.Restore {
		return
	}
	if _, err := mount.Medium.Stat(internalNamespace); err != nil {
		if !core.Is(err, fs.ErrNotExist) {
			core.Warn(
				"Files internal namespace unavailable",
				"mount",
				mount.ID,
				"err",
				err,
			)
			return
		}
		if err := mount.Medium.EnsureDir(internalNamespace); err != nil {
			core.Warn(
				"Files internal namespace creation failed",
				"mount",
				mount.ID,
				"err",
				err,
			)
			return
		}
		if err := mount.Medium.WriteMode(
			internalOwnerPath,
			internalOwnerDocument,
			0600,
		); err != nil {
			core.Warn(
				"Files internal owner marker failed",
				"mount",
				mount.ID,
				"err",
				err,
			)
			return
		}
		s.internalReady[mount.ID] = true
		return
	}
	owner, err := mount.Medium.Read(internalOwnerPath)
	if err != nil || owner != internalOwnerDocument {
		return
	}
	s.internalReady[mount.ID] = true
}

func (s *Service) copy(input TransferInput) core.Result {
	sourceMount, destinationMount, source, destination, err :=
		s.validateTransfer(input, false)
	if err != nil {
		return core.Fail(err)
	}
	unlock := s.lockMounts(sourceMount.ID, destinationMount.ID)
	defer unlock()
	if !s.internalReady[destinationMount.ID] {
		return core.Fail(capabilityFailure(
			"Copy",
			destinationMount.ID,
			destination.Path,
		))
	}
	manifest, conflict, err := s.preflightTransfer(
		"copy",
		sourceMount,
		destinationMount,
		source,
		destination,
	)
	if err != nil {
		return core.Fail(err)
	}
	if conflict != nil {
		return core.Ok(*conflict)
	}
	operationID := core.ID()
	if err := s.stageAndCommitTransfer(
		operationID,
		sourceMount,
		destinationMount,
		source,
		destination,
		manifest,
	); err != nil {
		return core.Fail(err)
	}
	s.fireEvent("copy", operationID, source, destination)
	return core.Ok(completedTransferResult(
		operationID,
		"copy",
		source,
		destination,
		"Copy completed.",
	))
}

func (s *Service) move(input TransferInput) core.Result {
	sourceMount, destinationMount, source, destination, err :=
		s.validateTransfer(input, true)
	if err != nil {
		return core.Fail(err)
	}
	unlock := s.lockMounts(sourceMount.ID, destinationMount.ID)
	defer unlock()
	if sourceMount.ID == destinationMount.ID {
		return s.moveWithinMount(sourceMount, source, destination)
	}
	if !sourceMount.Capabilities.CopyFrom ||
		!destinationMount.Capabilities.CopyTo ||
		!s.internalReady[destinationMount.ID] {
		return core.Fail(capabilityFailure(
			"Move",
			destinationMount.ID,
			destination.Path,
		))
	}
	manifest, conflict, err := s.preflightTransfer(
		"move",
		sourceMount,
		destinationMount,
		source,
		destination,
	)
	if err != nil {
		return core.Fail(err)
	}
	if conflict != nil {
		return core.Ok(*conflict)
	}
	operationID := core.ID()
	if err := s.stageAndCommitTransfer(
		operationID,
		sourceMount,
		destinationMount,
		source,
		destination,
		manifest,
	); err != nil {
		return core.Fail(err)
	}
	var deleteErr error
	if manifest.sourceKind == EntryDirectory {
		deleteErr = sourceMount.Medium.DeleteAll(source.Path)
	} else {
		deleteErr = sourceMount.Medium.Delete(source.Path)
	}
	if deleteErr != nil {
		s.fireEvent("partial", operationID, source, destination)
		return core.Ok(FileOperationResult{
			OperationID: operationID,
			Operation:   "move",
			Status:      OperationPartial,
			Code:        ErrorPartialMove,
			Source:      source,
			Destination: &destination,
			Affected:    []FileAddress{source, destination},
			Message:     "The copy completed, but the original could not be removed.",
		})
	}
	s.fireEvent("move", operationID, source, destination)
	return core.Ok(completedTransferResult(
		operationID,
		"move",
		source,
		destination,
		"Move completed.",
	))
}

func (s *Service) validateTransfer(
	input TransferInput,
	moving bool,
) (Mount, Mount, FileAddress, FileAddress, error) {
	sourceMount, err := s.mount(input.Source.MountID)
	if err != nil {
		return Mount{}, Mount{}, FileAddress{}, FileAddress{}, err
	}
	destinationMount, err := s.mount(input.Destination.MountID)
	if err != nil {
		return Mount{}, Mount{}, FileAddress{}, FileAddress{}, err
	}
	sourcePath, err := normaliseRelativePath(input.Source.Path, false)
	if err != nil {
		return Mount{}, Mount{}, FileAddress{}, FileAddress{},
			withAddress(err, sourceMount.ID, input.Source.Path)
	}
	destinationPath, err := normaliseRelativePath(
		input.Destination.Path,
		false,
	)
	if err != nil {
		return Mount{}, Mount{}, FileAddress{}, FileAddress{},
			withAddress(err, destinationMount.ID, input.Destination.Path)
	}
	source := FileAddress{MountID: sourceMount.ID, Path: sourcePath}
	destination := FileAddress{
		MountID: destinationMount.ID,
		Path:    destinationPath,
	}
	if sourceMount.ID == destinationMount.ID &&
		(destinationPath == sourcePath ||
			core.HasPrefix(destinationPath, core.Concat(sourcePath, "/"))) {
		return Mount{}, Mount{}, FileAddress{}, FileAddress{}, newFailure(
			ErrorBoundaryRejected,
			sourceMount.ID,
			sourcePath,
			"The destination cannot be the source or one of its children.",
			nil,
		)
	}
	if moving {
		if !sourceMount.Capabilities.Move {
			return Mount{}, Mount{}, FileAddress{}, FileAddress{},
				capabilityFailure("Move", sourceMount.ID, sourcePath)
		}
	} else {
		if !sourceMount.Capabilities.CopyFrom {
			return Mount{}, Mount{}, FileAddress{}, FileAddress{},
				capabilityFailure("Copy", sourceMount.ID, sourcePath)
		}
		if !destinationMount.Capabilities.CopyTo {
			return Mount{}, Mount{}, FileAddress{}, FileAddress{},
				capabilityFailure("Copy", destinationMount.ID, destinationPath)
		}
	}
	return sourceMount, destinationMount, source, destination, nil
}

func (s *Service) moveWithinMount(
	mount Mount,
	source FileAddress,
	destination FileAddress,
) core.Result {
	sourceInfo, err := mount.Medium.Stat(source.Path)
	if err != nil {
		return core.Fail(providerFailure("Move", mount.ID, source.Path, err))
	}
	sourceKind := entryKindFromInfo(sourceInfo)
	if sourceKind != EntryFile && sourceKind != EntryDirectory {
		return core.Fail(newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			source.Path,
			"Only regular files and directories can be moved.",
			nil,
		))
	}
	if info, statErr := mount.Medium.Stat(destination.Path); statErr == nil {
		return core.Ok(conflictResult(
			"move",
			source,
			destination,
			entryKindFromInfo(info),
		))
	} else if !core.Is(statErr, fs.ErrNotExist) {
		return core.Fail(providerFailure(
			"Move",
			mount.ID,
			destination.Path,
			statErr,
		))
	}
	if err := ensureMediumParent(mount.Medium, destination.Path); err != nil {
		return core.Fail(providerFailure(
			"Move",
			mount.ID,
			destination.Path,
			err,
		))
	}
	if err := mount.Medium.Rename(source.Path, destination.Path); err != nil {
		return core.Fail(providerFailure("Move", mount.ID, source.Path, err))
	}
	operationID := core.ID()
	s.fireEvent("move", operationID, source, destination)
	return core.Ok(completedTransferResult(
		operationID,
		"move",
		source,
		destination,
		"Move completed.",
	))
}

func (s *Service) preflightTransfer(
	operation string,
	sourceMount Mount,
	destinationMount Mount,
	source FileAddress,
	destination FileAddress,
) (transferManifest, *FileOperationResult, error) {
	if info, err := destinationMount.Medium.Stat(destination.Path); err == nil {
		conflict := conflictResult(
			operation,
			source,
			destination,
			entryKindFromInfo(info),
		)
		return transferManifest{}, &conflict, nil
	} else if !core.Is(err, fs.ErrNotExist) {
		return transferManifest{}, nil, providerFailure(
			"TransferPreflight",
			destinationMount.ID,
			destination.Path,
			err,
		)
	}
	rootInfo, err := sourceMount.Medium.Stat(source.Path)
	if err != nil {
		return transferManifest{}, nil, providerFailure(
			"TransferPreflight",
			sourceMount.ID,
			source.Path,
			err,
		)
	}
	manifest := transferManifest{
		entries: make([]transferEntry, 0),
	}
	if err := s.walkTransfer(
		sourceMount,
		source.Path,
		destination.Path,
		rootInfo,
		0,
		&manifest,
	); err != nil {
		return transferManifest{}, nil, err
	}
	manifest.sourceKind = manifest.entries[0].Kind
	return manifest, nil, nil
}

func (s *Service) walkTransfer(
	mount Mount,
	sourcePath string,
	destinationPath string,
	info fs.FileInfo,
	depth int,
	manifest *transferManifest,
) error {
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		return newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			sourcePath,
			"Links and special entries cannot be transferred.",
			nil,
		)
	}
	limits := s.transferLimits()
	if depth > limits.MaxRecursiveDepth {
		return newFailure(
			ErrorLimitExceeded,
			mount.ID,
			sourcePath,
			"The transfer exceeds the maximum directory depth.",
			nil,
		)
	}
	if len(manifest.entries)+1 > limits.MaxRecursiveItems {
		return newFailure(
			ErrorLimitExceeded,
			mount.ID,
			sourcePath,
			"The transfer contains too many entries.",
			nil,
		)
	}
	if info.Size() < 0 {
		return newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			sourcePath,
			"The provider returned an invalid entry size.",
			nil,
		)
	}
	if kind == EntryFile {
		if manifest.totalBytes+info.Size() > limits.MaxTransferBytes {
			return newFailure(
				ErrorLimitExceeded,
				mount.ID,
				sourcePath,
				"The transfer exceeds the maximum byte limit.",
				nil,
			)
		}
		manifest.totalBytes += info.Size()
	}
	manifest.entries = append(manifest.entries, transferEntry{
		SourcePath:      sourcePath,
		DestinationPath: destinationPath,
		Kind:            kind,
		Mode:            info.Mode(),
		SizeBytes:       info.Size(),
		Depth:           depth,
	})
	if kind != EntryDirectory {
		return nil
	}
	entries, err := mount.Medium.List(sourcePath)
	if err != nil {
		return providerFailure(
			"TransferPreflight",
			mount.ID,
			sourcePath,
			err,
		)
	}
	sort.Slice(entries, func(left, right int) bool {
		leftFolded := core.Lower(entries[left].Name())
		rightFolded := core.Lower(entries[right].Name())
		if leftFolded == rightFolded {
			return entries[left].Name() < entries[right].Name()
		}
		return leftFolded < rightFolded
	})
	for _, entry := range entries {
		childSource := joinRelative(sourcePath, entry.Name())
		childDestination := joinRelative(destinationPath, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 {
			return newFailure(
				ErrorUnsupportedEntry,
				mount.ID,
				childSource,
				"Links cannot be transferred.",
				nil,
			)
		}
		childInfo, infoErr := entry.Info()
		if infoErr != nil {
			return providerFailure(
				"TransferPreflight",
				mount.ID,
				childSource,
				infoErr,
			)
		}
		if err := s.walkTransfer(
			mount,
			childSource,
			childDestination,
			childInfo,
			depth+1,
			manifest,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) stageAndCommitTransfer(
	operationID string,
	sourceMount Mount,
	destinationMount Mount,
	source FileAddress,
	destination FileAddress,
	manifest transferManifest,
) error {
	operationRoot := core.Join(
		"/",
		internalNamespace,
		"staging",
		operationID,
	)
	payloadRoot := core.Join("/", operationRoot, "payload")
	if err := destinationMount.Medium.EnsureDir(operationRoot); err != nil {
		return providerFailure(
			"TransferStage",
			destinationMount.ID,
			destination.Path,
			err,
		)
	}
	cleanup := func() {
		if err := destinationMount.Medium.DeleteAll(operationRoot); err != nil &&
			!core.Is(err, fs.ErrNotExist) {
			core.Warn(
				"Files transfer staging cleanup failed",
				"mount",
				destinationMount.ID,
				"operation",
				operationID,
				"err",
				err,
			)
		}
	}
	actualTotal := int64(0)
	for _, entry := range manifest.entries {
		stagingPath := stagedTransferPath(
			payloadRoot,
			destination.Path,
			entry.DestinationPath,
		)
		if entry.Kind == EntryDirectory {
			if err := destinationMount.Medium.EnsureDir(stagingPath); err != nil {
				cleanup()
				return providerFailure(
					"TransferStage",
					destinationMount.ID,
					destination.Path,
					err,
				)
			}
			continue
		}
		written, err := s.streamTransferFile(
			sourceMount,
			destinationMount,
			entry,
			stagingPath,
			actualTotal,
		)
		if err != nil {
			cleanup()
			return err
		}
		actualTotal += written
	}
	if err := ensureMediumParent(
		destinationMount.Medium,
		destination.Path,
	); err != nil {
		cleanup()
		return providerFailure(
			"TransferCommit",
			destinationMount.ID,
			destination.Path,
			err,
		)
	}
	if _, err := destinationMount.Medium.Stat(destination.Path); err == nil {
		cleanup()
		return newFailure(
			ErrorConflict,
			destinationMount.ID,
			destination.Path,
			"The destination appeared during transfer.",
			nil,
		)
	} else if !core.Is(err, fs.ErrNotExist) {
		cleanup()
		return providerFailure(
			"TransferCommit",
			destinationMount.ID,
			destination.Path,
			err,
		)
	}
	if err := destinationMount.Medium.Rename(
		payloadRoot,
		destination.Path,
	); err != nil {
		cleanup()
		return providerFailure(
			"TransferCommit",
			destinationMount.ID,
			destination.Path,
			err,
		)
	}
	cleanup()
	_ = source
	return nil
}

func (s *Service) streamTransferFile(
	sourceMount Mount,
	destinationMount Mount,
	entry transferEntry,
	stagingPath string,
	alreadyWritten int64,
) (int64, error) {
	reader, err := sourceMount.Medium.ReadStream(entry.SourcePath)
	if err != nil {
		return 0, providerFailure(
			"TransferRead",
			sourceMount.ID,
			entry.SourcePath,
			err,
		)
	}
	writer, err := destinationMount.Medium.WriteStream(stagingPath)
	if err != nil {
		_ = reader.Close()
		return 0, providerFailure(
			"TransferWrite",
			destinationMount.ID,
			entry.DestinationPath,
			err,
		)
	}
	buffer := make([]byte, transferBufferBytes)
	written := int64(0)
	copyErr := error(nil)
	limits := s.transferLimits()
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			next := written + int64(count)
			if next > entry.SizeBytes ||
				alreadyWritten+next > limits.MaxTransferBytes {
				copyErr = newFailure(
					ErrorLimitExceeded,
					sourceMount.ID,
					entry.SourcePath,
					"The provider changed an entry beyond the transfer limit.",
					nil,
				)
				break
			}
			if err := writeAll(writer, buffer[:count]); err != nil {
				copyErr = providerFailure(
					"TransferWrite",
					destinationMount.ID,
					entry.DestinationPath,
					err,
				)
				break
			}
			written = next
		}
		if readErr != nil {
			if readErr != goio.EOF {
				copyErr = providerFailure(
					"TransferRead",
					sourceMount.ID,
					entry.SourcePath,
					readErr,
				)
			}
			break
		}
	}
	writerCloseErr := writer.Close()
	readerCloseErr := reader.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if writerCloseErr != nil {
		return written, providerFailure(
			"TransferWrite",
			destinationMount.ID,
			entry.DestinationPath,
			writerCloseErr,
		)
	}
	if readerCloseErr != nil {
		return written, providerFailure(
			"TransferRead",
			sourceMount.ID,
			entry.SourcePath,
			readerCloseErr,
		)
	}
	return written, nil
}

func writeAll(writer goio.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 {
			return goio.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func stagedTransferPath(
	payloadRoot string,
	destinationRoot string,
	entryDestination string,
) string {
	if entryDestination == destinationRoot {
		return payloadRoot
	}
	suffix := core.TrimPrefix(entryDestination, destinationRoot)
	suffix = core.TrimPrefix(suffix, "/")
	return joinRelative(payloadRoot, suffix)
}

func (s *Service) lockMounts(ids ...string) func() {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]bool)
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	sort.Strings(unique)
	for _, id := range unique {
		s.locks[id].Lock()
	}
	return func() {
		for index := len(unique) - 1; index >= 0; index-- {
			s.locks[unique[index]].Unlock()
		}
	}
}

func (s *Service) transferLimits() Limits {
	limits := s.limits
	defaults := DefaultLimits()
	if limits.MaxRecursiveDepth <= 0 {
		limits.MaxRecursiveDepth = defaults.MaxRecursiveDepth
	}
	if limits.MaxRecursiveItems <= 0 {
		limits.MaxRecursiveItems = defaults.MaxRecursiveItems
	}
	if limits.MaxTransferBytes <= 0 {
		limits.MaxTransferBytes = defaults.MaxTransferBytes
	}
	return limits
}

func completedTransferResult(
	operationID string,
	operation string,
	source FileAddress,
	destination FileAddress,
	message string,
) FileOperationResult {
	return FileOperationResult{
		OperationID: operationID,
		Operation:   operation,
		Status:      OperationCompleted,
		Source:      source,
		Destination: &destination,
		Affected:    []FileAddress{source, destination},
		Message:     message,
	}
}
