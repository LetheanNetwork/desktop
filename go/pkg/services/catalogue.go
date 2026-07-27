// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// CatalogueVersion is the only durable managed-service document version.
const CatalogueVersion = 1

// CatalogueDocument contains durable trusted definitions and policy only.
// Runtime desired state, process identity, output, and errors never belong
// here.
type CatalogueDocument struct {
	Version         int              `json:"version"`
	Definitions     []Definition     `json:"definitions"`
	PolicyOverrides []PolicyOverride `json:"policyOverrides"`
	UpdatedAt       string           `json:"updatedAt"`
}

// Catalogue is the durable definition boundary used by Service.
type Catalogue interface {
	Load() core.Result
	Save(CatalogueDocument) core.Result
}

type mediumCatalogue struct {
	medium     coreio.Medium
	path       string
	stagingDir string
	stagedPath string
	backupPath string
	limits     Limits
	mu         sync.Mutex
}

// NewMediumCatalogue creates a versioned catalogue whose complete storage
// lifecycle is transported by medium.
func NewMediumCatalogue(
	medium coreio.Medium,
	relativePath string,
	limits Limits,
) Catalogue {
	parent := path.Dir(relativePath)
	stagingDirectory := path.Join(parent, ".staging")
	return &mediumCatalogue{
		medium:     medium,
		path:       relativePath,
		stagingDir: stagingDirectory,
		stagedPath: path.Join(stagingDirectory, "catalogue.new.json"),
		backupPath: path.Join(stagingDirectory, "catalogue.backup.json"),
		limits:     limits,
	}
}

func (catalogue *mediumCatalogue) Load() core.Result {
	if catalogue == nil || catalogue.medium == nil {
		return catalogueFailure(
			ErrorServicesUnavailable,
			"services.Catalogue.Load",
			"Services catalogue storage is unavailable.",
			nil,
		)
	}
	catalogue.mu.Lock()
	defer catalogue.mu.Unlock()

	if result := catalogue.validatePath(); !result.OK {
		return result
	}
	content, err := catalogue.medium.Read(catalogue.path)
	if err == nil {
		return decodeCatalogueDocument(content, catalogue.limits)
	}
	if !core.Is(err, fs.ErrNotExist) {
		return catalogueFailure(
			ErrorServicesUnavailable,
			"services.Catalogue.Load",
			"Services catalogue storage could not be read.",
			err,
		)
	}
	return catalogue.recoverOrCreate()
}

