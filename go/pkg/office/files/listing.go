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
	rows := make([]FileEntry, 0, len(entries))
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
		rows = append(rows, fileEntry(relativePath, entry, info))
	}
	sort.Slice(rows, func(left, right int) bool {
		leftDirectory := rows[left].Kind == EntryDirectory
		rightDirectory := rows[right].Kind == EntryDirectory
		if leftDirectory != rightDirectory {
			return leftDirectory
		}
		leftFolded := core.Lower(rows[left].Name)
		rightFolded := core.Lower(rows[right].Name)
		if leftFolded == rightFolded {
			return rows[left].Name < rows[right].Name
		}
		return leftFolded < rightFolded
	})
	total := len(rows)
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
	page := append([]FileEntry(nil), rows[offset:end]...)
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

func fileEntry(parent string, entry fs.DirEntry, info fs.FileInfo) FileEntry {
	kind := EntryOther
	mode := info.Mode()
	switch {
	case entry.Type()&fs.ModeSymlink != 0 || mode&fs.ModeSymlink != 0:
		kind = EntryLink
	case info.IsDir():
		kind = EntryDirectory
	case mode.IsRegular():
		kind = EntryFile
	}
	return FileEntry{
		Name:         entry.Name(),
		RelativePath: joinRelative(parent, entry.Name()),
		Kind:         kind,
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
