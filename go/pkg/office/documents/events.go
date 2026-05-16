// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants for the documents surface.
// v1 is read-only — no events are emitted. Constants are reserved
// for v2 when an in-app editor adds Create / Save / Delete operations.

package documents

// EventCreated fires when a new document is created via the in-app
// editor (v2). Payload is the DocRow of the created document.
const EventCreated = "documents.created"

// EventUpdated fires when an existing document is saved (v2). Payload
// is the DocRow of the updated document.
const EventUpdated = "documents.updated"
