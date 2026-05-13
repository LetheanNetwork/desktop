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
	"dappco.re/go/process"
	"dappco.re/go/store"
	"dappco.re/go/stream"
	"dappco.re/lthn/desktop/pkg/bridge"
	lthni18n "dappco.re/lthn/desktop/pkg/i18n"
	"dappco.re/lthn/desktop/pkg/paths"
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
	dbPath := paths.StoreDB()
	if !dbPath.OK {
		core.Print(core.Stderr(), "lthn: %s\n", dbPath.Error())
		return nil
	}
	workspace := paths.WorkspaceDir()
	if !workspace.OK {
		core.Print(core.Stderr(), "lthn: %s\n", workspace.Error())
		return nil
	}
	dataDir := paths.DataDir()
	if !dataDir.OK {
		core.Print(core.Stderr(), "lthn: %s\n", dataDir.Error())
		return nil
	}
	configFile := paths.ConfigFile()
	if !configFile.OK {
		core.Print(core.Stderr(), "lthn: %s\n", configFile.Error())
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
	)

	if r := c.ServiceStartup(context.Background(), nil); !r.OK {
		core.Print(core.Stderr(), "lthn: startup failed: %s\n", r.Error())
		return nil
	}
	return c
}
