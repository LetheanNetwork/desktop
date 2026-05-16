// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the Office mail catalogue service. Methods are bound
// on the *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/office/mail/.

package mail

import (
	core "dappco.re/go"
)

// ListFolders returns all mail folders found in ~/Lethean/office/mail/.
// Returns an empty list (not an error) when the mail directory is empty
// or does not exist yet.
//
// Usage example:
//
//	r := svc.ListFolders()
//	if r.OK { out := r.Value.(mail.ListFoldersOutput) }
func (s *Service) ListFolders() core.Result {
	folders, err := scanFolders("")
	if err != nil {
		return core.Fail(core.E("mail.ListFolders", "scan failed", err))
	}
	if folders == nil {
		folders = []MailFolder{}
	}
	return core.Ok(ListFoldersOutput{Folders: folders})
}

// ListThreads returns threads for the given folder, sorted newest-first
// by LastTouched, capped at input.Limit (default 50).
//
// Usage example:
//
//	r := svc.ListThreads(mail.ListThreadsInput{FolderSlug: "inbox", Limit: 20})
//	if r.OK { out := r.Value.(mail.ListThreadsOutput) }
func (s *Service) ListThreads(input ListThreadsInput) core.Result {
	if !isValidSlug(input.FolderSlug) {
		return core.Fail(core.E("mail.ListThreads", "invalid folderSlug: "+input.FolderSlug, nil))
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	pathR := threadsFilePath(input.FolderSlug)
	if !pathR.OK {
		return core.Fail(core.E("mail.ListThreads", pathR.Error(), nil))
	}
	threadsPath := pathR.Value.(string)

	rawR := core.ReadFile(threadsPath)
	var records []MailThreadRecord
	if rawR.OK {
		raw, _ := rawR.Value.([]byte)
		recs, err := parseThreads(raw)
		if err != nil {
			return core.Fail(core.E("mail.ListThreads", "parse failed", err))
		}
		records = recs
	}
	// Empty threads.md or not-found → empty result, not an error.

	// Sort by LastTouched descending (newest first).
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && records[j].LastTouched.After(records[j-1].LastTouched); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}

	total := len(records)
	unreadCount := 0
	for _, r := range records {
		if r.Unread {
			unreadCount++
		}
	}

	now := core.Now()
	threads := make([]MailThread, 0, len(records))
	for _, r := range records {
		threads = append(threads, toThread(r, now))
	}
	if len(threads) > limit {
		threads = threads[:limit]
	}

	return core.Ok(ListThreadsOutput{
		Threads: threads,
		Total:   total,
		Unread:  unreadCount,
	})
}
