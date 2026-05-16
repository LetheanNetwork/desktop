// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing content calendar surface.
// Manages editorial items at ~/Lethean/marketing/content/{id}.md.
// Each file is a Trix document: YAML frontmatter + markdown body.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Content" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package content

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the marketing content surface.
//
// Usage example:
//
//	svc := content.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the content service against a Core container.
//
// Usage example:
//
//	svc := content.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the content service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-content", content.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Content.List()" etc.
func (s *Service) ServiceName() string { return "Content" }

// colSpec is ordered column metadata.
type colSpec struct {
	ID    string
	Label string
}

// columnOrder returns the canonical ordered column slice.
// The order matches the fixture column order in content.ts.
func columnOrder() []colSpec {
	return []colSpec{
		{ID: "idea", Label: "Ideas"},
		{ID: "draft", Label: "Drafting"},
		{ID: "review", Label: "Review"},
		{ID: "ready", Label: "Ready"},
		{ID: "live", Label: "Live"},
	}
}

// itemFrontmatter is the YAML shape of a content item file.
type itemFrontmatter struct {
	ID   string `yaml:"id"`
	T    string `yaml:"t"`
	Who  string `yaml:"who"`
	When string `yaml:"when"`
	Due  string `yaml:"due"`
	Col  string `yaml:"col"`
}

// contentDir resolves ~/Lethean/marketing/content/ and creates it if missing.
func contentDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "content")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugifyContent converts a title to a filesystem-safe slug.
func slugifyContent(title string) string {
	out := make([]byte, 0, len(title))
	for i := 0; i < len(title); i++ {
		b := title[i]
		if b >= 'A' && b <= 'Z' {
			out = append(out, b+32)
		} else if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' {
			out = append(out, b)
		} else if b == ' ' || b == '_' {
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// parseItem extracts frontmatter + body from a Trix-formatted file.
func parseItem(raw []byte) (ContentItem, error) {
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

	var fm itemFrontmatter
	fmBytes := content
	body := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return ContentItem{}, core.E("content.parseItem", "yaml unmarshal", err)
	}
	return ContentItem{
		ID:   fm.ID,
		T:    fm.T,
		Who:  fm.Who,
		When: fm.When,
		Due:  fm.Due,
		Col:  fm.Col,
		Body: body,
	}, nil
}

// writeItem serialises a ContentItem to Trix format and writes it to disk.
func writeItem(dir string, item ContentItem) core.Result {
	fm := itemFrontmatter{
		ID:   item.ID,
		T:    item.T,
		Who:  item.Who,
		When: item.When,
		Due:  item.Due,
		Col:  item.Col,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("content.writeItem", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if item.Body != "" {
		data = append(data, '\n')
		data = append(data, []byte(item.Body)...)
	}
	fpath := core.PathJoin(dir, item.ID+".md")
	if r := core.WriteFile(fpath, data, 0o644); !r.OK {
		return r
	}
	return core.Ok(nil)
}

// loadItems scans ~/Lethean/marketing/content/ and returns all parseable
// item records. Skips malformed files silently.
func loadItems() ([]ContentItem, error) {
	dirR := contentDir()
	if !dirR.OK {
		return nil, core.E("content.loadItems", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var items []ContentItem
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
		item, err := parseItem(raw.Value.([]byte))
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// nextCol returns the column ID following the given one in the canonical order.
// Returns empty string if already at "live".
func nextCol(current string) string {
	order := columnOrder()
	for i, spec := range order {
		if spec.ID == current && i+1 < len(order) {
			return order[i+1].ID
		}
	}
	return ""
}

// fireContentEvent publishes a content event on the Core ACTION bus.
func (s *Service) fireContentEvent(eventName, itemID, col string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(ContentEvent{
		EventName: eventName,
		ItemID:    itemID,
		Col:       col,
		At:        core.Now().UTC(),
	})
}
