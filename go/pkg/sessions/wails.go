// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the sessions package — wraps the free
// Create / Append / Read / List functions so Wails generates a TS
// binding at frontend-ng/bindings/dappco.re/lthn/desktop/pkg/sessions/.
// Bound by application.NewService(sessions.NewWailsService(core)) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (CLI verbs, Action bus consumers).

package sessions

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/paths"
)

// WailsService is the WebView-facing handle on the sessions store.
// Carries the *core.Core reference so methods can dispatch through
// the same store / stream / process services the package functions
// use without re-resolving the Core per call.
//
// Mantis #1418 (Cerberus M2 2026-05-16): per-session-id mutex map.
// Every read-modify-write path above the store (manifest mutations
// in Append, RemoveLast, ClearMessages, Rename, SetSystemPrompt,
// SetTags, Duplicate, Delete) acquires the id-keyed lock first.
// Concurrent calls on the SAME id serialise; calls on DIFFERENT ids
// don't contend. The lock lives at the Wails layer only — CLI verbs
// and Action-bus consumers managing their own concurrency invoke
// the package functions directly and won't double-lock.
type WailsService struct {
	core    *core.Core
	idLocks core.SyncMap
}

// NewWailsService binds the WebView surface to c. c must be the
// same Core the sessions package was registered against.
//
// Usage example:
//
//	application.NewService(sessions.NewWailsService(coreInstance))
func NewWailsService(c *core.Core) *WailsService { return &WailsService{core: c} }

// lockSession acquires the per-id mutex and returns an unlock
// closure suitable for `defer`. LoadOrStore-pattern ensures unrelated
// ids never share a lock; the empty-id case is treated as a global
// lock (defensive — empty ids reach this only via fail-fast caller
// guards but we don't want to silently no-op).
//
// Usage example:
//
//	defer s.lockSession(id)()
//	// RMW the manifest safely
func (s *WailsService) lockSession(id string) func() {
	if s == nil {
		return func() {}
	}
	val, _ := s.idLocks.LoadOrStore(id, &core.Mutex{})
	mu, ok := val.(*core.Mutex)
	if !ok {
		return func() {}
	}
	mu.Lock()
	return mu.Unlock
}

func (s *WailsService) ServiceName() string { return "Sessions" }
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Create starts a new session with the given title and returns its
// id. The id is a content-derived string suitable for filesystem
// paths.
func (s *WailsService) Create(title string) core.Result {
	r := Create(s.core, title)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Create", "create session failed", r.Value.(error)))
	}
	id, _ := r.Value.(string)
	return core.Ok(id)
}

// Append adds a single message to an existing session's history.
func (s *WailsService) Append(id, role, content string) core.Result {
	defer s.lockSession(id)()
	r := Append(s.core, id, role, content)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Append", "append session message failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Read returns every message in the session in chronological order.
func (s *WailsService) Read(id string) core.Result {
	r := Read(s.core, id)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Read", "read session failed", r.Value.(error)))
	}
	msgs, _ := r.Value.([]inference.Message)
	return core.Ok(msgs)
}

// Export writes a session's transcript as markdown to the given path.
// The chat-window's /export slash command calls this after the user
// picks a path via Dialogs.SaveFile.
//
// Mantis #1420 (Cerberus M4 2026-05-17): the WailsService boundary
// validates that path is absolute, NUL-free, path-clean, and resolves
// under ~/Lethean/. UI convention previously kept the SaveFile picker
// pointed at the workspace tree; an attacker-controlled WebView frame
// (extension, embedded iframe, future plugin tier) could bypass that
// convention and call this method with an arbitrary host-side path,
// turning Export into a write-anywhere primitive. The package-level
// sessions.Export stays caller-trust to keep CLI verbs + tests free
// to write under t.TempDir(); the lockdown lives on the boundary the
// WebView actually reaches.
//
// Usage example (TS):
//
//	import { Export } from "@desktop/sessions/wailsservice";
//	await Export(activeId, "/Users/snider/Lethean/exports/chat.md");
func (s *WailsService) Export(id, path string) core.Result {
	if err := validateExportPath(path); err != nil {
		return core.Fail(err)
	}
	r := Export(s.core, id, path)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Export", "export session failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// RemoveLast pops the trailing message from a session's history.
// Returns the popped Message so the caller can act on the role
// (typically asserting it was "assistant" before regenerating).
//
// Usage example (TS):
//
//	import { RemoveLast } from "@desktop/sessions/wailsservice";
//	const popped = await RemoveLast(activeId);
func (s *WailsService) RemoveLast(id string) core.Result {
	defer s.lockSession(id)()
	r := RemoveLast(s.core, id)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.RemoveLast", "remove last message failed", r.Value.(error)))
	}
	msg, _ := r.Value.(inference.Message)
	return core.Ok(msg)
}

