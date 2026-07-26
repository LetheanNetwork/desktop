// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"
	"path"
	"sort"
	"sync"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const (
	runtimeVersion    = 1
	maxRecentEntries  = 100
	runtimeStagingDir = ".lthn-files/staging/runtime"
)

type mediumRuntimeMetadata struct {
	mu     sync.Mutex
	medium coreio.Medium
	path   string
}

// NewMediumRuntimeMetadata creates a metadata document transported by the
// supplied Medium.
func NewMediumRuntimeMetadata(
	medium coreio.Medium,
	relativePath string,
) RuntimeMetadata {
	return &mediumRuntimeMetadata{medium: medium, path: relativePath}
}

// NewMemoryRuntimeMetadata uses the same Medium persistence path as production
// while keeping tests and explicit demo state ephemeral.
func NewMemoryRuntimeMetadata() RuntimeMetadata {
	return NewMediumRuntimeMetadata(coreio.NewMemoryMedium(), "runtime.json")
}

func (runtime *mediumRuntimeMetadata) Load() (RuntimeSnapshot, error) {
	if runtime == nil {
		return RuntimeSnapshot{}, newFailure(
			ErrorInvalidInput,
			"",
			"",
			"runtime metadata is unavailable",
			nil,
		)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.loadUnlocked()
}

func (runtime *mediumRuntimeMetadata) Save(snapshot RuntimeSnapshot) error {
	if runtime == nil {
		return newFailure(
			ErrorInvalidInput,
			"",
			"",
			"runtime metadata is unavailable",
			nil,
		)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.medium == nil {
		return newFailure(
			ErrorProviderUnavailable,
			"",
			"",
			"runtime Medium is unavailable",
			nil,
		)
	}
	runtimePath, err := runtime.validPath()
	if err != nil {
		return err
	}
	snapshot, err = validateAndNormaliseRuntimeSnapshot(snapshot)
	if err != nil {
		return err
	}
	marshalled := core.JSONMarshal(snapshot)
	if !marshalled.OK {
		return newFailure(
			ErrorInvalidInput,
			"",
			runtimePath,
			"runtime metadata could not be serialised",
			marshalled.Err(),
		)
	}
	payload, ok := marshalled.Value.([]byte)
	if !ok {
		return newFailure(
			ErrorInvalidInput,
			"",
			runtimePath,
			"runtime metadata serialisation was invalid",
			nil,
		)
	}
	if err := ensureMediumParent(runtime.medium, runtimePath); err != nil {
		return providerFailure("RuntimeSave", "", runtimePath, err)
	}
	if err := runtime.medium.EnsureDir(runtimeStagingDir); err != nil {
		return providerFailure(
			"RuntimeSave",
			"",
			runtimeStagingDir,
			err,
		)
	}
	operationID := core.ID()
	stagedPath := core.Join(
		"/",
		runtimeStagingDir,
		core.Concat(operationID, ".new.json"),
	)
	backupPath := core.Join(
		"/",
		runtimeStagingDir,
		core.Concat(operationID, ".backup.json"),
	)
	if err := runtime.medium.WriteMode(stagedPath, string(payload), 0600); err != nil {
		return providerFailure("RuntimeSave", "", stagedPath, err)
	}
	hadDocument := false
	if _, statErr := runtime.medium.Stat(runtimePath); statErr == nil {
		hadDocument = true
	} else if !core.Is(statErr, fs.ErrNotExist) {
		_ = deleteMediumEntry(runtime.medium, stagedPath)
		return providerFailure("RuntimeSave", "", runtimePath, statErr)
	}
	if hadDocument {
		if err := runtime.medium.Rename(runtimePath, backupPath); err != nil {
			_ = deleteMediumEntry(runtime.medium, stagedPath)
			return providerFailure("RuntimeSave", "", runtimePath, err)
		}
	}
	if err := runtime.medium.Rename(stagedPath, runtimePath); err != nil {
		restoreErr := error(nil)
		if hadDocument {
			restoreErr = runtime.medium.Rename(backupPath, runtimePath)
		}
		_ = deleteMediumEntry(runtime.medium, stagedPath)
		if restoreErr != nil {
			return newFailure(
				ErrorProviderUnavailable,
				"",
				runtimePath,
				"runtime metadata commit and recovery both failed",
				restoreErr,
			)
		}
		return providerFailure("RuntimeSave", "", runtimePath, err)
	}
	if hadDocument {
		if err := runtime.medium.Delete(backupPath); err != nil {
			return providerFailure("RuntimeSave", "", backupPath, err)
		}
	}
	return nil
}

func (runtime *mediumRuntimeMetadata) loadUnlocked() (RuntimeSnapshot, error) {
	if runtime.medium == nil {
		return RuntimeSnapshot{}, newFailure(
			ErrorProviderUnavailable,
			"",
			"",
			"runtime Medium is unavailable",
			nil,
		)
	}
	runtimePath, err := runtime.validPath()
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	content, err := runtime.medium.Read(runtimePath)
	if err == nil {
		return decodeRuntimeSnapshot(content)
	}
	if !core.Is(err, fs.ErrNotExist) {
		return RuntimeSnapshot{}, providerFailure(
			"RuntimeLoad",
			"",
			runtimePath,
			err,
		)
	}
	recovered, recoveryErr := runtime.recoverBackup(runtimePath)
	if recoveryErr != nil {
		return RuntimeSnapshot{}, recoveryErr
	}
	if recovered {
		content, err = runtime.medium.Read(runtimePath)
		if err != nil {
			return RuntimeSnapshot{}, providerFailure(
				"RuntimeLoad",
				"",
				runtimePath,
				err,
			)
		}
		return decodeRuntimeSnapshot(content)
	}
	return normaliseRuntimeSnapshot(RuntimeSnapshot{}), nil
}

func (runtime *mediumRuntimeMetadata) recoverBackup(
	runtimePath string,
) (bool, error) {
	entries, err := runtime.medium.List(runtimeStagingDir)
	if err != nil {
		if core.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, providerFailure(
			"RuntimeRecover",
			"",
			runtimeStagingDir,
			err,
		)
	}
	type candidate struct {
		path    string
		modTime core.Time
	}
	candidates := make([]candidate, 0, len(entries))
	invalidCandidate := false
	for _, entry := range entries {
		if entry.IsDir() || !core.HasSuffix(entry.Name(), ".backup.json") {
			continue
		}
		candidatePath := core.Join("/", runtimeStagingDir, entry.Name())
		content, readErr := runtime.medium.Read(candidatePath)
		if readErr != nil {
			return false, providerFailure(
				"RuntimeRecover",
				"",
				candidatePath,
				readErr,
			)
		}
		if _, decodeErr := decodeRuntimeSnapshot(content); decodeErr != nil {
			invalidCandidate = true
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return false, providerFailure(
				"RuntimeRecover",
				"",
				candidatePath,
				infoErr,
			)
		}
		candidates = append(candidates, candidate{
			path:    candidatePath,
			modTime: info.ModTime(),
		})
	}
	if len(candidates) == 0 {
		if invalidCandidate {
			return false, newFailure(
				ErrorProviderUnavailable,
				"",
				runtimeStagingDir,
				"runtime recovery contains no valid backup",
				nil,
			)
		}
		return false, nil
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].modTime.Equal(candidates[right].modTime) {
			return candidates[left].path > candidates[right].path
		}
		return candidates[left].modTime.After(candidates[right].modTime)
	})
	if err := ensureMediumParent(runtime.medium, runtimePath); err != nil {
		return false, providerFailure("RuntimeRecover", "", runtimePath, err)
	}
	if err := runtime.medium.Rename(candidates[0].path, runtimePath); err != nil {
		return false, providerFailure(
			"RuntimeRecover",
			"",
			runtimePath,
			err,
		)
	}
	return true, nil
}

