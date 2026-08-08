// SPDX-Licence-Identifier: EUPL-1.2

// Package sessions persists chat sessions through the registered
// dappco.re/go/store service. Each session is a JSON-encoded record
// under group "sessions" (per-session message log) plus a manifest
// entry under group "sessions:manifest" (id → title + timestamps).
//
// The package is a thin typed accessor — it does no protocol work
// of its own; it composes go-store actions through the Core bus.
//
// Schema:
//
//	store group "sessions"          key = session id      value = JSON([]Message)
//	store group "sessions:manifest" key = session id      value = JSON(SessionInfo)
//
// Usage example:
//
//	id := sessions.Create(c, "first chat")
//	sessions.Append(c, id, "user", "hello")
//	sessions.Append(c, id, "assistant", "hi")
//	msgs := sessions.Read(c, id)
package sessions

import (
	"sort"

	core "dappco.re/go"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/audit"
)

// SessionInfo is the manifest entry for a session — what `lthn
// sessions list` prints. Snippet is a truncated preview of the
// session's most recent message content so the rail can render a
// glance-level preview without an N+1 message read per row. Empty
// for sessions that have no messages yet; cleared by ClearMessages
// + recomputed by Append / RemoveLast.
//
// SystemPrompt is per-conversation runner steering — empty for a
// generic chat, populated by the right-rail "Instructions" field so
// users can shape one session's persona without affecting others.
// Send-time the chat-window prepends {role:"system", content:
// prompt} to the history before the runner call. No truncation cap
// on the storage side — prompts can be multi-paragraph.
type SessionInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	Messages     int    `json:"messages"`
	Snippet      string `json:"snippet,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	// Tags is the free-text label set the user has attached to this
	// session. Lowercased + de-duplicated on the way in (SetTags
	// enforces both). The rail renders chips beneath the title and a
	// future filter wires off the same field.
	Tags []string `json:"tags,omitempty"`
}

// SnippetMax caps the manifest snippet length so a multi-KB
// message doesn't bloat the catalogue. 96 RUNES is comfortable for a
// rail row (~10 words on average) and leaves room for the title +
// model badge underneath. Rune count, not byte length — keeps the
// visible width predictable across ASCII + emoji + CJK alike.
const SnippetMax = 96

// snippetInputCap caps the bytes of input renderSnippet inspects.
// Cerberus L1 2026-05-16: an adversarial 1 MiB message of single
// spaces would cause `for Contains "  "` to run quadratically.
// 64 KiB is generous for chat content; past that we slice down to
// SnippetMax runes anyway, so the extra bytes weren't going to land
// in the manifest.
const snippetInputCap = 64 * 1024

// renderSnippet collapses a multi-line message body into a single
// inline preview, trims surrounding whitespace, and caps the length
// at SnippetMax runes with a trailing ellipsis when truncated. Empty
// input yields an empty snippet so consumers can branch on a falsy
// check. Adversarial-input safe: caps content before the
// space-collapse loop (Cerberus L1 fix 2026-05-16).
func renderSnippet(content string) string {
	if content == "" {
		return ""
	}
	// Defensive cap before the O(n²) space-collapse — see snippetInputCap.
	if len(content) > snippetInputCap {
		content = content[:snippetInputCap]
	}
	// Collapse every newline / carriage return / tab to a single space
	// so a code block doesn't blow out the rail row vertically.
	flat := core.Replace(content, "\r", " ")
	flat = core.Replace(flat, "\n", " ")
	flat = core.Replace(flat, "\t", " ")
	// Collapse runs of consecutive spaces left by the replacements.
	for core.Contains(flat, "  ") {
		flat = core.Replace(flat, "  ", " ")
	}
	flat = core.Trim(flat)
	if flat == "" {
		return ""
	}
	if core.RuneCount(flat) <= SnippetMax {
		return flat
	}
	// Truncate at SnippetMax-1 runes + append "…" so the total stays
	// at exactly SnippetMax runes. Rune-aware slicing prevents
	// splitting a multi-byte codepoint mid-sequence.
	runes := []rune(flat)
	return string(runes[:SnippetMax-1]) + "…"
}

// SystemPromptMax caps the per-session system prompt to 16 KiB
// (Cerberus M1 2026-05-16). Long enough for a multi-paragraph
// persona; short enough that an adversary can't bloat List() reads.
const SystemPromptMax = 16 * 1024

// MaxTags caps how many labels a session can carry (Cerberus M1).
// 32 is more than any human curates by hand; rejecting beyond that
// blocks plugins from posting `make([]string, 1e7)`.
const MaxTags = 32

// MaxTagLength caps each tag at 64 RUNES (Cerberus M1). Rail chips
// can't usefully render longer than that anyway.
const MaxTagLength = 64

// SearchQueryMin is the shortest query Sessions.Search will accept.
// Cerberus H3 2026-05-16: blank/single-char queries trivially scan
// every message body across every session — keystroke-rate DoS.
// 2 is the same threshold every modern search field uses.
const SearchQueryMin = 2

// MaxSearchScan caps how many session manifest entries a single
// Search call will iterate before returning partial results.
// Cerberus H9-verify F5 / Mantis #1538: with no upper bound a user
// holding ~50k sessions would block the rail's keystroke handler
// for whole seconds per query; the cap bounds worst-case latency
// at the cost of completeness. Hits beyond the cap are simply not
// surfaced and the result is flagged Truncated so the UI can offer
// "narrow your query" guidance. 5000 is comfortably above any
// observed user catalogue (top observed ~600) and still finishes
// well inside the 60ms keystroke budget on cold storage.
const MaxSearchScan = 5000

// maxSearchScanCurrent is the live cap honoured by Search. Tests
// shrink it via SetMaxSearchScanForTest so they can prove the cap
// without populating thousands of sessions on every run. Production
// code must never touch this var — use the MaxSearchScan constant.
var maxSearchScanCurrent = MaxSearchScan

// SearchResult is the payload Search returns in core.Result.Value.
// Hits is the matching SessionInfo slice (possibly empty); Truncated
// is true when MaxSearchScan was hit before every session was
// examined — telling the UI to surface "narrow your query" guidance
// rather than mis-presenting an incomplete list as exhaustive.
//
// Usage example:
//
//	r := sessions.Search(c, "regex")
//	if r.OK {
//		sr := r.Value.(sessions.SearchResult)
//		_ = sr.Hits       // []SessionInfo
//		_ = sr.Truncated  // bool
//	}
type SearchResult struct {
	Hits      []SessionInfo `json:"hits"`
	Truncated bool          `json:"truncated"`
}

const (
	groupMessages  = "sessions"
	groupManifest  = "sessions:manifest"
	coreNilMessage = "core is nil"
)

// Create starts a new session with the given title. Returns the new
// session id (random hex) on success.
//
// Usage example:
//
//	r := sessions.Create(c, "first chat")
//	if r.OK { id := r.Value.(string); _ = id }
func Create(c *core.Core, title string) core.Result {
	if c == nil {
		fail := core.Fail(core.E("sessions.Create", coreNilMessage, nil))
		emitCreateFailed(title, fail)
		return fail
	}
	emitCreateRequested(title)
	idResult := core.RandomString(16)
	if !idResult.OK {
		emitCreateFailed(title, idResult)
		return idResult
	}
	id := idResult.Value.(string)
	now := core.UnixNow()
	info := SessionInfo{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  0,
	}
	if r := writeManifest(c, info); !r.OK {
		emitCreateFailed(title, r)
		return r
	}
	if r := writeMessages(c, id, []inference.Message{}); !r.OK {
		emitCreateFailed(title, r)
		return r
	}
	emitCreateSucceeded(id, title)
	return core.Ok(id)
}

// Append adds a message to the session, refreshes UpdatedAt and the
// message count in the manifest.
//
// Usage example:
//
//	sessions.Append(c, id, "user", "ping")
func Append(c *core.Core, id, role, content string) core.Result {
	if c == nil {
		fail := core.Fail(core.E("sessions.Append", coreNilMessage, nil))
		emitAppendFailed(id, role, content, fail)
		return fail
	}
	emitAppendRequested(id, role, content)
	msgsR := readMessages(c, id)
	if !msgsR.OK {
		emitAppendFailed(id, role, content, msgsR)
		return msgsR
	}
	msgs := msgsR.Value.([]inference.Message)
	msgs = append(msgs, inference.Message{Role: role, Content: content})
	if r := writeMessages(c, id, msgs); !r.OK {
		emitAppendFailed(id, role, content, r)
		return r
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		emitAppendFailed(id, role, content, infoR)
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	info.UpdatedAt = core.UnixNow()
	info.Messages = len(msgs)
	// Refresh snippet from the just-appended message so the rail
	// preview reflects the latest content.
	info.Snippet = renderSnippet(content)
	r := writeManifest(c, info)
	if !r.OK {
		emitAppendFailed(id, role, content, r)
		return r
	}
	emitAppendSucceeded(id, role, content)
	return r
}

// Read returns the full message log for the session.
//
// Usage example:
//
//	r := sessions.Read(c, id)
//	if r.OK { msgs := r.Value.([]inference.Message); _ = msgs }
func Read(c *core.Core, id string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Read", coreNilMessage, nil))
	}
	msgsR := readMessages(c, id)
	if !msgsR.OK {
		return msgsR
	}
	return msgsR
}

// RemoveLast pops the trailing message from a session's history and
// returns it. Used by the chat-window's regenerate affordance: the
// click drops the last assistant reply, then dispatches the trimmed
// history through the runner so a fresh reply replaces it.
//
// Returns OK with Value = inference.Message (the popped one) on
// success. Refuses an empty session — caller should check before
// calling, but the guard means a programmatic mistake fails fast
// instead of corrupting the manifest counter.
//
// Manifest is updated to reflect the new message count + bumped
// UpdatedAt timestamp, matching Append's contract.
//
// Usage example:
//
//	r := sessions.RemoveLast(c, id)
//	if r.OK { popped := r.Value.(inference.Message); _ = popped.Role }
func RemoveLast(c *core.Core, id string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.RemoveLast", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.RemoveLast", "empty id", nil))
	}
	msgsR := readMessages(c, id)
	if !msgsR.OK {
		return msgsR
	}
	msgs := msgsR.Value.([]inference.Message)
	if len(msgs) == 0 {
		return core.Fail(core.E("sessions.RemoveLast", "session is empty", nil))
	}
	popped := msgs[len(msgs)-1]
	trimmed := msgs[:len(msgs)-1]
	if r := writeMessages(c, id, trimmed); !r.OK {
		return r
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	info.UpdatedAt = core.UnixNow()
	info.Messages = len(trimmed)
	// Snippet follows the new tail — empty when nothing's left,
	// otherwise the previous message's content.
	if len(trimmed) == 0 {
		info.Snippet = ""
	} else {
		info.Snippet = renderSnippet(trimmed[len(trimmed)-1].Content)
	}
	if r := writeManifest(c, info); !r.OK {
		return r
	}
	return core.Ok(popped)
}

// ClearMessages wipes a session's message log while preserving the
// session row itself — manifest entry stays (title, created_at)
// with Messages reset to 0 and UpdatedAt bumped to now. Used by the
// chat-window's /clear slash command so users can keep a thread's
// title + identity while wiping the transcript.
//
// Idempotent: clearing an already-empty session succeeds quietly.
//
// Usage example:
//
//	r := sessions.ClearMessages(c, id)
//	if !r.OK { core.Warn("clear failed", "err", r.Error()) }
func ClearMessages(c *core.Core, id string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.ClearMessages", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.ClearMessages", "empty id", nil))
	}
	// Confirm the session exists — clearing an unknown id should
	// fail rather than silently create-then-clear the row.
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	if r := writeMessages(c, id, []inference.Message{}); !r.OK {
		return r
	}
	info := infoR.Value.(SessionInfo)
	info.Messages = 0
	info.UpdatedAt = core.UnixNow()
	info.Snippet = ""
	return writeManifest(c, info)
}

// Rename updates a session's manifest title + bumps UpdatedAt. The
// message log is untouched. Refuses an empty title — the rail's
// design assumes every session has a visible name, and the only
// caller is the inline-edit affordance which has its own non-empty
// validation; defending here keeps the contract crisp.
//
// Usage example:
//
//	r := sessions.Rename(c, id, "Drafting the launch announcement")
//	if !r.OK { core.Warn("rename failed", "err", r.Error()) }
func Rename(c *core.Core, id, title string) core.Result {
	if c == nil {
		fail := core.Fail(core.E("sessions.Rename", coreNilMessage, nil))
		emitRenameFailed(id, title, fail)
		return fail
	}
	if id == "" {
		fail := core.Fail(core.E("sessions.Rename", "empty id", nil))
		emitRenameFailed(id, title, fail)
		return fail
	}
	if title == "" {
		fail := core.Fail(core.E("sessions.Rename", "empty title", nil))
		emitRenameFailed(id, title, fail)
		return fail
	}
	emitRenameRequested(id)
	infoR := readManifest(c, id)
	if !infoR.OK {
		emitRenameFailed(id, title, infoR)
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	info.Title = title
	info.UpdatedAt = core.UnixNow()
	r := writeManifest(c, info)
	if !r.OK {
		emitRenameFailed(id, title, r)
		return r
	}
	emitRenameSucceeded(id, title)
	return r
}

// SetSystemPrompt persists the per-conversation runner steering. An
// empty string clears the prompt (manifest's omitempty drops the key
// from JSON, paying no storage for the default case). UpdatedAt
// bumps so the rail-bucket sort reflects the recency of the edit.
//
// Failure modes:
//   - nil core / empty id → fail
//   - session not found → fail (manifest read propagates)
//
// Usage example:
//
//	r := sessions.SetSystemPrompt(c, id, "You are a Go expert.")
//	if !r.OK { core.Warn("set prompt failed", "err", r.Error()) }
func SetSystemPrompt(c *core.Core, id, prompt string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.SetSystemPrompt", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.SetSystemPrompt", "empty id", nil))
	}
	// Cerberus M1 2026-05-16: cap unbounded input at the boundary
	// rather than letting it bloat the manifest. 16 KiB is generous
	// for a real prompt; past that = abuse.
	if len(prompt) > SystemPromptMax {
		return core.Fail(core.NewError("sessions.SetSystemPrompt: prompt exceeds 16 KiB"))
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	info.SystemPrompt = prompt
	info.UpdatedAt = core.UnixNow()
	return writeManifest(c, info)
}

// SetTags writes the tag set onto a session's manifest. Input is
// normalised: each tag trimmed, lowercased, blanks dropped,
// duplicates removed (first-occurrence wins). nil + []string{} both
// clear the field. UpdatedAt bumps so the rail reorder reflects the
// edit.
//
// Failure modes:
//   - nil core / empty id → fail
//   - session not found → fail (manifest read propagates)
//
// Usage example:
//
//	r := sessions.SetTags(c, id, []string{"work", "ops"})
//	if !r.OK { core.Warn("set tags failed", "err", r.Error()) }
func SetTags(c *core.Core, id string, tags []string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.SetTags", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.SetTags", "empty id", nil))
	}
	// Cerberus M1 2026-05-16: bound the input set BEFORE
	// normaliseTags allocates O(N) on it.
	if len(tags) > MaxTags {
		return core.Fail(core.NewError("sessions.SetTags: too many tags (max 32)"))
	}
	for _, t := range tags {
		if core.RuneCount(t) > MaxTagLength {
			return core.Fail(core.NewError("sessions.SetTags: tag exceeds 64 runes"))
		}
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	info.Tags = normaliseTags(tags)
	info.UpdatedAt = core.UnixNow()
	return writeManifest(c, info)
}

// normaliseTags trims + lowercases each tag, drops blanks, removes
// duplicates while preserving the first occurrence's order. Pure —
// drives SetTags but exported via package-level so tests can assert
// without hitting the store.
func normaliseTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		clean := core.Lower(core.Trim(t))
		if clean == "" {
			continue
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Duplicate forks an existing session into a new one. The new
// session gets a fresh id + bumped timestamps, a title suffixed with
// " (copy)" so the rail visibly distinguishes the fork, and an
// identical copy of every message. Used by the chat-window's
// "Duplicate" context-menu item so a user can branch a conversation
// without losing the original line of thought.
//
// Failure modes:
//   - nil core / empty id → fail
//   - source manifest unreadable → fail (propagates manifest error)
//   - source messages unreadable → fail (propagates read error)
//   - new id collides on disk → fail (writeManifest surfaces)
//
// Returns OK with Value = the new session id (string).
//
// Usage example:
//
//	r := sessions.Duplicate(c, "abc")
//	if r.OK { newId := r.Value.(string); _ = newId }
func Duplicate(c *core.Core, id string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Duplicate", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.Duplicate", "empty id", nil))
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	src := infoR.Value.(SessionInfo)
	msgsR := readMessages(c, id)
	if !msgsR.OK {
		return msgsR
	}
	msgs := msgsR.Value.([]inference.Message)

	idResult := core.RandomString(16)
	if !idResult.OK {
		return idResult
	}
	newID := idResult.Value.(string)
	now := core.UnixNow()
	// Copy tag slice so future SetTags on the copy doesn't share
	// backing storage with the source.
	var tagsCopy []string
	if len(src.Tags) > 0 {
		tagsCopy = make([]string, len(src.Tags))
		copy(tagsCopy, src.Tags)
	}
	dst := SessionInfo{
		ID:           newID,
		Title:        src.Title + " (copy)",
		CreatedAt:    now,
		UpdatedAt:    now,
		Messages:     len(msgs),
		Snippet:      src.Snippet,
		SystemPrompt: src.SystemPrompt,
		Tags:         tagsCopy,
	}
	if r := writeManifest(c, dst); !r.OK {
		return r
	}
	// Copy the message slice rather than reusing the pointer so a
	// future Append on the copy doesn't share backing storage with the
	// source (defensive — writeMessages serialises to disk anyway).
	out := make([]inference.Message, len(msgs))
	copy(out, msgs)
	if r := writeMessages(c, newID, out); !r.OK {
		return r
	}
	return core.Ok(newID)
}

// Export renders a session's transcript as markdown and writes it
// to the given filesystem path. The output is plain text with
// `You:` / `Assistant:` speaker prefixes followed by the message
// body, separated by blank lines — same shape as the in-app `/copy`
// transcript so the file is paste-ready.
//
// path is taken verbatim from the caller; safety + permission
// checks are the OS file dialog's responsibility (the chat-window
// only calls this after Dialogs.SaveFile returns a chosen path).
//
// Failure modes:
//   - nil core / empty id / empty path → fail
//   - session not found → fail (manifest read propagates)
//   - parent directory missing or unwritable → fail (WriteFile)
//
// Usage example:
//
//	r := sessions.Export(c, id, "/Users/snider/Desktop/chat.md")
//	if !r.OK { core.Warn("export failed", "err", r.Error()) }
func Export(c *core.Core, id, path string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Export", coreNilMessage, nil))
	}
	if id == "" {
		return core.Fail(core.E("sessions.Export", "empty id", nil))
	}
	if path == "" {
		return core.Fail(core.E("sessions.Export", "empty path", nil))
	}
	infoR := readManifest(c, id)
	if !infoR.OK {
		return infoR
	}
	info := infoR.Value.(SessionInfo)
	msgsR := readMessages(c, id)
	if !msgsR.OK {
		return msgsR
	}
	msgs := msgsR.Value.([]inference.Message)
	body := renderMarkdown(info, msgs)
	if w := core.WriteFile(path, []byte(body), 0o644); !w.OK {
		return core.Fail(core.E("sessions.Export", "write failed", w.Value.(error)))
	}
	return core.Ok(path)
}

// ExportAll writes every session in the catalogue to dir as markdown
// files named "{slugged-title}-{id}.md". The slug strips characters
// that won't survive a filesystem (spaces → -, drops anything not in
// [a-z0-9-_]). Empty / unwritten sessions still get a file (covers
// "I want everything backed up" rather than "I want non-empty
// transcripts only"). Returns the count of files written.
//
// Failure modes:
//   - nil core / empty dir → fail
//   - List() failure → propagates
//   - per-session Export failure → fail with the FIRST error encountered
//     (no partial-success silently)
//
// Usage example:
//
//	r := sessions.ExportAll(c, "/Users/snider/Documents/lthn-export")
//	if r.OK { count := r.Value.(int); _ = count }
func ExportAll(c *core.Core, dir string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.ExportAll", coreNilMessage, nil))
	}
	if dir == "" {
		return core.Fail(core.E("sessions.ExportAll", "empty dir", nil))
	}
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return core.Fail(core.E("sessions.ExportAll", "mkdir failed", r.Value.(error)))
	}
	listR := List(c)
	if !listR.OK {
		return listR
	}
	infos, _ := listR.Value.([]SessionInfo)
	count := 0
	for _, info := range infos {
		name := slugFilename(info.Title) + "-" + info.ID + ".md"
		path := core.PathJoin(dir, name)
		r := Export(c, info.ID, path)
		if !r.OK {
			return core.Fail(core.E("sessions.ExportAll", "export "+info.ID+" failed", r.Value.(error)))
		}
		count++
	}
	return core.Ok(count)
}

// SlugMax caps the rune length of an ExportAll filename slug
// (Cerberus H2 2026-05-16). 64 runes is comfortable for a
// filesystem entry; the appended "-{16-char id}.md" stays well
// under the 255-byte UNIX filename limit.
const SlugMax = 64

// SlugInputCap caps the bytes inspected by slugFilename so an
// adversarial multi-MiB title doesn't trigger O(N) allocation per
// session in ExportAll.
const SlugInputCap = 4 * 1024

// slugFilename normalises a session title into a filesystem-safe
// slug. Lowercases, replaces non-[a-z0-9-_] runs with a single dash,
// trims leading/trailing dashes. Empty titles become "untitled".
// Rune-aware (Cerberus L2 2026-05-16) so multi-byte codepoints don't
// get byte-mangled. Capped at SlugMax runes (Cerberus H2). Pure —
// exported via the ExportAll path; tests can drive directly.
func slugFilename(title string) string {
	if len(title) > SlugInputCap {
		title = title[:SlugInputCap]
	}
	s := core.Lower(core.Trim(title))
	if s == "" {
		return "untitled"
	}
	out := make([]rune, 0, SlugMax)
	prevDash := false
	for _, ch := range s {
		isAlnum := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_'
		if isAlnum {
			out = append(out, ch)
			prevDash = false
		} else if !prevDash {
			out = append(out, '-')
			prevDash = true
		}
		if len(out) >= SlugMax {
			break
		}
	}
	// Trim leading + trailing dashes.
	start, end := 0, len(out)
	for start < end && out[start] == '-' {
		start++
	}
	for end > start && out[end-1] == '-' {
		end--
	}
	if start == end {
		return "untitled"
	}
	return string(out[start:end])
}

// renderMarkdown turns a session manifest + message log into the
// markdown body the export writes. Format mirrors the frontend's
// buildTranscript helper so file output + clipboard output stay in
// sync. Pure function — exported via the Export path.
func renderMarkdown(info SessionInfo, msgs []inference.Message) string {
	var b core.Builder
	b.WriteString("# ")
	if info.Title != "" {
		b.WriteString(info.Title)
	} else {
		b.WriteString("Untitled conversation")
	}
	b.WriteString("\n\n")
	for i, m := range msgs {
		role := "Assistant"
		if m.Role == "user" {
			role = "You"
		}
		b.WriteString(role)
		b.WriteString(":\n")
		b.WriteString(m.Content)
		if i < len(msgs)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// Delete removes a session from both groups — its message log AND
// the manifest entry. Idempotent on a non-existent id: the underlying
// store.delete succeeds without error when the key is absent, so
// repeated calls don't fail.
//
// Returns OK with nil value on success. The order matters only for
// crash-recovery — manifest goes LAST so a mid-deletion crash leaves
// a manifest pointer to an empty message log, which List() still
// surfaces. The reverse order would orphan messages with no manifest,
// invisible to the UI but eating disk.
//
// Usage example:
//
//	r := sessions.Delete(c, id)
//	if !r.OK { core.Warn("delete failed", "err", r.Error()) }
func Delete(c *core.Core, id string) core.Result {
	if c == nil {
		fail := core.Fail(core.E("sessions.Delete", coreNilMessage, nil))
		emitDeleteFailed(id, fail)
		return fail
	}
	if id == "" {
		fail := core.Fail(core.E("sessions.Delete", "empty id", nil))
		emitDeleteFailed(id, fail)
		return fail
	}
	emitDeleteRequested(id)
	if r := c.Action("store.delete").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupMessages},
		core.Option{Key: "key", Value: id},
	)); !r.OK {
		emitDeleteFailed(id, r)
		return r
	}
	r := c.Action("store.delete").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
		core.Option{Key: "key", Value: id},
	))
	if !r.OK {
		emitDeleteFailed(id, r)
		return r
	}
	emitDeleteSucceeded(id)
	return r
}

// Search returns every session whose title OR any message content
// matches the query, case-insensitive substring. Empty query returns
// the same shape as List(c) — every session, no filter applied — so
// callers can pipe one path regardless of whether the user typed
// anything.
//
// Each match is the existing SessionInfo manifest entry; no snippet
// extraction yet (the rail's row render reads its own snippet from
// message data on demand). Adds N+1 disk reads on the hot path —
// linear in session count, fine at the typical scale (dozens-to-
// hundreds of sessions); revisit when stores grow past that.
//
// Hardened (Cerberus H9-verify F5 / Mantis #1538): scans no more
// than MaxSearchScan sessions per call. When the cap is reached
// the partial hit slice is returned with SearchResult.Truncated
// set so the UI can surface "narrow your query" guidance.
//
// Usage example:
//
//	r := sessions.Search(c, "regex")
//	if r.OK {
//		sr := r.Value.(sessions.SearchResult)
//		_ = sr.Hits       // []SessionInfo
//		_ = sr.Truncated  // bool
//	}
func Search(c *core.Core, query string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Search", coreNilMessage, nil))
	}
	q := core.Trim(query)
	if q == "" {
		// Empty query is "list everything" — defer to List and wrap
		// the slice into SearchResult so callers consume one shape.
		listR := List(c)
		if !listR.OK {
			return listR
		}
		all, _ := listR.Value.([]SessionInfo)
		return core.Ok(SearchResult{Hits: all, Truncated: false})
	}
	// Cerberus H3 2026-05-16: refuse single-char queries — they
	// trigger an N+1 disk-walk over every message body of every
	// session per keystroke. UI debounce + min-length both close
	// this surface; the backend guard is the load-bearing one.
	if core.RuneCount(q) < SearchQueryMin {
		return core.Ok(SearchResult{Hits: []SessionInfo{}, Truncated: false})
	}
	qLow := core.Lower(q)

	listR := List(c)
	if !listR.OK {
		return listR
	}
	all, ok := listR.Value.([]SessionInfo)
	if !ok {
		return core.Ok(SearchResult{Hits: []SessionInfo{}, Truncated: false})
	}

	cap := maxSearchScanCurrent
	out := make([]SessionInfo, 0, len(all))
	scanned := 0
	truncated := false
	for _, info := range all {
		if scanned >= cap {
			// Hit the worst-case bound — stop scanning + flag the
			// result so the UI knows the answer is partial.
			truncated = true
			break
		}
		scanned++
		if core.Contains(core.Lower(info.Title), qLow) {
			out = append(out, info)
			continue
		}
		// Title miss — inspect message content. readMessages
		// failures are skipped silently so one corrupt session
		// doesn't blank the entire search result set.
		msgsR := readMessages(c, info.ID)
		if !msgsR.OK {
			continue
		}
		msgs, _ := msgsR.Value.([]inference.Message)
		for _, m := range msgs {
			if core.Contains(core.Lower(m.Content), qLow) {
				out = append(out, info)
				break
			}
		}
	}
	return core.Ok(SearchResult{Hits: out, Truncated: truncated})
}

// SetMaxSearchScanForTest overrides the live MaxSearchScan cap so a
// test can prove the truncation path without populating thousands of
// sessions. The returned restore func MUST be deferred so a failure
// path doesn't leak the test cap into the next test.
//
// Usage example:
//
//	defer sessions.SetMaxSearchScanForTest(50)()
//	// ... populate 60 sessions and assert Truncated:true
func SetMaxSearchScanForTest(n int) func() {
	prev := maxSearchScanCurrent
	maxSearchScanCurrent = n
	return func() { maxSearchScanCurrent = prev }
}

// List returns the manifest entries for every session. Order is
// store-dependent; callers wanting recency-sorted output sort by
// UpdatedAt themselves.
//
// Usage example:
//
//	r := sessions.List(c)
//	if r.OK { infos := r.Value.([]SessionInfo); _ = infos }
func List(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.List", coreNilMessage, nil))
	}
	r := c.Action("store.get_all").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
	))
	if !r.OK {
		return r
	}
	raw, ok := r.Value.(map[string]string)
	if !ok {
		return core.Ok([]SessionInfo{})
	}
	out := make([]SessionInfo, 0, len(raw))
	for _, encoded := range raw {
		// Sealed manifests round-trip through unsealPayload so a
		// locked-account List() still surfaces the entries whose
		// envelope decodes successfully under the current unlock-
		// state. Sealed-but-unreadable entries are skipped (audit
		// emit on iteration is a future arc — for now silent skip
		// matches the legacy JSON-parse-failure behaviour).
		encodedBytes := []byte(encoded)
		plaintext, sealed := unsealPayload(c, encodedBytes)
		if sealed {
			if plaintext == nil {
				continue
			}
			encodedBytes = plaintext
		}
		var info SessionInfo
		if ur := core.JSONUnmarshal(encodedBytes, &info); !ur.OK {
			continue
		}
		out = append(out, info)
	}
	return core.Ok(out)
}

// Generation is one assistant reply lifted out of a session, paired
// with the user prompt that preceded it. Used by the Activity →
// History tab to render a flat across-sessions list — "what model
// said what, when, to which question" — without forcing the UI to
// walk every session itself.
//
// Timestamp granularity is the session's UpdatedAt because the
// legacy on-disk format doesn't carry per-message timestamps; a
// session that received three replies in a burst surfaces three rows
// sharing the same ts. When pkg/chathistory becomes the storage
// layer this collapses to true per-turn time (it stores created_at
// per row).
//
// Tokens is a char/4 estimate of the assistant content — the
// existing message log has no per-turn token counter, so the field
// gives a "ballpark for display" rather than billing-grade truth.
// Real counts will land once the runner threads its token usage
// through to Append.
type Generation struct {
	SessionID    string `json:"session_id"`
	SessionTitle string `json:"session_title"`
	UpdatedAt    int64  `json:"updated_at"`
	Prompt       string `json:"prompt"`
	Reply        string `json:"reply"`
	Tokens       int    `json:"tokens"`
}

// RecentGenerations returns the most-recent assistant turns across
// every session, flattened + sorted newest-first. Each row pairs the
// assistant message with the immediately preceding user prompt so
// the History grid can render the full ask→reply story per row.
//
// Limit caps the rows returned (default 20 when limit ≤ 0). Sessions
// are walked newest-first so the cap typically only reads the top
// few session message logs, not all of them.
//
// Usage example:
//
//	r := sessions.RecentGenerations(c, 20)
//	if r.OK { gens := r.Value.([]sessions.Generation); _ = gens }
func RecentGenerations(c *core.Core, limit int) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.RecentGenerations", coreNilMessage, nil))
	}
	if limit <= 0 {
		limit = 20
	}
	listR := List(c)
	if !listR.OK {
		return listR
	}
	infos, _ := listR.Value.([]SessionInfo)
	sort.SliceStable(infos, func(i, j int) bool {
		return infos[i].UpdatedAt > infos[j].UpdatedAt
	})
	out := make([]Generation, 0, limit)
	for _, info := range infos {
		if len(out) >= limit {
			break
		}
		msgsR := readMessages(c, info.ID)
		if !msgsR.OK {
			continue
		}
		msgs, _ := msgsR.Value.([]inference.Message)
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role != "assistant" {
				continue
			}
			prompt := ""
			for j := i - 1; j >= 0; j-- {
				if msgs[j].Role == "user" {
					prompt = msgs[j].Content
					break
				}
			}
			out = append(out, Generation{
				SessionID:    info.ID,
				SessionTitle: info.Title,
				UpdatedAt:    info.UpdatedAt,
				Prompt:       prompt,
				Reply:        msgs[i].Content,
				Tokens:       core.RuneCount(msgs[i].Content) / 4,
			})
			if len(out) >= limit {
				break
			}
		}
	}
	return core.Ok(out)
}

// writeMessages serialises the message log to JSON and dispatches it
// through go-store. When an account is unlocked the JSON payload is
// PGP-sealed under the user public key (Mantis #1736 / Cerberus #67
// F-1); when locked the legacy plaintext write path preserves
// pre-Stage-E behaviour so existing operator flows survive the
// unlock-window deferral without losing data.
func writeMessages(c *core.Core, id string, msgs []inference.Message) core.Result {
	encoded := core.JSONMarshal(msgs)
	if !encoded.OK {
		return encoded
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return core.Fail(core.E("sessions.writeMessages", "encode failed", nil))
	}
	// At-rest sealing — when unlocked, encrypt before handing to
	// store.set. The sealed envelope is itself a JSON string so
	// store.set's value-shape contract is preserved.
	if sealed, ok := sealPayload(c, bytes); ok {
		bytes = sealed
	}
	return c.Action("store.set").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupMessages},
		core.Option{Key: "key", Value: id},
		core.Option{Key: "value", Value: string(bytes)},
	))
}

// readMessages fetches the message log via go-store and decodes the
// JSON payload. Sealed envelopes (Mantis #1736) are detected by the
// `{"sealed":` prefix and decrypted via unsealPayload before the
// JSON decode; legacy plaintext payloads continue to decode directly.
// A sealed-but-unreadable payload (account locked, MAC tampered)
// fails closed with a typed error rather than serving a phantom
// empty message list.
func readMessages(c *core.Core, id string) core.Result {
	r := c.Action("store.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupMessages},
		core.Option{Key: "key", Value: id},
	))
	if !r.OK {
		return r
	}
	encoded, ok := r.Value.(string)
	if !ok || encoded == "" {
		return core.Ok([]inference.Message{})
	}
	raw := []byte(encoded)
	plaintext, sealed := unsealPayload(c, raw)
	if sealed {
		if plaintext == nil {
			return core.Fail(core.E("sessions.readMessages",
				"sealed payload unreadable (account locked or MAC invalid)", nil))
		}
		raw = plaintext
	}
	var msgs []inference.Message
	if ur := core.JSONUnmarshal(raw, &msgs); !ur.OK {
		return ur
	}
	return core.Ok(msgs)
}

// writeManifest serialises a SessionInfo to JSON and dispatches it
// through go-store. When an account is unlocked the JSON payload is
// PGP-sealed under the user public key so SystemPrompt + Tags + the
// Snippet preview stay confidential at rest; when locked the legacy
// plaintext write path is retained so first-launch flows can persist
// session creation before the user unlocks.
func writeManifest(c *core.Core, info SessionInfo) core.Result {
	encoded := core.JSONMarshal(info)
	if !encoded.OK {
		return encoded
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return core.Fail(core.E("sessions.writeManifest", "encode failed", nil))
	}
	if sealed, ok := sealPayload(c, bytes); ok {
		bytes = sealed
	}
	return c.Action("store.set").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
		core.Option{Key: "key", Value: info.ID},
		core.Option{Key: "value", Value: string(bytes)},
	))
}

// readManifest fetches a single SessionInfo via go-store and decodes
// it. Sealed envelopes (Mantis #1736) are detected + unsealed before
// the JSON decode; legacy plaintext entries decode directly. A
// sealed-but-unreadable entry (account locked) surfaces as a typed
// error so callers (Append / Rename / SetTags / Read) fail closed
// rather than corrupting on a phantom-empty SessionInfo.
func readManifest(c *core.Core, id string) core.Result {
	r := c.Action("store.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
		core.Option{Key: "key", Value: id},
	))
	if !r.OK {
		return r
	}
	encoded, ok := r.Value.(string)
	if !ok || encoded == "" {
		return core.Fail(core.E("sessions.readManifest", "not found", nil))
	}
	raw := []byte(encoded)
	plaintext, sealed := unsealPayload(c, raw)
	if sealed {
		if plaintext == nil {
			return core.Fail(core.E("sessions.readManifest",
				"sealed manifest unreadable (account locked or MAC invalid)", nil))
		}
		raw = plaintext
	}
	var info SessionInfo
	if ur := core.JSONUnmarshal(raw, &info); !ur.OK {
		return ur
	}
	return core.Ok(info)
}

// sessionsScope is the Event.Scope literal stamped on every typed audit
// row emitted from this package — 7th audit-cluster adoption following
// runner / sandbox / process / marketplace / gateway / downloader / queue
// per Cerberus #67 F-2 (Mantis #1737). The Operations panel's facet
// chrome learns this literal in the same commit a new scope ships.
const sessionsScope = "sessions"

// emitCreateRequested fires the Requested row at the top of Create —
// BEFORE the id allocation so a crash mid-call still leaves caller
// intent in the audit substrate. Title bytes are NEVER in Meta — only
// the rune count (PII discipline per H#212 chat-content).
//
// Usage example (internal):
//
//	emitCreateRequested(title)
func emitCreateRequested(title string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsCreateRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"title_len": core.RuneCount(title),
		},
	})
}

// emitCreateSucceeded fires the Succeeded row at the end of Create
// when the manifest-write + empty-message-log-write pipeline lands
// without error. session_id is the freshly-allocated identifier.
//
// Usage example (internal):
//
//	emitCreateSucceeded(id, title)
func emitCreateSucceeded(id, title string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsCreateSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id": id,
			"title_len":  core.RuneCount(title),
		},
	})
}

// emitCreateFailed fires the Failed row on any non-OK Create Result.
// r is the failed core.Result whose stable, bounded error_code is
// derived through audit.ErrorCode per the W1–W7 cascade contract — NEVER
// raw r.Error() prose. session_id is NOT in Meta on Failed because the
// id allocation may not have completed.
//
// Usage example (internal):
//
//	if !r.OK { emitCreateFailed(title, r); return r }
func emitCreateFailed(title string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsCreateFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"title_len":  core.RuneCount(title),
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitRenameRequested fires the Requested row at the top of Rename
// after the guards but BEFORE the manifest read. Empty-id / empty-title
// guards fire emitRenameFailed directly without a paired Requested so the
// forensic walker can pair them by absence.
//
// Usage example (internal):
//
//	emitRenameRequested(id)
func emitRenameRequested(id string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsRenameRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id": id,
		},
	})
}

// emitRenameSucceeded fires the Succeeded row at the end of Rename.
// new_title_len is the rune count of the post-rename title (PII-safe
// forensic discriminator).
//
// Usage example (internal):
//
//	emitRenameSucceeded(id, newTitle)
func emitRenameSucceeded(id, newTitle string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsRenameSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id":    id,
			"new_title_len": core.RuneCount(newTitle),
		},
	})
}

// emitRenameFailed fires the Failed row on any non-OK Rename Result —
// nil-core / empty-id / empty-title guards / session-not-found /
// manifest-write-failure. session_id may be empty when the empty-id
// guard fired. error_code is bounded via audit.ErrorCode(r).
//
// Usage example (internal):
//
//	if !r.OK { emitRenameFailed(id, title, r); return r }
func emitRenameFailed(id, _ string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsRenameFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"session_id": id,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitDeleteRequested fires the Requested row at the top of Delete
// AFTER the empty-id guard. Idempotent on absent id by design — the
// Requested row commits regardless of registry state so a forensic
// walker can correlate caller intent against the store at the moment
// of the call.
//
// Usage example (internal):
//
//	emitDeleteRequested(id)
func emitDeleteRequested(id string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsDeleteRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id": id,
		},
	})
}

// emitDeleteSucceeded fires the Succeeded row at the end of Delete
// when both store.delete calls (messages + manifest) land OK.
//
// Usage example (internal):
//
//	emitDeleteSucceeded(id)
func emitDeleteSucceeded(id string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsDeleteSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id": id,
		},
	})
}

// emitDeleteFailed fires the Failed row on any non-OK Delete Result —
// nil-core / empty-id / store.delete failure on either group. error_code
// is bounded via audit.ErrorCode(r); raw r.Error() prose never reaches
// the substrate.
//
// Usage example (internal):
//
//	if !r.OK { emitDeleteFailed(id, r); return r }
func emitDeleteFailed(id string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsDeleteFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"session_id": id,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitAppendRequested fires the Requested row at the top of Append —
// BEFORE the read-modify-write of the message log so a crash mid-call
// still leaves the append intent in the audit substrate. content bytes
// are NEVER in Meta — only the byte count. role is the closed-set
// inference role literal ("user" / "assistant" / "system").
//
// Usage example (internal):
//
//	emitAppendRequested(id, role, content)
func emitAppendRequested(id, role, content string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsAppendMessageRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id":  id,
			"role":        role,
			"content_len": len(content),
		},
	})
}

// emitAppendSucceeded fires the Succeeded row at the end of Append
// after both message-log-write + manifest-write land OK.
//
// Usage example (internal):
//
//	emitAppendSucceeded(id, role, content)
func emitAppendSucceeded(id, role, content string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsAppendMessageSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"session_id":  id,
			"role":        role,
			"content_len": len(content),
		},
	})
}

// emitAppendFailed fires the Failed row on any non-OK Append Result —
// nil-core / message-log read or write failure / manifest read or write
// failure. error_code is bounded via audit.ErrorCode(r); raw r.Error()
// prose never reaches the substrate. content bytes still never appear
// in Meta — only the byte count.
//
// Usage example (internal):
//
//	if !r.OK { emitAppendFailed(id, role, content, r); return r }
func emitAppendFailed(id, role, content string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSessionsAppendMessageFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   sessionsScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"session_id":  id,
			"role":        role,
			"content_len": len(content),
			"error_code":  audit.ErrorCode(r),
		},
	})
}
