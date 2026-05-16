// SPDX-Licence-Identifier: EUPL-1.2

// Package paths is the single source of truth for the lthn filesystem
// layout. Every service that touches disk resolves through here so the
// canonical visible roots stay uniform.
//
// Layout (Snider 2026-05-12 — no-hidden-bloat principle):
//
//	~/Lethean/                  visible root (NEVER ~/.lthn/)
//	  conf/
//	    lthn.yaml               user-facing settings
//	    models/                 model snapshots
//	  data/
//	    lthn.db                 SQLite KV via go-store
//	    workspace/              DuckDB workspace buffer
//	    sessions/               chat session blobs
//	  wallets/                  Lethean wallet keystores
//	  cli/                      shell completions, fixtures
//
// Each function returns the path AND ensures the parent exists.
// Returning core.Result so consumers branch on r.OK.
//
// Usage example:
//
//	r := paths.ConfDir()
//	if !r.OK { return r }
//	conf := r.Value.(string)
package paths

import core "dappco.re/go"

// Root returns ~/Lethean/. Creates the directory if missing.
//
// Usage example:
//
//	r := paths.Root()
//	if r.OK { root := r.Value.(string); _ = root }
func Root() core.Result {
	home := core.UserHomeDir()
	if !home.OK {
		return home
	}
	root := core.PathJoin(home.Value.(string), "Lethean")
	if r := core.MkdirAll(root, 0o755); !r.OK {
		return r
	}
	return core.Ok(root)
}

// ConfDir returns ~/Lethean/conf/. Creates if missing.
//
// Usage example:
//
//	r := paths.ConfDir()
//	if r.OK { conf := r.Value.(string); _ = conf }
func ConfDir() core.Result {
	return subdir("conf")
}

// DataDir returns ~/Lethean/data/. Creates if missing.
//
// Usage example:
//
//	r := paths.DataDir()
//	if r.OK { data := r.Value.(string); _ = data }
func DataDir() core.Result {
	return subdir("data")
}

// WalletsDir returns ~/Lethean/wallets/. Creates if missing.
//
// Usage example:
//
//	r := paths.WalletsDir()
//	if r.OK { wallets := r.Value.(string); _ = wallets }
func WalletsDir() core.Result {
	return subdir("wallets")
}

// CliDir returns ~/Lethean/cli/. Creates if missing.
//
// Usage example:
//
//	r := paths.CliDir()
//	if r.OK { cli := r.Value.(string); _ = cli }
func CliDir() core.Result {
	return subdir("cli")
}

