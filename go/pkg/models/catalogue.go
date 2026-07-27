// SPDX-Licence-Identifier: EUPL-1.2

// Package models contains renderer-safe discovery for local model artefacts.
// The Catalogue path is deliberately separate from the legacy convenience
// functions in models.go: every operation here flows through the supplied
// io.Medium and returns opaque model IDs.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"unicode/utf8"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const (
	maxCatalogueEntries = 512
	maxModelNameBytes   = 255
)

// CatalogueErrorCode is a stable trusted-Go classification for catalogue
// failures. The model-runtime bridge maps these onto its renderer-safe errors.
type CatalogueErrorCode string

const (
	// CatalogueUnavailable means the Medium or one of its trusted roots is
	// unavailable.
	CatalogueUnavailable CatalogueErrorCode = "catalogue_unavailable"
	// CatalogueInvalid means the provider returned data outside the bounded
	// catalogue contract.
	CatalogueInvalid CatalogueErrorCode = "catalogue_invalid"
	// CatalogueModelNotFound means an opaque ID is absent.
	CatalogueModelNotFound CatalogueErrorCode = "model_not_found"
	// CatalogueModelNotLoadable means a known model lacks the verified local
	// execution contract.
	CatalogueModelNotLoadable CatalogueErrorCode = "model_not_loadable"
)

// CatalogueFailure retains a stable code without publishing a provider path.
type CatalogueFailure struct {
	Code    CatalogueErrorCode
	Message string
	Cause   error
}

func (failure *CatalogueFailure) Error() string {
	if failure == nil {
		return ""
	}
	return failure.Message
}

func (failure *CatalogueFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// CatalogueErrorCodeOf extracts the stable code from a failed result.
func CatalogueErrorCodeOf(result core.Result) CatalogueErrorCode {
	if result.OK {
		return ""
	}
	var failure *CatalogueFailure
	if core.As(result.Err(), &failure) && failure != nil {
		return failure.Code
	}
	return ""
}

// CatalogueEntry is the path-free model metadata safe to expose through the
// model-runtime Wails facade.
type CatalogueEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Format      string `json:"format"`
	SizeBytes   int64  `json:"sizeBytes"`
	Loadable    bool   `json:"loadable"`
}

// Reference is trusted Go-only resolution data. It must never be returned by a
// Wails method, event, audit record, or public error.
type Reference struct {
	ID           string
	DisplayName  string
	RelativePath string
	NativePath   string
	Format       string
}

type catalogueRecord struct {
	entry        CatalogueEntry
	relativePath string
}

// Catalogue discovers verified model directories through one registered
// Medium while retaining the native execution root only in trusted Go.
type Catalogue struct {
	medium       coreio.Medium
	relativeRoot string
	nativeRoot   string
}

// NewCatalogue constructs a Medium-backed local model catalogue.
//
// Usage example:
//
//	catalogue := models.NewCatalogue(medium, "lem/models", nativeRoot)
//	result := catalogue.List()
func NewCatalogue(
	medium coreio.Medium,
	relativeRoot string,
	nativeRoot string,
) *Catalogue {
	return &Catalogue{
		medium:       medium,
		relativeRoot: normaliseProviderRoot(relativeRoot),
		nativeRoot:   strings.TrimSpace(nativeRoot),
	}
}

// List returns bounded path-free model entries.
func (catalogue *Catalogue) List() core.Result {
	records, result := catalogue.scan()
	if !result.OK {
		return result
	}
	entries := make([]CatalogueEntry, len(records))
	for index, record := range records {
		entries[index] = record.entry
	}
	return core.Ok(entries)
}

// Resolve converts one opaque model ID into trusted Go-only execution data.
func (catalogue *Catalogue) Resolve(id string) core.Result {
	if !validModelID(id) {
		return catalogueFailure(
			CatalogueModelNotFound,
			"The selected model is unavailable.",
			nil,
		)
	}
	records, result := catalogue.scan()
	if !result.OK {
		return result
	}
	for _, record := range records {
		if record.entry.ID != id {
			continue
		}
		if !record.entry.Loadable {
			return catalogueFailure(
				CatalogueModelNotLoadable,
				"The selected model is not available to the local runtime.",
				nil,
			)
		}
		return core.Ok(Reference{
			ID:           record.entry.ID,
			DisplayName:  record.entry.DisplayName,
			RelativePath: record.relativePath,
			NativePath:   core.PathJoin(catalogue.nativeRoot, record.entry.DisplayName),
			Format:       record.entry.Format,
		})
	}
	return catalogueFailure(
		CatalogueModelNotFound,
		"The selected model is unavailable.",
		nil,
	)
}

func (catalogue *Catalogue) scan() ([]catalogueRecord, core.Result) {
	if catalogue == nil ||
		catalogue.medium == nil ||
		catalogue.relativeRoot == "" ||
		catalogue.nativeRoot == "" {
		return nil, catalogueFailure(
			CatalogueUnavailable,
			"The local model catalogue is unavailable.",
			nil,
		)
	}
	entries, err := catalogue.medium.List(catalogue.relativeRoot)
	if err != nil {
		return nil, catalogueFailure(
			CatalogueUnavailable,
			"The local model catalogue is unavailable.",
			err,
		)
	}
	if len(entries) > maxCatalogueEntries {
		return nil, catalogueFailure(
			CatalogueInvalid,
			"The local model catalogue contains too many entries.",
			nil,
		)
	}
	records := make([]catalogueRecord, 0, len(entries))
	for _, directoryEntry := range entries {
		if directoryEntry == nil || !directoryEntry.IsDir() {
			continue
		}
		name := directoryEntry.Name()
		if !validModelName(name) {
			return nil, catalogueFailure(
				CatalogueInvalid,
				"The local model catalogue contains an invalid entry.",
				nil,
			)
		}
		relativePath := core.PathJoin(catalogue.relativeRoot, name)
		size := int64(0)
		if info, infoErr := catalogue.medium.Stat(relativePath); infoErr == nil && info != nil {
			size = info.Size()
		}
		records = append(records, catalogueRecord{
			entry: CatalogueEntry{
				ID:          opaqueModelID(name),
				DisplayName: name,
				Format:      "snapshot",
				SizeBytes:   size,
				Loadable: catalogue.medium.IsFile(
					core.PathJoin(relativePath, ".sha256"),
				),
			},
			relativePath: relativePath,
		})
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].entry.DisplayName < records[right].entry.DisplayName
	})
	return records, core.Ok(nil)
}

func normaliseProviderRoot(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" || strings.Contains(value, `\`) {
		return ""
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ""
		}
	}
	return value
}

func validModelName(name string) bool {
	if name == "" ||
		name == "." ||
		name == ".." ||
		strings.HasPrefix(name, ".") ||
		strings.ContainsAny(name, `/\`) ||
		len(name) > maxModelNameBytes ||
		!utf8.ValidString(name) {
		return false
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func opaqueModelID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "model-" + hex.EncodeToString(sum[:8])
}

func validModelID(id string) bool {
	if len(id) != len("model-")+16 || !strings.HasPrefix(id, "model-") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "model-"))
	return err == nil
}

func catalogueFailure(
	code CatalogueErrorCode,
	message string,
	cause error,
) core.Result {
	return core.Fail(&CatalogueFailure{
		Code:    code,
		Message: message,
		Cause:   cause,
	})
}
