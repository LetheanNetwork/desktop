// SPDX-Licence-Identifier: EUPL-1.2

package runbooks_test

import (
	"testing"

	subject "dappco.re/lthn/desktop/pkg/runbooks"
)

func TestListInput_Fields(t *testing.T) {
	in := subject.ListInput{Health: "stale", Area: "mail", Limit: 5}
	if in.Health != "stale" || in.Area != "mail" || in.Limit != 5 {
		t.Errorf("ListInput fields mismatch")
	}
}

func TestGetInput_Fields(t *testing.T) {
	in := subject.GetInput{ID: "R-01", Slug: "rotate-runtime-api-keys"}
	if in.ID == "" || in.Slug == "" {
		t.Errorf("GetInput fields mismatch")
	}
}

func TestSearchInput_Fields(t *testing.T) {
	in := subject.SearchInput{Query: "postfix", Limit: 10}
	if in.Query != "postfix" || in.Limit != 10 {
		t.Errorf("SearchInput fields mismatch")
	}
}

func TestMarkInput_Fields(t *testing.T) {
	in := subject.MarkInput{ID: "R-05"}
	if in.ID == "" {
		t.Errorf("MarkInput fields mismatch")
	}
}

func TestList_NilService_Nonempty(t *testing.T) {
	// List on a nil-core service should not panic — seeding may fail
	// but we should return an empty-ish result, not crash.
	svc := subject.NewService(nil)
	_ = svc.List(subject.ListInput{})
}
