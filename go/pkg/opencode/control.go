// SPDX-Licence-Identifier: EUPL-1.2

// HTTP control surface — POST /v1/api/opencode/sandbox spawns a new
// sandbox; GET /v1/api/opencode/sandbox lists running ones; DELETE
// /v1/api/opencode/sandbox/:id stops one. The CLI subcommand is a
// thin client over these endpoints so opencode lifecycle work always
// happens in the lthn-serve process — same Core, same proxy map.

package opencode

import (

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
)

// Event-name literals for the opencode HTTP control surface. Mantis
// #1602 HIGH (Cerberus #22) — every privilege-bearing endpoint in this
// file emits exactly one audit row per call. Reserved schema; renaming
// a literal without a spec bump breaks the Stage F log-tailer + the
// future Operations panel facet chrome.
//
// Outcome literals are reused from pkg/audit (OutcomeOK / OutcomeError
// / OutcomeDenied / OutcomeFailed) — see types.go in that package.
//
// Per Cerberus #22 + the redact.go secret-shape detector, Meta values
// MUST NEVER carry:
//
//   - OPENCODE_SERVER_PASSWORD bytes (Mantis #1600 keeps it off every
//     wire shape including audit)
//   - profile.Provider blocks (may carry apiKey / token / bearer for
//     upstream providers — only the profile NAME is emitted)
//   - host-config file BYTES (may carry user-supplied provider secrets
//     — only the resulting Path + bool Created flag are emitted)
//
// The redact.go detector enforces this server-side; the emit-sites
// below structurally cannot reach the credential bytes regardless.
const (
	// EventOpencodeSandboxWebURLIssued — webURL handler emits per
	// successful credential-free URL issuance (Mantis #1600 HIGH).
	// Meta: sandbox_id, auth_scheme, auth_via.
	EventOpencodeSandboxWebURLIssued = "opencode.sandbox.web_url_issued"

	// EventOpencodeSandboxSpawn — spawn handler emits per /sandbox POST.
	// Meta: profile, sandbox_id (on OK), error_code (on error).
	EventOpencodeSandboxSpawn = "opencode.sandbox.spawn"

	// EventOpencodeSandboxStop — stop handler emits per /sandbox/:id
	// DELETE. Meta: sandbox_id, error_code (on error).
	EventOpencodeSandboxStop = "opencode.sandbox.stop"

	// EventOpencodeProfileSave — profileSave handler emits per
	// /profile POST. Meta: profile_name, error_code (on error /
	// denied). Profile.Provider block is NEVER in Meta — may carry
	// upstream provider apiKey / token bytes.
	EventOpencodeProfileSave = "opencode.profile.save"

	// EventOpencodeEnable — enable handler emits per /enable POST.
	// Meta: profile, sandbox_id (on OK), error_code (on error).
	EventOpencodeEnable = "opencode.enable"

	// EventOpencodeHostConfigMerge — hostConfigMerge handler emits per
	// /host-config POST. Meta: profile, force, path, created (on OK),
	// error_code (on error / conflict). Bytes payload is NEVER in
	// Meta — may carry user-supplied provider secrets.
	EventOpencodeHostConfigMerge = "opencode.host_config.merge"

	// EventOpencodeTUIOpen — openTUI handler emits per /sandbox/:id/tui
	// POST. Meta: sandbox_id, error_code (on error).
	EventOpencodeTUIOpen = "opencode.tui.open"

	// EventOpencodeUpgrade — upgrade handler emits per /upgrade POST.
	// Meta: updated (bool), digest, restarted (count) on OK; error_code
	// on error.
	EventOpencodeUpgrade = "opencode.upgrade"
)

// ControlGroup implements coreapi.RouteGroup for the opencode HTTP
// control surface.
type ControlGroup struct {
	svc *Service
}

// NewControlGroup binds the route group to an opencode Service.
//
// Usage example:
//
//	engine.Register(opencode.NewControlGroup(opencodeSvc))
func NewControlGroup(svc *Service) *ControlGroup {
	return &ControlGroup{svc: svc}
}

// Name satisfies coreapi.RouteGroup.
func (g *ControlGroup) Name() string { return "opencode" }

// BasePath satisfies coreapi.RouteGroup.
func (g *ControlGroup) BasePath() string { return "/v1/api/opencode" }

