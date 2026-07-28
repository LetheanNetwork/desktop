// SPDX-License-Identifier: EUPL-1.2

package files

import (
	goio "io"
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const (
	maxHostItems     = 16
	maxHostMounts    = 128
	maxHostPathBytes = 4096
)

// HostItemView is the renderer-safe address of one operating-system supplied
// item. Provider roots and native paths never appear in this projection.
type HostItemView struct {
	MountID   string    `json:"mountId"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Kind      EntryKind `json:"kind"`
	MediaType string    `json:"mediaType"`
}

type preparedHostItem struct {
	view  HostItemView
	mount *Mount
}

// ResolveHostItems converts trusted native lifecycle paths into registered
// Files capabilities. It is deliberately a free Go function, not a method on
// the Wails-bound Service.
//
// Usage example:
//
//	result := files.ResolveHostItems(service, selectedPaths)
func ResolveHostItems(service *Service, paths []string) core.Result {
	if service == nil {
		return core.Fail(hostItemFailure(
			ErrorProviderUnavailable,
			"Files service is unavailable.",
			nil,
		))
	}
	if len(paths) == 0 {
		return core.Fail(hostItemFailure(
			ErrorInvalidInput,
			"At least one selected item is required.",
			nil,
		))
	}
	if len(paths) > maxHostItems {
		return core.Fail(hostItemFailure(
			ErrorLimitExceeded,
			"Too many selected items were supplied.",
			nil,
		))
	}

	normalised := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, raw := range paths {
		path, err := normaliseHostPath(raw)
		if err != nil {
			return core.Fail(err)
		}
		key := core.Lower(core.PathToSlash(path))
		if seen[key] {
			return core.Fail(hostItemFailure(
				ErrorInvalidInput,
				"Duplicate selected items are not accepted.",
				nil,
			))
		}
		seen[key] = true
		normalised = append(normalised, path)
	}

	prepared := make([]preparedHostItem, 0, len(normalised))
	for _, hostPath := range normalised {
		item, err := service.prepareHostItem(hostPath)
		if err != nil {
			closePreparedHostItems(prepared)
			return core.Fail(err)
		}
		prepared = append(prepared, item)
	}

	views := make([]HostItemView, 0, len(prepared))
	for index := range prepared {
		item := &prepared[index]
		if item.mount != nil {
			if err := service.registerHostMount(*item.mount); err != nil {
				closePreparedHostItems(prepared[index:])
				service.removeHostMounts(views)
				return core.Fail(err)
			}
		}
		views = append(views, item.view)
	}
	return core.Ok(views)
}

func (s *Service) prepareHostItem(hostPath string) (preparedHostItem, error) {
	for _, mount := range s.localMountsBySpecificity() {
		relativeResult := core.PathRel(mount.LocalRoot, hostPath)
		if !relativeResult.OK {
			continue
		}
		relative, ok := relativeResult.Value.(string)
		if !ok {
			continue
		}
		relative = core.PathToSlash(relative)
		if relative == ".." || core.HasPrefix(relative, "../") ||
			core.PathIsAbs(relative) {
			continue
		}
		if relative == "." {
			relative = ""
		}
		clean, err := normaliseRelativePath(relative, true)
		if err != nil {
			return preparedHostItem{}, err
		}
		info, err := mount.Medium.Stat(clean)
		if err != nil {
			return preparedHostItem{}, hostItemProviderFailure(err)
		}
		view, err := hostItemView(mount.ID, clean, info)
		if err != nil {
			return preparedHostItem{}, err
		}
		return preparedHostItem{view: view}, nil
	}

	factory := s.hostMediumFactory
	if factory == nil {
		return preparedHostItem{}, hostItemFailure(
			ErrorProviderUnavailable,
			"Selected host items are unavailable.",
			nil,
		)
	}
	parent := core.PathDir(hostPath)
	name := core.PathBase(hostPath)
	parentMedium, err := factory(parent)
	if err != nil || parentMedium == nil {
		return preparedHostItem{}, hostItemFailure(
			ErrorProviderUnavailable,
			"Selected host items are unavailable.",
			err,
		)
	}
	info, err := parentMedium.Stat(name)
	if err != nil {
		closeMedium(parentMedium)
		return preparedHostItem{}, hostItemProviderFailure(err)
	}
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		closeMedium(parentMedium)
		return preparedHostItem{}, hostItemFailure(
			ErrorUnsupportedEntry,
			"Only regular files and directories can be opened.",
			nil,
		)
	}

	var medium coreio.Medium
	relative := name
	if kind == EntryDirectory {
		closeMedium(parentMedium)
		medium, err = factory(hostPath)
		relative = ""
	} else {
		medium = newSelectedFileMedium(parentMedium, name)
	}
	if err != nil || medium == nil {
		closeMedium(medium)
		return preparedHostItem{}, hostItemProviderFailure(err)
	}
	mountIDResult := core.RandomString(12)
	if !mountIDResult.OK {
		closeMedium(medium)
		return preparedHostItem{}, hostItemFailure(
			ErrorProviderUnavailable,
			"Selected host items are unavailable.",
			mountIDResult.Err(),
		)
	}
	mountID := core.Concat("host-", mountIDResult.Value.(string))
	view, err := hostItemView(mountID, relative, info)
	if err != nil {
		closeMedium(medium)
		return preparedHostItem{}, err
	}
	return preparedHostItem{
		view: view,
		mount: &Mount{
			ID:        mountID,
			Name:      name,
			Kind:      "host",
			LocalRoot: hostItemRoot(hostPath, kind),
			Icon:      hostItemIcon(kind),
			Capabilities: Capabilities{
				List:    true,
				Preview: true,
				Open:    true,
				Reveal:  true,
			},
			Medium:             &readOnlyMedium{Medium: medium},
			Owned:              true,
			ContainmentAudited: true,
		},
	}, nil
}

func (s *Service) localMountsBySpecificity() []Mount {
	mounts := make([]Mount, 0, len(s.mounts))
	for _, mount := range s.mounts {
		if mount.Kind == "local" && mount.LocalRoot != "" {
			mounts = append(mounts, mount)
		}
	}
	for left := 0; left < len(mounts); left++ {
		for right := left + 1; right < len(mounts); right++ {
			if len(mounts[right].LocalRoot) > len(mounts[left].LocalRoot) {
				mounts[left], mounts[right] = mounts[right], mounts[left]
			}
		}
	}
	return mounts
}

func (s *Service) registerHostMount(mount Mount) error {
	if err := validateMountID(mount.ID); err != nil {
		return err
	}
	if mount.Name == "" || mount.Medium == nil ||
		mount.LocalRoot == "" || !core.PathIsAbs(mount.LocalRoot) ||
		!mount.ContainmentAudited || mount.Kind != "host" {
		return hostItemFailure(
			ErrorBoundaryRejected,
			"Selected host capability is invalid.",
			nil,
		)
	}
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	if len(s.hostMounts) >= maxHostMounts {
		return hostItemFailure(
			ErrorLimitExceeded,
			"Too many selected host capabilities are active.",
			nil,
		)
	}
	if _, exists := s.mounts[mount.ID]; exists {
		return hostItemFailure(
			ErrorConflict,
			"Selected host capability already exists.",
			nil,
		)
	}
	if _, exists := s.hostMounts[mount.ID]; exists {
		return hostItemFailure(
			ErrorConflict,
			"Selected host capability already exists.",
			nil,
		)
	}
	s.hostMounts[mount.ID] = mount
	s.hostOrder = append(s.hostOrder, mount.ID)
	return nil
}

func hostItemRoot(hostPath string, kind EntryKind) string {
	if kind == EntryDirectory {
		return hostPath
	}
	return core.PathDir(hostPath)
}

func (s *Service) hostMountSnapshot() []Mount {
	if s == nil {
		return nil
	}
	s.hostMu.RLock()
	defer s.hostMu.RUnlock()
	mounts := make([]Mount, 0, len(s.hostOrder))
	for _, id := range s.hostOrder {
		if mount, ok := s.hostMounts[id]; ok {
			mounts = append(mounts, mount)
		}
	}
	return mounts
}

func (s *Service) removeHostMounts(items []HostItemView) {
	if s == nil || len(items) == 0 {
		return
	}
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	remove := make(map[string]bool, len(items))
	for _, item := range items {
		remove[item.MountID] = true
		if mount, ok := s.hostMounts[item.MountID]; ok {
			closeMedium(mount.Medium)
			delete(s.hostMounts, item.MountID)
		}
	}
	kept := s.hostOrder[:0]
	for _, id := range s.hostOrder {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	s.hostOrder = kept
}

func normaliseHostPath(raw string) (string, error) {
	if raw == "" || len(raw) > maxHostPathBytes || !core.PathIsAbs(raw) ||
		hasUnsafePathRune(raw) {
		return "", hostItemFailure(
			ErrorInvalidInput,
			"Selected host path is invalid.",
			nil,
		)
	}
	slash := core.PathToSlash(raw)
	for _, segment := range core.Split(slash, "/") {
		if segment == "." || segment == ".." {
			return "", hostItemFailure(
				ErrorInvalidInput,
				"Selected host path traversal is not accepted.",
				nil,
			)
		}
	}
	clean := core.CleanPath(raw, string(core.PathSeparator))
	if clean == "." || !core.PathIsAbs(clean) {
		return "", hostItemFailure(
			ErrorInvalidInput,
			"Selected host path is invalid.",
			nil,
		)
	}
	return clean, nil
}

func hostItemView(
	mountID string,
	relativePath string,
	info fs.FileInfo,
) (HostItemView, error) {
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		return HostItemView{}, hostItemFailure(
			ErrorUnsupportedEntry,
			"Only regular files and directories can be opened.",
			nil,
		)
	}
	name := ""
	if info != nil {
		name = info.Name()
	}
	if name == "" || name == "." {
		name = core.PathBase(relativePath)
	}
	if name == "" || name == "." || len(name) > 255 ||
		hasUnsafePathRune(name) {
		return HostItemView{}, hostItemFailure(
			ErrorInvalidInput,
			"Selected host item name is invalid.",
			nil,
		)
	}
	mediaType := "inode/directory"
	if kind == EntryFile {
		mediaType = previewMIME(relativePath, false)
	}
	return HostItemView{
		MountID:   mountID,
		Path:      relativePath,
		Name:      name,
		Kind:      kind,
		MediaType: mediaType,
	}, nil
}

func hostItemIcon(kind EntryKind) string {
	if kind == EntryDirectory {
		return "folder"
	}
	return "file"
}

func hostItemProviderFailure(err error) *Failure {
	code := ErrorProviderUnavailable
	if core.Is(err, fs.ErrNotExist) {
		code = ErrorMissingEntry
	} else if core.Is(err, fs.ErrPermission) {
		code = ErrorCapabilityDenied
	}
	return hostItemFailure(
		code,
		"The selected host item is unavailable.",
		err,
	)
}

func hostItemFailure(
	code ErrorCode,
	message string,
	cause error,
) *Failure {
	return newFailure(code, "", "", message, cause)
}

func closePreparedHostItems(items []preparedHostItem) {
	for _, item := range items {
		if item.mount != nil {
			closeMedium(item.mount.Medium)
		}
	}
}

func closeMedium(medium coreio.Medium) {
	if closer, ok := medium.(goio.Closer); ok && closer != nil {
		_ = closer.Close()
	}
}

type readOnlyMedium struct {
	coreio.Medium
}

func (medium *readOnlyMedium) Write(string, string) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) WriteMode(
	string,
	string,
	fs.FileMode,
) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) EnsureDir(string) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) Delete(string) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) DeleteAll(string) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) Rename(string, string) error {
	return fs.ErrPermission
}

func (medium *readOnlyMedium) Create(string) (goio.WriteCloser, error) {
	return nil, fs.ErrPermission
}

func (medium *readOnlyMedium) Append(string) (goio.WriteCloser, error) {
	return nil, fs.ErrPermission
}

func (medium *readOnlyMedium) WriteStream(string) (goio.WriteCloser, error) {
	return nil, fs.ErrPermission
}

func (medium *readOnlyMedium) Close() error {
	if medium == nil {
		return nil
	}
	if closer, ok := medium.Medium.(goio.Closer); ok {
		return closer.Close()
	}
	return nil
}

type selectedFileMedium struct {
	coreio.Medium
	name string
}

func newSelectedFileMedium(
	medium coreio.Medium,
	name string,
) coreio.Medium {
	return &selectedFileMedium{Medium: medium, name: name}
}

func (medium *selectedFileMedium) Read(path string) (string, error) {
	if !medium.accepts(path, false) {
		return "", fs.ErrPermission
	}
	return medium.Medium.Read(path)
}

func (medium *selectedFileMedium) IsFile(path string) bool {
	return medium.accepts(path, false) && medium.Medium.IsFile(path)
}

func (medium *selectedFileMedium) List(path string) ([]fs.DirEntry, error) {
	if path != "" && path != "." {
		return nil, fs.ErrPermission
	}
	entries, err := medium.Medium.List("")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() == medium.name {
			return []fs.DirEntry{entry}, nil
		}
	}
	return []fs.DirEntry{}, nil
}

func (medium *selectedFileMedium) Stat(path string) (fs.FileInfo, error) {
	if path == "" || path == "." {
		return medium.Medium.Stat("")
	}
	if !medium.accepts(path, false) {
		return nil, fs.ErrPermission
	}
	return medium.Medium.Stat(path)
}

func (medium *selectedFileMedium) Open(path string) (fs.File, error) {
	if !medium.accepts(path, false) {
		return nil, fs.ErrPermission
	}
	return medium.Medium.Open(path)
}

func (medium *selectedFileMedium) ReadStream(
	path string,
) (goio.ReadCloser, error) {
	if !medium.accepts(path, false) {
		return nil, fs.ErrPermission
	}
	return medium.Medium.ReadStream(path)
}

func (medium *selectedFileMedium) Exists(path string) bool {
	if path == "" || path == "." {
		return true
	}
	return medium.accepts(path, false) && medium.Medium.Exists(path)
}

func (medium *selectedFileMedium) IsDir(path string) bool {
	return path == "" || path == "."
}

func (medium *selectedFileMedium) Close() error {
	if medium == nil {
		return nil
	}
	if closer, ok := medium.Medium.(goio.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (medium *selectedFileMedium) accepts(
	path string,
	allowRoot bool,
) bool {
	if path == "" || path == "." {
		return allowRoot
	}
	clean, err := normaliseRelativePath(path, false)
	return err == nil && clean == medium.name
}
