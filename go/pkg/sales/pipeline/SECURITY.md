# pkg/sales/pipeline — security discipline

This file records the security boundary `pipeline.MoveDeal` enforces and
the discipline contributors MUST follow when changing the handler. The
discipline is load-bearing: pipeline state drives forecast, commission
triggers, Blesta off-boarding automation, and lthn-mlx Vi callouts.
A silent illegal flip distorts every downstream surface.

## Threat model

The handler is reachable from three callers:

1. The salesperson via the `<lthn-view-pipeline>` Lit view (legitimate).
2. Vi (the LLM sidecar) via the Core ACTION bus — Vi can be tricked by
   prompt-injected content carried in any source Vi reads (PR bodies,
   commit messages, customer email surfaced via the activity view,
   plugin-view iframe messages).
3. Any process holding a valid session bearer-token that can reach the
   Wails surface (account compromise, malicious sibling-plugin running
   under the same session).

Caller (1) is in-scope for the UX; callers (2) and (3) are the threat.
The handler MUST behave identically regardless of caller — auth-edge
filtering is the session-token layer's job, this handler is the
last-line defence per defence-in-depth.

The Cerberus pass-9 DREAD score for "MoveDeal accepts any
{DealID, ToStage} from any bearer-holder" was **31 (HIGH)** — Mantis
#1488. The defence-stack here is the response.

## Defence stack

`MoveDeal` runs every move through five gates in fixed order. Each gate
short-circuits with a typed `core.NewCode` failure AND emits a
structured audit event so Stage F log-tailing sees every attempt.

1. **`paths.IsValidID(DealID)`** — path-traversal defence per Mantis
   #1486. Rejects empty / dotted / slashed / null-byte ids before any
   filesystem join.
2. **Stage-membership check** — `ToStage` MUST be in the canonical
   `stageOrder()` set. Defends against typos AND prompt-injection
   introducing a synthetic stage ("victory") the renderer might display
   without forecast aggregation.
3. **Disk read for `from` stage** — the legality check needs the
   current stage; a well-formed but absent `DealID` rejects with
   `pipeline.deal_not_found`. The audit emission still fires because
   missing-deal probes look identical to active reconnaissance.
4. **`IsLegalTransition(from, to)`** — the load-bearing gate. The
   `LegalTransitions` map in `types.go` enumerates every permitted
   edge in the funnel graph. Edges NOT in the map reject with
   `transition_illegal`. The graph forbids:
   - **Skip-levels** (`qual → won`, `qual → close`, `engage → won`,
     etc.) — the primary prompt-injection signature.
   - **Multi-step rewinds** (`propose → qual`, `close → qual`,
     `close → engage`) — one-step backwards is permitted so the
     salesperson can correct a mis-classification; bigger jumps signal
     either prompt confusion OR active attempts to obscure history.
   - **Terminal-out** (`won → *`, `lost → *`) — won and lost have no
     outgoing edges. A legitimate restore is an exceptional admin verb,
     not a normal-path move (see *Force escape valve* below).
   - **Self-transitions** (`qual → qual`, etc.) — reflexive entries are
     omitted from the map so the back-stop catches them.
5. **`writeDealStage` via `paths.AtomicWriteWithVersion`** — optimistic-
   lock guarantee per Mantis #1540. The legality check runs against the
   `from` stage read in step 3; the persist gates on `IfVersion=N` so a
   concurrent writer that flipped the stage between steps 3 and 5
   loses (stale-version) and surfaces a `pipeline.update.conflict`
   envelope to the frontend `<lthn-conflict-toast>`.

## Audit event schema (reserved)

The handler emits `event=sales.pipeline.move_attempted` for every
call — including rejected calls — with fields:

```
event=sales.pipeline.move_attempted
  deal_id=<id>
  from_stage=<id>           (empty when unknown — invalid_id, unknown_stage)
  to_stage=<id>
  ts=<unix-seconds>
  outcome=<literal>         (see below)
  reason=<free-text>        (empty for ok / forced; non-empty for rejections)
  force=<true|false>        (mirrors MoveInput.Force)
```

`outcome` is a closed enumeration:

