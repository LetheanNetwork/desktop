// SPDX-Licence-Identifier: EUPL-1.2

package contacts_test

import (
	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// ExampleService_List shows the standard list-all invocation.
func ExampleService_List() {
	svc := contacts.NewService(nil)
	r := svc.List(contacts.ListInput{})
	if !r.OK {
		return
	}
	_ = r.Value.(contacts.ListOutput)
}

// ExampleService_Create shows the standard create invocation.
func ExampleService_Create() {
	svc := contacts.NewService(nil)
	r := svc.Create(contacts.CreateInput{
		Name: "Ada Penley",
		Role: "CTO · Heritage Law",
		Next: "call · Fri",
	})
	if !r.OK {
		return
	}
	_ = r.Value.(contacts.Contact)
}