// ClearMessages wipes a session's message log + zeroes its manifest
// message count. Used by the chat-window's /clear slash command.
//
// Usage example (TS):
//
//	import { ClearMessages } from "@desktop/sessions/wailsservice";
//	await ClearMessages(activeId);
func (s *WailsService) ClearMessages(id string) core.Result {
	defer s.lockSession(id)()
	r := ClearMessages(s.core, id)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.ClearMessages", "clear messages failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Rename updates a session's manifest title. Used by the chat-window's
// inline-edit affordance to give a conversation a meaningful name.
//
// Usage example (TS):
//
//	import { Rename } from "@desktop/sessions/wailsservice";
//	await Rename(id, "Draft: launch announcement");
func (s *WailsService) Rename(id, title string) core.Result {
	defer s.lockSession(id)()
	r := Rename(s.core, id, title)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Rename", "rename session failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// SetSystemPrompt persists the session's per-conversation runner
// steering. An empty string clears it. Used by the chat-window's
// right-rail "Instructions" field so users can shape one session's
// persona without affecting others.
//
// Usage example (TS):
//
//	import { SetSystemPrompt } from "@desktop/sessions/wailsservice";
//	await SetSystemPrompt(activeId, "Answer in haiku.");
func (s *WailsService) SetSystemPrompt(id, prompt string) core.Result {
	defer s.lockSession(id)()
	r := SetSystemPrompt(s.core, id, prompt)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.SetSystemPrompt", "set system prompt failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// ExportAll backs up every session in the catalogue to dir as
// markdown files. Returns the count of files written. Used by the
// chat-window's /export-all slash command.
//
// Mantis #1420 — same boundary-layer path lockdown as Export above.
// dir must resolve under ~/Lethean/; an arbitrary host-side dir from
// an attacker-controlled WebView caller would otherwise let ExportAll
// scatter session markdown across the filesystem.
//
// Usage example (TS):
//
//	import { ExportAll } from "@desktop/sessions/wailsservice";
//	const n = await ExportAll("/Users/snider/Lethean/exports/backup");
func (s *WailsService) ExportAll(dir string) core.Result {
	if err := validateExportPath(dir); err != nil {
		return core.Fail(err)
	}
	r := ExportAll(s.core, dir)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.ExportAll", "export all failed", r.Value.(error)))
	}
	count, _ := r.Value.(int)
	return core.Ok(count)
}

// SetTags persists the tag set on a session. Tags are trimmed,
// lowercased, and de-duplicated server-side — clients can send the
// raw user input verbatim.
//
// Usage example (TS):
//
//	import { SetTags } from "@desktop/sessions/wailsservice";
//	await SetTags(activeId, ["work", "ops"]);
func (s *WailsService) SetTags(id string, tags []string) core.Result {
	defer s.lockSession(id)()
	r := SetTags(s.core, id, tags)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.SetTags", "set tags failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Duplicate forks a session — clones manifest + messages into a new
// id with " (copy)" suffixed onto the title. Returns the new id.
//
// Usage example (TS):
//
//	import { Duplicate } from "@desktop/sessions/wailsservice";
//	const newId = await Duplicate(activeConversationId);
func (s *WailsService) Duplicate(id string) core.Result {
	defer s.lockSession(id)()
	r := Duplicate(s.core, id)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Duplicate", "duplicate session failed", r.Value.(error)))
	}
	newID, _ := r.Value.(string)
	return core.Ok(newID)
}

// Delete removes a session — both its message log and its manifest
// entry. Idempotent: deleting an unknown id succeeds. Used by the
// chat-window's per-conversation delete affordance.
//
// Usage example (TS):
//
//	import { Delete } from "@desktop/sessions/wailsservice";
//	await Delete(activeConversationId);
func (s *WailsService) Delete(id string) core.Result {
	defer s.lockSession(id)()
	r := Delete(s.core, id)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Delete", "delete session failed", r.Value.(error)))
	}
	// Cerberus pass-2 followup — lock-map cleanup. Without this the
	// idLocks map grew unboundedly as sessions came and went; benign
	// today but a real footprint when plugin tier (post #1421) spawns
	// many sessions. Safe because the deferred unlock at the top of
	// this function has already run by the time Delete returns —
	// removing the entry now means any in-flight reader of the same
	// id (a concurrent Read/List/Search) will see the canonical
	// "session not found" from the underlying store rather than
	// blocking on a stale mutex.
	s.idLocks.Delete(id)
	return core.Ok(nil)
}

// Search returns sessions matching the query in title OR message
// content. Empty query is equivalent to List(). Drives the chat
// rail's "find a past conversation by what we discussed" path.
//
// Returns a SearchResult (Hits + Truncated). Truncated:true means
// MaxSearchScan was hit before every session was examined —
// frontend should surface "narrow your query" guidance rather than
// presenting the partial list as exhaustive (Mantis #1538).
//
// Usage example (TS):
//
//	import { Search } from "@desktop/sessions/wailsservice";
//	const sr = await Search("regex pitfalls");
//	// sr.hits: SessionInfo[], sr.truncated: boolean
func (s *WailsService) Search(query string) core.Result {
	r := Search(s.core, query)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.Search", "search sessions failed", r.Value.(error)))
	}
	sr, _ := r.Value.(SearchResult)
	return core.Ok(sr)
}

