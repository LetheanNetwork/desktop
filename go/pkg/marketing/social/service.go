// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing social post queue.
// Manages posts at ~/Lethean/marketing/social/{id}.md.
// Each file is a Trix document: YAML frontmatter + markdown body (the post text).
//
// Channels are stored as a comma-separated string in frontmatter and split
// back to []string on parse — avoids YAML array quoting complexity while
// keeping the file human-readable.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Social" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package social

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the marketing social surface.
//
// Usage example:
//
//	svc := social.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the social service against a Core container.
//
// Usage example:
//
//	svc := social.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the social service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-social", social.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Social.List()" etc.
func (s *Service) ServiceName() string { return "Social" }

// postFrontmatter is the YAML shape stored in each social post file.
type postFrontmatter struct {
	ID     string `yaml:"id"`
	Ch     string `yaml:"ch"` // comma-separated channels
	When   string `yaml:"when"`
	State  string `yaml:"state"`
	Attach string `yaml:"attach"`
}

// socialDir resolves ~/Lethean/marketing/social/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): unsent post drafts + scheduled
// content shape — owner-only at rest.
func socialDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "social")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// splitChannels splits a comma-separated channel string into a slice.
// Returns nil (not empty slice) when the input is empty.
func splitChannels(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			part := trimSpace(raw[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// trimSpace trims leading and trailing ASCII spaces from s.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && s[start] == ' ' {
		start++
	}
	end := len(s)
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

// joinChannels joins a channel slice into a comma-separated string.
func joinChannels(ch []string) string {
	if len(ch) == 0 {
		return ""
	}
	n := len(ch) - 1
	for _, c := range ch {
		n += len(c)
	}
	b := make([]byte, 0, n)
	for i, c := range ch {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, c...)
	}
	return string(b)
}

// parsePost extracts frontmatter + text body from a Trix-formatted file.
func parsePost(raw []byte) (SocialPost, error) {
	content := raw

	open := []byte("---\n")
	if len(content) >= len(open) {
		match := true
		for i, b := range open {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(open):]
		}
	}

	closeIdx := -1
	for i := 0; i < len(content)-2; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var fm postFrontmatter
	fmBytes := content
	text := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		// Strip the newline written immediately after "---" by writePost.
		for len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		// Strip trailing newline.
		for len(rest) > 0 && rest[len(rest)-1] == '\n' {
			rest = rest[:len(rest)-1]
		}
		text = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return SocialPost{}, core.E("social.parsePost", "yaml unmarshal", err)
	}
	return SocialPost{
		ID:     fm.ID,
		Ch:     splitChannels(fm.Ch),
		When:   fm.When,
		State:  fm.State,
		Text:   text,
		Attach: fm.Attach,
	}, nil
}

// writePost serialises a SocialPost to Trix format and writes it to disk.
// Post text lives in the body (not frontmatter) to avoid YAML quoting of
// newlines and special characters.
// Cerberus #1486: p.ID lands directly in the filename — validate.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest.
func writePost(dir string, p SocialPost) core.Result {
	if err := paths.IsValidID(p.ID); err != nil {
		return core.Fail(err)
	}
	fm := postFrontmatter{
		ID:     p.ID,
		Ch:     joinChannels(p.Ch),
		When:   p.When,
		State:  p.State,
		Attach: p.Attach,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("social.writePost", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if p.Text != "" {
		data = append(data, '\n')
		data = append(data, []byte(p.Text)...)
	}
	fpath := core.PathJoin(dir, p.ID+".md")
	if r := core.WriteFile(fpath, data, 0o600); !r.OK {
		return r
	}
	return core.Ok(nil)
}

// loadPosts scans ~/Lethean/marketing/social/ and returns all parseable
// post records. Skips malformed files silently.
func loadPosts() ([]SocialPost, error) {
	dirR := socialDir()
	if !dirR.OK {
		return nil, core.E("social.loadPosts", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var posts []SocialPost
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		raw := core.ReadFile(core.PathJoin(dir, name))
		if !raw.OK {
			continue
		}
		p, err := parsePost(raw.Value.([]byte))
		if err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// containsChannel returns true if ch slice contains the given channel.
func containsChannel(ch []string, target string) bool {
	for _, c := range ch {
		if c == target {
			return true
		}
	}
	return false
}

// fireSocialEvent publishes a social event on the Core ACTION bus.
func (s *Service) fireSocialEvent(eventName, postID, state string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(SocialEvent{
		EventName: eventName,
		PostID:    postID,
		State:     state,
		At:        core.Now().UTC(),
	})
}
