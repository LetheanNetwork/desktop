// SPDX-Licence-Identifier: EUPL-1.2

package forecast

// Forecast is read-only in v1 — no events are emitted.
// This file exists to maintain the consistent 5-file package shape.
//
// Future: EventTargetUpdated when per-quarter targets become configurable
// via the frontend. Payload would carry the quarter label + new target.
