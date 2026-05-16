// SPDX-Licence-Identifier: EUPL-1.2

package documents_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/documents"
)

// ExampleService_List shows how to call the List method and inspect the result.
func ExampleService_List() {
	c := core.New()
	svc := documents.NewService(c)

	r := svc.List(documents.ListInput{State: "draft"})
	if !r.OK {
		return // handle error
	}
	out := r.Value.(documents.ListOutput)
	for _, d := range out.Docs {
		_ = d.Title // "v0.2 release notes"
		_ = d.State // "draft"
	}
}

// ExampleService_Get shows how to retrieve a single document with its body.
func ExampleService_Get() {
	c := core.New()
	svc := documents.NewService(c)

	r := svc.Get(documents.GetInput{Slug: "release-notes"})
	if !r.OK {
		return // not found or invalid slug
	}
	detail := r.Value.(documents.DocDetail)
	_ = detail.Title  // "Release notes"
	_ = detail.Body   // "# Release notes\n\n..."
}
