// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	"context"

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
	"dappco.re/lthn/desktop/pkg/bridge"
	lthni18n "dappco.re/lthn/desktop/pkg/i18n"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/sandbox"
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
	)

	if r := c.ServiceStartup(context.Background(), nil); !r.OK {
		core.Print(core.Stderr(), "lthn: startup failed: %s\n", r.Error())
		return nil
	}

	// orm bootstrap — lib not service. Register + mount the default
	// Memium + register schemas for every subsystem that uses orm-backed
	// records. Pattern mirrors core/ide's pkg/server/orm_init.go.
	if r := orm.Register(c); !r.OK {
		core.Print(core.Stderr(), "lthn: orm register failed: %s\n", r.Error())
		return nil
	}
	memium := orm.NewMemium()
	if r := orm.Mount(c, "default", memium); !r.OK {
		core.Print(core.Stderr(), "lthn: orm mount failed: %s\n", r.Error())
		return nil
	}
	for _, schema := range []orm.Schema{
		opencode.Sandbox{}.Schema(),
		opencode.ImportedProject{}.Schema(),
		opencode.ImportedProvider{}.Schema(),
	} {
		if r := orm.RegisterSchema(c, schema); !r.OK {
			core.Print(core.Stderr(), "lthn: orm schema %s failed: %s\n", schema.Name, r.Error())
			return nil
		}
		memium.RegisterTable(schema.Name, schema)
	}

	// Wire imported worktrees into the repos surface — opencode
	// imports show up in the multi-repo dashboard alongside scanned
	// roots. Future codex/claude/pi imports plug in the same way.
	if reposSvc, _ := core.ServiceFor[*repos.Service](c, "repos"); reposSvc != nil {
		reposSvc.RegisterSource("opencode-imports", func(_ context.Context) []string {
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
