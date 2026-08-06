// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"
	"sort"
	"strconv"

	core "dappco.re/go"
)

const defaultListLimit = 200

func (s *Service) listMounts() core.Result {
	if s == nil || s.runtime == nil {
		return core.Fail(newFailure(
			ErrorInvalidInput,
			"",
			"",
			"Files service is not registered",
			nil,
		))
	}
	runtime, err := s.runtime.Load()
	if err != nil {
		return core.Fail(providerFailure("ListMounts", "", "", err))
	}
	runtime, err = validateAndNormaliseRuntimeSnapshot(runtime)
	if err != nil {
		return core.Fail(err)
	}
	mounts := make([]FileMount, 0, len(s.pendingMounts))
	for _, configured := range s.pendingMounts {
		mount, ok := s.mounts[configured.ID]
		if !ok {
			continue
		}
		mounts = append(mounts, s.publicMount(mount))
	}
	for _, mount := range s.hostMountSnapshot() {
		mounts = append(mounts, s.publicMount(mount))
	}
	favourites := make([]Favourite, 0, len(runtime.Favourites))
	for _, favourite := range runtime.Favourites {
		if _, ok := s.mounts[favourite.MountID]; ok {
			favourites = append(favourites, favourite)
		}
	}
	recents := make([]Recent, 0, len(runtime.Recent))
	for _, recent := range runtime.Recent {
		if _, ok := s.mounts[recent.MountID]; ok {
			recents = append(recents, recent)
		}
	}
	return core.Ok(MountCatalogue{
		Mounts:     mounts,
		Favourites: favourites,
		Recent:     recents,
	})
}

func (s *Service) listDirectory(input ListDirectoryInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.List {
		return core.Fail(capabilityFailure(
			"ListDirectory",
			mount.ID,
			input.Path,
		))
	}
	relativePath, err := normaliseRelativePath(input.Path, true)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.Path))
	}
	offset, err := listOffset(input.Cursor)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, relativePath))
	}
	limit, err := s.listLimit(input.Limit)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, relativePath))
	}
	entries, err := mount.Medium.List(relativePath)
	if err != nil {
		return core.Fail(providerFailure(
			"ListDirectory",
			mount.ID,
			relativePath,
			err,
		))
	}
	// Two-phase build: classify + sort every entry cheaply first (no
	// string formatting/allocation beyond Name), then materialise the
	// full FileEntry (ModifiedAt formatting, RelativePath join) ONLY
	// for the entries that survive pagination. A large directory with
	// a small requested page previously paid the full per-entry
	// formatting cost for every discarded row — entry.Info() itself
	// is unavoidable (needed for sort classification, and already
	// satisfied without an extra syscall on top of the medium's List)
	// but the FileEntry string-building is real, avoidable, per-record
	// work that scales with collection size regardless of page size.
	indexed := make([]indexedEntry, 0, len(entries))
	for _, entry := range entries {
		if relativePath == "" && entry.Name() == internalNamespace {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return core.Fail(providerFailure(
				"ListDirectory",
				mount.ID,
				joinRelative(relativePath, entry.Name()),
				infoErr,
			))
		}
		indexed = append(indexed, indexedEntry{
			entry: entry,
			info:  info,
			kind:  entryKind(entry, info),
			name:  entry.Name(),
		})
	}
	sort.Slice(indexed, func(left, right int) bool {
		leftDirectory := indexed[left].kind == EntryDirectory
		rightDirectory := indexed[right].kind == EntryDirectory
		if leftDirectory != rightDirectory {
			return leftDirectory
		}
		leftFolded := core.Lower(indexed[left].name)
		rightFolded := core.Lower(indexed[right].name)
		if leftFolded == rightFolded {
			return indexed[left].name < indexed[right].name
		}
		return leftFolded < rightFolded
	})
	total := len(indexed)
	if offset > total {
		return core.Fail(newFailure(
			ErrorInvalidInput,
			mount.ID,
			relativePath,
			"directory cursor is outside the available range",
			nil,
		))
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := make([]FileEntry, 0, end-offset)
	for _, ie := range indexed[offset:end] {
		page = append(page, fileEntry(relativePath, ie.entry, ie.info))
	}
	nextCursor := ""
	if end < total {
		nextCursor = strconv.Itoa(end)
	}
	return core.Ok(DirectorySnapshot{
		Mount:       s.publicMount(mount),
		Path:        relativePath,
		Breadcrumbs: breadcrumbs(relativePath),
		Entries:     page,
		NextCursor:  nextCursor,
		TotalKnown:  total,
		RefreshedAt: core.Now().UTC().Format(core.RFC3339Nano),
	})
}

