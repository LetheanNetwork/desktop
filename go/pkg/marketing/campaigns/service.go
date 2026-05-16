// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing campaigns surface.
// Manages campaign threads at ~/Lethean/marketing/campaigns/{slug}.md.
// Each file is a Trix document: YAML frontmatter + markdown body.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Campaigns" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package campaigns

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the marketing campaigns surface.
//
// Usage example:
//
//	svc := campaigns.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the campaigns service against a Core container.
//
// Usage example:
//
//	svc := campaigns.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the campaigns service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-campaigns", campaigns.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Campaigns.List()" etc.
func (s *Service) ServiceName() string { return "Campaigns" }

// campaignFrontmatter is the minimal shape parsed from each campaign file.
//
// Cascade W2 (RFC §B.3 row 7) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write through writeCampaign stamps version=1.
type campaignFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	State   string `yaml:"state"`
	Reach   string `yaml:"reach"`
	Convert string `yaml:"convert"`
	Spend   string `yaml:"spend"`
	Channel string `yaml:"channel"`
}

// campaignsDir resolves ~/Lethean/marketing/campaigns/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): campaign bodies + spend + reach
// metrics — owner-only at rest.
func campaignsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "campaigns")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugify converts a name to a filesystem-safe slug: lowercase, spaces to
// hyphens, strips non-alphanumeric except hyphens.
func slugify(name string) string {
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
		// all other chars dropped
	}
	// trim trailing hyphen
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// parseCampaign extracts frontmatter + body from a Trix-formatted file.
func parseCampaign(raw []byte) (Campaign, error) {
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

	var fm campaignFrontmatter
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
		return Campaign{}, core.E("campaigns.parseCampaign", "yaml unmarshal", err)
	}
	return Campaign{
		ID:      fm.ID,
		Name:    fm.Name,
		State:   fm.State,
		Reach:   fm.Reach,
		Convert: fm.Convert,
		Spend:   fm.Spend,
		Channel: fm.Channel,
		Body:    body,
		Version: fm.Version,
	}, nil
}

// writeCampaign serialises a Campaign to Trix format and writes it via
// paths.AtomicWriteWithVersion (Cascade W2, RFC §B.3 row 7).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseCampaign), or 0 for first-writes /
// legacy-file upgrades. writeCampaign stamps the next monotonic
// version (ifVersion+1) into the marshalled frontmatter so subsequent
// reads see version=1,2,3... monotonically.
//
// Cerberus #1486: c.ID lands directly in the filename — validate.
//
// Return shape (Mantis #1544 gating, inherited from W1): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "campaigns.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeCampaign(dir, c, prior.Version); !wr.OK {
//	    return wr
//	}
func writeCampaign(dir string, c Campaign, ifVersion int) core.Result {
	if err := paths.IsValidID(c.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := campaignFrontmatter{
		Version: nextVersion,
		ID:      c.ID,
		Name:    c.Name,
		State:   c.State,
		Reach:   c.Reach,
		Convert: c.Convert,
		Spend:   c.Spend,
		Channel: c.Channel,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("campaigns.writeCampaign", "yaml marshal", err))
	}
	content := append([]byte("---\n"), fmBytes...)
	content = append(content, []byte("---\n")...)
	if c.Body != "" {
		content = append(content, '\n')
		content = append(content, []byte(c.Body)...)
	}
	fpath := core.PathJoin(dir, c.ID+".md")
	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      content,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"campaigns.update.conflict", stale))
	}
	return core.Fail(core.E("campaigns.writeCampaign",
		"write failed: "+res.Error(), nil))
}

// loadCampaigns scans ~/Lethean/marketing/campaigns/ and returns all parseable
// campaign records. Skips malformed files silently.
func loadCampaigns() ([]Campaign, error) {
	dirR := campaignsDir()
	if !dirR.OK {
		return nil, core.E("campaigns.loadCampaigns", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var cs []Campaign
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
		c, err := parseCampaign(raw.Value.([]byte))
		if err != nil {
			continue
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// fireCampaignEvent publishes a campaign event on the Core ACTION bus.
func (s *Service) fireCampaignEvent(eventName, campaignID string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(CampaignEvent{
		EventName:  eventName,
		CampaignID: campaignID,
		At:         core.Now().UTC(),
	})
}
