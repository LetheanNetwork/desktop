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

// EventOpencodeSandboxWebURLIssued is the audit event the webURL
// handler emits per successful credential-free URL issuance. Mantis
// #1600 HIGH (Cerberus #22 #1602 audit-gap finding, narrowed to this
// endpoint only — the wider opencode audit sweep is its own ticket).
//
// Meta keys:
//
//	sandbox_id — the opencode sandbox container identifier
//	auth_scheme — WebAuthScheme literal ("basic")
//	auth_via — WebAuthVia literal ("header")
//
// The password is NEVER in Meta — the whole point of the Mantis #1600
// fix is to keep that byte-string off every wire shape including the
// audit substrate. The redact.go secret-shape detector would refuse
// the record anyway if a future contributor wired it in by mistake.
const EventOpencodeSandboxWebURLIssued = "opencode.sandbox.web_url_issued"

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
func (g *ControlGroup) webURL(c *gin.Context) {
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
		RequestID: c.GetHeader("X-Request-Id"),
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
func (g *ControlGroup) upgrade(c *gin.Context) {
	r := g.svc.Upgrade()
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	res, _ := r.Value.(UpgradeResult)
	c.JSON(core.StatusOK, res)
}

// openTUI POST /v1/api/opencode/sandbox/:id/tui → spawns the user's
// default terminal running `<runtime> exec -it <container> opencode`.
func (g *ControlGroup) openTUI(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.OpenTUI(id)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
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
func (g *ControlGroup) enable(c *gin.Context) {
	var req struct {
		Profile string `json:"profile"`
	}
	_ = c.ShouldBindJSON(&req)
	r := g.svc.Enable(req.Profile)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	id, _ := r.Value.(string)
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
	var opts MergeHostConfigOptions
	// Body is optional; empty body uses defaults (profile=default,
	// force=false).
	_ = c.ShouldBindJSON(&opts)
	r := g.svc.MergeHostConfig(opts)
	if !r.OK {
		// Conflict surfaces as 409 so the frontend can distinguish
		// "needs user confirmation" from "actually broken".
		if r.Code() == HostConfigConflict {
			c.JSON(core.StatusConflict, gin.H{
				"error": r.Error(),
				"code":  HostConfigConflict,
			})
			return
		}
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	res, _ := r.Value.(MergeHostConfigResult)
	c.JSON(core.StatusOK, res)
}

// spawn POST /v1/api/opencode/sandbox → spawns a new container.
// Optional JSON body: {"profile": "<name>"} — selects the lthn-side
// opencode profile to apply via PATCH /config after spawn. Empty
// or missing body uses "default".
//
// Returns {id, url, profile} on success.
func (g *ControlGroup) spawn(c *gin.Context) {
	var req struct {
		Profile string `json:"profile"`
	}
	// Body is optional; bind failures (empty body / wrong shape)
	// fall through to default profile.
	_ = c.ShouldBindJSON(&req)
	r := g.svc.Start(req.Profile)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	id, _ := r.Value.(string)
	profile := req.Profile
	if profile == "" {
		profile = DefaultProfile
	}
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
func (g *ControlGroup) stop(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.Stop(id)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
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
func (g *ControlGroup) profileSave(c *gin.Context) {
	var p Profile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(core.StatusBadRequest, gin.H{"error": "invalid profile JSON: " + err.Error()})
		return
	}
	r := g.svc.SaveProfile(p)
	if !r.OK {
		c.JSON(core.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
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
