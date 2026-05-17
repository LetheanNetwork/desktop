// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the campaigns service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketing/campaigns/service.

package campaigns

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all campaigns, optionally filtered by state.
//
// Usage example:
//
//	r := svc.List(campaigns.ListInput{})
//	if r.OK { out := r.Value.(campaigns.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	cs, err := loadCampaigns()
	if err != nil {
		return core.Fail(core.E("campaigns.List", "scan failed", err))
	}

	liveCount := 0
	scheduledCount := 0
	for _, c := range cs {
		if c.State == "live" {
			liveCount++
		}
		if c.State == "scheduled" {
			scheduledCount++
		}
	}

	filtered := cs
	if input.State != "" {
		filtered = make([]Campaign, 0, len(cs))
		for _, c := range cs {
			if c.State == input.State {
				filtered = append(filtered, c)
			}
		}
	}

	if filtered == nil {
		filtered = []Campaign{}
	}

	return core.Ok(ListOutput{
		Campaigns:      filtered,
		LiveCount:      liveCount,
		ScheduledCount: scheduledCount,
	})
}

// Get returns a single campaign by ID.
//
// Usage example:
//
//	r := svc.Get("v02-launch-20260516")
//	if r.OK { c := r.Value.(campaigns.Campaign) }
func (s *Service) Get(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	dirR := campaignsDir()
	if !dirR.OK {
		return core.Fail(core.E("campaigns.Get", dirR.Error(), nil))
	}
	// Cerberus #1486 belt: WithinDir check after the join.
	fpath, jerr := paths.JoinAndCheck(dirR.Value.(string), id+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("campaigns.Get", "not found: "+id, nil))
	}
	c, err := parseCampaign(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("campaigns.Get", "parse failed", err))
	}
	return core.Ok(c)
}

// Create creates a new campaign record with sensible defaults.
//
// Usage example:
//
//	r := svc.Create(campaigns.CreateInput{Name: "Product Hunt launch", Channel: "earned"})
//	if r.OK { c := r.Value.(campaigns.Campaign) }
func (s *Service) Create(input CreateInput) core.Result {
	if input.Name == "" {
		return core.Fail(core.E("campaigns.Create", "name is required", nil))
	}
	dirR := campaignsDir()
	if !dirR.OK {
		return core.Fail(core.E("campaigns.Create", dirR.Error(), nil))
	}

	// Apply defaults.
	state := input.State
	if state == "" {
		state = "draft"
	}
	reach := input.Reach
	if reach == "" {
		reach = "—"
	}
	conv := input.Convert
	if conv == "" {
		conv = "—"
	}
	spend := input.Spend
	if spend == "" {
		spend = "£0"
	}
	channel := input.Channel
	if channel == "" {
		channel = "earned"
	}

	ts := core.Now().UTC().Unix()
	slug := slugify(input.Name)
	if slug == "" {
		slug = "campaign"
	}
	id := core.Sprintf("%s-%d", slug, ts)

	c := Campaign{
		ID:      id,
		Name:    input.Name,
		State:   state,
		Reach:   reach,
		Convert: conv,
		Spend:   spend,
		Channel: channel,
		Body:    input.Body,
	}

	// Cascade W2 (RFC §B.3 row 7) — Create is an unconditional first-
	// write (ifVersion=0). writeCampaign stamps Version=1 into the
	// marshalled frontmatter. Conflict-path (rare on Create — only
	// fires if another goroutine races on the same slug) returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeCampaign.
	if wr := writeCampaign(dirR.Value.(string), c, 0); !wr.OK {
		return wr
	}
	c.Version = 1
	s.fireCampaignEvent(EventCampaignCreated, id)
	return core.Ok(c)
}

// Update applies patch fields to an existing campaign record.
//
// Usage example:
//
//	r := svc.Update(campaigns.UpdateInput{ID: "v02-launch-20260516", State: "complete"})
//	if r.OK { c := r.Value.(campaigns.Campaign) }
func (s *Service) Update(input UpdateInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	dirR := campaignsDir()
	if !dirR.OK {
		return core.Fail(core.E("campaigns.Update", dirR.Error(), nil))
	}

	// Cerberus #1486 belt: WithinDir check after the join.
	fpath, jerr := paths.JoinAndCheck(dirR.Value.(string), input.ID+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("campaigns.Update", "not found: "+input.ID, nil))
	}
	c, err := parseCampaign(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("campaigns.Update", "parse failed", err))
	}

	priorVersion := c.Version

	// Patch non-empty fields.
	if input.State != "" {
		c.State = input.State
	}
	if input.Reach != "" {
		c.Reach = input.Reach
	}
	if input.Convert != "" {
		c.Convert = input.Convert
	}
	if input.Spend != "" {
		c.Spend = input.Spend
	}
	if input.Channel != "" {
		c.Channel = input.Channel
	}
	if input.Body != "" {
		c.Body = input.Body
	}

	// Cascade W2 (RFC §B.3 row 7) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check.
	// priorVersion=0 (legacy file predating cutover) skips the check
	// and stamps Version=1 on the upgrade write. Conflict-path returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeCampaign
	// (Mantis #1544 gating shape inherited from W1).
	if wr := writeCampaign(dirR.Value.(string), c, priorVersion); !wr.OK {
		return wr
	}
	c.Version = priorVersion + 1
	if c.Version < 1 {
		c.Version = 1
	}
	s.fireCampaignEvent(EventCampaignUpdated, c.ID)
	return core.Ok(c)
}
