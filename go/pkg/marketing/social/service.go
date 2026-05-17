// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing social post queue.
// Manages posts at ~/Lethean/marketing/social/{id}.md.
// Each file is a Trix document: YAML frontmatter + markdown body (the post text).
//
// Channels are stored as a comma-separated string in frontmatter and split
// back to []string on parse — avoids YAML array quoting complexity while
// keeping the file human-readable.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Social" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package social

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// SessionGate is the minimal consumer-defined interface satisfied by
// *account.Service. Live-read at every gate check — no cached bool, no
// subscribe/event bus (RFC.stage-e-unlockgate v2 §1.1 — Pushback 2
// CONFIRMED by Cerberus #27/#28). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	socialSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (marketing/social) and satisfied by the producer (*account.Service).
// No pkg/account import lands in pkg/marketing/social.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// Service owns the marketing social surface.
//
// Usage example:
//
//	svc := social.NewService(c)
//	svc.SetSessionGate(accountSvc)
type Service struct {
	core *core.Core

	// gateMu guards reads/writes of the session gate reference. A
	// sync.RWMutex protects against the wire/Stop race where app.go
	// SetSessionGate runs concurrent with a late-arriving Wails call
	// reading the reference. Read-heavy access (every write gates
	// once) — RWMutex.RLock is microseconds.
	gateMu sync.RWMutex
	// gate is the live-read session source (RFC §1.1). nil before
	// SetSessionGate runs in app.go and after Stop nils it; the
	// nilWarned one-shot warning fires on the first nil-hit to
	// surface wire-ordering bugs without log spam (§2.2 ADD-1.5).
	gate SessionGate
	// nilWarned is the one-shot guard for the nil-gate fail-safe
	// (§2.2 / Cerberus #28 Q2). CompareAndSwap-on-first-hit emits
	// core.Warn exactly once per Service instance.
	nilWarned atomic.Bool
}

// NewService constructs the social service against a Core container.
//
// Usage example:
//
//	svc := social.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the office/mail.AccountProvider setter pattern. Live-read on
// every gate check — no event-bus reliability concerns, no cache
// coherence concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	socialSvc.SetSessionGate(accountSvc)
func (s *Service) SetSessionGate(g SessionGate) {
	s.gateMu.Lock()
	s.gate = g
	s.gateMu.Unlock()
}

// Stop nils the SessionGate reference so a draining Service
// fails-closed on any late-arriving write (§B.2 mirror mail's drain
// hygiene / Cerberus #28 ADD-5). Read-only methods (List, Get) continue
// to function — Stop only severs the write gate.
//
// Usage example:
//
//	_ = svc.Stop(core.Background())
func (s *Service) Stop(_ core.Context) core.Result {
	s.gateMu.Lock()
	s.gate = nil
	s.gateMu.Unlock()
	return core.Ok(nil)
}

// assertUnlocked returns a Fail result when the session is locked or
// the session gate is not wired. Called at the top of every write
// method before any FS touch.
//
// Live-read semantics (RFC §1.1): consults s.gate.UnlockedAccountIDs()
// at every call — no cached bool — so a lock transition is observable
// on the very next write attempt.
//
// Fail-safe on nil gate (§2.2 / Cerberus #28 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
//
// Usage example:
//
//	if fail, ok := s.assertUnlocked("social.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("social: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "social.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "social.session.locked", nil)), false
	}
	return core.Result{}, true
}

// Register constructs the social service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-social", social.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Social.List()" etc.
func (s *Service) ServiceName() string { return "Social" }

// postFrontmatter is the YAML shape stored in each social post file.
//
// Cascade W2 (RFC §B.3 row 5) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write through writePost stamps version=1.
type postFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	Ch      string `yaml:"ch"` // comma-separated channels
	When    string `yaml:"when"`
	State   string `yaml:"state"`
	Attach  string `yaml:"attach"`
}