func (s *Service) listLimit(requested int) (int, error) {
	maximum := s.limits.MaxListEntries
	if maximum <= 0 {
		maximum = DefaultLimits().MaxListEntries
	}
	if requested < 0 {
		return 0, newFailure(
			ErrorInvalidInput,
			"",
			"",
			"directory limit cannot be negative",
			nil,
		)
	}
	if requested == 0 {
		requested = defaultListLimit
	}
	if requested > maximum {
		requested = maximum
	}
	return requested, nil
}

func listOffset(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, newFailure(
			ErrorInvalidInput,
			"",
			"",
			"directory cursor is invalid",
			err,
		)
	}
	return offset, nil
}

// indexedEntry is the cheap, pre-format intermediate used to classify
// and sort a full directory listing before pagination decides which
// entries are worth the fuller FileEntry build (string formatting,
// path join). entry.Info() has already run by the time this is
// populated — the same fs.FileInfo is reused for the final FileEntry
// so a page entry's fields are computed identically to the pre-split
// code path, just deferred until after slicing.
type indexedEntry struct {
	entry fs.DirEntry
	info  fs.FileInfo
	kind  EntryKind
	name  string
}

// entryKind classifies a directory entry. Symlinks take priority
// (checked via both the cheap DirEntry type bits and the stat'd mode,
// matching legacy behaviour for filesystems where the directory read
// alone can't disambiguate); otherwise directory / regular file /
// other.
func entryKind(entry fs.DirEntry, info fs.FileInfo) EntryKind {
	mode := info.Mode()
	switch {
	case entry.Type()&fs.ModeSymlink != 0 || mode&fs.ModeSymlink != 0:
		return EntryLink
	case info.IsDir():
		return EntryDirectory
	case mode.IsRegular():
		return EntryFile
	}
	return EntryOther
}

func fileEntry(parent string, entry fs.DirEntry, info fs.FileInfo) FileEntry {
	mode := info.Mode()
	return FileEntry{
		Name:         entry.Name(),
		RelativePath: joinRelative(parent, entry.Name()),
		Kind:         entryKind(entry, info),
		SizeBytes:    info.Size(),
		ModifiedAt:   info.ModTime().UTC().Format(core.RFC3339Nano),
		Mode:         uint32(mode),
		Hidden:       core.HasPrefix(entry.Name(), "."),
	}
}

func (s *Service) publicMount(mount Mount) FileMount {
	capabilities := mount.Capabilities
	if !s.internalReady[mount.ID] {
		capabilities.CopyTo = false
		capabilities.Trash = false
		capabilities.Restore = false
	}
	return FileMount{
		ID:           mount.ID,
		Name:         mount.Name,
		Kind:         mount.Kind,
		Icon:         mount.Icon,
		Brand:        mount.Brand,
		Capabilities: capabilities,
	}
}

func breadcrumbs(relativePath string) []Breadcrumb {
	if relativePath == "" {
		return []Breadcrumb{}
	}
	parts := core.Split(relativePath, "/")
	rows := make([]Breadcrumb, 0, len(parts))
	for index, name := range parts {
		rows = append(rows, Breadcrumb{
			Name: name,
			Path: core.Join("/", parts[:index+1]...),
		})
	}
	return rows
}

func joinRelative(parent, name string) string {
	if parent == "" {
		return name
	}
	return core.Join("/", parent, name)
}

func capabilityFailure(operation, mountID, relativePath string) *Failure {
	return newFailure(
		ErrorCapabilityDenied,
		mountID,
		relativePath,
		core.Concat(operation, " is not permitted for this mount"),
		nil,
	)
}

func providerFailure(
	operation string,
	mountID string,
	relativePath string,
	err error,
) *Failure {
	code := ErrorProviderUnavailable
	message := "The storage provider is unavailable."
	switch {
	case core.Is(err, fs.ErrNotExist):
		code = ErrorMissingEntry
		message = "The requested entry is no longer available."
	case core.Is(err, fs.ErrPermission):
		code = ErrorCapabilityDenied
		message = "The storage provider denied this operation."
	}
	core.Warn(
		"Files provider operation failed",
		"operation",
		operation,
		"mount",
		mountID,
		"path",
		relativePath,
		"err",
		err,
	)
	return newFailure(code, mountID, relativePath, message, err)
}

func withAddress(err error, mountID, relativePath string) error {
	failure, ok := err.(*Failure)
	if !ok {
		return providerFailure("Validate", mountID, relativePath, err)
	}
	copy := *failure
	copy.MountID = mountID
	copy.Path = relativePath
	return &copy
}
