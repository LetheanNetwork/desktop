// SPDX-Licence-Identifier: EUPL-1.2

// Package files is the lthn-side Office filesystem browser service.
// Surfaces saved locations (workspace dirs), recent files, and disk
// usage for <lthn-view-files> in the Office role view.
//
// v1 scope: read-only catalogue from canonical workspace roots.
// No inotify / FSEvents watcher; no user-configurable custom locations.
// v2 (separate Mantis ticket) adds FSEvents push + user locations.
//
// Wire shape matches LocationRow / RecentRow / DiskMeter consumed by
// <lthn-view-files> in the Office role view.
//
// Usage example (Wails):
//
//	r := filesSvc.ListLocations()
//	if r.OK { out := r.Value.(files.ListLocationsOutput) }
package files

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// EntryKind is the provider-neutral kind exposed to the renderer.
type EntryKind string

const (
	EntryFile      EntryKind = "file"
	EntryDirectory EntryKind = "directory"
	EntryLink      EntryKind = "link"
	EntryOther     EntryKind = "other"
)

// ErrorCode is the stable renderer-visible Files failure namespace.
type ErrorCode string

const (
	ErrorInvalidInput        ErrorCode = "files.invalid_input"
	ErrorInvalidMount        ErrorCode = "files.invalid_mount"
	ErrorBoundaryRejected    ErrorCode = "files.boundary_rejected"
	ErrorCapabilityDenied    ErrorCode = "files.capability_denied"
	ErrorMissingEntry        ErrorCode = "files.missing_entry"
	ErrorConflict            ErrorCode = "files.conflict"
	ErrorProviderUnavailable ErrorCode = "files.provider_unavailable"
	ErrorLimitExceeded       ErrorCode = "files.limit_exceeded"
	ErrorUnsupportedEntry    ErrorCode = "files.unsupported_entry"
	ErrorPartialMove         ErrorCode = "files.partial_move"
)

// Capabilities declares the operations a registered mount permits.
type Capabilities struct {
	List            bool `json:"list"`
	Preview         bool `json:"preview"`
	CreateDirectory bool `json:"createDirectory"`
	Write           bool `json:"write"`
	Rename          bool `json:"rename"`
	CopyFrom        bool `json:"copyFrom"`
	CopyTo          bool `json:"copyTo"`
	Move            bool `json:"move"`
	Trash           bool `json:"trash"`
	Restore         bool `json:"restore"`
	Delete          bool `json:"delete"`
}

// ReadWriteCapabilities returns the complete mutable Files capability set.
func ReadWriteCapabilities() Capabilities {
	return Capabilities{
		List:            true,
		Preview:         true,
		CreateDirectory: true,
		Write:           true,
		Rename:          true,
		CopyFrom:        true,
		CopyTo:          true,
		Move:            true,
		Trash:           true,
		Restore:         true,
		Delete:          true,
	}
}

// Limits bounds provider work before it reaches a renderer-facing operation.
type Limits struct {
	MaxListEntries    int
	MaxPreviewBytes   int64
	MaxRecursiveDepth int
	MaxRecursiveItems int
	MaxTransferBytes  int64
}

// DefaultLimits returns the Files server limits.
func DefaultLimits() Limits {
	return Limits{
		MaxListEntries:    2_000,
		MaxPreviewBytes:   512 * 1024,
		MaxRecursiveDepth: 64,
		MaxRecursiveItems: 100_000,
		MaxTransferBytes:  64 * 1024 * 1024 * 1024,
	}
}

// Mount is trusted Go composition. Its Medium and provider details never cross
// the Wails boundary.
type Mount struct {
	ID                 string        `json:"-"`
	Name               string        `json:"-"`
	Kind               string        `json:"-"`
	Icon               string        `json:"-"`
	Brand              bool          `json:"-"`
	Capabilities       Capabilities  `json:"-"`
	Medium             coreio.Medium `json:"-"`
	ContainmentAudited bool          `json:"-"`
}

// Options configures the canonical Files service.
type Options struct {
	Mounts  []Mount
	Runtime RuntimeMetadata
	Limits  Limits
	Core    *core.Core
}

// RuntimeMetadata persists renderer-independent Files state through a Medium.
type RuntimeMetadata interface {
	Load() (RuntimeSnapshot, error)
	Save(RuntimeSnapshot) error
}

// RuntimeSnapshot is the complete versioned Files metadata document.
type RuntimeSnapshot struct {
	Version    int            `json:"version"`
	Favourites []Favourite    `json:"favourites"`
	Recent     []Recent       `json:"recent"`
	Trash      []TrashReceipt `json:"trash"`
}

// Favourite addresses one provider-relative Files location.
type Favourite struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

