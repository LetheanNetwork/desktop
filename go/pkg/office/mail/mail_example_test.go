// SPDX-Licence-Identifier: EUPL-1.2

package mail_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/mail"
)

// ExampleService_ListFolders shows how to list all mailbox folders.
func ExampleService_ListFolders() {
	c := core.New()
	svc := mail.NewService(c)

	r := svc.ListFolders()
	if !r.OK {
		return
	}
	out := r.Value.(mail.ListFoldersOutput)
	for _, f := range out.Folders {
		_ = f.Label  // "Inbox"
		_ = f.Unread // 24
	}
}

// ExampleService_ListThreads shows how to list threads for a folder.
func ExampleService_ListThreads() {
	c := core.New()
	svc := mail.NewService(c)

	r := svc.ListThreads(mail.ListThreadsInput{FolderSlug: "inbox", Limit: 20})
	if !r.OK {
		return
	}
	out := r.Value.(mail.ListThreadsOutput)
	for _, t := range out.Threads {
		_ = t.From // "Ada Penley"
		_ = t.Subj // "Re: SOW v2"
	}
}
