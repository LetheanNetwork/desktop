// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the contacts service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/sales/contacts/service.

package contacts

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all contacts, optionally filtered by warmth and/or a
// free-text query on name + role. Results are sorted by last-touch
// descending (hottest first).
//
// Usage example:
//
//	r := svc.List(contacts.ListInput{Warmth: "hot"})
//	if r.OK { out := r.Value.(contacts.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	records, err := loadAll()
	if err != nil {
		return core.Fail(core.E("contacts.List", "scan failed", err))
	}

	now := core.Now()
	all := make([]Contact, 0, len(records))
	for _, r := range records {
		all = append(all, toContact(r, now))
	}
	total := len(all)

	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}

	filtered := all[:0]
	for _, c := range all {
		if input.Warmth != "" && c.Warmth != input.Warmth {
			continue
		}
		if input.Query != "" {
			if !containsCI(c.Name, input.Query) && !containsCI(c.Role, input.Query) {
				continue
			}
		}
		filtered = append(filtered, c)
	}

	// Sort by warmth priority (hot > warm > cool), then name for stable order.
	// Simple insertion sort — contact lists are small (< 1000).
	warmthRank := func(w string) int {
		switch w {
		case "hot":
			return 0
		case "warm":
			return 1
		default:
			return 2
		}
	}
	for i := 1; i < len(filtered); i++ {
		for j := i; j > 0; j-- {
			ri := warmthRank(filtered[j].Warmth)
			rj := warmthRank(filtered[j-1].Warmth)
			if ri < rj {
				filtered[j], filtered[j-1] = filtered[j-1], filtered[j]
			} else {
				break
			}
		}
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return core.Ok(ListOutput{
		Contacts: filtered,
		Total:    total,
	})
}

// Get returns a single contact by ID with the full notes body.
//
// Usage example:
//
//	r := svc.Get(contacts.GetInput{ID: "ada-penley"})
//	if r.OK { detail := r.Value.(contacts.ContactDetail) }
func (s *Service) Get(input GetInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	dirR := contactsDir()
	if !dirR.OK {
		return core.Fail(core.E("contacts.Get", dirR.Error(), nil))
	}
	fpath := core.PathJoin(dirR.Value.(string), input.ID+".md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("contacts.Get", "not found: "+input.ID, nil))
	}
	rec, err := parseContact(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("contacts.Get", "parse failed", err))
	}
	now := core.Now()
	return core.Ok(ContactDetail{
		Contact: toContact(rec, now),
		Notes:   rec.Notes,
	})
}

