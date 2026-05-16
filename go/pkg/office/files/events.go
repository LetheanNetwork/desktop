// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants for the files surface.
// v1 is read-only — no events are emitted. Constants are reserved
// for v2 when FSEvents / inotify watchers push change notifications.

package files

// EventLocationChanged fires when a watched location's contents change (v2).
// Payload is the location Name that changed.
const EventLocationChanged = "files.location.changed"

// EventRecentUpdated fires when the recent-files list changes (v2).
// Payload is the updated []RecentRow.
const EventRecentUpdated = "files.recent.updated"