// RegisterRoutes satisfies coreapi.RouteGroup.
func (g *ControlGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/sandbox", g.spawn)
	rg.GET("/sandbox", g.list)
	rg.DELETE("/sandbox/:id", g.stop)
	rg.GET("/sandbox/:id", g.inspect)

	// Profile CRUD — per-task config templates stored in the DuckDB
	// profile store; applied to opencode-serve at spawn time via
	// PATCH /global/config. See pkg/opencode/profile.go.
	rg.GET("/profile", g.profileList)
	rg.GET("/profile/:name", g.profileGet)
	rg.POST("/profile", g.profileSave)
	rg.DELETE("/profile/:name", g.profileDelete)

	// Host-config merge — RFC.opencode.md §3.3 "easy mode" path.
	// POSTs into ~/.config/opencode/opencode.json so users running
	// opencode directly on the host pick up the lthn provider.
	rg.POST("/host-config", g.hostConfigMerge)

	// Provider enumeration — RFC.opencode.md §4.3 + §5.1. Returns
	// opencode-serve's /provider response for the named sandbox.
	// Fleet → Agents renders cards from this.
	rg.GET("/sandbox/:id/providers", g.providerList)

	// Enable / Disable — RFC.opencode.md §4.3 + §7. Persist the
	// "should opencode-serve be running" flag + drive lifecycle.
	rg.POST("/enable", g.enable)
	rg.POST("/disable", g.disable)
	rg.GET("/enabled", g.enabled)

	// Open TUI — RFC.opencode.md §6. Spawn opencode inside the
	// user's default terminal, attached to the named sandbox.
	rg.POST("/sandbox/:id/tui", g.openTUI)

	// Open Studio — RFC.opencode.md §6. Launches OpenCode's native
	// desktop app if installed on the host. GET reports presence
	// (so the frontend hides the button when the app isn't there).
	rg.GET("/studio", g.studio)
	rg.POST("/studio", g.openStudio)

	// Upgrade — RFC.opencode.md §7 "Image bump". Pulls the
	// configured image + restarts running sandboxes on the new
	// digest. User-driven; auto-detect notification is v2.
	rg.POST("/upgrade", g.upgrade)

	// Web UI — opencode-web ships an SPA at root in addition to the
	// JSON API endpoints. GET returns the direct-bind URL with Basic
	// auth embedded; POST opens it in an lthn Wails window (requires
	// GUI mode).
	rg.GET("/sandbox/:id/web", g.webURL)
	rg.POST("/sandbox/:id/web", g.openWebWindow)

	// Import — datamine the user's HOST opencode for projects +
	// provider credentials. Source-agnostic orm types so future
	// codex/claude/pi imports reuse the same shape.
	rg.POST("/import", g.importFromHost)
	rg.GET("/imports", g.listImports)
	rg.GET("/imports/providers", g.listImportedProviders)
}

// importFromHost POST /v1/api/opencode/import → spawns host
// `opencode serve`, drains /project + /provider, persists rows.
// Returns ImportSummary.
func (g *ControlGroup) importFromHost(c *gin.Context) {
	r := g.svc.ImportFromHost()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, r.Value)
}

// listImports GET /v1/api/opencode/imports → every imported
// project, most-recent first.
func (g *ControlGroup) listImports(c *gin.Context) {
	r := g.svc.ListImports()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, gin.H{"projects": r.Value})
}

// listImportedProviders GET /v1/api/opencode/imports/providers →
// every imported provider definition + auth metadata. The auth_key
// field IS included (local-only surface) — the frontend MUST treat
// it as sensitive and never render it in cleartext UI.
func (g *ControlGroup) listImportedProviders(c *gin.Context) {
	r := g.svc.ListImportedProviders()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, gin.H{"providers": r.Value})
}

