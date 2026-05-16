// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — Stage F.B Phase 2.4 cross-cut. Subscribes to the
// pipeline.PipelineMovedEvent typed payload (the typed half of the
// dual-shape pipeline bus) and projects each event into pkg/audit
// via RecordBatch.
//
// Pipeline uses BOTH the named-action shape (c.Subscribe(name, fn))
// AND the typed-message shape (c.RegisterAction with type assert)
// per Q-A.2 ruling in RFC.stage-f.md §6.7. F.B subscribes via the
// typed shape since it carries the structured fields without
// going through the core.Options map+round-trip; F+ work will
// consolidate the named-action shape into the typed one across
// pipeline + incidents.

package pipeline

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// Reserved audit event-name. Parity-grep contract — declared at
// package scope so the test side AST-walks the constant alongside
// the emit-site's c.ACTION.
const AuditEventStageChanged = "pipeline.stage_changed"

// AttachAudit subscribes the audit recorder to PipelineMovedEvent.
// Boot path SHOULD call exactly once.
//
// Usage example:
//
//	pipeline.AttachAudit(c)
func AttachAudit(c *core.Core) {
	if c == nil {
		return
	}
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		ev, ok := msg.(PipelineMovedEvent)
		if !ok {
			return core.Result{OK: true}
		}
		_ = audit.Default().Record(audit.Event{
			Event:   AuditEventStageChanged,
			Scope:   "pipeline.move",
			Outcome: audit.OutcomeOK,
			TS:      ev.At.Unix(),
			Meta: map[string]any{
				"deal_id":    ev.DealID,
				"from_stage": ev.FromStage,
				"to_stage":   ev.ToStage,
				"__emit_ts":  ev.At.UnixNano(),
			},
		})
		return core.Result{OK: true}
	})
}
