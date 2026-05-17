// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing content calendar surface.
// Manages editorial items at ~/Lethean/marketing/content/{id}.md.
// Each file is a Trix document: YAML frontmatter + markdown body.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Content" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     sales/contacts + sales/deals + sales/pipeline +
//     office/mail.AccountProvider; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package content

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
//	contentSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer (content)
// and satisfied by the producer (*account.Service). The duplication
// across sales/contacts, sales/deals, sales/pipeline, incidents,
// runbooks, marketing/* etc. IS the AX-8 boundary — each consumer owns
// its own contract, no shared types package importing producer.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// Service owns the marketing content surface.
//
// Usage example:
//
//	svc := content.NewService(c)
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

// NewService constructs the content service against a Core container.
//
// Usage example:
//
//	svc := content.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the content service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-content", content.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Content.List()" etc.
func (s *Service) ServiceName() string { return "Content" }

// colSpec is ordered column metadata.
type colSpec struct {
	ID    string
	Label string
}

// columnOrder returns the canonical ordered column slice.
// The order matches the fixture column order in content.ts.
func columnOrder() []colSpec {
	return []colSpec{
		{ID: "idea", Label: "Ideas"},
		{ID: "draft", Label: "Drafting"},
		{ID: "review", Label: "Review"},
		{ID: "ready", Label: "Ready"},
		{ID: "live", Label: "Live"},
	}
}

// itemFrontmatter is the YAML shape of a content item file.
//
// Cascade W2 (RFC §B.3 row 4) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write through writeItem stamps version=1.
type itemFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	T       string `yaml:"t"`
	Who     string `yaml:"who"`
	When    string `yaml:"when"`
	Due     string `yaml:"due"`
	Col     string `yaml:"col"`
}

// contentDir resolves ~/Lethean/marketing/content/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): content drafts + Due dates carry
// pre-publication strategy — owner-only at rest.
func contentDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "content")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugifyContent converts a title to a filesystem-safe slug.
func slugifyContent(title string) string {
	out := make([]byte, 0, len(title))
	for i := 0; i < len(title); i++ {
		b := title[i]
		if b >= 'A' && b <= 'Z' {
			out = append(out, b+32)
		} else if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' {
			out = append(out, b)
		} else if b == ' ' || b == '_' {
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// parseItem extracts frontmatter + body from a Trix-formatted file.
func parseItem(raw []byte) (ContentItem, error) {
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

	var fm itemFrontmatter
	fmBytes := content
	body := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return ContentItem{}, core.E("content.parseItem", "yaml unmarshal", err)
	}
	return ContentItem{
		ID:      fm.ID,
		T:       fm.T,
		Who:     fm.Who,
		When:    fm.When,
		Due:     fm.Due,
		Col:     fm.Col,
		Body:    body,
		Version: fm.Version,
	}, nil
}

// writeItem serialises a ContentItem to Trix format and writes it via
// paths.AtomicWriteWithVersion (Cascade W2, RFC §B.3 row 4).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseItem), or 0 for first-writes /
// legacy-file upgrades. writeItem stamps the next monotonic version
// (ifVersion+1) into the marshalled frontmatter so subsequent reads
// see version=1,2,3... monotonically.
//
// Cerberus #1486: item.ID lands directly in the filename — validate.
//
// Return shape (Mantis #1544 gating, inherited from W1): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "content.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeItem(dir, item, prior.Version); !wr.OK {
//	    return wr
//	}
func writeItem(dir string, item ContentItem, ifVersion int) core.Result {
	if err := paths.IsValidID(item.ID); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent writes increment.
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := itemFrontmatter{
		Version: nextVersion,
		ID:      item.ID,
		T:       item.T,
		Who:     item.Who,
		When:    item.When,
		Due:     item.Due,
		Col:     item.Col,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("content.writeItem", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if item.Body != "" {
		data = append(data, '\n')
		data = append(data, []byte(item.Body)...)
	}
	fpath := core.PathJoin(dir, item.ID+".md")
	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      data,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"content.update.conflict", stale))
	}
	return core.Fail(core.E("content.writeItem",
		"write failed: "+res.Error(), nil))
}

// loadItems scans ~/Lethean/marketing/content/ and returns all parseable
// item records. Skips malformed files silently.
func loadItems() ([]ContentItem, error) {
	dirR := contentDir()
	if !dirR.OK {
		return nil, core.E("content.loadItems", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var items []ContentItem
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
		item, err := parseItem(raw.Value.([]byte))
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// nextCol returns the column ID following the given one in the canonical order.
// Returns empty string if already at "live".
func nextCol(current string) string {
	order := columnOrder()
	for i, spec := range order {
		if spec.ID == current && i+1 < len(order) {
			return order[i+1].ID
		}
	}
	return ""
}

// fireContentEvent publishes a content event on the Core ACTION bus.
func (s *Service) fireContentEvent(eventName, itemID, col string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(ContentEvent{
		EventName: eventName,
		ItemID:    itemID,
		Col:       col,
		At:        core.Now().UTC(),
	})
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the sales/contacts + sales/deals + sales/pipeline +
// office/mail.AccountProvider setter pattern. Live-read on every gate
// check — no event-bus reliability concerns, no cache coherence
// concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	contentSvc.SetSessionGate(accountSvc)
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
//	if fail, ok := s.assertUnlocked("content.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("content: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "content.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "content.session.locked", nil)), false
	}
	return core.Result{}, true
}
