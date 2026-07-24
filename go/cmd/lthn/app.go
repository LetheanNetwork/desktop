// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
	"dappco.re/go/api"
	"dappco.re/go/config"
	"dappco.re/go/i18n"
	"dappco.re/go/io"
	"dappco.re/go/mcp/pkg/mcp"
	"dappco.re/go/orm"
	"dappco.re/go/process"
	"dappco.re/go/store"
	"dappco.re/go/ws"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/agents"
	lthnai "dappco.re/lthn/desktop/pkg/ai"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/deploys"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/gateway"
	lthni18n "dappco.re/lthn/desktop/pkg/i18n"
	"dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/marketing/analytics"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"dappco.re/lthn/desktop/pkg/marketing/content"
	"dappco.re/lthn/desktop/pkg/marketing/social"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/mdns"
	lthnml "dappco.re/lthn/desktop/pkg/ml"
	"dappco.re/lthn/desktop/pkg/office/documents"
	"dappco.re/lthn/desktop/pkg/office/files"
	"dappco.re/lthn/desktop/pkg/office/mail"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/plugin"
	lthnprocess "dappco.re/lthn/desktop/pkg/process"
	"dappco.re/lthn/desktop/pkg/queue"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/runbooks"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/forecast"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
	"dappco.re/lthn/desktop/pkg/sandbox"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"dappco.re/lthn/desktop/pkg/tasks"
	lthnupdate "dappco.re/lthn/desktop/pkg/update"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newAppCore constructs the shared *core.Core for any lthn CLI verb