| Literal              | Trigger                                                          |
|----------------------|------------------------------------------------------------------|
| `ok`                 | Move landed via normal-path (legality check passed)              |
| `forced`             | Move landed via `Force: true` bypass                             |
| `transition_illegal` | IsLegalTransition rejected                                       |
| `deal_not_found`     | DealID well-formed but no file on disk                           |
| `unknown_stage`      | ToStage not in stageOrder()                                      |
| `invalid_id`         | paths.IsValidID rejected (path traversal, control bytes, empty)  |
| `write_failed`       | writeDealStage failed (incl. stale-version conflict)             |

The closed set is reserved schema. Adding an outcome is a security-
policy change — the Stage F log-tailer's outcome aggregator and the
Operations panel's filter chrome MUST learn the new literal in the same
commit.

A typed `PipelineMovedEvent` ALSO emits on the Core ACTION bus for
every successful move (`ok` and `forced`). The `audit.go` `AttachAudit`
subscriber projects this into `pkg/audit` via `Recorder.Record` so the
NDJSON forensic record persists alongside the textual `core.Print`
trail.

## User-notify on terminal transitions

Per Mantis #1488: terminal-stage moves (`won` / `lost`) MUST be
visible to the operator. The handler fires a SECOND typed event,
`PipelineDealTerminalEvent`, on the Core ACTION bus for these moves.
The app-shell toast subscriber renders a calm-voice pop so a
prompt-injection that DID land — or any legitimate close — announces
itself rather than silently flipping the forecast.

The frontend toast surface is mounted in `<lthn-app-shell>` (separate
ticket — backend notify-event suffices for beta per the SECURITY-NOTE
escape valve on the original brief). The typed event is the contract;
the toast UI is the consumer.

## `Force: true` escape valve

`MoveInput.Force` bypasses the `IsLegalTransition` gate so the
salesperson can perform an exceptional move — restore a lost deal a
customer revived, multi-step rewind after a data migration, etc.

Forced moves leave a DISTINCT audit shape (`outcome="forced"` AND
`force=true` on the textual record; `Force=true` on the typed
`PipelineMovedEvent`) so the Stage F log-tailer can categorise them
separately from normal-path moves and surface a pattern of forced
moves as a possible misuse signal.

The frontend MUST treat the flag as an explicit user gesture
(confirm-dialog + reason capture) — never auto-set, never default-true.
A Vi prompt-injection submitting `{Force: true}` is structurally
indistinguishable from a legitimate force, so the frontend gesture is
the only friction preventing routine abuse. This is by design — the
backend gate is for the silent-flip threat; the force flag is for the
deliberate-override case.

## Contributor checklist

When changing `MoveDeal` (or any function it calls):

- [ ] New `LegalTransitions` edges require Cerberus DREAD review +
      Mantis ticket trail. Every new edge widens the prompt-injection
      blast radius.
- [ ] Schema changes to the audit event (new field, renamed field,
      removed field, new `outcome` literal) require coordinated frontend
      + Stage F log-tailer updates in the same commit.
- [ ] New rejection paths MUST emit `auditMoveAttempt` BEFORE returning
      the `core.Fail` so the rejected attempt leaves a trail. The
      negative test for the new path MUST assert on the `outcome`
      literal, not just the response code.
- [ ] Tests cover the new shape under the `TestMoveDeal_*_{Good,Bad,Ugly}`
      naming convention. Mantis #1488 prescribed matrix:
      `LinearForwardLegal_Good`, `RewindRejected_Bad`,
      `SkipAheadRejected_Bad`, `AnyToLostAllowed_Good`,
      `ForceFlagAllowsRewind_Ugly`, `WonTransitionTriggersNotify_Good`,
      `AuditEmitted_Good`.
- [ ] The `path allowlist` for any commit touching this package MUST
      stay inside `pkg/sales/pipeline/`. Frontend wire-ups land in
      separate tickets.

## References

- Mantis #1488 — Cerberus pass-9 HIGH, DREAD 31, the originating finding
- Mantis #1486 — paths.IsValidID, the path-traversal layer beneath this
- Mantis #1540 — paths.AtomicWriteWithVersion, optimistic-lock substrate
- Mantis #1544 — conflict envelope wire shape
- RFC `plans/code/lthn/desktop/auth-gate/RFC.stage-f.md` §6 — audit schema
- Cerberus security pipeline memory: `design_security_pipeline_athena_cerberus_hephaestus.md`
