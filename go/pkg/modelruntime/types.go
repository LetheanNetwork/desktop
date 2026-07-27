// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"time"

	core "dappco.re/go"
)

const (
	inferenceServiceID = "inference"
	maxHistorySamples  = 720
)

// State is the renderer-safe model-runtime lifecycle state.
type State string

const (
	StateUnavailable State = "unavailable"
	StateStopped     State = "stopped"
	StateStarting    State = "starting"
	StateModelLess   State = "model-less"
	StateLoading     State = "loading"
	StateReady       State = "ready"
	StateDegraded    State = "degraded"
	StateFailed      State = "failed"
	StateStopping    State = "stopping"
)

// ErrorCode is the closed renderer-safe failure set.
type ErrorCode string

const (
	ErrorRuntimeUnavailable   ErrorCode = "runtime_unavailable"
	ErrorBinaryMissing        ErrorCode = "binary_missing"
	ErrorRuntimeStopped       ErrorCode = "runtime_stopped"
	ErrorRuntimeStartFailed   ErrorCode = "runtime_start_failed"
	ErrorRuntimeNotReady      ErrorCode = "runtime_not_ready"
	ErrorCatalogueUnavailable ErrorCode = "catalogue_unavailable"
	ErrorModelNotFound        ErrorCode = "model_not_found"
	ErrorModelNotLoadable     ErrorCode = "model_not_loadable"
	ErrorModelLoadFailed      ErrorCode = "model_load_failed"
	ErrorModelUnloadFailed    ErrorCode = "model_unload_failed"
	ErrorAdminUnauthorised    ErrorCode = "admin_unauthorised"
	ErrorOperationInProgress  ErrorCode = "operation_in_progress"
	ErrorResponseInvalid      ErrorCode = "response_invalid"
	ErrorResponseTooLarge     ErrorCode = "response_too_large"
	ErrorRequestTimeout       ErrorCode = "request_timeout"
	ErrorRuntimeStopFailed    ErrorCode = "runtime_stop_failed"
)

// Failure retains an internal cause while exposing only stable bounded text.
type Failure struct {
	Code      ErrorCode
	Operation string
	Message   string
	Cause     error
}

func (failure *Failure) Error() string {
	if failure == nil {
		return ""
	}
	if failure.Operation == "" {
		return failure.Message
	}
	return core.Concat(failure.Operation, ": ", failure.Message)
}

func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// ErrorCodeOf extracts a stable model-runtime code.
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

// FailureView is safe to cross Wails.
type FailureView struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// ModelView is path-free model catalogue and load-state metadata.
type ModelView struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	Format        string `json:"format"`
	Loadable      bool   `json:"loadable"`
	Loaded        bool   `json:"loaded"`
	Runtime       string `json:"runtime,omitempty"`
	ContextLength int    `json:"contextLength,omitempty"`
	LoadedAt      string `json:"loadedAt,omitempty"`
}

// MetricsView uses pointers so unsupported values remain absent rather than
// becoming misleading zeroes.
type MetricsView struct {
	PromptTokensPerSecond *float64 `json:"promptTokensPerSecond,omitempty"`
	DecodeTokensPerSecond *float64 `json:"decodeTokensPerSecond,omitempty"`
	ActiveMemoryBytes     *uint64  `json:"activeMemoryBytes,omitempty"`
	PeakMemoryBytes       *uint64  `json:"peakMemoryBytes,omitempty"`
	KVCacheBytes          *uint64  `json:"kvCacheBytes,omitempty"`
	UptimeSeconds         *int64   `json:"uptimeSeconds,omitempty"`
}

// SampleView is one bounded in-memory runtime observation.
type SampleView struct {
	State                 State    `json:"state"`
	At                    string   `json:"at"`
	PromptTokensPerSecond *float64 `json:"promptTokensPerSecond,omitempty"`
	DecodeTokensPerSecond *float64 `json:"decodeTokensPerSecond,omitempty"`
	ActiveMemoryBytes     *uint64  `json:"activeMemoryBytes,omitempty"`
	PeakMemoryBytes       *uint64  `json:"peakMemoryBytes,omitempty"`
	KVCacheBytes          *uint64  `json:"kvCacheBytes,omitempty"`
}

// Snapshot is the complete immutable renderer-safe runtime view.
type Snapshot struct {
	State         State        `json:"state"`
	Desired       bool         `json:"desired"`
	ActiveModelID string       `json:"activeModelId"`
	Models        []ModelView  `json:"models"`
	Metrics       MetricsView  `json:"metrics"`
	History       []SampleView `json:"history"`
	RefreshedAt   string       `json:"refreshedAt"`
	LastHealthyAt string       `json:"lastHealthyAt"`
	Stale         bool         `json:"stale"`
	LastError     *FailureView `json:"lastError"`
}

// LoadRequest contains only the opaque model ID selected by the renderer.
type LoadRequest struct {
	ModelID string `json:"modelId"`
}

// Event is invalidation metadata only.
type Event struct {
	Reason string `json:"reason"`
	State  State  `json:"state"`
	At     string `json:"at"`
}

func fail(
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

func failureView(result core.Result, fallback ErrorCode, message string) *FailureView {
	code := ErrorCodeOf(result)
	if code == "" {
		code = fallback
	}
	return &FailureView{Code: code, Message: message}
}

func timestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