func (catalogue *mediumCatalogue) Save(document CatalogueDocument) core.Result {
	if catalogue == nil || catalogue.medium == nil {
		return catalogueFailure(
			ErrorServicesUnavailable,
			"services.Catalogue.Save",
			"Services catalogue storage is unavailable.",
			nil,
		)
	}
	catalogue.mu.Lock()
	defer catalogue.mu.Unlock()

	if result := catalogue.validatePath(); !result.OK {
		return result
	}
	document = cloneCatalogueDocument(document)
	if result := validateCatalogueDocument(document, catalogue.limits); !result.OK {
		return result
	}
	encoded := core.JSONMarshal(document)
	if !encoded.OK {
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue.Save",
			"Services catalogue could not be serialised.",
			encoded.Err(),
		)
	}
	payload, ok := encoded.Value.([]byte)
	if !ok {
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue.Save",
			"Services catalogue serialisation was invalid.",
			nil,
		)
	}
	if err := catalogue.ensureDirectories(); err != nil {
		return catalogue.providerFailure("services.Catalogue.Save", err)
	}

	hadTarget, result := catalogue.targetExists()
	if !result.OK {
		return result
	}
	if !hadTarget {
		hasBackup, backupResult := catalogue.entryExists(catalogue.backupPath)
		if !backupResult.OK {
			return backupResult
		}
		if hasBackup {
			return catalogueFailure(
				ErrorServicesUnavailable,
				"services.Catalogue.Save",
				"Services catalogue recovery is required before saving.",
				nil,
			)
		}
	}
	if result := catalogue.deleteIfExists(catalogue.stagedPath); !result.OK {
		return result
	}
	if hadTarget {
		if result := catalogue.deleteIfExists(catalogue.backupPath); !result.OK {
			return result
		}
	}
	if err := catalogue.medium.WriteMode(catalogue.stagedPath, string(payload), 0o600); err != nil {
		return catalogue.providerFailure("services.Catalogue.Save", err)
	}
	staged, err := catalogue.medium.Read(catalogue.stagedPath)
	if err != nil {
		_ = catalogue.medium.Delete(catalogue.stagedPath)
		return catalogue.providerFailure("services.Catalogue.Save", err)
	}
	if staged != string(payload) {
		_ = catalogue.medium.Delete(catalogue.stagedPath)
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue.Save",
			"Services catalogue staging verification failed.",
			nil,
		)
	}
	if result := decodeCatalogueDocument(staged, catalogue.limits); !result.OK {
		_ = catalogue.medium.Delete(catalogue.stagedPath)
		return result
	}

	if hadTarget {
		if err := catalogue.medium.Rename(catalogue.path, catalogue.backupPath); err != nil {
			_ = catalogue.medium.Delete(catalogue.stagedPath)
			return catalogue.providerFailure("services.Catalogue.Save", err)
		}
	}
	if err := catalogue.medium.Rename(catalogue.stagedPath, catalogue.path); err != nil {
		restoreErr := error(nil)
		if hadTarget {
			restoreErr = catalogue.medium.Rename(catalogue.backupPath, catalogue.path)
		}
		_ = catalogue.medium.Delete(catalogue.stagedPath)
		if restoreErr != nil {
			return catalogueFailure(
				ErrorServicesUnavailable,
				"services.Catalogue.Save",
				"Services catalogue commit and recovery both failed.",
				restoreErr,
			)
		}
		return catalogue.providerFailure("services.Catalogue.Save", err)
	}
	if hadTarget {
		if err := catalogue.medium.Delete(catalogue.backupPath); err != nil {
			return catalogue.providerFailure("services.Catalogue.Save", err)
		}
	}
	return core.Ok(nil)
}

func (catalogue *mediumCatalogue) recoverOrCreate() core.Result {
	content, err := catalogue.medium.Read(catalogue.backupPath)
	if err != nil {
		if core.Is(err, fs.ErrNotExist) {
			return core.Ok(CatalogueDocument{
				Version:         CatalogueVersion,
				Definitions:     []Definition{},
				PolicyOverrides: []PolicyOverride{},
			})
		}
		return catalogue.providerFailure("services.Catalogue.Load", err)
	}
	decoded := decodeCatalogueDocument(content, catalogue.limits)
	if !decoded.OK {
		return decoded
	}
	if err := catalogue.ensureDirectories(); err != nil {
		return catalogue.providerFailure("services.Catalogue.Load", err)
	}
	if err := catalogue.medium.Rename(catalogue.backupPath, catalogue.path); err != nil {
		return catalogue.providerFailure("services.Catalogue.Load", err)
	}
	return decoded
}

func (catalogue *mediumCatalogue) ensureDirectories() error {
	parent := path.Dir(catalogue.path)
	if parent != "." && parent != "" {
		if err := catalogue.medium.EnsureDir(parent); err != nil {
			return err
		}
	}
	return catalogue.medium.EnsureDir(catalogue.stagingDir)
}

func (catalogue *mediumCatalogue) validatePath() core.Result {
	if !validProviderRelativePath(catalogue.path) {
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue",
			"Services catalogue path is invalid.",
			nil,
		)
	}
	return core.Ok(nil)
}

func (catalogue *mediumCatalogue) targetExists() (bool, core.Result) {
	return catalogue.entryExists(catalogue.path)
}

func (catalogue *mediumCatalogue) entryExists(entryPath string) (bool, core.Result) {
	info, err := catalogue.medium.Stat(entryPath)
	if err == nil {
		if info.IsDir() {
			return false, catalogueFailure(
				ErrorCatalogueInvalid,
				"services.Catalogue",
				"Services catalogue path is a directory.",
				nil,
			)
		}
		return true, core.Ok(nil)
	}
	if core.Is(err, fs.ErrNotExist) {
		return false, core.Ok(nil)
	}
	return false, catalogue.providerFailure("services.Catalogue", err)
}