// that needs the service bus. Registers the phase-1 service stack —
// store / ws / process / i18n / io — with paths resolved through
// pkg/paths so the canonical ~/Lethean/ layout is uniform.
//
// Returns the started Core. The caller MUST defer
// c.ServiceShutdown(core.Background()) to flush + close.
//
// Usage example:
//
//	c := newAppCore()
//	if c == nil { return 1 }
//	defer c.ServiceShutdown(core.Background())
//	r := c.Action("store.get").Run(core.Background(), opts)
func newAppCore() *core.Core {
	const appErrorFormat = "lthn: %s\n"

	dbPath := paths.StoreDB()
	if !dbPath.OK {
		core.Print(core.Stderr(), appErrorFormat, dbPath.Error())
		return nil
	}
	workspace := paths.WorkspaceDir()
	if !workspace.OK {
		core.Print(core.Stderr(), appErrorFormat, workspace.Error())
		return nil
	}
	dataDir := paths.DataDir()
	if !dataDir.OK {
		core.Print(core.Stderr(), appErrorFormat, dataDir.Error())
		return nil
	}
	configFile := paths.ConfigFile()
	if !configFile.OK {
		core.Print(core.Stderr(), appErrorFormat, configFile.Error())
		return nil
	}

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      configFile.Value.(string),
			EnvPrefix: "LTHN",
		})),
		core.WithName("store", store.NewService(store.StoreConfig{
			DatabasePath:            dbPath.Value.(string),
			WorkspaceStateDirectory: workspace.Value.(string),
		})),
		core.WithName("ws", ws.NewService(ws.DefaultHubConfig())),
		core.WithName("process", process.NewService(process.Options{})),
		core.WithName("i18n", i18n.NewCoreService(i18n.ServiceOptions{
			Language: "en-GB",
			Fallback: "en",
			ExtraFS: []i18n.FSSource{
				// pkg/i18n/locales/{en,en_au,...}.json — embedded at
				// compile time. en is the canonical UK English (Snider
				// canon: colour / organisation / centre); en_au is the
				// Australian variant. LC_ALL / LANGUAGE / LC_MESSAGES /
				// LANG select at runtime; explicit Language above wins
				// when set. See pkg/i18n/locales.go for the embed shape.
				lthni18n.Source(),
			},
		})),
		core.WithName("io", io.NewService(io.IOConfig{
			Root: dataDir.Value.(string),
		})),
		// api — the Gin-based polyglot HTTP gateway. Its Engine
		// drives route-group registration (api.Engine.Register(grp)).
		// pkg/desktop mounts the Engine's http.Handler at /api/*
		// so the same surface that runs standalone via `lthn serve`
		// also lights up inside the Wails WebView's same-origin
		// context. See pkg/desktop/subsystems.go.
		core.WithName("api", api.NewService(api.ApiConfig{})),
		// mcp — the Model Context Protocol service. Registered on
		// the Core today but NOT yet HTTP-mounted (the upstream
		// Service exposes ServeHTTP as an entry point rather than
		// an http.Handler accessor). Reachable via stdio in the
		// meantime; HTTP mount lands when we add an upstream
		// Handler() accessor on *mcp.Service.
		core.WithName("mcp", mcp.NewService(mcp.Options{
			WorkspaceRoot: dataDir.Value.(string),
		})),
		// plugin — the plugin host. Owns ~/Lethean/conf/plugins/,
		// supervises plugin binaries via process.Service, and
		// mounts a reverse-proxy on the coreapi.Engine at
		// /v1/api/plugin/<code>/*. See docs/plugin-host-scope.md.
		core.WithName("plugin", plugin.NewService(plugin.Options{})),
		// sandbox — spawn OCI containers via dappco.re/go/container
		// (AppleProvider) or runtime CLI (docker/podman). Proof-of-life
		// today: Spawn() runs a one-shot command and returns stdout.
		core.WithName("sandbox", sandbox.NewService(sandbox.Options{})),
		// opencode — long-running opencode-serve containers (one per
		// sandbox-id), reachable via the reverse-proxy mount at
		// /v1/api/sandbox/<id>/*. Container lifecycle via go-process
		// + docker; persistence via dappco.re/go/orm (typed Sandbox
		// record). ProxyGroup is registered with the api.Engine in
		// pkg/desktop/subsystems.go alongside the plugin proxy.
		core.WithName("opencode", opencode.NewService(opencode.Options{})),
		// repos — multi-repo dashboard. Scans canonical workspace
		// roots ($HOME/Code/{core,lthn,host-uk,lab,snider}) for git
		// children + accepts external paths via RegisterSource so
		// imports (opencode + future codex/claude/pi) surface
		// alongside scanned repos.
		core.WithName("repos", repos.Register),
		// queue — throttled background job substrate. Single
		// worker processes pending Jobs sequentially; subsystems
		// register kind handlers via queue.RegisterKind and enqueue
		// work via queue.Enqueue. Per design_cooperative_task_queue:
		// capture-greedy / execute-throttled, behaves with other
		// apps. Service.OnStart spawns the worker after schemas are
		// registered + ServiceStartup runs.
		core.WithName("queue", queue.Register),
		// marketplace — lthn-vm bundle catalogue + install lifecycle.
		// Provides marketplace.Service to subsystems.go for MCP tool
		// registration (marketplace_list/install/launch/stop/uninstall).
		core.WithName("marketplace", marketplace.Register),
		// gateway — runtime data firewall (RFC.marketplace.md §7a).
		// Plugins call /v1/api/gateway/<scope>/<mode> with a Bundle-ID
		// header; gateway.CheckPermission gates against the bundle's
		// installed Permissions snapshot. Mounted on the api Engine
		// in cmdServe alongside opencode + plugin proxy groups.
		core.WithName("gateway", gateway.Register),
		// mdns — LAN-discovery broadcast of the lthn HTTP server as
		// _http._tcp.local under "lthn" (resolves to lthn.local).
		// Service registers in disabled state; cmdServe calls
		// Configure + OnStart with the resolved port. User-facing
		// toggle binds via mdns.Service.SetDiscoverable.
		core.WithName("mdns", mdns.Register),
		// lthn-process — lthn-side consumer wrapper for the upstream
		// dappco.re/go/process service. Owns the CLI verbs (run /
		// start / kill / list / get) and publishes the /api/process
		// REST routes via RoutesProvider so pkg/server auto-mounts
		// them at engine construction. Registering here keeps the
		// route declaration next to the service and out of cmd/lthn.
		core.WithName("lthn-process", lthnprocess.Register),
		// keys — encrypted-at-rest provider-credentials store under
		// ~/Lethean/data/keys/. Wails frontends write provider API
		// keys via the binding; plaintext never crosses the WebView.
		core.WithName("keys", keys.Register),
		// fleet — compute-fleet view (machines, agents, routing
		// rules) backed by master DuckDB. Frontends consume via the
		// fleet-window Wails binding.
		core.WithName("fleet", fleet.Register),
		core.WithName("agents", agents.Register),
		// runner — talk-surface owning the inference-route table.
		// Built from Core config (routes.<name>.{kind,base_url,...});
		// SetDynamicRoutes refreshes the live router when opencode
		// sandboxes spawn / exit.
		core.WithName("runner", runner.Register),
		// ai — persistent state for the AI subsystem. Backs
		// ~/Lethean/data/desktop/ai.duckdb so chats / provider routes
		// / tool transcripts grow without contending against the
		// fleet/tasks master DB lock.
		core.WithName("ai", lthnai.Register),
		// ml — persistent state for the ML subsystem. Backs
		// ~/Lethean/data/desktop/ml.duckdb so training runs / LoRA
		// jobs / dataset manifests / model-pack registry grow without
		// contending against the master DB.
		core.WithName("ml", lthnml.Register),
		// vi — Lethean Desktop mascot's data spine. Populates the
		// Sites slot of the four-slot Vi data contract via the queue
		// substrate: a "vi-probe-site" handler self-reschedules per
		// catalogue entry, with results persisted as SiteProbe rows.
		// Briefs / Activity slots remain fixture-data until separate
		// follow-up tickets (mirrors RFC.vi.md §"Open tickets").
		core.WithName("vi", vi.Register),
		// incidents — Operations view's incident log. Reads/writes
		// Trix-style markdown files from ~/Lethean/incidents/{YYYY}/{MM}/.
		// Wails surface: Incidents.List / Get / Create / UpdateState /
		// AddPostmortem. Events: incidents.opened + incidents.transitioned.
		core.WithName("incidents", incidents.Register),
		// runbooks — Operations view's runbook library. Scans
		// ~/Lethean/runbooks/*.md for procedures with freshness tracking
		// (last_rehearsed frontmatter). Seeds seven default runbooks on
		// first launch. Wails surface: Runbooks.List / Get / Search /
		// MarkRehearsed. Event: runbooks.rehearsed.
		core.WithName("runbooks", runbooks.Register),
		// sales/contacts — CRM contact catalogue. Reads/writes Trix-style
		// markdown files from ~/Lethean/sales/contacts/{slug}.md. Warmth
		// recomputed on read from last_touch timestamp (≤7d=hot, 8-21d=warm,
		// >21d=cool). Wails surface: Contacts.List/Get/Create/Update.
		core.WithName("sales-contacts", contacts.Register),
		// sales/deals — deal record + activity log. Reads/writes Trix-style
		// markdown files from ~/Lethean/sales/deals/{id}.md. Source-of-truth
		// for pipeline stage. Wails: Deals.List/Get/Create/UpdateStage/AddActivity.
		core.WithName("sales-deals", deals.Register),
		// sales/pipeline — derived Kanban rollup from deal files. Groups
		// deal records by stage into PipelineColumn values; no separate
		// persistence. Wails: Pipeline.List/MoveDeal.
		core.WithName("sales-pipeline", pipeline.Register),
		// sales/forecast — quarterly probability-weighted rollup from deals.
		// Read-only; no separate persistence. Wails: Forecast.Quarterly.
		core.WithName("sales-forecast", forecast.Register),
		// marketing/campaigns — campaign thread catalogue. Reads/writes Trix-style
		// markdown files from ~/Lethean/marketing/campaigns/{slug}.md. Wails:
		// Campaigns.List/Get/Create/Update. Events: marketing.campaigns.{created,updated}.
		core.WithName("marketing-campaigns", campaigns.Register),
		// marketing/content — editorial content calendar. Reads/writes Trix-style
		// markdown files from ~/Lethean/marketing/content/{id}.md. Wails:
		// Content.List/Get/Create/Advance. Events: marketing.content.{created,advanced}.
		core.WithName("marketing-content", content.Register),
		// marketing/social — social post queue. Reads/writes Trix-style
		// markdown files from ~/Lethean/marketing/social/post-{ts}.md. Wails:
		// Social.List/Get/Create/MarkSent. Events: marketing.social.{created,sent}.
		core.WithName("marketing-social", social.Register),
		// marketing/audience — subscriber segment catalogue. Reads/writes Trix-style
		// markdown files from ~/Lethean/marketing/audience/{slug}.md. Wails:
		// Audience.List/Get/Create/Update. Events: marketing.audience.{created,updated}.
		core.WithName("marketing-audience", audience.Register),
		// marketing/analytics — web analytics rollup (read-only). Derived from the
		// self-hosted Plausible install; returns fixture data when unconfigured.
		// Wails: Analytics.Get. No events (read-only).
		core.WithName("marketing-analytics", analytics.Register),
		// office/documents — Office role document catalogue. Reads and
		// indexes markdown files from ~/Lethean/office/docs/. Wails
		// surface: Documents.List / Get. Read-only in v1.
		core.WithName("office-documents", documents.Register),
		// office/mail — Office role mailbox catalogue (v1). Reads
		// ~/Lethean/office/mail/{folder-slug}/threads.md — YAML thread
		// frontmatter, no IMAP fetch. Wails: Mail.ListFolders / ListThreads.
		core.WithName("office-mail", mail.Register),
		// office/files — Office role filesystem browser. Surfaces canonical
		// workspace locations, recent files, and disk usage. Read-only v1.
		// Wails: Files.ListLocations / ListRecent / GetDiskUsage.
		core.WithName("office-files", files.Register),
		// coding/deploys — Coding role deploy history catalogue. Reads and
		// writes Trix-style markdown files from ~/Lethean/deploys/. v1 scope:
		// List / Get / Create. Wails: Deploys.List / Get / Create.
		core.WithName("coding-deploys", deploys.Register),
		// serverkey — Stage B of the first-run auth-gate (Mantis #1474,
		// plans RFC at code/lthn/desktop/auth-gate/RFC.md). Owns the
		// "base egg" PGP key at ~/Lethean/wallets/server.key + its
		// HKDF-derived passphrase root at ~/Lethean/wallets/.seed.
		// Issues + verifies the short-lived bootstrap tokens the
		// account-creation endpoint family consumes. Bootstrap()
		// runs explicitly below — Register only constructs.
		core.WithName("serverkey", serverkey.Register),
		// account — Stage B' of the first-run auth-gate (Mantis #1474
		// successor). Owns the /v1/account/create endpoint handler
		// that closes the seam left open by Stage B: pkg/server's
		// BootstrapAuthMiddleware gates the path with scope=account.create,
		// then dispatches to account.Service.Create which enforces the
		// four Cerberus #1460 MUST-NOTs (no overwrite, ID match,
		// atomic write, nonce-consumption-before-write). REST surface
		// auto-mounts via pkg/server.RoutesProvider discovery.
		core.WithName("account", account.Register),
		// update — self-update against the LetheanNetwork/desktop
		// GitHub release feed. Constructed with CheckOnStartup =
		// NoCheck so registering is offline-cheap; consumers call
		// Service.Start() to fire the check. Current version flows
		// from `-ldflags -X dappco.re/lthn/desktop.Version=...` at
		// task build time, and pkg/update syncs that into go-update.
		core.WithName("update", lthnupdate.Register),
	)

	// Mantis #1522 — audit.Default() init promoted ahead of
	// c.ServiceStartup so the noop recorder window (between
	// audit's package-level lazy default and the explicit boot wire)
	// can't swallow events fired from service OnStart hooks. Today
	// no registered OnStart emits audit (grep-verified), but Stage X.B
	// Phase 2c moves serverkey.AccountStatus / account.Provision boot
	// probes earlier in the lifecycle — first-boot setup-flow events
	// from those probes would otherwise land in the noopRecorder.
	//
	// Constructed with Options{} so AuditSecret defaults to a
	// process-local random fallback. The "non-persistent" warning
	// fires at New-time per the audit.New contract. Once serverkey
	// .Bootstrap lands ~/Lethean/wallets/.seed below, we swap to the
	// serverkey-derived HMAC via auditSvc.SetSecret so account_id
	// hashing for the rest of the process matches the canonical
	// HKDF derivation (RFC.stage-f.md §6.4). Events emitted between
	// here and SetSecret carry account_id hashed under the random
	// fallback — they survive the process for queries within this
	// session but won't cross-correlate to events from later runs,
	// which is the same posture audit.New documents for Options{}
	// users generally.
	//
	// RegisterService wires the Service into the core's IPC discovery
	// path so pkg/server's RoutesProvider auto-discovery picks up the
	// GET /v1/audit/events surface from pkg/audit's RouteGroups().
	auditSvc := audit.New(c, audit.Options{})
	audit.SetDefault(auditSvc)
	if r := c.RegisterService("audit", auditSvc); !r.OK {
		core.Print(core.Stderr(), "lthn: audit RegisterService failed: %s\n", r.Error())
	}

	// go-process global manager init — crew supervision (pkg/fleet) and
	// any other package-level process.Start caller use the default global
	// service, which is distinct from the named "process" service
	// registered above (that one backs the lthn-process CLI verbs +
	// /api/process). Promoted ahead of c.ServiceStartup, like audit above,
	// so a service that spawns a sidecar from its OnStart hook finds the
	// manager live. Idempotent via sync.Once — safe though every lthn verb
	// calls newAppCore.
	if r := process.Init(c); !r.OK {
		core.Print(core.Stderr(), "lthn: process.Init failed: %s\n", r.Error())
	}

	if r := c.ServiceStartup(core.Background(), nil); !r.OK {
		core.Print(core.Stderr(), "lthn: startup failed: %s\n", r.Error())
		return nil
	}

	// serverkey Bootstrap — Stage B of the first-run auth-gate
	// (Mantis #1474). Resolves ~/Lethean/wallets/.seed (creates on
	// first run, mode 0600), derives the HKDF-SHA256 passphrase, and
	// loads or generates ~/Lethean/wallets/server.key (mode 0600).
	// MUST run before any HTTP server starts listening so the
	// bootstrap-auth middleware's Verifier reference is live before
	// the WebView can issue an account-creation request.
	//
	// Bootstrap is idempotent — every verb (gui, serve, doctor)
	// calls newAppCore which invokes this, so the on-disk key only
	// gets generated once across the install lifetime; subsequent
	// calls re-load from disk via the HKDF-derived passphrase.
	if serverkeySvc, _ := core.ServiceFor[*serverkey.Service](c, "serverkey"); serverkeySvc != nil {
		if r := serverkeySvc.Bootstrap(); !r.OK {
			core.Print(core.Stderr(), "lthn: serverkey bootstrap failed: %s\n", r.Error())
			return nil
		}

		// Stage E.K.C tier-0 KEK provider wire (Mantis #1625,
		// RFC.stage-e-keys-partition v3 §4.2). Derives the tier-0 KEK
		// via HKDF over ~/Lethean/wallets/.seed using the salt+info
		// constants from pkg/keys. The .seed file is bootstrap
		// substrate per the auth-gate v1 ([[project_auth_gate_v1_landed_2026_05_16]])
		// — serverkey.Bootstrap above just ensured it exists on disk,
		// mode 0600 — so the closure is normally live from this point
		// forward. Pattern mirrors serverkey/audit.go's AuditHMACSecret
		// (.seed → HKDF derivation).
		//
		// CRITICAL — wire ORDER per Cerberus #43 + #1625 + #1653:
		// tier-0 (SetKEKProviderTier0) MUST fire BEFORE tier-1
		// (SetKEKProvider). The migrateTier1Locked Step 3b.5 guard
		// (H#168, refining Cerberus #40) refuses to write
		// .master-tier1 if legacy single-instance.aead is present and
		// the tier-0 KEK provider isn't live — otherwise the
		// retry-half-state path (.master-tier1 written but
		// single-instance.aead orphaned) bites on next boot
		// (separate ticket for the cleanup path; this wire avoids
		// hitting it in the first place).
		//
		// Closure derives a fresh KEK on every call rather than
		// caching: keys.Service treats the returned bytes as opaque
		// per the KEKProvider contract (service.go:113-125), so
		// repeated derivation is fine — .seed reads are cheap.
		if keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys"); keysSvc != nil {
			walletsR := paths.WalletsDir()
			if walletsR.OK {
				seedPath := core.PathJoin(walletsR.Value.(string), ".seed")
				keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
					seedR := core.ReadFile(seedPath)
					if !seedR.OK {
						return nil, false
					}
					kekR := core.HKDF("sha256", seedR.Value.([]byte),
						[]byte(keys.KEKHKDFSalt),
						[]byte(keys.KEKHKDFInfoTier0), 32)
					if !kekR.OK {
						return nil, false
					}
					kek, ok := kekR.Value.([]byte)
					if !ok || len(kek) != 32 {
						return nil, false
					}
					return kek, true
				})
			} else {
				core.Print(core.Stderr(),
					"lthn: tier-0 KEK provider wire skipped — wallets dir resolve failed: %s\n",
					walletsR.Error())
			}
		}

		// Stage E.B integration cutover (Mantis #1480) — wire the
		// session-token issuer into pkg/account so Unlock can mint
		// LTHN-SESS-1.* tokens on successful passphrase decrypt. The
		// same serverkey instance the bearer middleware verifies
		// session-tokens against is the one minting them — no
		// rotation skew between issue + verify possible.
		if accountSvc, _ := core.ServiceFor[*account.Service](c, "account"); accountSvc != nil {
			accountSvc.SetServerKey(serverkeySvc)

			// LTHN-SESS-1: writer session-gate wires (RFC.stage-e-unlockgate §2.1)
			// Each writer pkg defines its own SessionGate interface (consumer-
			// defines pattern per Cerberus #27 pushback 1); accountSvc satisfies
			// all 10 because the interface shape is identical —
			// UnlockedAccountIDs() []string. Explicit named lookups (NOT a
			// dynamic loop per Cerberus #27 ADD-3 rejection) so static analysis
			// + grep can audit the full wire surface. analytics is intentionally
			// excluded — read-only/derived per H#159 SECURITY-NOTE.
			if documentsSvc, _ := core.ServiceFor[*documents.Service](c, "office-documents"); documentsSvc != nil {
				documentsSvc.SetSessionGate(accountSvc)
			}
			if contactsSvc, _ := core.ServiceFor[*contacts.Service](c, "sales-contacts"); contactsSvc != nil {
				contactsSvc.SetSessionGate(accountSvc)
			}
			if incidentsSvc, _ := core.ServiceFor[*incidents.Service](c, "incidents"); incidentsSvc != nil {
				incidentsSvc.SetSessionGate(accountSvc)
			}
			if runbooksSvc, _ := core.ServiceFor[*runbooks.Service](c, "runbooks"); runbooksSvc != nil {
				runbooksSvc.SetSessionGate(accountSvc)
			}
			if dealsSvc, _ := core.ServiceFor[*deals.Service](c, "sales-deals"); dealsSvc != nil {
				dealsSvc.SetSessionGate(accountSvc)
			}
			if campaignsSvc, _ := core.ServiceFor[*campaigns.Service](c, "marketing-campaigns"); campaignsSvc != nil {
				campaignsSvc.SetSessionGate(accountSvc)
			}
			if audienceSvc, _ := core.ServiceFor[*audience.Service](c, "marketing-audience"); audienceSvc != nil {
				audienceSvc.SetSessionGate(accountSvc)
			}
			if pipelineSvc, _ := core.ServiceFor[*pipeline.Service](c, "sales-pipeline"); pipelineSvc != nil {
				pipelineSvc.SetSessionGate(accountSvc)
			}
			if socialSvc, _ := core.ServiceFor[*social.Service](c, "marketing-social"); socialSvc != nil {
				socialSvc.SetSessionGate(accountSvc)
			}
			if contentSvc, _ := core.ServiceFor[*content.Service](c, "marketing-content"); contentSvc != nil {
				contentSvc.SetSessionGate(accountSvc)
			}

			// Mantis #1624 — gate pkg/keys's master decrypt on a
			// user-PGP-derived KEK once the account is unlocked.
			// Pre-unlock (headless `lthn serve`, account locked) the
			// provider returns (_, false) so the legacy raw-master
			// path stays live (single-instance key generated pre-
			// unlock keeps working). Post-unlock the provider derives
			// a 32-byte KEK via core.HKDF over the unlocked PGP
			// private key bytes; pkg/keys then seals/unseals the on-
			// disk master under that KEK, so losing the unlocked
			// account's private key (or the .seed used to encrypt it)
			// is now sufficient to render every stored secret
			// recoverable only by the rightful user.
			//
			// PrivateKeyHandle.Use zeroises the bytes on closure
			// return — derive KEK INSIDE the Use callback, hand the
			// derived KEK back to the keys layer, never retain a
			// reference to the raw private key past the closure.
			// Mantis #1589 / Cerberus #18 single-use handle discipline.
			//
			// When more than one account is unlocked concurrently we
			// pick the alphabetically-first id (UnlockedAccountIDs
			// sorts before returning). Multi-account KEK rotation is
			// a separate ticket — Stage F design owns the routing
			// policy. For Stage X.B v1 the first-id default matches
			// the one-account-per-install norm.
			if keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys"); keysSvc != nil {
				keysSvc.SetKEKProvider(func() ([]byte, bool) {
					ids := accountSvc.UnlockedAccountIDs()
					if len(ids) == 0 {
						return nil, false
					}
					activeID := ids[0]
					if !accountSvc.HasUnlocked(activeID) {
						return nil, false
					}
					handle, ok := accountSvc.PrivateKeyFor(activeID)
					if !ok {
						return nil, false
					}
					var kek []byte
					_ = handle.Use(func(priv []byte) error {
						kekR := core.HKDF("sha256", priv,
							[]byte(keys.KEKHKDFSalt),
							[]byte(keys.KEKHKDFInfo), 32)
						if !kekR.OK {
							return nil
						}
						kek = kekR.Value.([]byte)
						return nil
					})
					if len(kek) != 32 {
						return nil, false
					}
					return kek, true
				})
			}

			// Mantis #1657 / RFC.provider-creds-tier1.md §6 Option A —
			// drain any deferred plaintext `api_key:` routes into the
			// tier-1 substrate as soon as the KEK provider is wired.
			// MigrateLegacyKeys is idempotent + best-effort: at cold
			// boot no account is unlocked yet so the tier-1 PutTier1
			// call short-circuits with no_tier1_provider and migrated
			// returns 0 — the boot-scan
			// EventProviderCredentialMigrationPendingObserved row from
			// runner.ServiceStartup carries the deferred signal to
			// observability while the latch waits for first unlock.
			//
			// SECURITY-NOTE — true post-unlock drain requires an
			// unlock-event subscription surface in pkg/account that
			// doesn't exist today (no OnUnlock hook, no event-bus
			// broadcast from account.Service.Unlock). Until that lands
			// the deferred plaintext stays on disk between cold boot
			// and the next MigrateLegacyKeys trigger; calling at boot
			// gives the foundational anchor + an idempotent retry
			// shape for any caller that re-invokes after unlock.
			// Surface as a separate ticket (account.Service unlock-
			// hook surface) — keeping it out of this wire so the
			// path allowlist stays clean.
			if n := runner.MigrateLegacyKeys(c); n > 0 {
				core.Print(core.Stdout(),
					"lthn: migrated %d legacy provider credential(s) into tier-1\n", n)
			}
		}

		// Stage F.B Phase 2 boot wiring (Mantis #1509, refined by
		// Mantis #1522) — swap the audit Service's HMAC secret to the
		// serverkey-derived value so the at-rest account_id hashing
		// (RFC.stage-f.md §6.4) survives process restarts. The Service
		// itself was constructed pre-ServiceStartup above so the
		// noopRecorder window can't swallow OnStart-emitted events
		// (Mantis #1522 LOW). Calling SetSecret here (rather than
		// reconstructing the Service) preserves the live day-file
		// handle + rotation goroutine + already-landed events.
		//
		// AuditHMACSecret reads ~/Lethean/wallets/.seed which only
		// exists after serverkey.Bootstrap landed it; calling SetSecret
		// from outside the serverkeySvc != nil branch would deadlock on
		// .seed-not-yet-written. Inside this branch we know Bootstrap
		// returned OK so the secret is available.
		auditSvc.SetSecret(serverkeySvc.AuditHMACSecret())

		// Stage F.B paths-audit wire (Mantis #1521) — the H#4 atomic-
		// write substrate (pkg/paths/events.go) fires LockEvents on
		// every AtomicWriteWithVersion / AtomicAppendLine success +
		// version-stale rejection. Without this adapter every emission
		// lands in the package-level noopAuditRecorder; the typed bus
		// stays useful in-process but the audit trail is dark.
		//
		// Adapter routes by ev.Mode: AuditModeSync → RecordSync (auth-
		// substrate, fsync per event), AuditModeBatch → RecordBatch
		// (cascade, page-boundary flush). HKDF path-hash secret is
		// sourced from the same serverkey instance the audit Service
		// uses for account_id hashing — one secret, two domain-
		// separated info-strings ("paths.lock.v1|<id>" vs
		// "audit.path.v1|<id>") per RFC.atomic-write.md §6.2 MED-3.
		//
		// SetCurrentAccountIDProvider is intentionally NOT wired here —
		// session-tier wiring lands separately so the boot path stays
		// session-agnostic. Local-tier callers default to "" (single-
		// tenant degenerate but still domain-separated).
		paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
		paths.SetAuditSecretProvider(serverkeySvc.AuditHMACSecret)

		// Mantis #1533 — flush any LockEvents that were dropped during
		// the pre-secret cold-boot window (between paths-package init
		// and the SetAuditSecretProvider line above). emitLockEvent
		// drops + increments auditDegradedCount when the secret is
		// unavailable rather than emit a path_hash="" event that would
		// leak "something happened" without the per-account domain
		// separation. The single summary event lands as soon as the
		// secret becomes available — which is here, immediately after
		// SetAuditSecretProvider, because serverkey.Bootstrap above
		// already landed ~/Lethean/wallets/.seed so AuditHMACSecret()
		// returns 32 bytes at this point. Calling here (rather than
		// post first-user-unlock) is safer per the H#13 brief: the
		// secret is already available; no need to wait.
		if n := paths.FlushDegradedCount(); n > 0 {
			core.Print(core.Stdout(),
				"lthn: paths.FlushDegradedCount summary emitted, count=%d\n", n)
		}
	}

	// orm bootstrap — lib not service. Register + mount the DuckDB
	// medium (persistent across serve restarts) under "default" +
	// register schemas. Pattern mirrors core/ide's pkg/server/orm_init.go,
	// but consumes orm.NewDuckDB now that it lives in core/orm proper
	// (was a private duckDBMedium in core/ide pre-2026-05-15).
	//
	// Memium fallback: if DuckDB open fails (path unwritable, etc.),
	// drop to in-memory so startup doesn't abort. Imports + sandbox
	// records won't persist in that path but the user can still use
	// the binary.
	if r := orm.Register(c); !r.OK {
		core.Print(core.Stderr(), "lthn: orm register failed: %s\n", r.Error())
		return nil
	}
	dataR := paths.DataDir()
	if !dataR.OK {
		core.Print(core.Stderr(), "lthn: data dir resolve failed: %s\n", dataR.Error())
		return nil
	}
	// File is named for the substrate (tasks), not the storage tech
	// (orm/duckdb). Per design_cooperative_task_queue, the trajectory
	// is for everything (sandboxes, imports, watchers, etc.) to land
	// as Issues / events hanging off tasks — the file name reflects
	// where this is heading, not just today's mixed contents.
	ormPath := core.PathJoin(dataR.Value.(string), "tasks.duckdb")
	// Migration: if a legacy orm.duckdb exists from before the rename
	// (~2026-05-15), point at it and rename in place — saves the user
	// from re-running `lthn opencode import`.
	legacyPath := core.PathJoin(dataR.Value.(string), "orm.duckdb")
	if core.Stat(legacyPath).OK && !core.Stat(ormPath).OK {
		_ = core.Rename(legacyPath, ormPath)
		// WAL files migrate too; missing rename is non-fatal — DuckDB
		// recovers cleanly from a missing WAL on next open.
		_ = core.Rename(legacyPath+".wal", ormPath+".wal")
	}
	var duck *orm.DuckDBMedium
	if r := orm.NewDuckDB(ormPath); r.OK {
		duck = r.Value.(*orm.DuckDBMedium)
		if mr := orm.Mount(c, "default", duck); !mr.OK {
			core.Print(core.Stderr(), "lthn: orm DuckDB mount failed: %s\n", mr.Error())
			return nil
		}
	} else {
		core.Print(core.Stderr(), "lthn: orm DuckDB open failed (%s); falling back to in-memory\n", r.Error())
		memium := orm.NewMemium()
		if mr := orm.Mount(c, "default", memium); !mr.OK {
			core.Print(core.Stderr(), "lthn: orm mount failed: %s\n", mr.Error())
			return nil
		}
	}
	schemas := []orm.Schema{
		opencode.Sandbox{}.Schema(),
		opencode.ImportedProject{}.Schema(),
		opencode.ImportedProvider{}.Schema(),
	}
	// tasks subsystem — Issue + Note tables; substrate for the
	// cooperative task queue (see design_cooperative_task_queue).
	schemas = append(schemas, tasks.Schemas()...)
	// queue subsystem — Job table for the throttled background
	// substrate. Worker spawns via Service.OnStart after the
	// service bus runs ServiceStartup.
	schemas = append(schemas, queue.Schemas()...)
	// marketplace subsystem — InstalledBundle table tracking which
	// lthn-vm bundles the user has installed + their lifecycle state.
	schemas = append(schemas, marketplace.InstalledBundle{}.Schema())
	// vi subsystem — SiteProbe table backing the Sites slot of Vi's
	// data contract. One row per probe tick; LatestByDomain rolls
	// up to the per-site latest for the Wails Sites() response.
	schemas = append(schemas, vi.Schemas()...)
	// benchmark subsystem — runner-agnostic results substrate. Stores
	// inference benchmark Runs from any registered Bencher adapter
	// (own runner, llama.cpp, ollama, NIM, opencode, future go-ai).
	schemas = append(schemas, benchmark.Schemas()...)
	for _, schema := range schemas {
		if r := orm.RegisterSchema(c, schema); !r.OK {
			core.Print(core.Stderr(), "lthn: orm schema %s failed: %s\n", schema.Name, r.Error())
			return nil
		}
		if duck != nil {
			duck.RegisterTable(schema.Name, schema)
		}
	}

	// Wire imported worktrees into the repos surface — opencode
	// imports show up in the multi-repo dashboard alongside scanned
	// roots. Future codex/claude/pi imports plug in the same way.
	if reposSvc, _ := core.ServiceFor[*repos.Service](c, "repos"); reposSvc != nil {
		reposSvc.RegisterSource("opencode-imports", func(_ core.Context) []string {
			r := orm.Of[opencode.ImportedProject](c).
				Where("worktree", "!=", "").
				Get()
			if !r.OK {
				return nil
			}
			rows, ok := r.Value.([]opencode.ImportedProject)
			if !ok {
				return nil
			}
			out := make([]string, 0, len(rows))
			for _, p := range rows {
				if p.Worktree == "" || p.Worktree == "/" {
					continue
				}
				out = append(out, p.Worktree)
			}
			return out
		})
	}

	// Cerberus #70 F-2 MED — one boot-time row stamping the at-boot
	// service-set + Wails-binding surface fingerprint. Forensic question
	// "which services were live at boot when X happened?" was previously
	// unanswerable from the audit trail; this row closes the gap. See
	// pkg/audit/types.go EventCompositionServicesRegistered for the full
	// substrate-contract preamble + Meta-key vocabulary.
	emitCompositionAudit(c, auditSvc)

	return c
}

