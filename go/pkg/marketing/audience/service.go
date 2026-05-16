// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing audience segment surface.
// Manages subscriber segments at ~/Lethean/marketing/audience/{slug}.md.
// Each file is a Trix document: YAML frontmatter + markdown body (notes).
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Audience" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package audience

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the marketing audience surface.
//
// Usage example:
//
//	svc := audience.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the audience service against a Core container.
//
// Usage example:
//
//	svc := audience.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the audience service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-audience", audience.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Audience.List()" etc.
func (s *Service) ServiceName() string { return "Audience" }

// segmentFrontmatter is the YAML shape stored in each segment file.
type segmentFrontmatter struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	N      int    `yaml:"n"`
	Growth string `yaml:"growth"`
	Src    string `yaml:"src"`
	Spark  string `yaml:"spark"`
}

// audienceDir resolves ~/Lethean/marketing/audience/ and creates it if missing.
func audienceDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "audience")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugifyAudience converts a segment name to a filesystem-safe slug.
func slugifyAudience(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		b := name[i]
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

// parseSegment extracts frontmatter from a Trix-formatted file.
func parseSegment(raw []byte) (Segment, error) {
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

	var fm segmentFrontmatter
	fmBytes := content
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return Segment{}, core.E("audience.parseSegment", "yaml unmarshal", err)
	}
	return Segment{
		ID:     fm.ID,
		Name:   fm.Name,
		N:      fm.N,
		Growth: fm.Growth,
		Src:    fm.Src,
		Spark:  fm.Spark,
	}, nil
}

// writeSegment serialises a Segment to Trix format and writes it to disk.
func writeSegment(dir string, seg Segment) core.Result {
	fm := segmentFrontmatter{
		ID:     seg.ID,
		Name:   seg.Name,
		N:      seg.N,
		Growth: seg.Growth,
		Src:    seg.Src,
		Spark:  seg.Spark,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("audience.writeSegment", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	fpath := core.PathJoin(dir, seg.ID+".md")
	if r := core.WriteFile(fpath, data, 0o644); !r.OK {
		return r
	}
	return core.Ok(nil)
}

// loadSegments scans ~/Lethean/marketing/audience/ and returns all parseable
// segment records with the "all" segment sorted first.
func loadSegments() ([]Segment, error) {
	dirR := audienceDir()
	if !dirR.OK {
		return nil, core.E("audience.loadSegments", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var allSeg *Segment
	var rest []Segment

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
		seg, err := parseSegment(raw.Value.([]byte))
		if err != nil {
			continue
		}
		if seg.Src == "all" || seg.ID == "all-subscribers" || seg.Name == "All subscribers" {
			cp := seg
			allSeg = &cp
		} else {
			rest = append(rest, seg)
		}
	}

	var out []Segment
	if allSeg != nil {
		out = append(out, *allSeg)
	}
	out = append(out, rest...)
	return out, nil
}

// fireAudienceEvent publishes an audience event on the Core ACTION bus.
func (s *Service) fireAudienceEvent(eventName, segmentID string, n int) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(AudienceEvent{
		EventName: eventName,
		SegmentID: segmentID,
		N:         n,
		At:        core.Now().UTC(),
	})
}