func (catalogue *mediumCatalogue) deleteIfExists(entryPath string) core.Result {
	exists, result := catalogue.entryExists(entryPath)
	if !result.OK || !exists {
		return result
	}
	if err := catalogue.medium.Delete(entryPath); err != nil {
		return catalogue.providerFailure("services.Catalogue.Save", err)
	}
	return core.Ok(nil)
}

func (catalogue *mediumCatalogue) providerFailure(operation string, cause error) core.Result {
	return catalogueFailure(
		ErrorServicesUnavailable,
		operation,
		"Services catalogue storage operation failed.",
		cause,
	)
}

func decodeCatalogueDocument(content string, limits Limits) core.Result {
	if !core.JSONValid(core.AsBytes(content)) {
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue.Load",
			"Services catalogue is malformed.",
			nil,
		)
	}
	var document CatalogueDocument
	decoder := core.JSONNewDecoder(core.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return catalogueFailure(
			ErrorCatalogueInvalid,
			"services.Catalogue.Load",
			"Services catalogue has an unknown or invalid field.",
			err,
		)
	}
	if result := validateCatalogueDocument(document, limits); !result.OK {
		return result
	}
	return core.Ok(cloneCatalogueDocument(document))
}

func validateCatalogueDocument(document CatalogueDocument, limits Limits) core.Result {
	if document.Version != CatalogueVersion {
		return invalidCatalogue("Services catalogue version is unsupported.")
	}
	if limits.MaxDefinitions <= 0 || len(document.Definitions) > limits.MaxDefinitions {
		return invalidCatalogue("Services catalogue contains too many definitions.")
	}
	if _, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil {
		return invalidCatalogue("Services catalogue update time is invalid.")
	}
	definitionIDs := make(map[string]bool, len(document.Definitions))
	for _, definition := range document.Definitions {
		if result := ValidateDefinition(definition, limits); !result.OK {
			return catalogueFailure(
				ErrorCatalogueInvalid,
				"services.Catalogue.Validate",
				"Services catalogue contains an invalid definition.",
				result.Err(),
			)
		}
		if definitionIDs[definition.ID] {
			return invalidCatalogue("Services catalogue contains a duplicate definition.")
		}
		definitionIDs[definition.ID] = true
	}
	overrideIDs := make(map[string]bool, len(document.PolicyOverrides))
	for _, override := range document.PolicyOverrides {
		if !definitionIDPattern.MatchString(override.ID) ||
			!validRestartPolicy(override.RestartPolicy) ||
			override.GracePeriodMillis <= 0 ||
			override.GracePeriodMillis > limits.MaxGracePeriodMillis {
			return invalidCatalogue("Services catalogue contains an invalid policy override.")
		}
		if overrideIDs[override.ID] {
			return invalidCatalogue("Services catalogue contains a duplicate policy override.")
		}
		overrideIDs[override.ID] = true
	}
	return core.Ok(nil)
}

func invalidCatalogue(message string) core.Result {
	return catalogueFailure(
		ErrorCatalogueInvalid,
		"services.Catalogue.Validate",
		message,
		nil,
	)
}

func catalogueFailure(
	code ErrorCode,
	operation string,
	message string,
	cause error,
) core.Result {
	return core.Fail(&Failure{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	})
}

func cloneCatalogueDocument(document CatalogueDocument) CatalogueDocument {
	clone := document
	clone.Definitions = make([]Definition, len(document.Definitions))
	for index, definition := range document.Definitions {
		clone.Definitions[index] = cloneDefinition(definition)
	}
	clone.PolicyOverrides = append([]PolicyOverride(nil), document.PolicyOverrides...)
	return clone
}

func validProviderRelativePath(value string) bool {
	if value == "" ||
		strings.HasPrefix(value, "/") ||
		path.IsAbs(value) ||
		!utf8SafePath(value) ||
		path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(strings.ReplaceAll(value, `\`, "/"), "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func utf8SafePath(value string) bool {
	return len(value) <= 4_096 && !containsControl(value)
}
