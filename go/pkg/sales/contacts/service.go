// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the contacts surface. Reads and
// writes Trix-style markdown files from ~/Lethean/sales/contacts/.
// No queue, no orm — filesystem is the persistence layer.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Contacts" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     office/mail.AccountProvider; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.WriteFile /
// core.ReadDir / core.DirFS / core.MkdirAll / core.PathJoin).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package contacts

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
// CONFIRMED by Cerberus #27). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	contactsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (contacts) and satisfied by the producer (*account.Service). No
// pkg/account import lands in pkg/sales/contacts.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// Service owns the contacts surface.
//
// Usage example:
//
//	svc := contacts.NewService(c)
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
	// (§2.2 / Cerberus #27 Q2). CompareAndSwap-on-first-hit emits
	// core.Warn exactly once per Service instance.
	nilWarned atomic.Bool
}

// NewService constructs the contacts service against a Core container.
// Wired via core.WithName("sales-contacts", contacts.Register) in app.go.
//
// Usage example:
//
//	svc := contacts.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the contacts service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-contacts", contacts.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Contacts.List()" etc.
func (s *Service) ServiceName() string { return "Contacts" }

// contactsDir resolves ~/Lethean/sales/contacts/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): contact records carry PII (names,
// emails, role context, notes) that the auth-gate footer promises stays
// on this Mac. World-readable mode would make that promise false.
func contactsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "sales", "contacts")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugify converts a display name to a filesystem-safe slug.
// Lowercases the input and replaces non-alphanumeric bytes with hyphens.
// Uses byte iteration — no stdlib strings import.
func slugify(name string) string {
	b := []byte(name)
	out := make([]byte, 0, len(b))
	prevHyphen := false
	for _, ch := range b {
		var c byte
		switch {
		case ch >= 'A' && ch <= 'Z':
			c = ch + 32 // toLower
		case ch >= 'a' && ch <= 'z':
			c = ch
		case ch >= '0' && ch <= '9':
			c = ch
		default:
			c = '-'
		}
		if c == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		out = append(out, c)
	}
	// Trim trailing hyphen.
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// warmthFor computes the warmth bucket from elapsed days since lastTouch.
// ≤7 days = "hot"; 8–21 days = "warm"; >21 days = "cool".
// Zero lastTouch returns "cool".
func warmthFor(lastTouch core.Time, now core.Time) string {
	if lastTouch.IsZero() {
		return "cool"
	}
	diff := now.Sub(lastTouch)
	if diff < 0 {
		diff = -diff
	}
	const day = 24 * core.Hour
	days := int(diff / day)
	switch {
	case days <= 7:
		return "hot"
	case days <= 21:
		return "warm"
	default:
		return "cool"
	}
}

// lastTouchLabel formats the last-touch duration as a short opaque string
// matching the fixture data shape: "replied N d", "N d ago".
// v1: returns "N d ago" universally. The "replied"/"meeting" prefix is
// follow-up work (requires tracking the kind of last interaction).
func lastTouchLabel(lastTouch core.Time, now core.Time) string {
	if lastTouch.IsZero() {
		return ""
	}
	diff := now.Sub(lastTouch)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour
	const week = 7 * day
	switch {
	case diff < minute:
		return "just now"
	case diff < hour:
		return core.Sprintf("%d min ago", int(diff/minute))
	case diff < day:
		return core.Sprintf("%d h ago", int(diff/hour))
	default:
		days := int(diff / day)
		if days < 7 {
			return core.Sprintf("%d d ago", days)
		}
		return core.Sprintf("%d w ago", int(diff/week))
	}
}

// parseContact splits a Trix file into frontmatter + body and decodes
// the YAML header into a ContactRecord.
func parseContact(raw []byte) (ContactRecord, error) {
	content := raw

	// Skip the opening ---\n
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

	// Find the closing ---
	closeIdx := -1
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var rec ContactRecord
	var body string
	if closeIdx < 0 {
		if err := yaml.Unmarshal(content, &rec); err != nil {
			return ContactRecord{}, core.E("contacts.parse", "yaml unmarshal", err)
		}
	} else {
		fm := content[:closeIdx]
		if err := yaml.Unmarshal(fm, &rec); err != nil {
			return ContactRecord{}, core.E("contacts.parse", "yaml unmarshal", err)
		}
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	rec.Notes = body
	return rec, nil
}

// marshalContact serialises a ContactRecord to the Trix file format.
func marshalContact(r ContactRecord) ([]byte, error) {
	// Marshal struct (Notes field is yaml:"-", so excluded).
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("contacts.marshal", "yaml marshal", err)
	}
	var out []byte
	out = append(out, []byte("---\n")...)
	out = append(out, fm...)
	out = append(out, []byte("---\n")...)
	if r.Notes != "" {
		out = append(out, []byte(r.Notes)...)
	}
	return out, nil
}

// toContact converts a ContactRecord to the wire type.
// now is passed so the caller controls the clock reference (test-friendly).
func toContact(r ContactRecord, now core.Time) Contact {
	return Contact{
		ID:     r.ID,
		Name:   r.Name,
		Role:   r.Role,
		Last:   lastTouchLabel(r.LastTouch, now),
		Warmth: warmthFor(r.LastTouch, now),
		Next:   r.Next,
	}
}

// loadAll scans ~/Lethean/sales/contacts/ and returns all parseable
// ContactRecord values. The Notes body is NOT loaded — use Get for full
// records.
func loadAll() ([]ContactRecord, error) {
	dirR := contactsDir()
	if !dirR.OK {
		return nil, core.E("contacts.loadAll", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		// Directory may exist but be unreadable — treat as empty.
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var records []ContactRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		fpath := core.PathJoin(dir, name)
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		rec, err := parseContact(raw.Value.([]byte))
		if err != nil {
			continue
		}
		// Strip body for list performance.
		rec.Notes = ""
		records = append(records, rec)
	}
	return records, nil
}

// fireEvent publishes a contact event on the Core ACTION bus.
func (s *Service) fireEvent(name string, contact Contact) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(ContactEvent{
		EventName: name,
		Contact:   contact,
		At:        core.Now().UTC(),
	})
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
//	contactsSvc.SetSessionGate(accountSvc)
//
//wails:ignore
func (s *Service) SetSessionGate(g SessionGate) {
	s.gateMu.Lock()
	s.gate = g
	s.gateMu.Unlock()
}

// Stop nils the SessionGate reference so a draining Service
// fails-closed on any late-arriving write (§B.2 mirror mail's drain
// hygiene / Cerberus #27 ADD-5). Read-only methods (List, Get)
// continue to function — Stop only severs the write gate.
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
// Fail-safe on nil gate (§2.2 / Cerberus #27 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
//
// Usage example:
//
//	if fail, ok := s.assertUnlocked("contacts.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("contacts: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "contacts.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "contacts.session.locked", nil)), false
	}
	return core.Result{}, true
}

// containsCI reports whether haystack contains needle (case-insensitive).
// Pure byte scan — no stdlib strings import.
func containsCI(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	h := []byte(haystack)
	n := []byte(needle)
	// toLower for comparison.
	for i := range h {
		if h[i] >= 'A' && h[i] <= 'Z' {
			h[i] += 32
		}
	}
	for i := range n {
		if n[i] >= 'A' && n[i] <= 'Z' {
			n[i] += 32
		}
	}
	if len(n) > len(h) {
		return false
	}
	for i := 0; i <= len(h)-len(n); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
