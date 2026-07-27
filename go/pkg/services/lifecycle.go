// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"time"
	"unicode/utf8"

	core "dappco.re/go"
	coreprocess "dappco.re/go/process"
)

// Start explicitly starts a known definition through the named go-process
// runtime. Merely loading or reading the catalogue never calls this method.
func (service *Service) Start(id string) core.Result {
	service.mu.Lock()
	if result := service.readyLocked("services.Service.Start"); !result.OK {
		service.mu.Unlock()
		return result
	}
	record, ok := service.records[id]
	if !ok {
		service.mu.Unlock()
		return definitionNotFound("services.Service.Start", id)
	}
	if record.snapshot.State == StateRunning && record.process != nil {
		snapshot := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		return core.Ok(snapshot)
	}
	if record.operation {
		service.mu.Unlock()
		return operationInProgress("services.Service.Start", id)
	}
	if service.runningCountLocked() >= service.options.Limits.MaxRunning {
		service.mu.Unlock()
		return failureResult(
			ErrorRunningLimitReached,
			"services.Service.Start",
			"The managed service running limit has been reached.",
			nil,
		)
	}
	record.operation = true
	record.generation++
	generation := record.generation
	previous := cloneSnapshot(record.snapshot)
	record.snapshot.State = StateStarting
	record.snapshot.Desired = true
	record.snapshot.LastError = nil
	definition := cloneDefinition(record.definition)
	service.mu.Unlock()

	directoryResult := service.options.WorkingDirectoryResolver.Resolve(
		definition.WorkingDirectory,
	)
	if !directoryResult.OK {
		return service.failStart(
			id,
			generation,
			previous,
			failureFromResult(
				directoryResult,
				ErrorWorkingDirectoryUnsupported,
				"services.Service.Start",
				"Service working directory is unavailable.",
			),
		)
	}
	directory, ok := directoryResult.Value.(string)
	if !ok {
		return service.failStart(
			id,
			generation,
			previous,
			&Failure{
				Code:      ErrorWorkingDirectoryUnsupported,
				Operation: "services.Service.Start",
				Message:   "Service working directory resolution was invalid.",
			},
		)
	}
	started := service.options.Process.StartWithOptions(
		service.ctx,
		coreprocess.RunOptions{
			Command:        definition.Command,
			Args:           append([]string(nil), definition.Arguments...),
			Dir:            directory,
			DisableCapture: false,
			Detach:         true,
			GracePeriod: time.Duration(
				definition.GracePeriodMillis,
			) * time.Millisecond,
			KillGroup: true,
		},
	)
	if !started.OK {
		return service.failStart(
			id,
			generation,
			previous,
			&Failure{
				Code:      ErrorProcessStartFailed,
				Operation: "services.Service.Start",
				Message:   "The service process could not be started.",
				Cause:     started.Err(),
			},
		)
	}
	process, ok := started.Value.(ProcessHandle)
	if !ok || process == nil {
		return service.failStart(
			id,
			generation,
			previous,
			&Failure{
				Code:      ErrorProcessStartFailed,
				Operation: "services.Service.Start",
				Message:   "The process service returned an invalid process handle.",
			},
		)
	}
	info := process.Info()
	if info.ID == "" || info.PID <= 0 || !info.Running {
		_ = process.Shutdown()
		return service.failStart(
			id,
			generation,
			previous,
			&Failure{
				Code:      ErrorProcessStartFailed,
				Operation: "services.Service.Start",
				Message:   "The process service returned an invalid running process.",
			},
		)
	}

	service.mu.Lock()
	record, ok = service.records[id]
	if !ok || record.generation != generation || service.shuttingDown {
		service.mu.Unlock()
		_ = process.Shutdown()
		return failureResult(
			ErrorServicesUnavailable,
			"services.Service.Start",
			"Services manager stopped while the process was starting.",
			nil,
		)
	}
	record.process = process
	record.operation = false
	record.snapshot.State = StateRunning
	record.snapshot.Desired = true
	record.snapshot.ProcessID = info.ID
	record.snapshot.PID = info.PID
	record.snapshot.StartedAt = info.StartedAt.UTC().Format(time.RFC3339Nano)
	record.snapshot.StoppedAt = ""
	record.snapshot.ExitCode = 0
	record.snapshot.LastError = nil
	snapshot := cloneSnapshot(record.snapshot)
	service.mu.Unlock()

	go service.observeExit(id, generation, process)
	return core.Ok(snapshot)
}

// Stop idempotently and gracefully stops a known go-process identity.
func (service *Service) Stop(id string) core.Result {
	service.mu.Lock()
	if result := service.readyLocked("services.Service.Stop"); !result.OK {
		service.mu.Unlock()
		return result
	}
	record, ok := service.records[id]
	if !ok {
		service.mu.Unlock()
		return definitionNotFound("services.Service.Stop", id)
	}
	if record.operation {
		service.mu.Unlock()
		return operationInProgress("services.Service.Stop", id)
	}
	if record.process == nil || record.snapshot.ProcessID == "" {
		record.snapshot.Desired = false
		snapshot := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		return core.Ok(snapshot)
	}
	record.operation = true
	record.snapshot.State = StateStopping
	record.snapshot.Desired = false
	generation := record.generation
	processID := record.snapshot.ProcessID
	service.mu.Unlock()

	resolved := service.options.Process.Get(processID)
	if !resolved.OK {
		return service.failStop(
			id,
			generation,
			ErrorProcessLookupFailed,
			"The managed process identity could not be verified.",
			resolved.Err(),
		)
	}
	process, ok := resolved.Value.(ProcessHandle)
	if !ok || process == nil || process.Info().ID != processID {
		return service.failStop(
			id,
			generation,
			ErrorProcessLookupFailed,
			"The managed process identity did not match.",
			nil,
		)
	}
	stopped := process.Shutdown()
	if !stopped.OK {
		return service.failStop(
			id,
			generation,
			ErrorProcessStopFailed,
			"The service process could not be stopped.",
			stopped.Err(),
		)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	record, ok = service.records[id]
	if !ok || record.generation != generation {
		return failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Stop",
			"The service process generation changed while stopping.",
			nil,
		)
	}
	record.operation = false
	record.process = nil
	record.snapshot.State = StateStopped
	record.snapshot.Desired = false
	record.snapshot.ProcessID = ""
	record.snapshot.PID = 0
	record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	record.snapshot.LastError = nil
	return core.Ok(cloneSnapshot(record.snapshot))
}

