// SPDX-Licence-Identifier: EUPL-1.2

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
	"dappco.re/go/stream"
	"dappco.re/lthn/desktop/pkg/account"
	lthnai "dappco.re/lthn/desktop/pkg/ai"
	"dappco.re/lthn/desktop/pkg/bridge"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/gateway"
	lthni18n "dappco.re/lthn/desktop/pkg/i18n"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/mdns"
	lthnml "dappco.re/lthn/desktop/pkg/ml"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/plugin"
	lthnprocess "dappco.re/lthn/desktop/pkg/process"
	"dappco.re/lthn/desktop/pkg/queue"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/sandbox"
	"dappco.re/lthn/desktop/pkg/tasks"
	lthnupdate "dappco.re/lthn/desktop/pkg/update"
	"dappco.re/lthn/desktop/pkg/vi"
	"dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/runbooks"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/forecast"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"dappco.re/lthn/desktop/pkg/marketing/content"
	"dappco.re/lthn/desktop/pkg/marketing/social"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
	"dappco.re/lthn/desktop/pkg/marketing/analytics"
	"dappco.re/lthn/desktop/pkg/office/documents"
	"dappco.re/lthn/desktop/pkg/office/mail"
	"dappco.re/lthn/desktop/pkg/office/files"
	"dappco.re/lthn/desktop/pkg/deploys"
	"dappco.re/lthn/desktop/pkg/serverkey"
)

// newAppCore constructs the shared *core.Core for any lthn CLI verb
// that needs the service bus. Registers the phase-1 service stack —
// store / stream / process / i18n / io — with paths resolved through
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
		core.WithName("stream", stream.NewService(stream.DefaultHubConfig())),
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
		// bridge — local MCP HTTP server on 127.0.0.1:9879 letting an
		// external agent (Cladius / Codex / any MCP client) drive +
		// observe the WebView. Console + error capture via the JS
		// shim in frontend/index.html, webview_eval with fetch-back
		// via /internal/eval-reply. Dev-mode focused — bound to
		// localhost so it never leaves this Mac. See pkg/bridge/.
		core.WithName("bridge", bridge.RegisterService(bridge.Options{})),
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

	return c
}