// Recent records a successfully previewed provider-relative entry.
type Recent struct {
	MountID  string    `json:"mountId"`
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Kind     EntryKind `json:"kind"`
	OpenedAt string    `json:"openedAt"`
}

// TrashReceipt is trusted metadata for an item moved into a mount's owned
// internal namespace.
type TrashReceipt struct {
	ID           string `json:"id"`
	MountID      string `json:"mountId"`
	OriginalPath string `json:"originalPath"`
	TrashPath    string `json:"trashPath"`
	TrashedAt    string `json:"trashedAt"`
}

// FileMount is the provider-neutral mount catalogue row exposed to Angular.
type FileMount struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Icon         string       `json:"icon"`
	Brand        bool         `json:"brand,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
	Capacity     *Capacity    `json:"capacity,omitempty"`
}

// Capacity is optional provider-reported capacity data.
type Capacity struct {
	FreeBytes  int64 `json:"freeBytes"`
	TotalBytes int64 `json:"totalBytes"`
}

// FileEntry is one provider-relative directory row.
type FileEntry struct {
	Name         string    `json:"name"`
	RelativePath string    `json:"relativePath"`
	Kind         EntryKind `json:"kind"`
	SizeBytes    int64     `json:"sizeBytes"`
	ModifiedAt   string    `json:"modifiedAt"`
	Mode         uint32    `json:"mode"`
	Hidden       bool      `json:"hidden"`
}

// Breadcrumb is a reversible provider-relative navigation segment.
type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// MountCatalogue is the Files Home response.
type MountCatalogue struct {
	Mounts     []FileMount `json:"mounts"`
	Favourites []Favourite `json:"favourites"`
	Recent     []Recent    `json:"recent"`
}

// ListDirectoryInput contains only a registered mount ID and provider-relative
// address.
type ListDirectoryInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// DirectorySnapshot is a deterministic bounded directory page.
type DirectorySnapshot struct {
	Mount       FileMount    `json:"mount"`
	Path        string       `json:"path"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	Entries     []FileEntry  `json:"entries"`
	NextCursor  string       `json:"nextCursor,omitempty"`
	TotalKnown  int          `json:"totalKnown"`
	RefreshedAt string       `json:"refreshedAt"`
}

// PreviewInput addresses one provider-relative entry.
type PreviewInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

// FilePreview is a bounded renderer-safe file preview.
type FilePreview struct {
	MountID      string `json:"mountId"`
	RelativePath string `json:"relativePath"`
	Name         string `json:"name"`
	Content      string `json:"content,omitempty"`
	MIME         string `json:"mime"`
	BytesRead    int64  `json:"bytesRead"`
	SizeBytes    int64  `json:"sizeBytes"`
	Lines        int    `json:"lines"`
	Truncated    bool   `json:"truncated"`
	Binary       bool   `json:"binary"`
}

// OperationStatus is the stable mutation outcome namespace.
type OperationStatus string

const (
	OperationCompleted OperationStatus = "completed"
	OperationConflict  OperationStatus = "conflict"
	OperationPartial   OperationStatus = "partial"
)

// FileAddress contains only a registered mount and provider-relative path.
type FileAddress struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
}

// FileConflict preserves both sides of a non-destructive conflict.
type FileConflict struct {
	Source      FileAddress `json:"source"`
	Destination FileAddress `json:"destination"`
	Kind        EntryKind   `json:"kind"`
}

// FileOperationResult is the common renderer-facing mutation result.
type FileOperationResult struct {
	OperationID string          `json:"operationId"`
	Operation   string          `json:"operation"`
	Status      OperationStatus `json:"status"`
	Code        ErrorCode       `json:"code,omitempty"`
	Source      FileAddress     `json:"source"`
	Destination *FileAddress    `json:"destination,omitempty"`
	Affected    []FileAddress   `json:"affected"`
	Conflict    *FileConflict   `json:"conflict,omitempty"`
	Message     string          `json:"message"`
}

// CreateDirectoryInput creates one validated child of a relative parent.
type CreateDirectoryInput struct {
	MountID    string `json:"mountId"`
	ParentPath string `json:"parentPath,omitempty"`
	Name       string `json:"name"`
}

// RenameInput replaces only the final name of an existing relative path.
type RenameInput struct {
	MountID string `json:"mountId"`
	Path    string `json:"path"`
	Name    string `json:"name"`
}

// FileEvent is a small invalidation signal broadcast after mutation.
type FileEvent struct {
	Operation   string    `json:"operation"`
	OperationID string    `json:"operationId"`
	MountIDs    []string  `json:"mountIds"`
	Paths       []string  `json:"paths"`
	At          core.Time `json:"at"`
}

