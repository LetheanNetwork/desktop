// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — Stage F.B Phase 2.4 cross-cut. Subscribes to the
// incidents.IncidentEvent typed payload and projects each event into
// pkg/audit via RecordBatch.
//
// Incidents share pipeline's dual-shape bus pattern (named-action
// c.Subscribe(EventOpened/EventTransitioned, fn) AND typed
// c.RegisterAction handling). F.B uses the typed form per Q-A.2
// ruling; F+ work consolidates the named-action shape into the
// typed one across incidents + pipeline.

package incidents

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// Reserved audit event-names. Parity-grep contract sees these
// constants when AST-walking the package; emit-side projects the
// same constants.
const (
	AuditEventCreated      = "incidents.created"
	AuditEventTransitioned = "incidents.transitioned"
)

// AttachAudit subscribes the audit recorder to IncidentEvent.
//
// Usage example:
//
//	incidents.AttachAudit(c)
func AttachAudit(c *core.Core) {
	if c == nil {
		return
	}
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		ev, ok := msg.(IncidentEvent)
		if !ok {
			return core.Result{OK: true}
		}
		name := mapEventNameToAuditEvent(ev.EventName)
		if name == "" {
			return core.Result{OK: true}
		}
		_ = audit.Default().Record(audit.Event{
			Event:   name,
			Scope:   "incidents.transition",
			Outcome: audit.OutcomeOK,
			TS:      ev.At.Unix(),
			Meta: map[string]any{
				"incident_id": ev.Entry.ID,
				"state":       ev.Entry.State,
				"__emit_ts":   ev.At.UnixNano(),
			},
		})
		return core.Result{OK: true}
	})
}

// mapEventNameToAuditEvent translates the incidents EventOpened /
// EventTransitioned constants to the reserved audit event names.
// Unknown event-names return "" so future incidents-package
// additions don't accidentally land in the audit stream without
// a deliberate parity update.
func mapEventNameToAuditEvent(name string) string {
	switch name {
	case EventOpened:
		return AuditEventCreated
	case EventTransitioned:
		return AuditEventTransitioned
	}
	return ""
}
