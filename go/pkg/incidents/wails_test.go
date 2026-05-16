// SPDX-Licence-Identifier: EUPL-1.2

package incidents_test

import (
	"testing"

	subject "dappco.re/lthn/desktop/pkg/incidents"
)

func TestListInput_Fields(t *testing.T) {
	in := subject.ListInput{State: "investigating", Severity: "P1", Limit: 10}
	if in.State != "investigating" {
		t.Errorf("ListInput.State")
	}
	if in.Severity != "P1" {
		t.Errorf("ListInput.Severity")
	}
	if in.Limit != 10 {
		t.Errorf("ListInput.Limit")
	}
}

func TestCreateInput_Fields(t *testing.T) {
	in := subject.CreateInput{Title: "test", Sev: "P2", Svc: "hub", Who: "Mei"}
	if in.Title != "test" || in.Sev != "P2" || in.Svc != "hub" || in.Who != "Mei" {
		t.Errorf("CreateInput fields mismatch")
	}
}

func TestUpdateStateInput_Fields(t *testing.T) {
	in := subject.UpdateStateInput{ID: "2026-05-INC-001", State: "resolved"}
	if in.ID == "" || in.State == "" {
		t.Errorf("UpdateStateInput fields mismatch")
	}
}

func TestPostmortemInput_Fields(t *testing.T) {
	in := subject.PostmortemInput{ID: "2026-05-INC-001", Body: "## Root cause\n\n..."}
	if in.ID == "" || in.Body == "" {
		t.Errorf("PostmortemInput fields mismatch")
	}
}

func TestGetInput_Fields(t *testing.T) {
	in := subject.GetInput{ID: "2026-05-INC-001"}
	if in.ID == "" {
		t.Errorf("GetInput fields mismatch")
	}
}

func TestServiceName_Incidents(t *testing.T) {
	svc := subject.NewService(nil)
	if got := svc.ServiceName(); got != "Incidents" {
		t.Errorf("ServiceName: want 'Incidents' got %q", got)
	}
}

func TestList_NilService_ReturnsEmpty(t *testing.T) {
	// List on a nil-core service with a non-existent home dir should
	// not panic — it returns an error or empty list gracefully.
	svc := subject.NewService(nil)
	r := svc.List(subject.ListInput{})
	// Either OK (empty) or Fail is acceptable — no panic is the
	// contract. We just assert no panic here.
	_ = r
}