// ModelsDir returns the models directory. Honours the user-set
// override at ~/Lethean/conf/paths.json when present (set via
// SetModelsDirOverride), otherwise falls through to the default
// ~/Lethean/conf/models/. Creates if missing.
//
// The override hot-path is the read-side counterpart to the
// SetModelsDirOverride / ClearModelsDirOverride producers in
// models_override.go. readModelsDirOverride returns the empty
// string on no-override / read-failure / parse-failure, so a
// corrupt or absent paths.json never blocks ModelsDir() — it just
// falls back to the default. Validation happens at set-time
// (validateOverridePath); read-time trusts what's persisted.
//
// Usage example:
//
//	r := paths.ModelsDir()
//	if r.OK { models := r.Value.(string); _ = models }
func ModelsDir() core.Result {
	if override := readModelsDirOverride(); override != "" {
		if r := core.MkdirAll(override, 0o755); !r.OK {
			return r
		}
		return core.Ok(override)
	}
	conf := ConfDir()
	if !conf.OK {
		return conf
	}
	dir := core.PathJoin(conf.Value.(string), "models")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// ConfigFile returns ~/Lethean/conf/lthn.yaml. Path only — does not
// ensure the file exists. Use for read attempts that should branch on
// file-not-found.
//
// Usage example:
//
//	r := paths.ConfigFile()
//	if r.OK { _ = r.Value.(string) }
func ConfigFile() core.Result {
	conf := ConfDir()
	if !conf.OK {
		return conf
	}
	return core.Ok(core.PathJoin(conf.Value.(string), "lthn.yaml"))
}

// StoreDB returns ~/Lethean/data/lthn.db. Path only — go-store creates
// the file on first open.
//
// Usage example:
//
//	r := paths.StoreDB()
//	if r.OK { _ = r.Value.(string) }
func StoreDB() core.Result {
	data := DataDir()
	if !data.OK {
		return data
	}
	return core.Ok(core.PathJoin(data.Value.(string), "lthn.db"))
}

// MasterDB returns ~/Lethean/data/lthn.duckdb. The master relational
// database — tasks, agent_activity, fleet machines/routing, connector
// sync state, plugin state. Sits alongside lthn.db (SQLite KV) and
// workspace/ (DuckDB scratch); this is the canonical relational store
// for the desktop runtime. Path only — store.OpenDuckDB() creates the
// file on first open.
//
// Usage example:
//
//	r := paths.MasterDB()
//	if r.OK { _ = r.Value.(string) }
func MasterDB() core.Result {
	data := DataDir()
	if !data.OK {
		return data
	}
	return core.Ok(core.PathJoin(data.Value.(string), "lthn.duckdb"))
}

// DesktopDir returns ~/Lethean/data/desktop/. Per-app namespace for
// lthn/desktop's persistent state. Separate per-subsystem DBs live
// here (ai.duckdb, ml.duckdb) so each can grow without contention
// against the shared master DB and so future apps (ofm, hostuk) can
// land alongside under their own ~/Lethean/data/<app>/ subfolder.
// Mode 0o755 (owner read+write, group/other read).
//
// Usage example:
//
//	r := paths.DesktopDir()
//	if r.OK { _ = r.Value.(string) }
func DesktopDir() core.Result {
	data := DataDir()
	if !data.OK {
		return data
	}
	dir := core.PathJoin(data.Value.(string), "desktop")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// AIDB returns ~/Lethean/data/desktop/ai.duckdb. Persistent settings
// + state for the ai subsystem — provider routes, conversation
// history, model metadata. Sized to grow independently of the master
// DB; ai workloads (long chats, tool transcripts, training-data
// candidates) can balloon and shouldn't contend with fleet/tasks for
// the master lock. Path only — store.OpenDuckDB() creates the file on
// first open.
//
// Usage example:
//
//	r := paths.AIDB()
//	if r.OK { _ = r.Value.(string) }
func AIDB() core.Result {
	dir := DesktopDir()
	if !dir.OK {
		return dir
	}
	return core.Ok(core.PathJoin(dir.Value.(string), "ai.duckdb"))
}

// MLDB returns ~/Lethean/data/desktop/ml.duckdb. Persistent settings
// + state for the ml subsystem — training run records, LoRA / GRPO
// job state, dataset manifests, evaluation results, model-pack
// registry. Like ai.duckdb, sized to grow independently so heavy ml
// workloads (long training logs, weight checkpoint metadata) don't
// pressure the master DB. Path only — store.OpenDuckDB() creates the
// file on first open.
//
// Usage example:
//
//	r := paths.MLDB()
//	if r.OK { _ = r.Value.(string) }
func MLDB() core.Result {
	dir := DesktopDir()
	if !dir.OK {
		return dir
	}
	return core.Ok(core.PathJoin(dir.Value.(string), "ml.duckdb"))
}

// KeysDir returns ~/Lethean/data/keys/. Encrypted-at-rest blobs
// (sealed-box / age) — wallet seeds, API tokens, signing keys. Never
// flat files. Mode 0700 (owner-only).
//
// Usage example:
//
//	r := paths.KeysDir()
//	if r.OK { _ = r.Value.(string) }
func KeysDir() core.Result {
	data := DataDir()
	if !data.OK {
		return data
	}
	dir := core.PathJoin(data.Value.(string), "keys")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// WorkspaceDir returns ~/Lethean/data/workspace/. Used by go-store for
// the DuckDB workspace buffer.
//
// Usage example:
//
//	r := paths.WorkspaceDir()
//	if r.OK { _ = r.Value.(string) }
func WorkspaceDir() core.Result {
	data := DataDir()
	if !data.OK {
		return data
	}
	dir := core.PathJoin(data.Value.(string), "workspace")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

func subdir(name string) core.Result {
	root := Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), name)
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}
