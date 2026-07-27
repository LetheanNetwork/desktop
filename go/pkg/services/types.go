// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	core "dappco.re/go"
)

// Kind identifies the role of a managed background capability.
type Kind string

// RestartPolicy controls whether an explicitly started capability is
// restarted after it exits.
type RestartPolicy string

// State is the current observed lifecycle state of a managed capability.
type State string

// ErrorCode is a stable renderer-safe managed-service failure code.
type ErrorCode string

const (
	// KindService identifies a long-running background service.
	KindService Kind = "service"
	// KindApp identifies an optional background application.
	KindApp Kind = "app"
	// KindProcess identifies a managed utility process.
	KindProcess Kind = "process"

	// RestartNever leaves an exited process stopped.
	RestartNever RestartPolicy = "never"
	// RestartOnFailure restarts a desired process only after a failed exit.
	RestartOnFailure RestartPolicy = "on-failure"
	// RestartAlways restarts a desired process after any unrequested exit.
	RestartAlways RestartPolicy = "always"

	// StateStopped means no managed process is running.
	StateStopped State = "stopped"
	// StateStarting means an explicit start is in progress.
	StateStarting State = "starting"
	// StateRunning means the managed process is running.
	StateRunning State = "running"
	// StateStopping means a graceful shutdown is in progress.
	StateStopping State = "stopping"
	// StateExited means the process exited successfully without a restart.
	StateExited State = "exited"
	// StateFailed means start, stop, restart, or process execution failed.
	StateFailed State = "failed"

	// ErrorServicesUnavailable means a required manager dependency is absent.
	ErrorServicesUnavailable ErrorCode = "services_unavailable"
	// ErrorCatalogueInvalid means durable catalogue evidence is malformed.
	ErrorCatalogueInvalid ErrorCode = "catalogue_invalid"
	// ErrorDefinitionNotFound means the requested trusted ID is unknown.
	ErrorDefinitionNotFound ErrorCode = "definition_not_found"
	// ErrorDefinitionInvalid means a definition violates the trusted schema.
	ErrorDefinitionInvalid ErrorCode = "definition_invalid"
	// ErrorDefinitionConflict means ownership or replacement rules conflict.
	ErrorDefinitionConflict ErrorCode = "definition_conflict"
	// ErrorOperationInProgress means another lifecycle mutation owns the ID.
	ErrorOperationInProgress ErrorCode = "operation_in_progress"
	// ErrorWorkingDirectoryUnsupported means a Medium cannot supply a native
	// working directory for the requested reference.
	ErrorWorkingDirectoryUnsupported ErrorCode = "working_directory_unsupported"
	// ErrorRunningLimitReached means the configured concurrent limit is full.
	ErrorRunningLimitReached ErrorCode = "running_limit_reached"
	// ErrorProcessStartFailed means go-process could not start the definition.
	ErrorProcessStartFailed ErrorCode = "process_start_failed"
	// ErrorProcessLookupFailed means a recorded go-process identity is absent.
	ErrorProcessLookupFailed ErrorCode = "process_lookup_failed"
	// ErrorProcessStopFailed means go-process could not stop the process tree.
	ErrorProcessStopFailed ErrorCode = "process_stop_failed"
	// ErrorRestartBudgetExhausted means crash-loop protection stopped retries.
	ErrorRestartBudgetExhausted ErrorCode = "restart_budget_exhausted"
	// ErrorShutdownIncomplete means one or more managed processes did not stop.
	ErrorShutdownIncomplete ErrorCode = "shutdown_incomplete"
	// ErrorSignalUnknown means the requested signal name is not one this
	// manager delivers.
	ErrorSignalUnknown ErrorCode = "signal_unknown"
	// ErrorSignalUnsupported means the named signal has no meaning on this
	// platform.
	ErrorSignalUnsupported ErrorCode = "signal_unsupported"
	// ErrorServiceNotRunning means there is no managed process to signal.
	ErrorServiceNotRunning ErrorCode = "service_not_running"
	// ErrorProcessSignalFailed means the operating system refused delivery.
	ErrorProcessSignalFailed ErrorCode = "process_signal_failed"
)

// Signal is a named signal. The name is the whole wire contract: a kernel
// constant never crosses the Wails boundary, for the same reason a command or
// an absolute path never does.
type Signal string

const (
	// SignalTerminate asks a process to stop. Catchable, so a service may
	// flush state and remove its socket before exiting.
	SignalTerminate Signal = "terminate"
	// SignalInterrupt is what a terminal sends on Ctrl-C.
	SignalInterrupt Signal = "interrupt"
	// SignalHangup means reload, by long convention.
	SignalHangup Signal = "hangup"
	// SignalKill cannot be caught. Use it when nothing else has worked.
	SignalKill Signal = "kill"
)