func (runtime *mediumRuntimeMetadata) validPath() (string, error) {
	return normaliseRelativePath(runtime.path, false)
}

func decodeRuntimeSnapshot(content string) (RuntimeSnapshot, error) {
	var snapshot RuntimeSnapshot
	result := core.JSONUnmarshalString(content, &snapshot)
	if !result.OK {
		return RuntimeSnapshot{}, newFailure(
			ErrorProviderUnavailable,
			"",
			"",
			"runtime metadata is malformed",
			result.Err(),
		)
	}
	return validateAndNormaliseRuntimeSnapshot(snapshot)
}

func validateAndNormaliseRuntimeSnapshot(
	snapshot RuntimeSnapshot,
) (RuntimeSnapshot, error) {
	snapshot = normaliseRuntimeSnapshot(snapshot)
	if snapshot.Version != runtimeVersion {
		return RuntimeSnapshot{}, newFailure(
			ErrorInvalidInput,
			"",
			"",
			"runtime metadata version is unsupported",
			nil,
		)
	}
	for _, favourite := range snapshot.Favourites {
		if err := validateMountID(favourite.MountID); err != nil {
			return RuntimeSnapshot{}, err
		}
		if _, err := normaliseRelativePath(favourite.Path, true); err != nil {
			return RuntimeSnapshot{}, err
		}
	}
	type recentWithTime struct {
		recent Recent
		at     core.Time
	}
	seenRecent := make(map[string]bool)
	recents := make([]recentWithTime, 0, len(snapshot.Recent))
	for _, recent := range snapshot.Recent {
		if err := validateMountID(recent.MountID); err != nil {
			return RuntimeSnapshot{}, err
		}
		relativePath, err := normaliseRelativePath(recent.Path, false)
		if err != nil {
			return RuntimeSnapshot{}, err
		}
		if recent.Kind != EntryFile &&
			recent.Kind != EntryDirectory &&
			recent.Kind != EntryLink &&
			recent.Kind != EntryOther {
			return RuntimeSnapshot{}, newFailure(
				ErrorInvalidInput,
				recent.MountID,
				relativePath,
				"recent entry kind is invalid",
				nil,
			)
		}
		parsed := core.TimeParse(core.RFC3339Nano, recent.OpenedAt)
		if !parsed.OK {
			return RuntimeSnapshot{}, newFailure(
				ErrorInvalidInput,
				recent.MountID,
				relativePath,
				"recent timestamp is invalid",
				parsed.Err(),
			)
		}
		at, ok := parsed.Value.(core.Time)
		if !ok {
			return RuntimeSnapshot{}, newFailure(
				ErrorInvalidInput,
				recent.MountID,
				relativePath,
				"recent timestamp is invalid",
				nil,
			)
		}
		recent.Path = relativePath
		recents = append(recents, recentWithTime{recent: recent, at: at})
	}
	sort.SliceStable(recents, func(left, right int) bool {
		return recents[left].at.After(recents[right].at)
	})
	normalisedRecent := make([]Recent, 0, len(recents))
	for _, row := range recents {
		key := core.Concat(row.recent.MountID, "\x00", row.recent.Path)
		if seenRecent[key] {
			continue
		}
		seenRecent[key] = true
		normalisedRecent = append(normalisedRecent, row.recent)
		if len(normalisedRecent) == maxRecentEntries {
			break
		}
	}
	snapshot.Recent = normalisedRecent
	for _, receipt := range snapshot.Trash {
		if receipt.ID == "" {
			return RuntimeSnapshot{}, newFailure(
				ErrorInvalidInput,
				receipt.MountID,
				receipt.OriginalPath,
				"trash receipt ID is required",
				nil,
			)
		}
		if err := validateMountID(receipt.MountID); err != nil {
			return RuntimeSnapshot{}, err
		}
		if _, err := normaliseRelativePath(receipt.OriginalPath, false); err != nil {
			return RuntimeSnapshot{}, err
		}
		if err := validateTrustedTrashPath(receipt.TrashPath); err != nil {
			return RuntimeSnapshot{}, err
		}
	}
	return snapshot, nil
}

func validateTrustedTrashPath(relativePath string) error {
	prefix := core.Concat(internalNamespace, "/trash/")
	if !core.HasPrefix(relativePath, prefix) {
		return newFailure(
			ErrorBoundaryRejected,
			"",
			"",
			"trash receipt path is outside the owned namespace",
			nil,
		)
	}
	for _, component := range core.Split(relativePath, "/") {
		if component == "" ||
			component == "." ||
			component == ".." ||
			hasUnsafePathRune(component) {
			return newFailure(
				ErrorBoundaryRejected,
				"",
				"",
				"trash receipt path is invalid",
				nil,
			)
		}
	}
	return nil
}

func ensureMediumParent(medium coreio.Medium, relativePath string) error {
	parent := path.Dir(relativePath)
	if parent == "." || parent == "" {
		return nil
	}
	return medium.EnsureDir(parent)
}

func deleteMediumEntry(medium coreio.Medium, relativePath string) error {
	_, err := medium.Stat(relativePath)
	if core.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return medium.Delete(relativePath)
}
