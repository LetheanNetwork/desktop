// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
)

const serviceAuditScope = "service"

// Subscribe registers a typed managed-service listener on Core's ACTION bus.
func Subscribe(c *core.Core, fn func(*core.Core, Event)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, message core.Message) core.Result {
		if event, ok := message.(Event); ok {
			fn(c, event)
		}
		return core.Ok(nil)
	})
}

type defaultAuditRecorder struct{}

func (defaultAuditRecorder) Record(event audit.Event) core.Result {
	return audit.Default().Record(event)
}

func (service *Service) coreApp() *core.Core {
	if service == nil || service.ServiceRuntime == nil {
		return nil
	}
	return service.Core()
}

func (service *Service) fireEvent(
	operation string,
	previous Snapshot,
	next Snapshot,
	code ErrorCode,
) {
	coreApp := service.coreApp()
	if coreApp == nil {
		return
	}
	_ = coreApp.ACTION(Event{
		ID:        next.Definition.ID,
		Operation: operation,
		Previous:  previous.State,
		State:     next.State,
		Desired:   next.Desired,
		ProcessID: next.ProcessID,
		ErrorCode: code,
		At:        service.options.Now().UTC().Format(core.RFC3339Nano),
	})
}

func (service *Service) auditRequested(
	name string,
	id string,
	policy RestartPolicy,
) {
	if service == nil || service.options.Audit == nil {
		return
	}
	_ = service.options.Audit.Record(audit.Event{
		Event:   name,
		TS:      service.options.Now().UTC().Unix(),
		Scope:   serviceAuditScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"service_id":     id,
			"restart_policy": string(policy),
		},
	})
}

func (service *Service) auditSucceeded(
	name string,
	id string,
	processID string,
) {
	if service == nil || service.options.Audit == nil {
		return
	}
	meta := map[string]any{"service_id": id}
	if processID != "" {
		meta["process_id"] = processID
	}
	_ = service.options.Audit.Record(audit.Event{
		Event:   name,
		TS:      service.options.Now().UTC().Unix(),
		Scope:   serviceAuditScope,
		Outcome: audit.OutcomeOK,
		Meta:    meta,
	})
}

func (service *Service) auditFailed(
	name string,
	id string,
	result core.Result,
) {
	if service == nil || service.options.Audit == nil {
		return
	}
	code := ErrorCodeOf(result)
	if code == "" {
		code = ErrorServicesUnavailable
	}
	_ = service.options.Audit.Record(audit.Event{
		Event:   name,
		TS:      service.options.Now().UTC().Unix(),
		Scope:   serviceAuditScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"service_id":            id,
			audit.MetaKeyErrorCode:  string(code),
			audit.MetaKeyErrorScope: serviceErrorScope(result),
		},
	})
}

func (service *Service) auditDefinitionChanged(id string) {
	if service == nil || service.options.Audit == nil {
		return
	}
	_ = service.options.Audit.Record(audit.Event{
		Event:   audit.EventServiceDefinitionChanged,
		TS:      service.options.Now().UTC().Unix(),
		Scope:   serviceAuditScope,
		Outcome: audit.OutcomeOK,
		Meta:    map[string]any{"service_id": id},
	})
}

func serviceErrorScope(result core.Result) string {
	var failure *Failure
	if !result.OK && core.As(result.Err(), &failure) && failure != nil {
		return failure.Operation
	}
	return "services"
}