// SignalRequest names one managed service and one signal.
type SignalRequest struct {
	ID     string `json:"id"`
	Signal Signal `json:"signal"`
}

var definitionIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// WorkingDirectory is a provider-relative directory reference. It never
// contains a provider root or renderer-supplied native path.
type WorkingDirectory struct {
	MountID string `json:"mountId,omitempty"`
	Path    string `json:"path,omitempty"`
}

// Definition is trusted Go-owned execution data for one managed capability.
// Renderer lifecycle calls address only ID and never receive Command,
// Arguments, or WorkingDirectory.
type Definition struct {
	ID               string           `json:"id"`
	DisplayName      string           `json:"displayName"`
	Description      string           `json:"description"`
	Kind             Kind             `json:"kind"`
	Command          string           `json:"command"`
	Arguments        []string         `json:"arguments,omitempty"`
	WorkingDirectory WorkingDirectory `json:"workingDirectory,omitempty"`
	RestartPolicy    RestartPolicy    `json:"restartPolicy"`
	// GracePeriodMillis is the bounded graceful process-tree shutdown period.
	GracePeriodMillis int64  `json:"gracePeriodMillis"`
	Owner             string `json:"owner"`
}

// DefinitionView is the renderer-safe catalogue metadata for a definition.
type DefinitionView struct {
	ID                string        `json:"id"`
	DisplayName       string        `json:"displayName"`
	Description       string        `json:"description"`
	Kind              Kind          `json:"kind"`
	RestartPolicy     RestartPolicy `json:"restartPolicy"`
	GracePeriodMillis int64         `json:"gracePeriodMillis"`
	Owner             string        `json:"owner"`
}

// PolicyOverride is the renderer-writable bounded policy for a known ID.
type PolicyOverride struct {
	ID                string        `json:"id"`
	RestartPolicy     RestartPolicy `json:"restartPolicy"`
	GracePeriodMillis int64         `json:"gracePeriodMillis"`
}

// FailureView is a stable, bounded failure safe to send to a renderer.
type FailureView struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Snapshot is an immutable caller-facing observation of one definition.
type Snapshot struct {
	Definition   DefinitionView `json:"definition"`
	State        State          `json:"state"`
	Desired      bool           `json:"desired"`
	ProcessID    string         `json:"processId"`
	PID          int            `json:"pid"`
	StartedAt    string         `json:"startedAt"`
	StoppedAt    string         `json:"stoppedAt"`
	ExitCode     int            `json:"exitCode"`
	RestartCount int            `json:"restartCount"`
	LastError    *FailureView   `json:"lastError"`
}

// CatalogueView is the renderer-safe collection of managed snapshots.
type CatalogueView struct {
	Services    []Snapshot `json:"services"`
	RefreshedAt string     `json:"refreshedAt"`
}

// OutputRequest asks for a bounded tail from one known managed service.
type OutputRequest struct {
	ID    string `json:"id"`
	Limit int    `json:"limit"`
}

// OutputView is a transient bounded output tail. It is never persisted.
type OutputView struct {
	ID         string `json:"id"`
	ProcessID  string `json:"processId"`
	Generation uint64 `json:"generation"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated"`
	ObservedAt string `json:"observedAt"`
}

// Event is bounded lifecycle invalidation metadata published on Core ACTION.
type Event struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Previous  State     `json:"previous"`
	State     State     `json:"state"`
	Desired   bool      `json:"desired"`
	ProcessID string    `json:"processId"`
	ErrorCode ErrorCode `json:"errorCode"`
	At        string    `json:"at"`
}

// Limits bounds catalogue, process, output, shutdown, and restart behaviour.
type Limits struct {
	MaxDefinitions       int
	MaxArguments         int
	MaxArgumentBytes     int
	MaxRunning           int
	MaxOutputBytes       int
	MaxGracePeriodMillis int64
	RestartLimit         int
	RestartWindow        time.Duration
	RestartBaseDelay     time.Duration
	RestartMaxDelay      time.Duration
	RestartExponentCap   int
}

// Failure retains a stable code and internal wrapped cause. Only Code and the
// bounded Message belong in renderer or audit views.
type Failure struct {
	Code      ErrorCode
	Operation string
	Message   string
	Cause     error
}

// Error implements error without exposing an internal cause.
func (failure *Failure) Error() string {
	if failure == nil {
		return ""
	}
	if failure.Operation == "" {
		return failure.Message
	}
	return core.Concat(failure.Operation, ": ", failure.Message)
}

