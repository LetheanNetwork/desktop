// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	"io/fs"
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type documentEnvelope[T any] struct {
	Version   int    `json:"version"`
	Revision  uint64 `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
	Payload   T      `json:"payload"`
}

type documentOptions[T any] struct {
	Version      int
	MaximumBytes int
	Validate     func(T) core.Result
}

type document[T any] struct {
	medium  coreio.Medium
	path    string
	options documentOptions[T]
	mu      sync.Mutex
}

func newDocument[T any](
	medium coreio.Medium,
	path string,
	options documentOptions[T],
) *document[T] {
	return &document[T]{
		medium:  medium,
		path:    path,
		options: options,
	}
}

func (document *document[T]) Load() core.Result {
	document.mu.Lock()
	defer document.mu.Unlock()
	return document.loadUnlocked()
}

func (document *document[T]) Save(
	expectedRevision uint64,
	payload T,
) core.Result {
	document.mu.Lock()
	defer document.mu.Unlock()

	if invalid := document.validateConfiguration("Save"); !invalid.OK {
		return invalid
	}
	if validated := document.validatePayload(payload); !validated.OK {
		return validated
	}

	currentResult := document.loadUnlocked()
	if !currentResult.OK {
		return currentResult
	}
	current := currentResult.Value.(documentEnvelope[T])
	if current.Revision != expectedRevision {
		return stateFailure(
			ErrorStateConflict,
			"desktopstate.document.Save",
			"desktop state changed since it was loaded",
			nil,
		)
	}

	next := documentEnvelope[T]{
		Version:   document.options.Version,
		Revision:  current.Revision + 1,
		UpdatedAt: core.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	}
	encodedResult := core.JSONMarshalIndent(next, "", "  ")
	if !encodedResult.OK {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Save",
			"desktop state could not be encoded",
			encodedResult.Err(),
		)
	}
	encoded := encodedResult.Value.([]byte)
	if len(encoded) > document.options.MaximumBytes {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Save",
			"desktop state exceeds its size limit",
			nil,
		)
	}

	if err := document.medium.EnsureDir(document.parentDir()); err != nil {
		return document.providerFailure("Save", err)
	}
	if err := document.medium.EnsureDir(document.stagingDir()); err != nil {
		return document.providerFailure("Save", err)
	}
	stagedPath := document.stagedPath()
	backupPath := document.backupPath()
	if document.medium.IsFile(stagedPath) {
		if err := document.medium.Delete(stagedPath); err != nil {
			return document.providerFailure("Save", err)
		}
	}
	if err := document.medium.WriteMode(stagedPath, string(encoded), 0o600); err != nil {
		return document.providerFailure("Save", err)
	}
	stagedContent, err := document.medium.Read(stagedPath)
	if err != nil {
		_ = document.medium.Delete(stagedPath)
		return document.providerFailure("Save", err)
	}
	if verified := document.decode(stagedContent); !verified.OK {
		_ = document.medium.Delete(stagedPath)
		return verified
	}

	hadDocument := document.medium.IsFile(document.path)
	if hadDocument && document.medium.IsFile(backupPath) {
		if err := document.medium.Delete(backupPath); err != nil {
			_ = document.medium.Delete(stagedPath)
			return document.providerFailure("Save", err)
		}
	}
	if hadDocument {
		if err := document.medium.Rename(document.path, backupPath); err != nil {
			_ = document.medium.Delete(stagedPath)
			return document.providerFailure("Save", err)
		}
	}
	if err := document.medium.Rename(stagedPath, document.path); err != nil {
		var recoveryErr error
		if hadDocument {
			recoveryErr = document.medium.Rename(backupPath, document.path)
		}
		_ = document.medium.Delete(stagedPath)
		if recoveryErr != nil {
			return stateFailure(
				ErrorStateUnavailable,
				"desktopstate.document.Save",
				"desktop state commit and recovery failed",
				recoveryErr,
			)
		}
		return document.providerFailure("Save", err)
	}
	if hadDocument {
		if err := document.medium.Delete(backupPath); err != nil {
			return document.providerFailure("Save", err)
		}
	}
	return core.Ok(next)
}

func (document *document[T]) loadUnlocked() core.Result {
	if invalid := document.validateConfiguration("Load"); !invalid.OK {
		return invalid
	}
	content, err := document.medium.Read(document.path)
	if err == nil {
		return document.decode(content)
	}
	if !core.Is(err, fs.ErrNotExist) {
		return document.providerFailure("Load", err)
	}

	backupPath := document.backupPath()
	backup, backupErr := document.medium.Read(backupPath)
	if backupErr != nil {
		if core.Is(backupErr, fs.ErrNotExist) {
			var zero T
			return core.Ok(documentEnvelope[T]{
				Version: document.options.Version,
				Payload: zero,
			})
		}
		return document.providerFailure("Load", backupErr)
	}
	decoded := document.decode(backup)
	if !decoded.OK {
		return decoded
	}
	if err := document.medium.EnsureDir(document.parentDir()); err != nil {
		return document.providerFailure("Recover", err)
	}
	if err := document.medium.Rename(backupPath, document.path); err != nil {
		return document.providerFailure("Recover", err)
	}
	return decoded
}

func (document *document[T]) decode(content string) core.Result {
	if len(content) > document.options.MaximumBytes {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Load",
			"desktop state exceeds its size limit",
			nil,
		)
	}
	var envelope documentEnvelope[T]
	decoded := core.JSONUnmarshalString(content, &envelope)
	if !decoded.OK {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Load",
			"desktop state is malformed",
			decoded.Err(),
		)
	}
	if envelope.Version != document.options.Version || envelope.Revision == 0 {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Load",
			"desktop state version or revision is unsupported",
			nil,
		)
	}
	if parsed := core.TimeParse(time.RFC3339Nano, envelope.UpdatedAt); !parsed.OK {
		return stateFailure(
			ErrorStateInvalid,
			"desktopstate.document.Load",
			"desktop state update time is invalid",
			parsed.Err(),
		)
	}
	if validated := document.validatePayload(envelope.Payload); !validated.OK {
		return validated
	}
	return core.Ok(envelope)
}

func (document *document[T]) validateConfiguration(operation string) core.Result {
	if document == nil || document.medium == nil {
		return stateFailure(
			ErrorStateUnavailable,
			core.Concat("desktopstate.document.", operation),
			"desktop state Medium is unavailable",
			nil,
		)
	}
	if document.options.Version <= 0 ||
		document.options.MaximumBytes <= 0 ||
		document.options.Validate == nil {
		return stateFailure(
			ErrorStateInvalid,
			core.Concat("desktopstate.document.", operation),
			"desktop state document options are invalid",
			nil,
		)
	}
	if !validProviderPath(document.path) {
		return stateFailure(
			ErrorStateInvalid,
			core.Concat("desktopstate.document.", operation),
			"desktop state path is invalid",
			nil,
		)
	}
	return core.Ok(nil)
}

func (document *document[T]) validatePayload(payload T) core.Result {
	result := document.options.Validate(payload)
	if result.OK {
		return result
	}
	return stateFailure(
		ErrorStateInvalid,
		"desktopstate.document.Validate",
		"desktop state payload is invalid",
		result.Err(),
	)
}

func (document *document[T]) providerFailure(
	operation string,
	cause error,
) core.Result {
	return stateFailure(
		ErrorStateUnavailable,
		core.Concat("desktopstate.document.", operation),
		"desktop state provider is unavailable",
		cause,
	)
}

func (document *document[T]) parentDir() string {
	parent := core.PathDir(document.path)
	if parent == "." {
		return ""
	}
	return parent
}

func (document *document[T]) stagingDir() string {
	parent := document.parentDir()
	if parent == "" {
		return ".staging"
	}
	return core.JoinPath(parent, ".staging")
}

func (document *document[T]) stagedPath() string {
	return core.JoinPath(
		document.stagingDir(),
		core.Concat(core.PathBase(document.path), ".new"),
	)
}

func (document *document[T]) backupPath() string {
	return core.JoinPath(
		document.stagingDir(),
		core.Concat(core.PathBase(document.path), ".backup"),
	)
}

func validProviderPath(value string) bool {
	if value == "" || core.PathIsAbs(value) || core.Contains(value, `\`) {
		return false
	}
	clean := core.CleanPath(value, "/")
	return clean == value &&
		clean != "." &&
		clean != ".." &&
		!core.HasPrefix(clean, "../")
}