// webURL GET /v1/api/opencode/sandbox/:id/web → returns the direct
// container-port URL plus auth-scheme metadata. CREDENTIAL-FREE per
// Mantis #1600 HIGH (Cerberus #22) — the URL has no embedded
// userinfo; callers inject the credential at navigation time via
// the Authorization header per the WebInfo.Auth envelope.
//
// Emits the EventOpencodeSandboxWebURLIssued audit event on success
// per Cerberus #22 #1602 (audit-gap finding) — narrowed to this
// endpoint; the broader opencode-control audit sweep is a follow-up.
//
// RequestID is server-generated per Cerberus #18 / Mantis #1511 / #1605
// — caller-supplied X-Request-Id is intentionally dropped so an
// attacker cannot mint forged audit-JOIN keys. The server's UUIDv4 is
// echoed in the response X-Request-Id header so the legitimate caller
// can still correlate to the audit log.
func (g *ControlGroup) webURL(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.WebURL(id)
	if !r.OK {
		c.JSON(core.StatusNotFound, gin.H{"error": r.Error()})
		return
	}
	info, _ := r.Value.(WebInfo)
	// Audit emission — failure is intentionally swallowed; audit
	// failures MUST NEVER block an auth-adjacent path. The redact.go
	// secret-shape detector enforces the no-password invariant.
	_ = audit.Default().Record(audit.Event{
		Event:     EventOpencodeSandboxWebURLIssued,
		TS:        core.Now().UTC().Unix(),
		Scope:     "opencode.sandbox.web",
		Outcome:   audit.OutcomeOK,
		RequestID: srvReqID,
		Meta: map[string]any{
			"sandbox_id":  id,
			"auth_scheme": info.Auth.Scheme,
			"auth_via":    info.Auth.Via,
		},
	})
	c.JSON(core.StatusOK, info)
}

// openWebWindow POST /v1/api/opencode/sandbox/:id/web → spawns an
// lthn Wails window pointing at the web UI. Fails when not in
// GUI mode (window.open action isn't registered in serve mode).
func (g *ControlGroup) openWebWindow(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.OpenWebWindow(id)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, r.Value)
}