// List returns the session catalogue — one entry per stored
// session, with id / title / timestamps / message count.
func (s *WailsService) List() core.Result {
	r := List(s.core)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.List", "list sessions failed", r.Value.(error)))
	}
	infos, _ := r.Value.([]SessionInfo)
	return core.Ok(infos)
}

// RecentGenerations returns the most-recent assistant turns across
// every session, flattened newest-first. Backs the Activity →
// History tab so the user sees real "what did the model say recently
// and to what prompt" rows instead of a fixture.
//
//	import { RecentGenerations } from "@desktop/sessions/wailsservice";
//	const gens = await RecentGenerations(20);
func (s *WailsService) RecentGenerations(limit int) core.Result {
	r := RecentGenerations(s.core, limit)
	if !r.OK {
		return core.Fail(core.E("sessions.WailsService.RecentGenerations", "recent generations failed", r.Value.(error)))
	}
	gens, _ := r.Value.([]Generation)
	return core.Ok(gens)
}

// validateExportPath gates the WebView-facing Export / ExportAll
// surface against arbitrary-write attacks (Mantis #1420, Cerberus M4
// 2026-05-17). path must be absolute, NUL-free, path-clean, and
// resolve under the user-visible workspace root at ~/Lethean/.
// Resolution of the root is deferred to call time so a test that
// re-points HOME via t.Setenv("HOME", ...) sees the test-controlled
// tree; production callers see the real workspace.
//
// Failure mode unifies on the error code "sessions.export.path_invalid"
// so the frontend gate (lit toast) can render a friendly message
// regardless of which underlying clause tripped — empty / relative /
// NUL / unclean / escape-from-root all collapse to one code.
//
// Usage example:
//
//	if err := validateExportPath(path); err != nil {
//	    return core.Fail(err)
//	}
func validateExportPath(path string) error {
	rootR := paths.Root()
	if !rootR.OK {
		return core.E("sessions.export.path_invalid",
			"workspace root unavailable", rootR.Value.(error))
	}
	root, _ := rootR.Value.(string)
	if err := paths.IsValidAbsoluteUnderRoot(root, path); err != nil {
		return core.E("sessions.export.path_invalid",
			"export path must be absolute and resolve under the Lethean workspace", err)
	}
	return nil
}
