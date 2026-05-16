// SPDX-Licence-Identifier: EUPL-1.2

package contacts_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// TestServiceName_Good — ServiceName returns "Contacts".
func TestServiceName_Good(t *testing.T) {
	svc := contacts.NewService(nil)
	if svc.ServiceName() != "Contacts" {
		t.Fatalf("expected Contacts, got %q", svc.ServiceName())
	}
}

// TestListInput_Fields_Good — ListInput has expected fields.
func TestListInput_Fields_Good(t *testing.T) {
	in := contacts.ListInput{
		Warmth: "hot",
		Query:  "heritage",
		Limit:  10,
	}
	_ = in.Warmth
	_ = in.Query
	_ = in.Limit
}

// TestCreateInput_Fields_Good — CreateInput has expected fields.
func TestCreateInput_Fields_Good(t *testing.T) {
	in := contacts.CreateInput{
		Name: "Ada Penley",
		Role: "CTO · Heritage Law",
		Next: "call · Fri",
	}
	_ = in.Name
	_ = in.Role
	_ = in.Next
}

// TestUpdateInput_Fields_Good — UpdateInput has expected fields.
func TestUpdateInput_Fields_Good(t *testing.T) {
	in := contacts.UpdateInput{
		ID:    "ada-penley",
		Role:  "Partner · Heritage Law",
		Next:  "contract",
		Notes: "## Discussion\n...",
	}
	_ = in.ID
	_ = in.Role
	_ = in.Next
	_ = in.Notes
}

// TestListOutput_Fields_Good — ListOutput has expected fields.
func TestListOutput_Fields_Good(t *testing.T) {
	out := contacts.ListOutput{
		Contacts: []contacts.Contact{},
		Total:    0,
	}
	_ = out.Contacts
	_ = out.Total
}

// TestContactDetail_Fields_Good — ContactDetail has Contact + Notes.
func TestContactDetail_Fields_Good(t *testing.T) {
	d := contacts.ContactDetail{
		Contact: contacts.Contact{Name: "Ada Penley"},
		Notes:   "some notes",
	}
	_ = d.Contact
	_ = d.Notes
}