// upgrade POST /v1/api/opencode/upgrade → pulls lthn/dev:latest +
// restarts any running sandboxes on the new image when the digest
// changed. Returns UpgradeResult (updated flag + new digest +
// list of restarted sandbox ids).
//
// Emits EventOpencodeUpgrade (Mantis #1602 HIGH) per call. RequestID
// server-generated per Cerberus #18 / Mantis #1511.
func (g *ControlGroup) upgrade(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	r := g.svc.Upgrade()
	if !r.OK {
		emitControlAudit(EventOpencodeUpgrade, "opencode.upgrade",
			audit.OutcomeError, srvReqID, map[string]any{
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	res, _ := r.Value.(UpgradeResult)
	emitControlAudit(EventOpencodeUpgrade, "opencode.upgrade",
		audit.OutcomeOK, srvReqID, map[string]any{
			"updated":   res.Updated,
			"digest":    res.Digest,
			"restarted": len(res.Restarted),
		})
	c.JSON(core.StatusOK, res)
}

// openTUI POST /v1/api/opencode/sandbox/:id/tui → spawns the user's
// default terminal running `<runtime> exec -it <container> opencode`.
//
// Emits EventOpencodeTUIOpen (Mantis #1602 HIGH) per call. RequestID
// server-generated per Cerberus #18 / Mantis #1511. The
// OPENCODE_SERVER_PASSWORD that flows into the shell composition
// inside Service.OpenTUI is NEVER in Meta — only the sandbox id.
func (g *ControlGroup) openTUI(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.OpenTUI(id)
	if !r.OK {
		emitControlAudit(EventOpencodeTUIOpen, "opencode.tui.open",
			audit.OutcomeError, srvReqID, map[string]any{
				"sandbox_id": id,
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	emitControlAudit(EventOpencodeTUIOpen, "opencode.tui.open",
		audit.OutcomeOK, srvReqID, map[string]any{
			"sandbox_id": id,
		})
	c.JSON(core.StatusOK, gin.H{"opened": id})
}

// studio GET /v1/api/opencode/studio → reports whether the host's
// OpenCode native app is installed.
func (g *ControlGroup) studio(c *gin.Context) {
	c.JSON(core.StatusOK, gin.H{"installed": g.svc.IsStudioInstalled()})
}

// openStudio POST /v1/api/opencode/studio → launches the host's
// OpenCode native app. 4xx when not installed.
func (g *ControlGroup) openStudio(c *gin.Context) {
	if !g.svc.IsStudioInstalled() {
		c.JSON(core.StatusNotFound, gin.H{
			"error": "OpenCode native app is not installed on this host",
		})
		return
	}
	r := g.svc.OpenStudio()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, gin.H{"opened": true})
}

// enable POST /v1/api/opencode/enable → persists the enabled flag
// + spawns a sandbox if none is running. Optional body {profile}.
//
// Emits EventOpencodeEnable (Mantis #1602 HIGH) per call. RequestID
// server-generated per Cerberus #18 / Mantis #1511.
func (g *ControlGroup) enable(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	var req struct {
		Profile string `json:"profile"`
	}
	_ = c.ShouldBindJSON(&req)
	profile := req.Profile
	if profile == "" {
		profile = DefaultProfile
	}
	r := g.svc.Enable(req.Profile)
	if !r.OK {
		emitControlAudit(EventOpencodeEnable, "opencode.enable",
			audit.OutcomeError, srvReqID, map[string]any{
				"profile":    profile,
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	id, _ := r.Value.(string)
	emitControlAudit(EventOpencodeEnable, "opencode.enable",
		audit.OutcomeOK, srvReqID, map[string]any{
			"profile":    profile,
			"sandbox_id": id,
		})
	c.JSON(core.StatusOK, gin.H{"id": id, "enabled": true})
}

// disable POST /v1/api/opencode/disable → persists the disabled
// flag + stops any running sandboxes.
func (g *ControlGroup) disable(c *gin.Context) {
	r := g.svc.Disable()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, gin.H{"enabled": false})
}

// enabled GET /v1/api/opencode/enabled → returns the persisted
// flag. Cheap — no upstream call.
func (g *ControlGroup) enabled(c *gin.Context) {
	c.JSON(core.StatusOK, gin.H{"enabled": g.svc.IsEnabled()})
}

// providerList GET /v1/api/opencode/sandbox/:id/providers → returns
// opencode-serve's /provider response (raw JSON pass-through).
func (g *ControlGroup) providerList(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.ProviderList(id)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	body, _ := r.Value.(string)
	c.Data(core.StatusOK, "application/json", []byte(body))
}

// hostConfigMerge POST /v1/api/opencode/host-config → merges the
// named profile's provider block into the user's global opencode
// config. Body: MergeHostConfigOptions JSON. Returns
// MergeHostConfigResult on success; 409 Conflict (with the conflict
// code in the body) when provider.lthn already exists with a
// different baseURL and force was not passed.
func (g *ControlGroup) hostConfigMerge(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	var opts MergeHostConfigOptions
	// Body is optional; empty body uses defaults (profile=default,
	// force=false).
	_ = c.ShouldBindJSON(&opts)
	profile := opts.Profile
	if profile == "" {
		profile = DefaultProfile
	}
	r := g.svc.MergeHostConfig(opts)
	if !r.OK {
		// Conflict surfaces as 409 so the frontend can distinguish
		// "needs user confirmation" from "actually broken". Conflict
		// is OutcomeDenied (the user-supplied request was rejected by
		// the substrate); other failures are OutcomeError (substrate
		// itself broke).
		if r.Code() == HostConfigConflict {
			emitControlAudit(EventOpencodeHostConfigMerge, "opencode.host_config.merge",
				audit.OutcomeDenied, srvReqID, map[string]any{
					"profile":    profile,
					"force":      opts.Force,
					"error_code": HostConfigConflict,
				})
			c.JSON(core.StatusConflict, gin.H{
				"error": r.Error(),
				"code":  HostConfigConflict,
			})
			return
		}
		emitControlAudit(EventOpencodeHostConfigMerge, "opencode.host_config.merge",
			audit.OutcomeError, srvReqID, map[string]any{
				"profile":    profile,
				"force":      opts.Force,
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	res, _ := r.Value.(MergeHostConfigResult)
	// Emit success row with the path + created flag — the BYTES of the
	// merged JSON are intentionally NOT in Meta; they may carry the
	// user's provider apiKey / token bytes (see Profile.Provider
	// constraints at the const-block top of this file).
	emitControlAudit(EventOpencodeHostConfigMerge, "opencode.host_config.merge",
		audit.OutcomeOK, srvReqID, map[string]any{
			"profile": profile,
			"force":   opts.Force,
			"path":    res.Path,
			"created": res.Created,
		})
	c.JSON(core.StatusOK, res)
}

// spawn POST /v1/api/opencode/sandbox → spawns a new container.
// Optional JSON body: {"profile": "<name>"} — selects the lthn-side
// opencode profile to apply via PATCH /config after spawn. Empty
// or missing body uses "default".
//
// Returns {id, url, profile} on success.
//
// Emits EventOpencodeSandboxSpawn (Mantis #1602 HIGH) per call —
// OK on success with the resolved {profile, sandbox_id}; error with
// {profile, error_code} on Service.Start failure. RequestID is
// server-generated per Mantis #1511 / Cerberus #18 X-Request-Id
// discipline.
func (g *ControlGroup) spawn(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	var req struct {
		Profile string `json:"profile"`
	}
	// Body is optional; bind failures (empty body / wrong shape)
	// fall through to default profile.
	_ = c.ShouldBindJSON(&req)
	profile := req.Profile
	if profile == "" {
		profile = DefaultProfile
	}
	r := g.svc.Start(req.Profile)
	if !r.OK {
		emitControlAudit(EventOpencodeSandboxSpawn, "opencode.spawn",
			audit.OutcomeError, srvReqID, map[string]any{
				"profile":    profile,
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	id, _ := r.Value.(string)
	emitControlAudit(EventOpencodeSandboxSpawn, "opencode.spawn",
		audit.OutcomeOK, srvReqID, map[string]any{
			"profile":    profile,
			"sandbox_id": id,
		})
	c.JSON(core.StatusOK, gin.H{
		"id":      id,
		"url":     "/v1/api/sandbox/" + id,
		"profile": profile,
	})
}

// list GET /v1/api/opencode/sandbox → returns all running sandboxes.
func (g *ControlGroup) list(c *gin.Context) {
	r := g.svc.Status()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	list, _ := r.Value.([]Sandbox)
	c.JSON(core.StatusOK, gin.H{"sandboxes": list})
}

// stop DELETE /v1/api/opencode/sandbox/:id → stops + removes one.
//
// Emits EventOpencodeSandboxStop (Mantis #1602 HIGH) per call.
// RequestID server-generated per Cerberus #18 / Mantis #1511.
func (g *ControlGroup) stop(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.Stop(id)
	if !r.OK {
		emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
			audit.OutcomeError, srvReqID, map[string]any{
				"sandbox_id": id,
				"error_code": r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
		audit.OutcomeOK, srvReqID, map[string]any{
			"sandbox_id": id,
		})
	c.JSON(core.StatusOK, gin.H{"stopped": id})
}

// inspect GET /v1/api/opencode/sandbox/:id → returns one record.
func (g *ControlGroup) inspect(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.Inspect(id)
	if !r.OK {
		c.JSON(core.StatusNotFound, gin.H{"error": r.Error()})
		return
	}
	sb, _ := r.Value.(Sandbox)
	c.JSON(core.StatusOK, sb)
}

// profileList GET /v1/api/opencode/profile → all stored profiles.
func (g *ControlGroup) profileList(c *gin.Context) {
	r := g.svc.ListProfiles()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	list, _ := r.Value.([]Profile)
	c.JSON(core.StatusOK, gin.H{"profiles": list})
}

// profileGet GET /v1/api/opencode/profile/:name → one profile record.
func (g *ControlGroup) profileGet(c *gin.Context) {
	name := core.TrimCutset(c.Param("name"), "/ ")
	r := g.svc.GetProfile(name)
	if !r.OK {
		c.JSON(core.StatusNotFound, gin.H{"error": r.Error()})
		return
	}
	p, _ := r.Value.(Profile)
	c.JSON(core.StatusOK, p)
}

// profileSave POST /v1/api/opencode/profile → upsert. Body = Profile
// JSON (must include "name"). Returns the saved record.
//
// Emits EventOpencodeProfileSave (Mantis #1602 HIGH) per call —
// denied on JSON bind failure, error on Service.SaveProfile failure,
// OK on success. Profile.Provider block is NEVER in Meta — may carry
// upstream provider apiKey / token bytes; only the profile name is
// emitted. RequestID server-generated per Cerberus #18 / Mantis #1511.
func (g *ControlGroup) profileSave(c *gin.Context) {
	srvReqID := newRequestID()
	c.Header("X-Request-Id", srvReqID)
	var p Profile
	if err := c.ShouldBindJSON(&p); err != nil {
		emitControlAudit(EventOpencodeProfileSave, "opencode.profile.save",
			audit.OutcomeDenied, srvReqID, map[string]any{
				"profile_name": p.Name,
				"error_code":   "opencode.profile.invalid_json",
			})
		c.JSON(core.StatusBadRequest, gin.H{"error": "invalid profile JSON: " + err.Error()})
		return
	}
	r := g.svc.SaveProfile(p)
	if !r.OK {
		emitControlAudit(EventOpencodeProfileSave, "opencode.profile.save",
			audit.OutcomeError, srvReqID, map[string]any{
				"profile_name": p.Name,
				"error_code":   r.Code(),
			})
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	emitControlAudit(EventOpencodeProfileSave, "opencode.profile.save",
		audit.OutcomeOK, srvReqID, map[string]any{
			"profile_name": p.Name,
		})
	c.JSON(core.StatusOK, p)
}

// profileDelete DELETE /v1/api/opencode/profile/:name → drop one.
// "default" cannot be deleted (it's the safety floor for spawn).
func (g *ControlGroup) profileDelete(c *gin.Context) {
	name := core.TrimCutset(c.Param("name"), "/ ")
	r := g.svc.DeleteProfile(name)
	if !r.OK {
		c.JSON(core.StatusBadRequest, gin.H{"error": r.Error()})
		return
	}
	c.JSON(core.StatusOK, gin.H{"deleted": name})
}

// emitControlAudit is the shared emit helper for every audit row this
// control surface produces. Centralised so the field shape stays
// consistent across handlers + so a future sweep (e.g. wiring the
// account_id hash from session context per RFC §6.4) has a single
// site to update. Failure is intentionally swallowed — audit failures
// MUST NEVER block an auth-adjacent path; the redact.go secret-shape
// detector enforces the no-credential invariant server-side.
//
// Usage example:
//
//	emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
//	    audit.OutcomeOK, srvReqID, map[string]any{"sandbox_id": id})
func emitControlAudit(event, scope, outcome, requestID string, meta map[string]any) {
	_ = audit.Default().Record(audit.Event{
		Event:     event,
		TS:        core.Now().UTC().Unix(),
		Scope:     scope,
		Outcome:   outcome,
		RequestID: requestID,
		Meta:      meta,
	})
}

// newRequestID generates a UUIDv4 used as the server-authoritative
// audit RequestID for every emit-site on the opencode control surface.
// Mirrors pkg/server/plugin_view_capability.newCorrelationID() — RFC
// 4122 §4.4 random UUID with version + variant bits set. Returns the
// empty string on core.RandomBytes failure; the audit row tolerates
// the missing field per the Stage F substrate contract.
//
// The caller's X-Request-Id header is INTENTIONALLY DROPPED per
// Cerberus #18 / Mantis #1511 — trusting an attacker-supplied value
// for the audit substrate JOIN key enables forensic deniability (an
// attacker forging arbitrary values to mimic a legitimate caller's
// audit-JOIN key, defeating the disambiguation property the field
// exists to provide). The server's UUID is echoed in the response
// X-Request-Id header so the legitimate caller can still correlate
// their request to the audit log.
//
// CoreGO gap: core.UUIDv4 doesn't exist yet (logged at
// project_corego_export_gaps). Local stand-in until the export lands.
//
// Usage example:
//
//	srvReqID := newRequestID()
//	c.Header("X-Request-Id", srvReqID)
//	emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
//	    audit.OutcomeOK, srvReqID, map[string]any{"sandbox_id": id})
func newRequestID() string {
	r := core.RandomBytes(16)
	if !r.OK {
		return ""
	}
	b, ok := r.Value.([]byte)
	if !ok || len(b) != 16 {
		return ""
	}
	// RFC 4122 §4.4 — version 4 (random) UUID. Top nibble of
	// time_hi_and_version (byte 6) = 0100 = 4. Top two bits of
	// clock_seq_hi_and_reserved (byte 8) = 10 (variant 1).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return core.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
