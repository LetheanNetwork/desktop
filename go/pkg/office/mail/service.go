// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the v1 mailbox catalogue. Reads
// ~/Lethean/office/mail/{folder-slug}/threads.md files (YAML thread
// frontmatter blocks) and returns typed structs to the Wails frontend.
// Read-only in v1; no IMAP fetch or SMTP send.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Mail" for the Wails namespace
//
// All I/O via CoreGO wrappers: core.ReadFile / core.MkdirAll /
// core.ReadDir / core.DirFS. Banned: os, path/filepath, strings,
// encoding/json, fmt, log, errors.

package mail

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the Office mail catalogue surface.
//
// Usage example:
//
//	svc := mail.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the mail service against a Core container.
// Wired via core.WithName("office-mail", mail.Register) in app.go.
//
// Usage example:
//
//	svc := mail.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the mail service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("office-mail", mail.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Mail.ListFolders()" etc.
func (s *Service) ServiceName() string { return "Mail" }

// mailDir resolves ~/Lethean/office/mail/ and creates it if missing.
// Mode 0o700 — mail metadata is PII (Cerberus #1487 mandate).
func mailDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "office", "mail")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// folderDir resolves ~/Lethean/office/mail/{slug}/ and creates it.
// Validates slug via paths.IsValidID before path join.
func folderDir(slug string) core.Result {
	if err := paths.IsValidID(slug); err != nil {
		return core.Fail(core.E("mail.folderDir", "invalid folder slug", err))
	}
	base := mailDir()
	if !base.OK {
		return base
	}
	dir := core.PathJoin(base.Value.(string), slug)
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// threadsFilePath returns the absolute path to a folder's threads.md.
// Does not create the file.
func threadsFilePath(folderSlug string) core.Result {
	dirR := folderDir(folderSlug)
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), "threads.md"))
}

// parseThreads decodes a threads.md file (list of YAML documents
// separated by "---" delimiters) into a slice of MailThreadRecord.
// Empty blocks are skipped; malformed blocks are warned and skipped.
func parseThreads(raw []byte) ([]MailThreadRecord, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Split on "---" delimiter lines. The file format is a series of
	// YAML documents; each block starts with "---\n" and may or may
	// not end with one.
	var blocks [][]byte
	var current []byte
	i := 0
	for i < len(raw) {
		// Find end of line.
		lineEnd := i
		for lineEnd < len(raw) && raw[lineEnd] != '\n' {
			lineEnd++
		}
		line := raw[i:lineEnd]

		// Check if line is exactly "---".
		isDivider := len(line) == 3 && line[0] == '-' && line[1] == '-' && line[2] == '-'
		if line != nil && len(line) > 0 && line[len(line)-1] == '\r' {
			// strip CR
			isDivider = len(line) == 4 && line[0] == '-' && line[1] == '-' && line[2] == '-'
		}
		if isDivider {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
		} else {
			current = append(current, line...)
			current = append(current, '\n')
		}
		i = lineEnd + 1
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	var records []MailThreadRecord
	for _, block := range blocks {
		// Skip purely whitespace blocks.
		allSpace := true
		for _, b := range block {
			if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
				allSpace = false
				break
			}
		}
		if allSpace {
			continue
		}
		var rec MailThreadRecord
		if err := yaml.Unmarshal(block, &rec); err != nil {
			core.Warn("mail: malformed thread block skipped", "err", err)
			continue
		}
		if rec.ID == "" && rec.From == "" {
			// Completely empty decode — skip.
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// relativeWhen formats a timestamp as:
// "now" (< 5 min), "HH:MM" (today), "yest" (yesterday), "X d" (older).
// Matches the fixture strings in frontend/src/lit/views/office/mail.ts.
func relativeWhen(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour

	switch {
	case diff < 5*minute:
		return "now"
	case diff < 20*hour:
		// Same calendar day — show HH:MM.
		h, m, _ := t.Clock()
		return core.Sprintf("%02d:%02d", h, m)
	case diff < 48*hour:
		return "yest"
	default:
		return core.Sprintf("%d d", int(diff/day))
	}
}

// toThread converts a MailThreadRecord to the wire type.
func toThread(r MailThreadRecord, now core.Time) MailThread {
	return MailThread{
		ID:     r.ID,
		From:   r.From,
		Subj:   r.Subj,
		When:   relativeWhen(r.LastTouched, now),
		Unread: r.Unread,
		Body:   r.Snippet,
	}
}

// scanFolders reads all subdirectories of mailDir() and returns a
// MailFolder slice. Inbox is promoted to index 0 if present; others are
// alphabetical. Unread count is derived from each folder's threads.md.
func scanFolders(activeFolderSlug string) ([]MailFolder, error) {
	base := mailDir()
	if !base.OK {
		return nil, core.E("mail.scanFolders", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	entriesR := core.ReadDir(core.DirFS(baseDir), ".")
	if !entriesR.OK {
		// No mail dir yet — return empty.
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var folders []MailFolder
	var inbox *MailFolder

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()

		// Count unread threads.
		threadsPath := core.PathJoin(baseDir, slug, "threads.md")
		unread := 0
		rawR := core.ReadFile(threadsPath)
		if rawR.OK {
			raw, _ := rawR.Value.([]byte)
			recs, _ := parseThreads(raw)
			for _, r := range recs {
				if r.Unread {
					unread++
				}
			}
		}

		f := MailFolder{
			Label:  folderLabel(slug),
			Slug:   slug,
			Unread: unread,
			Active: slug == activeFolderSlug,
		}
		if slug == "inbox" {
			inbox = &f
		} else {
			folders = append(folders, f)
		}
	}

	// Promote inbox to front.
	if inbox != nil {
		result := []MailFolder{*inbox}
		result = append(result, folders...)
		return result, nil
	}
	return folders, nil
}