// Output returns a UTF-8-safe bounded tail from the current go-process
// generation.
func (service *Service) Output(request OutputRequest) core.Result {
	if request.Limit <= 0 || request.Limit > service.options.Limits.MaxOutputBytes {
		return failureResult(
			ErrorDefinitionInvalid,
			"services.Service.Output",
			"Output limit is outside the allowed range.",
			nil,
		)
	}
	service.mu.RLock()
	if result := service.readyLocked("services.Service.Output"); !result.OK {
		service.mu.RUnlock()
		return result
	}
	record, ok := service.records[request.ID]
	if !ok {
		service.mu.RUnlock()
		return definitionNotFound("services.Service.Output", request.ID)
	}
	if record.process == nil || record.snapshot.ProcessID == "" {
		service.mu.RUnlock()
		return failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Output",
			"This service has no current process output.",
			nil,
		)
	}
	processID := record.snapshot.ProcessID
	generation := record.generation
	service.mu.RUnlock()

	resolved := service.options.Process.Get(processID)
	if !resolved.OK {
		return failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Output",
			"The managed process identity could not be verified.",
			resolved.Err(),
		)
	}
	process, ok := resolved.Value.(ProcessHandle)
	if !ok || process == nil || process.Info().ID != processID {
		return failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Output",
			"The managed process identity did not match.",
			nil,
		)
	}
	output, truncated := boundedUTF8Tail(process.Output(), request.Limit)

	service.mu.RLock()
	current, stillCurrent := service.records[request.ID]
	if !stillCurrent ||
		current.generation != generation ||
		current.snapshot.ProcessID != processID {
		service.mu.RUnlock()
		return failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Output",
			"The service process changed while output was read.",
			nil,
		)
	}
	service.mu.RUnlock()
	return core.Ok(OutputView{
		ID:         request.ID,
		ProcessID:  processID,
		Generation: generation,
		Output:     output,
		Truncated:  truncated,
		ObservedAt: service.options.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (service *Service) failStart(
	id string,
	generation uint64,
	_ Snapshot,
	failure *Failure,
) core.Result {
	service.mu.Lock()
	if record, ok := service.records[id]; ok && record.generation == generation {
		record.operation = false
		record.process = nil
		record.snapshot.State = StateFailed
		record.snapshot.Desired = false
		record.snapshot.ProcessID = ""
		record.snapshot.PID = 0
		record.snapshot.LastError = failureView(failure)
	}
	service.mu.Unlock()
	return core.Fail(failure)
}

func (service *Service) failStop(
	id string,
	generation uint64,
	code ErrorCode,
	message string,
	cause error,
) core.Result {
	failure := &Failure{
		Code:      code,
		Operation: "services.Service.Stop",
		Message:   message,
		Cause:     cause,
	}
	service.mu.Lock()
	if record, ok := service.records[id]; ok && record.generation == generation {
		record.operation = false
		record.snapshot.State = StateFailed
		record.snapshot.Desired = false
		record.snapshot.LastError = failureView(failure)
		if code == ErrorProcessLookupFailed {
			record.process = nil
			record.snapshot.ProcessID = ""
			record.snapshot.PID = 0
			record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	service.mu.Unlock()
	return core.Fail(failure)
}

func (service *Service) observeExit(
	id string,
	generation uint64,
	process ProcessHandle,
) {
	waited := process.Wait()
	info := process.Info()
	service.mu.Lock()
	defer service.mu.Unlock()
	record, ok := service.records[id]
	if !ok || record.generation != generation || record.process != process {
		return
	}
	record.process = nil
	record.snapshot.PID = 0
	record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	record.snapshot.ExitCode = info.ExitCode
	if !record.snapshot.Desired || record.snapshot.State == StateStopping {
		record.snapshot.State = StateStopped
		record.snapshot.Desired = false
		record.snapshot.LastError = nil
		return
	}
	record.snapshot.Desired = false
	if waited.OK && info.ExitCode == 0 {
		record.snapshot.State = StateExited
		record.snapshot.LastError = nil
		return
	}
	record.snapshot.State = StateFailed
	record.snapshot.LastError = &FailureView{
		Code:    ErrorProcessStartFailed,
		Message: "The service process exited unexpectedly.",
	}
}

func (service *Service) runningCountLocked() int {
	count := 0
	for _, record := range service.records {
		switch record.snapshot.State {
		case StateStarting, StateRunning, StateStopping:
			count++
		}
	}
	return count
}

func failureView(failure *Failure) *FailureView {
	if failure == nil {
		return nil
	}
	return &FailureView{
		Code:    failure.Code,
		Message: failure.Message,
	}
}

func boundedUTF8Tail(output string, limit int) (string, bool) {
	if len(output) <= limit {
		return output, false
	}
	start := len(output) - limit
	for start < len(output) && !utf8.RuneStart(output[start]) {
		start++
	}
	return output[start:], true
}