// Create writes a new contact Trix file to ~/Lethean/sales/contacts/
// and fires the sales.contacts.created event.
//
// Usage example:
//
//	r := svc.Create(contacts.CreateInput{
//	    Name: "Ada Penley", Role: "CTO · Heritage Law",
//	    Next: "call · Fri",
//	})
func (s *Service) Create(input CreateInput) core.Result {
	if input.Name == "" {
		return core.Fail(core.E("contacts.Create", "name is required", nil))
	}

	id := slugify(input.Name)
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}

	dirR := contactsDir()
	if !dirR.OK {
		return core.Fail(core.E("contacts.Create", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)

	fpath := core.PathJoin(dir, id+".md")
	if core.Stat(fpath).OK {
		return core.Fail(core.E("contacts.Create", "contact already exists: "+id, nil))
	}

	lastTouch := input.LastTouch
	if lastTouch.IsZero() {
		lastTouch = core.Now().UTC()
	}

	rec := ContactRecord{
		ID:        id,
		Name:      input.Name,
		Role:      input.Role,
		LastTouch: lastTouch,
		Next:      input.Next,
	}

	// Cascade W1 (Mantis #1540) — stamp version=1 on creation. Route
	// through paths.AtomicWriteWithVersion with IfVersion=0 so the
	// primitive performs an unconditional first-write (the Create-path
	// pre-checks Stat above for the exists-rejection).
	rec.Version = 1

	raw, err := marshalContact(rec)
	if err != nil {
		return core.Fail(core.E("contacts.Create", "marshal", err))
	}
	if err := atomicWriteFile(fpath, raw, 0); err != nil {
		return core.Fail(core.E("contacts.Create", err.Error(), err))
	}

	contact := toContact(rec, core.Now())
	s.fireEvent(EventContactCreated, contact)
	return core.Ok(contact)
}

// Update applies partial updates to a contact record. Only non-zero
// fields in the input are applied. Fires the sales.contacts.updated event.
//
// Usage example:
//
//	r := svc.Update(contacts.UpdateInput{
//	    ID: "ada-penley", Next: "contract", LastTouch: core.Now(),
//	})
func (s *Service) Update(input UpdateInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}

	dirR := contactsDir()
	if !dirR.OK {
		return core.Fail(core.E("contacts.Update", dirR.Error(), nil))
	}
	fpath := core.PathJoin(dirR.Value.(string), input.ID+".md")

	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("contacts.Update", "not found: "+input.ID, nil))
	}
	rec, err := parseContact(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("contacts.Update", "parse failed", err))
	}

	if input.Role != "" {
		rec.Role = input.Role
	}
	if !input.LastTouch.IsZero() {
		rec.LastTouch = input.LastTouch.UTC()
	}
	if input.Next != "" {
		rec.Next = input.Next
	}
	if input.Notes != "" {
		rec.Notes = input.Notes
	}

	// Cascade W1 (Mantis #1540) — bump version + route through
	// paths.AtomicWriteWithVersion. IfVersion=rec.Version (the value
	// observed on disk via parseContact) gates the write under the
	// primitive's optimistic-lock check. rec.Version=0 (legacy file
	// predating cutover) skips the check and stamps version=1 on the
	// upgrade write.
	priorVersion := rec.Version
	rec.Version = priorVersion + 1
	if rec.Version < 1 {
		rec.Version = 1
	}

	updated, err := marshalContact(rec)
	if err != nil {
		return core.Fail(core.E("contacts.Update", "marshal", err))
	}
	if err := atomicWriteFile(fpath, updated, priorVersion); err != nil {
		return core.Fail(core.E("contacts.Update", err.Error(), err))
	}

	contact := toContact(rec, core.Now())
	s.fireEvent(EventContactUpdated, contact)
	return core.Ok(contact)
}

// atomicWriteFile routes contact writes through paths.AtomicWriteWithVersion
// (Cascade W1, Mantis #1540). The IfVersion gate uses the version the
// caller observed on disk; pass 0 for first-writes / legacy upgrades so
// the primitive performs an unconditional write. Stale writes surface as
// "contacts.update.conflict" (or "contacts.create.conflict" on the
// create path) preserving the paths.VersionStale envelope under the
// returned error's cause — recoverable via paths.VersionStaleFromError.
//
// Audit emission is automatic via paths.AuditModeForPath (sales/* paths
// route through AuditModeBatch per RFC §6.1) — callers do not need to
// emit RecordSync/RecordBatch manually.
//
// Usage example:
//
//	if err := atomicWriteFile(fpath, body, prior.Version); err != nil {
//	    return core.Fail(core.E("contacts.Update", err.Error(), err))
//	}
func atomicWriteFile(path string, content []byte, ifVersion int) error {
	r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body:      content,
		IfVersion: ifVersion,
	})
	if r.OK {
		return nil
	}
	if core.Contains(r.Error(), paths.CodeVersionStale) {
		// Wrap the primitive's typed VersionStale envelope so callers
		// keep matching on the contacts.<verb>.conflict code while the
		// structured payload remains accessible via
		// paths.VersionStaleFromError.
		return core.E("contacts.atomicWriteFile", "contacts.update.conflict",
			versionStaleAsError(r.Value))
	}
	return core.E("contacts.atomicWriteFile", "write failed: "+r.Error(), nil)
}

// versionStaleAsError extracts the structured paths.VersionStale payload
// from a Result.Value and returns it as an error whose Error() reports
// the canonical "contacts.update.conflict" code. The envelope stays
// accessible via paths.VersionStaleFromError on the returned cause.
func versionStaleAsError(v any) error {
	if e, ok := v.(error); ok {
		return e
	}
	return core.NewCode("contacts.update.conflict", "version_stale")
}
