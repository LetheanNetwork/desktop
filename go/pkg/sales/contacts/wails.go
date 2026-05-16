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

	raw, err := marshalContact(rec)
	if err != nil {
		return core.Fail(core.E("contacts.Create", "marshal", err))
	}
	// 0o600 (Cerberus #1487 PR-1): PII at rest — owner-only.
	if w := core.WriteFile(fpath, raw, 0o600); !w.OK {
		return core.Fail(core.E("contacts.Create", w.Error(), nil))
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

	updated, err := marshalContact(rec)
	if err != nil {
		return core.Fail(core.E("contacts.Update", "marshal", err))
	}
	// 0o600 (Cerberus #1487 PR-1): PII at rest — owner-only.
	if w := core.WriteFile(fpath, updated, 0o600); !w.OK {
		return core.Fail(core.E("contacts.Update", w.Error(), nil))
	}

	contact := toContact(rec, core.Now())
	s.fireEvent(EventContactUpdated, contact)
	return core.Ok(contact)
}