// wailsBindingCatalogue is the forensic-contract list of Wails-binding
// names exposed to the renderer at boot. Mirrors the
// `application.NewService(...)` block in pkg/desktop/desktop.go around
// the wailsServices initialiser (the static at-boot surface bound to
// the WebView). Hand-maintained alongside the pkg/desktop list — drift
// IS the regression signal per the H#241 SECURITY-NOTE escape valve
// (the hash flips when pkg/desktop adds / removes / renames a binding
// without updating this catalogue; the next audit reader sees the
// inconsistency immediately).
//
// Names are the binding-name segment of the generated TS path
// `frontend/bindings/dappco.re/.../<pkg>/<service>` — the lowercased
// service-pkg identifier the WebView reaches via
// `wails-runtime → Service.Method` calls. Two-name discipline:
// short-name when the pkg owns one canonical service-type; pkg-prefixed
// when the pkg ships multiple (e.g. "sales/contacts").
//
// Sort-stability: emitCompositionAudit sorts the slice before hashing,
// so the declared order here doesn't affect the on-disk hash; ordering
// the slice the way pkg/desktop ships its initialiser keeps the diff
// review cheap.
var wailsBindingCatalogue = []string{
	"runner",
	"server",
	"sessions",
	"models",
	"downloader",
	"firstlaunch",
	"integrations",
	"apikey",
	"git",
	"build",
	"container",
	"lint",
	"marketplace",
	"lthnphp",
	"plugin",
	"sandbox",
	"opencode",
	"repos",
	"tasks",
	"vi",
	"incidents",
	"runbooks",
	"sales-contacts",
	"sales-deals",
	"sales-pipeline",
	"sales-forecast",
	"marketing-campaigns",
	"marketing-content",
	"marketing-social",
	"marketing-audience",
	"marketing-analytics",
	"office-documents",
	"office-mail",
	"office-files",
	"coding-deploys",
	"serverkey",
	"account",
	"fleet",
	"keys",
	"tools",
	"validator",
	"telemetry",
	"lthnservices",
	"i18n",
	"config",
	"window",
}

