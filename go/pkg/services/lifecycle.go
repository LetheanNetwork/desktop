// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"time"
	"unicode/utf8"

	core "dappco.re/go"
	coreprocess "dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/audit"
)

// Start explicitly starts a known definition through the named go-process
// runtime. Merely loading or reading the catalogue never calls this method.
func (service *Service) Start(id string) core.Result {
	return service.startManaged(id, "start", false, true, false)
}

func (service *Service) startManaged(
	id string,
	operation string,
	preclaimed bool,
	emitRequested bool,
	automatic bool,
) core.Result {
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
	requestedName, succeededName, failedName := lifecycleAuditNames(operation)
	if record.snapshot.State == StateRunning && record.process != nil {
		snapshot := cloneSnapshot(record.snapshot)
		policy := record.definition.RestartPolicy
		service.mu.Unlock()
		if emitRequested {
			service.auditRequested(requestedName, id, policy)
		}
		service.auditSucceeded(succeededName, id, snapshot.ProcessID)
		return core.Ok(snapshot)
	}
	if preclaimed {
		if !record.operation {
			service.mu.Unlock()
			return operationInProgress("services.Service.Start", id)
		}
	} else {
		if record.operation {
			service.mu.Unlock()
			return operationInProgress("services.Service.Start", id)
		}
		if record.restartCancel != nil {
			record.restartCancel()
			record.restartCancel = nil
		}
		record.operation = true
	}
	if service.runningCountLocked() >= service.options.Limits.MaxRunning {
		record.operation = false
		service.mu.Unlock()
		result := failureResult(
			ErrorRunningLimitReached,
			"services.Service.Start",
			"The managed service running limit has been reached.",
			nil,
		)
		if emitRequested {
			service.auditRequested(requestedName, id, record.definition.RestartPolicy)
		}
		service.auditFailed(failedName, id, result)
		return result
	}
	if !automatic {
		record.restartTimes = nil
		record.snapshot.RestartCount = 0
	}
	record.generation++
	generation := record.generation
	previous := cloneSnapshot(record.snapshot)
	record.snapshot.State = StateStarting
	record.snapshot.Desired = true
	record.snapshot.LastError = nil
	starting := cloneSnapshot(record.snapshot)
	definition := cloneDefinition(record.definition)
	service.mu.Unlock()

	if emitRequested {
		service.auditRequested(requestedName, id, definition.RestartPolicy)
	}
	service.fireEvent(operation, previous, starting, "")

	directoryResult := service.options.WorkingDirectoryResolver.Resolve(
		definition.WorkingDirectory,
	)
	if !directoryResult.OK {
		return service.failStart(
			id,
			generation,
			operation,
			starting,
			failedName,
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
			operation,
			starting,
			failedName,
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
			operation,
			starting,
			failedName,
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
			operation,
			starting,
			failedName,
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
			operation,
			starting,
			failedName,
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
		result := failureResult(
			ErrorServicesUnavailable,
			"services.Service.Start",
			"Services manager stopped while the process was starting.",
			nil,
		)
		service.auditFailed(failedName, id, result)
		return result
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

	service.fireEvent(operation, starting, snapshot, "")
	service.auditSucceeded(succeededName, id, snapshot.ProcessID)
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
	policy := record.definition.RestartPolicy
	if record.restartCancel != nil && record.process == nil {
		previous := cloneSnapshot(record.snapshot)
		record.restartCancel()
		record.restartCancel = nil
		record.snapshot.State = StateStopped
		record.snapshot.Desired = false
		record.snapshot.LastError = nil
		next := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		service.auditRequested(audit.EventServiceStopRequested, id, policy)
		service.fireEvent("stop", previous, next, "")
		service.auditSucceeded(audit.EventServiceStopSucceeded, id, "")
		return core.Ok(next)
	}
	if record.process == nil || record.snapshot.ProcessID == "" {
		record.snapshot.Desired = false
		snapshot := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		service.auditRequested(audit.EventServiceStopRequested, id, policy)
		service.auditSucceeded(audit.EventServiceStopSucceeded, id, "")
		return core.Ok(snapshot)
	}
	record.operation = true
	previous := cloneSnapshot(record.snapshot)
	record.snapshot.State = StateStopping
	record.snapshot.Desired = false
	stopping := cloneSnapshot(record.snapshot)
	generation := record.generation
	processID := record.snapshot.ProcessID
	service.mu.Unlock()

	service.auditRequested(audit.EventServiceStopRequested, id, policy)
	service.fireEvent("stop", previous, stopping, "")
	resolved := service.options.Process.Get(processID)
	if !resolved.OK {
		return service.failStop(
			id,
			generation,
			"stop",
			stopping,
			audit.EventServiceStopFailed,
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
			"stop",
			stopping,
			audit.EventServiceStopFailed,
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
			"stop",
			stopping,
			audit.EventServiceStopFailed,
			ErrorProcessStopFailed,
			"The service process could not be stopped.",
			stopped.Err(),
		)
	}

	service.mu.Lock()
	record, ok = service.records[id]
	if !ok || record.generation != generation {
		service.mu.Unlock()
		result := failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Stop",
			"The service process generation changed while stopping.",
			nil,
		)
		service.auditFailed(audit.EventServiceStopFailed, id, result)
		return result
	}
	record.operation = false
	record.process = nil
	record.snapshot.State = StateStopped
	record.snapshot.Desired = false
	record.snapshot.ProcessID = ""
	record.snapshot.PID = 0
	record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	record.snapshot.LastError = nil
	snapshot := cloneSnapshot(record.snapshot)
	service.mu.Unlock()

	service.fireEvent("stop", stopping, snapshot, "")
	service.auditSucceeded(audit.EventServiceStopSucceeded, id, processID)
	return core.Ok(snapshot)
}

// Restart performs one serialised graceful stop followed by an explicit
// replacement start.
func (service *Service) Restart(id string) core.Result {
	service.mu.Lock()
	if result := service.readyLocked("services.Service.Restart"); !result.OK {
		service.mu.Unlock()
		return result
	}
	record, ok := service.records[id]
	if !ok {
		service.mu.Unlock()
		return definitionNotFound("services.Service.Restart", id)
	}
	if record.operation {
		service.mu.Unlock()
		return operationInProgress("services.Service.Restart", id)
	}
	if record.restartCancel != nil {
		record.restartCancel()
		record.restartCancel = nil
	}
	record.operation = true
	record.restartTimes = nil
	record.snapshot.RestartCount = 0
	policy := record.definition.RestartPolicy
	previous := cloneSnapshot(record.snapshot)
	processID := record.snapshot.ProcessID
	generation := record.generation
	if record.process != nil && processID != "" {
		record.snapshot.State = StateStopping
	} else {
		record.snapshot.State = StateStopped
	}
	record.snapshot.Desired = false
	stopping := cloneSnapshot(record.snapshot)
	service.mu.Unlock()

	service.auditRequested(audit.EventServiceRestartRequested, id, policy)
	service.fireEvent("restart", previous, stopping, "")
	if processID != "" {
		resolved := service.options.Process.Get(processID)
		if !resolved.OK {
			return service.failStop(
				id,
				generation,
				"restart",
				stopping,
				audit.EventServiceRestartFailed,
				ErrorProcessLookupFailed,
				"The managed process identity could not be verified.",
				resolved.Err(),
			)
		}
		process, valid := resolved.Value.(ProcessHandle)
		if !valid || process == nil || process.Info().ID != processID {
			return service.failStop(
				id,
				generation,
				"restart",
				stopping,
				audit.EventServiceRestartFailed,
				ErrorProcessLookupFailed,
				"The managed process identity did not match.",
				nil,
			)
		}
		if result := process.Shutdown(); !result.OK {
			return service.failStop(
				id,
				generation,
				"restart",
				stopping,
				audit.EventServiceRestartFailed,
				ErrorProcessStopFailed,
				"The service process could not be stopped for restart.",
				result.Err(),
			)
		}
	}

	service.mu.Lock()
	record, ok = service.records[id]
	if !ok || record.generation != generation {
		service.mu.Unlock()
		result := failureResult(
			ErrorProcessLookupFailed,
			"services.Service.Restart",
			"The service process generation changed during restart.",
			nil,
		)
		service.auditFailed(audit.EventServiceRestartFailed, id, result)
		return result
	}
	record.process = nil
	record.snapshot.State = StateStopped
	record.snapshot.ProcessID = ""
	record.snapshot.PID = 0
	record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	stoppedSnapshot := cloneSnapshot(record.snapshot)
	service.mu.Unlock()

	service.fireEvent("restart", stopping, stoppedSnapshot, "")
	return service.startManaged(id, "restart", true, false, false)
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

// OnShutdown prevents new starts, cancels pending restart work, and
// gracefully shuts down every current process without skipping failures.
func (service *Service) OnShutdown(ctx core.Context) core.Result {
	if service == nil {
		return core.Ok(nil)
	}
	if ctx == nil {
		ctx = core.Background()
	}
	type shutdownItem struct {
		id         string
		generation uint64
		process    ProcessHandle
		processID  string
		previous   Snapshot
		stopping   Snapshot
		policy     RestartPolicy
	}
	service.mu.Lock()
	if service.shuttingDown {
		service.mu.Unlock()
		return core.Ok(nil)
	}
	service.shuttingDown = true
	items := make([]shutdownItem, 0)
	for id, record := range service.records {
		if record.restartCancel != nil {
			record.restartCancel()
			record.restartCancel = nil
		}
		record.snapshot.Desired = false
		if record.process == nil {
			if record.snapshot.State == StateStarting ||
				record.snapshot.State == StateExited ||
				record.snapshot.State == StateFailed {
				record.snapshot.State = StateStopped
			}
			continue
		}
		previous := cloneSnapshot(record.snapshot)
		record.operation = true
		record.snapshot.State = StateStopping
		stopping := cloneSnapshot(record.snapshot)
		items = append(items, shutdownItem{
			id:         id,
			generation: record.generation,
			process:    record.process,
			processID:  record.snapshot.ProcessID,
			previous:   previous,
			stopping:   stopping,
			policy:     record.definition.RestartPolicy,
		})
	}
	service.mu.Unlock()

	for _, item := range items {
		service.auditRequested(audit.EventServiceStopRequested, item.id, item.policy)
		service.fireEvent("shutdown", item.previous, item.stopping, "")
	}
	type shutdownResult struct {
		item   shutdownItem
		result core.Result
	}
	results := make(chan shutdownResult, len(items))
	for _, item := range items {
		go func(item shutdownItem) {
			results <- shutdownResult{item: item, result: item.process.Shutdown()}
		}(item)
	}

	failures := 0
	received := 0
	for received < len(items) {
		select {
		case completed := <-results:
			received++
			service.mu.Lock()
			record, ok := service.records[completed.item.id]
			if ok && record.generation == completed.item.generation {
				record.operation = false
				if completed.result.OK {
					record.process = nil
					record.snapshot.State = StateStopped
					record.snapshot.ProcessID = ""
					record.snapshot.PID = 0
					record.snapshot.StoppedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
					record.snapshot.LastError = nil
				} else {
					failures++
					failure := &Failure{
						Code:      ErrorShutdownIncomplete,
						Operation: "services.Service.OnShutdown",
						Message:   "A managed service did not stop cleanly.",
						Cause:     completed.result.Err(),
					}
					record.snapshot.State = StateFailed
					record.snapshot.LastError = failureView(failure)
				}
				next := cloneSnapshot(record.snapshot)
				service.mu.Unlock()
				if completed.result.OK {
					service.fireEvent("shutdown", completed.item.stopping, next, "")
					service.auditSucceeded(
						audit.EventServiceStopSucceeded,
						completed.item.id,
						completed.item.processID,
					)
				} else {
					result := core.Fail(&Failure{
						Code:      ErrorShutdownIncomplete,
						Operation: "services.Service.OnShutdown",
						Message:   "A managed service did not stop cleanly.",
						Cause:     completed.result.Err(),
					})
					service.fireEvent(
						"shutdown",
						completed.item.stopping,
						next,
						ErrorShutdownIncomplete,
					)
					service.auditFailed(
						audit.EventServiceStopFailed,
						completed.item.id,
						result,
					)
				}
			} else {
				service.mu.Unlock()
				failures++
			}
		case <-ctx.Done():
			failures += len(items) - received
			received = len(items)
		}
	}
	if failures > 0 {
		return failureResult(
			ErrorShutdownIncomplete,
			"services.Service.OnShutdown",
			core.Sprintf("%d managed service shutdown operation(s) were incomplete.", failures),
			ctx.Err(),
		)
	}
	return core.Ok(nil)
}

func (service *Service) failStart(
	id string,
	generation uint64,
	operation string,
	previous Snapshot,
	failedName string,
	failure *Failure,
) core.Result {
	service.mu.Lock()
	next := previous
	if record, ok := service.records[id]; ok && record.generation == generation {
		record.operation = false
		record.process = nil
		record.snapshot.State = StateFailed
		record.snapshot.Desired = false
		record.snapshot.ProcessID = ""
		record.snapshot.PID = 0
		record.snapshot.LastError = failureView(failure)
		next = cloneSnapshot(record.snapshot)
	}
	service.mu.Unlock()
	result := core.Fail(failure)
	service.fireEvent(operation, previous, next, failure.Code)
	service.auditFailed(failedName, id, result)
	return result
}

func (service *Service) failStop(
	id string,
	generation uint64,
	operation string,
	previous Snapshot,
	failedName string,
	code ErrorCode,
	message string,
	cause error,
) core.Result {
	failure := &Failure{
		Code:      code,
		Operation: core.Concat("services.Service.", operation),
		Message:   message,
		Cause:     cause,
	}
	service.mu.Lock()
	next := previous
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
		next = cloneSnapshot(record.snapshot)
	}
	service.mu.Unlock()
	result := core.Fail(failure)
	service.fireEvent(operation, previous, next, code)
	service.auditFailed(failedName, id, result)
	return result
}

func (service *Service) observeExit(
	id string,
	generation uint64,
	process ProcessHandle,
) {
	waited := process.Wait()
	info := process.Info()
	now := service.options.Now()
	service.mu.Lock()
	record, ok := service.records[id]
	if !ok || record.generation != generation || record.process != process {
		service.mu.Unlock()
		return
	}
	previous := cloneSnapshot(record.snapshot)
	record.process = nil
	record.snapshot.PID = 0
	record.snapshot.StoppedAt = now.UTC().Format(time.RFC3339Nano)
	record.snapshot.ExitCode = info.ExitCode
	if !record.snapshot.Desired ||
		record.snapshot.State == StateStopping ||
		service.shuttingDown {
		record.snapshot.State = StateStopped
		record.snapshot.Desired = false
		record.snapshot.LastError = nil
		next := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		service.fireEvent("exit", previous, next, "")
		return
	}

	failed := !waited.OK || info.ExitCode != 0 ||
		info.Status == coreprocess.StatusFailed ||
		info.Status == coreprocess.StatusKilled
	if failed {
		record.snapshot.State = StateFailed
		record.snapshot.LastError = &FailureView{
			Code:    ErrorProcessStartFailed,
			Message: "The service process exited unexpectedly.",
		}
	} else {
		record.snapshot.State = StateExited
		record.snapshot.LastError = nil
	}
	shouldRestart := record.definition.RestartPolicy == RestartAlways ||
		(record.definition.RestartPolicy == RestartOnFailure && failed)
	if !shouldRestart {
		record.snapshot.Desired = false
		next := cloneSnapshot(record.snapshot)
		code := ErrorCode("")
		if failed {
			code = ErrorProcessStartFailed
		}
		service.mu.Unlock()
		service.fireEvent("exit", previous, next, code)
		return
	}

	windowStart := now.Add(-service.options.Limits.RestartWindow)
	recent := record.restartTimes[:0]
	for _, restartedAt := range record.restartTimes {
		if restartedAt.After(windowStart) {
			recent = append(recent, restartedAt)
		}
	}
	record.restartTimes = recent
	if len(record.restartTimes) >= service.options.Limits.RestartLimit {
		record.snapshot.Desired = false
		record.snapshot.State = StateFailed
		record.snapshot.LastError = &FailureView{
			Code:    ErrorRestartBudgetExhausted,
			Message: "Automatic restart stopped to prevent a crash loop.",
		}
		next := cloneSnapshot(record.snapshot)
		service.mu.Unlock()
		service.fireEvent(
			"exit",
			previous,
			next,
			ErrorRestartBudgetExhausted,
		)
		return
	}
	record.restartTimes = append(record.restartTimes, now)
	record.snapshot.RestartCount++
	restartContext, cancel := core.WithCancel(core.Background())
	record.restartCancel = cancel
	delay := service.restartDelay(len(record.restartTimes) - 1)
	next := cloneSnapshot(record.snapshot)
	service.mu.Unlock()

	code := ErrorCode("")
	if failed {
		code = ErrorProcessStartFailed
	}
	service.fireEvent("exit", previous, next, code)
	go service.waitAndRestart(id, generation, restartContext, delay)
}

func (service *Service) waitAndRestart(
	id string,
	generation uint64,
	ctx core.Context,
	delay time.Duration,
) {
	select {
	case <-service.options.After(delay):
	case <-ctx.Done():
		return
	}
	if ctx.Err() != nil {
		return
	}
	service.mu.Lock()
	record, ok := service.records[id]
	if !ok ||
		record.generation != generation ||
		!record.snapshot.Desired ||
		service.shuttingDown {
		service.mu.Unlock()
		return
	}
	record.restartCancel = nil
	service.mu.Unlock()
	_ = service.startManaged(id, "restart", false, true, true)
}

func (service *Service) restartDelay(attempt int) time.Duration {
	exponent := attempt
	if exponent > service.options.Limits.RestartExponentCap {
		exponent = service.options.Limits.RestartExponentCap
	}
	if exponent > 30 {
		exponent = 30
	}
	delay := service.options.Limits.RestartBaseDelay * time.Duration(1<<exponent)
	if delay > service.options.Limits.RestartMaxDelay {
		return service.options.Limits.RestartMaxDelay
	}
	return delay
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

func lifecycleAuditNames(operation string) (string, string, string) {
	if operation == "restart" {
		return audit.EventServiceRestartRequested,
			audit.EventServiceRestartSucceeded,
			audit.EventServiceRestartFailed
	}
	return audit.EventServiceStartRequested,
		audit.EventServiceStartSucceeded,
		audit.EventServiceStartFailed
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