// Failure is a stable Files error which deliberately omits provider roots,
// endpoints, credentials, and low-level causes from its rendered text.
type Failure struct {
	Code    ErrorCode `json:"code"`
	MountID string    `json:"mountId,omitempty"`
	Path    string    `json:"path,omitempty"`
	Message string    `json:"message,omitempty"`
	cause   error
}

func (failure *Failure) Error() string {
	if failure == nil {
		return string(ErrorProviderUnavailable)
	}
	if failure.Message == "" {
		return string(failure.Code)
	}
	return core.Concat(string(failure.Code), ": ", failure.Message)
}

func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func newFailure(
	code ErrorCode,
	mountID string,
	relativePath string,
	message string,
	cause error,
) *Failure {
	return &Failure{
		Code:    code,
		MountID: mountID,
		Path:    relativePath,
		Message: message,
		cause:   cause,
	}
}

func normaliseRuntimeSnapshot(snapshot RuntimeSnapshot) RuntimeSnapshot {
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	if snapshot.Favourites == nil {
		snapshot.Favourites = []Favourite{}
	}
	if snapshot.Recent == nil {
		snapshot.Recent = []Recent{}
	}
	if snapshot.Trash == nil {
		snapshot.Trash = []TrashReceipt{}
	}
	return snapshot
}

// LocationRow is the JSON wire type for a saved location in the left
// rail. Matches the LocationRow interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	loc := files.LocationRow{Name: "Code", Count: 124, Size: "4.2 GB"}
type LocationRow struct {
	// Name is the display name shown to the user ("Code", "Documents").
	Name string `json:"name"`

	// Count is the number of items under this location.
	Count int `json:"count"`

	// Size is the human-readable rolled-up size ("4.2 GB", "180 MB").
	Size string `json:"size"`

	// Brand is true for Lethean-managed locations (brand-coloured in UI).
	Brand bool `json:"brand,omitempty"`
}

// RecentRow is the JSON wire type for a recently-modified file.
// Matches the RecentRow interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	row := files.RecentRow{
//	    Name: "sow-v2.md", Path: "~/Documents/sales/",
//	    When: "14:42", Size: "38 KB",
//	}
type RecentRow struct {
	// Name is the file basename.
	Name string `json:"name"`

	// Path is the parent directory with $HOME collapsed to "~".
	Path string `json:"path"`

	// When is the human-readable last-edit time:
	// "HH:MM" (today), "yest" (yesterday), "X d" (older).
	When string `json:"when"`

	// Size is the human-readable file size.
	Size string `json:"size"`
}

// DiskMeter is the JSON wire type for the disk-free summary.
// Matches the DiskMeter interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	meter := files.DiskMeter{Free: "312 GB", Total: "1 TB", Used: 68}
type DiskMeter struct {
	// Free is the human-readable free-space figure ("312 GB").
	Free string `json:"free"`

	// Total is the human-readable total-disk figure ("1 TB").
	Total string `json:"total"`

	// Used is the fill percentage 0–100 for the UI bar.
	Used int `json:"used"`
}

// ListLocationsOutput is the ListLocations response envelope.
//
// Usage example:
//
//	out := r.Value.(files.ListLocationsOutput)
//	for _, loc := range out.Locations { _ = loc.Name }
type ListLocationsOutput struct {
	// Locations is the ordered list of canonical workspace locations.
	Locations []LocationRow `json:"locations"`
}

// ListRecentInput drives the ListRecent method.
//
// Usage example:
//
//	r := svc.ListRecent(files.ListRecentInput{LocationName: "Code", Limit: 10})
type ListRecentInput struct {
	// LocationName filters to one named location. Empty string = all.
	LocationName string `json:"locationName,omitempty"`

	// Limit caps the result count. Zero defaults to 20.
	Limit int `json:"limit,omitempty"`
}

// ListRecentOutput is the ListRecent response envelope.
//
// Usage example:
//
//	out := r.Value.(files.ListRecentOutput)
//	for _, f := range out.Recent { _ = f.Name }
type ListRecentOutput struct {
	// Recent is the sorted (newest-first) list of recently-modified files.
	Recent []RecentRow `json:"recent"`

	// Total is the unfiltered count before the limit is applied.
	Total int `json:"total"`
}

// DiskUsageOutput is the GetDiskUsage response envelope.
//
// Usage example:
//
//	out := r.Value.(files.DiskUsageOutput)
//	_ = out.Disk.Free
type DiskUsageOutput struct {
	// Disk carries the free / total / used-percent figures.
	Disk DiskMeter `json:"disk"`
}

// locationSpec is the internal description of a canonical workspace
// location — used by scanLocation and collectRecent.
type locationSpec struct {
	// Name is the display name shown in the UI.
	Name string
	// Path is the absolute path to scan.
	Path string
	// Brand is true for Lethean-managed locations.
	Brand bool
}