// emitCompositionAudit fires the single boot-time composition row per
// Cerberus #70 F-2 MED. Idempotent at the audit layer — calling more
// than once writes more than one row (the boot wire calls exactly once
// from newAppCore; the test path calls once per test invocation).
//
// auditSvc may be nil under exotic paths (e.g. the test bench
// constructs a Service standalone via audit.New); in that case the
// helper degrades to audit.Default() which lands the row in whichever
// Recorder is the package default. Same Recorder semantics as the rest
// of the codebase — the per-callsite `_ = audit.Default().Record(...)`
// idiom.
//
// Usage example (boot wire — end of newAppCore):
//
//	emitCompositionAudit(c, auditSvc)
//
// Usage example (test — exercises the emit shape):
//
//	auditSvc := audit.New(nil, audit.Options{})
//	audit.SetDefault(auditSvc)
//	c := core.New(core.WithName("alpha", svc), core.WithName("beta", svc))
//	_ = c.ServiceStartup(core.Background(), nil)
//	emitCompositionAudit(c, auditSvc)
func emitCompositionAudit(c *core.Core, auditSvc *audit.Service) {
	serviceNames := []string{}
	if c != nil {
		serviceNames = append(serviceNames, c.Services()...)
	}
	core.SliceSort(serviceNames)
	// 16-char hex prefix (64 bits) — forensic uniqueness within a host's
	// boot history while staying under pkg/audit/redact.go's 32-char
	// secret-shape floor (full SHA-256 hex is 64 chars and would trip
	// the redactor). Same truncation idiom as pkg/plugin/proxy.go:208
	// and pkg/recordfile/atrest_schema.go:310.
	serviceNamesHash := core.SHA256HexString(core.Join(",", serviceNames...))[:16]

	bindings := append([]string{}, wailsBindingCatalogue...)
	core.SliceSort(bindings)
	bindingsHash := core.SHA256HexString(core.Join(",", bindings...))[:16]

	ev := audit.Event{
		Event:   audit.EventCompositionServicesRegistered,
		TS:      core.Now().UTC().Unix(),
		Scope:   "composition",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"service_count":       len(serviceNames),
			"service_names_hash":  serviceNamesHash,
			"wails_binding_count": len(bindings),
			"wails_bindings_hash": bindingsHash,
			"version":             lthn.Version,
		},
	}

	if auditSvc != nil {
		_ = auditSvc.Record(ev)
		return
	}
	_ = audit.Default().Record(ev)
}