// Unwrap exposes the internal cause to trusted Go callers.
func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// ErrorCodeOf returns the stable managed-service code carried by a failure.
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

// DefaultLimits returns conservative manager bounds suitable for Desktop.
func DefaultLimits() Limits {
	return Limits{
		MaxDefinitions:       256,
		MaxArguments:         128,
		MaxArgumentBytes:     16 * 1024,
		MaxRunning:           32,
		MaxOutputBytes:       256 * 1024,
		MaxGracePeriodMillis: 60_000,
		RestartLimit:         5,
		RestartWindow:        5 * time.Minute,
		RestartBaseDelay:     250 * time.Millisecond,
		RestartMaxDelay:      30 * time.Second,
		RestartExponentCap:   7,
	}
}

// ValidateDefinition validates trusted execution data before it is published
// or persisted.
func ValidateDefinition(definition Definition, limits Limits) core.Result {
	if !definitionIDPattern.MatchString(definition.ID) {
		return invalidDefinition("service ID is invalid")
	}
	if !definitionIDPattern.MatchString(definition.Owner) {
		return invalidDefinition("service owner is invalid")
	}
	if invalidText(definition.DisplayName, 256) {
		return invalidDefinition("display name is invalid")
	}
	if invalidText(definition.Description, 2_048) {
		return invalidDefinition("description is invalid")
	}
	if !validKind(definition.Kind) {
		return invalidDefinition("service kind is invalid")
	}
	if invalidText(definition.Command, 4_096) {
		return invalidDefinition("command is invalid")
	}
	if !validRestartPolicy(definition.RestartPolicy) {
		return invalidDefinition("restart policy is invalid")
	}
	if limits.MaxGracePeriodMillis <= 0 ||
		definition.GracePeriodMillis <= 0 ||
		definition.GracePeriodMillis > limits.MaxGracePeriodMillis {
		return invalidDefinition("grace period is outside the allowed range")
	}
	if limits.MaxArguments < 0 || len(definition.Arguments) > limits.MaxArguments {
		return invalidDefinition("too many command arguments")
	}
	argumentBytes := 0
	for _, argument := range definition.Arguments {
		if containsControl(argument) || !utf8.ValidString(argument) {
			return invalidDefinition("command argument contains invalid bytes")
		}
		argumentBytes += len(argument)
		if limits.MaxArgumentBytes < 0 || argumentBytes > limits.MaxArgumentBytes {
			return invalidDefinition("command arguments exceed the allowed size")
		}
	}
	if !validWorkingDirectory(definition.WorkingDirectory) {
		return invalidDefinition("working directory reference is invalid")
	}
	return core.Ok(nil)
}

func invalidDefinition(message string) core.Result {
	return core.Fail(&Failure{
		Code:      ErrorDefinitionInvalid,
		Operation: "services.ValidateDefinition",
		Message:   message,
	})
}

func validKind(kind Kind) bool {
	switch kind {
	case KindService, KindApp, KindProcess:
		return true
	default:
		return false
	}
}

func validRestartPolicy(policy RestartPolicy) bool {
	switch policy {
	case RestartNever, RestartOnFailure, RestartAlways:
		return true
	default:
		return false
	}
}

func invalidText(value string, limit int) bool {
	return strings.TrimSpace(value) == "" ||
		len(value) > limit ||
		!utf8.ValidString(value) ||
		containsControl(value)
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func validWorkingDirectory(directory WorkingDirectory) bool {
	if directory.MountID == "" {
		return directory.Path == ""
	}
	if !definitionIDPattern.MatchString(directory.MountID) ||
		len(directory.Path) > 4_096 ||
		!utf8.ValidString(directory.Path) ||
		containsControl(directory.Path) ||
		strings.HasPrefix(directory.Path, "/") ||
		path.IsAbs(directory.Path) {
		return false
	}
	for _, segment := range strings.Split(strings.ReplaceAll(directory.Path, `\`, "/"), "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func definitionView(definition Definition) DefinitionView {
	return DefinitionView{
		ID:                definition.ID,
		DisplayName:       definition.DisplayName,
		Description:       definition.Description,
		Kind:              definition.Kind,
		RestartPolicy:     definition.RestartPolicy,
		GracePeriodMillis: definition.GracePeriodMillis,
		Owner:             definition.Owner,
	}
}

func cloneDefinition(definition Definition) Definition {
	clone := definition
	clone.Arguments = append([]string(nil), definition.Arguments...)
	return clone
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	if snapshot.LastError != nil {
		failure := *snapshot.LastError
		clone.LastError = &failure
	}
	return clone
}
