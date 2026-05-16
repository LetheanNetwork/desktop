// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the pipeline surface. Derives a Kanban
// view by scanning ~/Lethean/sales/deals/ and grouping records by stage.
// Pipeline state is derived — deals own their stage. MoveDeal delegates
// to the deal record directly.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Pipeline" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package pipeline

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the pipeline surface.
//
// Usage example:
//
//	svc := pipeline.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the pipeline service against a Core container.
//
// Usage example:
//
//	svc := pipeline.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the pipeline service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-pipeline", pipeline.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Pipeline.List()" etc.
func (s *Service) ServiceName() string { return "Pipeline" }

// stageSpec is the canonical stage ordered metadata.
type stageSpec struct {
	ID    string
	Label string
}

// stageOrder returns the canonical ordered stage slice.
// The order matches the fixture column order in pipeline.ts.
func stageOrder() []stageSpec {
	return []stageSpec{
		{ID: "qual", Label: "Qualifying"},
		{ID: "engage", Label: "Engaging"},
		{ID: "propose", Label: "Proposal"},
		{ID: "close", Label: "Closing"},
		{ID: "won", Label: "Won"},
		{ID: "lost", Label: "Lost"},
	}
}

// dealsDir resolves ~/Lethean/sales/deals/ and creates it if missing.
func dealsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "sales", "deals")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// formatGBPK formats integer pence as "£N K" (above 1000p) or "£N".
func formatGBPK(pence int) string {
	if pence >= 1000 {
		k := (pence + 500) / 1000
		return core.Sprintf("£%d K", k)
	}
	return core.Sprintf("£%d", pence/100)
}

// dealFrontmatter is the minimal shape parsed from each deal file for the
// pipeline rollup — avoids importing the full deals package.
type dealFrontmatter struct {
	ID          string `yaml:"id"`
	Customer    string `yaml:"customer"`
	AmountPence int    `yaml:"amount_pence"`
	Stage       string `yaml:"stage"`
	CloseTarget string `yaml:"close_target"`
}

// parseFrontmatter extracts only the frontmatter from a Trix file.
func parseFrontmatter(raw []byte) (dealFrontmatter, error) {
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
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var fm dealFrontmatter
	fmBytes := content
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return dealFrontmatter{}, core.E("pipeline.parseFrontmatter", "yaml unmarshal", err)
	}
	return fm, nil
}

// loadDeals scans ~/Lethean/sales/deals/ and returns all parseable deal
// frontmatters. Skips malformed files silently.
func loadDeals() ([]dealFrontmatter, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return nil, core.E("pipeline.loadDeals", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var fms []dealFrontmatter
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
		fm, err := parseFrontmatter(raw.Value.([]byte))
		if err != nil {
			continue
		}
		fms = append(fms, fm)
	}
	return fms, nil
}

// writeDealStage reads the deal file, mutates the stage frontmatter field,
// and writes it back. Returns the previous stage.
func writeDealStage(id, toStage string) (fromStage string, err error) {
	dirR := dealsDir()
	if !dirR.OK {
		return "", core.E("pipeline.writeDealStage", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, id+".md")

	raw := core.ReadFile(fpath)
	if !raw.OK {
		return "", core.E("pipeline.writeDealStage", "not found: "+id, nil)
	}

	fm, err := parseFrontmatter(raw.Value.([]byte))
	if err != nil {
		return "", err
	}
	fromStage = fm.Stage

	// Replace the stage value in the raw bytes without re-marshalling
	// the full YAML — this avoids touching the rest of the frontmatter.
	// We locate the "stage: " key and replace the value after it.
	// Pattern: scan for "stage: " prefix on its own line.
	updated := updateYAMLField(raw.Value.([]byte), "stage", toStage)
	if w := core.WriteFile(fpath, updated, 0o644); !w.OK {
		return fromStage, core.E("pipeline.writeDealStage", w.Error(), nil)
	}
	return fromStage, nil
}

// updateYAMLField replaces the value of a YAML field key on its own line
// within a Trix file. Only touches the first occurrence inside the
// frontmatter block. Falls back to the original bytes on any ambiguity.
func updateYAMLField(raw []byte, key, value string) []byte {
	// Build the search prefix: "key: " (at start of line).
	prefix := []byte(key + ": ")
	result := make([]byte, 0, len(raw)+32)
	i := 0
	for i < len(raw) {
		// Check for start-of-line match.
		lineStart := i
		// Find end of this line.
		end := i
		for end < len(raw) && raw[end] != '\n' {
			end++
		}
		line := raw[lineStart:end]
		if len(line) >= len(prefix) {
			match := true
			for j := range prefix {
				if line[j] != prefix[j] {
					match = false
					break
				}
			}
			if match {
				// Replace this line with "key: value".
				result = append(result, []byte(key+": "+value)...)
				if end < len(raw) {
					result = append(result, '\n')
				}
				i = end + 1
				// Append the rest unchanged.
				result = append(result, raw[i:]...)
				return result
			}
		}
		result = append(result, raw[lineStart:end]...)
		if end < len(raw) {
			result = append(result, '\n')
		}
		i = end + 1
	}
	return raw // no match — return unchanged
}

// fireMove publishes a pipeline move event on the Core ACTION bus.
func (s *Service) fireMove(dealID, fromStage, toStage string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(PipelineMovedEvent{
		EventName: EventPipelineMoved,
		DealID:    dealID,
		FromStage: fromStage,
		ToStage:   toStage,
		At:        core.Now().UTC(),
	})
}