// pathsAuditAdapter bridges pkg/paths.AuditRecorder onto pkg/audit's
// Service. Lives at the boot layer rather than inside either package
// because pkg/audit imports pkg/paths (for paths.Root) — a reverse
// dependency in either direction would cycle.
//
// Routes by LockEvent.Mode per RFC.atomic-write.md §6.1: AuditModeSync
// auth-substrate writes get RecordSync (fsync per event); AuditModeBatch
// cascade writes get RecordBatch (page-boundary flush). Outcome is
// derived from Kind — write-success / lock-acquired / lock-released map
// to OutcomeOK; version-stale (write rejected) maps to OutcomeFailed.
// audit.Service.recordCommon rejects empty Outcome with
// codeAuditEventInvalid, so the mapping is load-bearing.
//
// Usage example (boot wire):
//
//	paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
type pathsAuditAdapter struct{ svc *audit.Service }

// RecordPathsEvent implements paths.AuditRecorder. The returned
// core.Result surfaces audit-side failures up through paths' emit
// site. Sync-audit failure propagates to the AtomicWriteWithVersion
// caller per Mantis #1530 (commit 1303368) — auth-substrate writes
// fail-stop when the audit recorder errors, so the silent-swallow
// gap previously flagged here is closed.
func (a *pathsAuditAdapter) RecordPathsEvent(ev paths.LockEvent) core.Result {
	ae := audit.Event{
		Event:   ev.Kind,
		TS:      ev.At.Unix(),
		Outcome: pathsEventOutcome(ev.Kind),
		Meta: map[string]any{
			"path_hash": ev.PathHash,
			"caller":    ev.Caller,
			"code":      ev.Code,
			"version":   ev.Version,
		},
	}
	if ev.Mode == paths.AuditModeSync {
		return a.svc.RecordSync(ae)
	}
	return a.svc.RecordBatch(ae)
}

// pathsEventOutcome maps a paths.LockEvent kind to the closed-set
// audit Outcome literal. version-stale is the only failed case in the
// current event-schema (the write was rejected because the on-disk
// version moved out from under the caller); every other event records
// a successful primitive step.
func pathsEventOutcome(kind string) string {
	switch kind {
	case paths.EventVersionStale:
		return audit.OutcomeFailed
	default:
		return audit.OutcomeOK
	}
}
