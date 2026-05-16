// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants for the mail surface.
// v1 is read-only — no events are emitted. Constants are reserved
// for v2 when IMAP push integration and read-state sync are added.

package mail

// EventThreadReceived fires when a new thread arrives via IMAP push (v2).
// Payload is the MailThread of the received thread.
const EventThreadReceived = "mail.thread.received"

// EventThreadRead fires when a thread's read state changes (v2).
// Payload is the thread ID and the new unread state.
const EventThreadRead = "mail.thread.read"
