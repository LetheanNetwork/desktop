// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import core "dappco.re/go"

// ErrorCode is a stable renderer-safe desktop-state failure code.
type ErrorCode string

const (
	// ErrorStateUnavailable means a required Medium operation failed.
	ErrorStateUnavailable ErrorCode = "desktop_state_unavailable"
	// ErrorStateInvalid means durable state or a proposed payload is invalid.
	ErrorStateInvalid ErrorCode = "desktop_state_invalid"
	// ErrorStateConflict means the caller attempted to overwrite a newer revision.
	ErrorStateConflict ErrorCode = "desktop_state_conflict"
)

// Failure carries a stable code while retaining a trusted internal cause.
type Failure struct {
	Code      ErrorCode
	Operation string
	Message   string
	Cause     error
}

// Error implements error without exposing provider details.
func (failure *Failure) Error() string {
	if failure == nil {
		return ""
	}
	if failure.Operation == "" {
		return failure.Message
	}
	return core.Concat(failure.Operation, ": ", failure.Message)
}

// Unwrap exposes the provider cause to trusted Go callers.
func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func stateFailure(
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

// ErrorCodeOf returns the stable code carried by a failed Result.
func ErrorCodeOf(result core.Result) ErrorCode {
	if result.OK {
		return ""
	}
	var failure *Failure
	if core.As(result.Err(), &failure) && failure != nil {
		return failure.Code
	}
	return ""
}