// socialDir resolves ~/Lethean/marketing/social/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): unsent post drafts + scheduled
// content shape — owner-only at rest.
func socialDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "social")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// splitChannels splits a comma-separated channel string into a slice.
// Returns nil (not empty slice) when the input is empty.
func splitChannels(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			part := trimSpace(raw[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// trimSpace trims leading and trailing ASCII spaces from s.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && s[start] == ' ' {
		start++
	}
	end := len(s)
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

// joinChannels joins a channel slice into a comma-separated string.
func joinChannels(ch []string) string {
	if len(ch) == 0 {
		return ""
	}
	n := len(ch) - 1
	for _, c := range ch {
		n += len(c)
	}
	b := make([]byte, 0, n)
	for i, c := range ch {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, c...)
	}
	return string(b)
}

// parsePost extracts frontmatter + text body from a Trix-formatted file.
func parsePost(raw []byte) (SocialPost, error) {
	content := raw

	open := []byte("---\n")
	if len(content) >= len(open) {
		match := true
		for i, b := range open {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(open):]
		}
	}

	closeIdx := -1
	for i := 0; i < len(content)-2; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var fm postFrontmatter
	fmBytes := content
	text := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		// Strip the newline written immediately after "---" by writePost.
		for len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		// Strip trailing newline.
		for len(rest) > 0 && rest[len(rest)-1] == '\n' {
			rest = rest[:len(rest)-1]
		}
		text = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return SocialPost{}, core.E("social.parsePost", "yaml unmarshal", err)
	}
	return SocialPost{
		ID:      fm.ID,
		Ch:      splitChannels(fm.Ch),
		When:    fm.When,
		State:   fm.State,
		Text:    text,
		Attach:  fm.Attach,
		Version: fm.Version,
	}, nil
}

// writePost serialises a SocialPost to Trix format and writes it via
// paths.AtomicWriteWithVersion (Cascade W2, RFC §B.3 row 5). Post
// text lives in the body (not frontmatter) to avoid YAML quoting of
// newlines and special characters.
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parsePost), or 0 for first-writes /
// legacy-file upgrades. writePost stamps the next monotonic version
// (ifVersion+1) into the marshalled frontmatter so subsequent reads
// see version=1,2,3... monotonically.
//
// Cerberus #1486: p.ID lands directly in the filename — validate.
//
// Return shape (Mantis #1544 gating, inherited from W1): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "social.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writePost(dir, p, prior.Version); !wr.OK {
//	    return wr
//	}
func writePost(dir string, p SocialPost, ifVersion int) core.Result {
	if err := paths.IsValidID(p.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := postFrontmatter{
		Version: nextVersion,
		ID:      p.ID,
		Ch:      joinChannels(p.Ch),
		When:    p.When,
		State:   p.State,
		Attach:  p.Attach,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("social.writePost", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if p.Text != "" {
		data = append(data, '\n')
		data = append(data, []byte(p.Text)...)
	}
	fpath := core.PathJoin(dir, p.ID+".md")
	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      data,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"social.update.conflict", stale))
	}
	return core.Fail(core.E("social.writePost",
		"write failed: "+res.Error(), nil))
}

// loadPosts scans ~/Lethean/marketing/social/ and returns all parseable
// post records. Skips malformed files silently.
func loadPosts() ([]SocialPost, error) {
	dirR := socialDir()
	if !dirR.OK {
		return nil, core.E("social.loadPosts", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var posts []SocialPost
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		raw := core.ReadFile(core.PathJoin(dir, name))
		if !raw.OK {
			continue
		}
		p, err := parsePost(raw.Value.([]byte))
		if err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// containsChannel returns true if ch slice contains the given channel.
func containsChannel(ch []string, target string) bool {
	for _, c := range ch {
		if c == target {
			return true
		}
	}
	return false
}

// fireSocialEvent publishes a social event on the Core ACTION bus.
func (s *Service) fireSocialEvent(eventName, postID, state string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(SocialEvent{
		EventName: eventName,
		PostID:    postID,
		State:     state,
		At:        core.Now().UTC(),
	})
}
