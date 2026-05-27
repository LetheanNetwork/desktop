// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// cmdMarketplace dispatches `lthn marketplace <verb>` — in-process
// calls against marketplace.Service, NOT a thin HTTP client (compare
// cmd/lthn/opencode.go which IS HTTP-shaped because opencode state
// lives in the long-running serve daemon). Every other persistent
// subsystem in this binary (state / events / process / permissions /
// sessions / models / api / fleet) is in-process; marketplace install
// state is orm-backed (queryable directly), so HTTP indirection would
// be ceremony without payoff.
//
// Verbs:
//
//	list                          installed bundles
//	search [QUERY]                browse the catalogue
//	install <bundle-id>           install a bundle from the catalogue
//	start <bundle-id>             launch a stopped bundle
//	stop <bundle-id>              stop a running bundle
//	uninstall <bundle-id>         remove a bundle (sandboxes + record)
//	status <bundle-id>            print one bundle's current state
//	update <bundle-id>            re-pull + restart (uninstall + install)
//
// Usage example:
//
//	lthn marketplace list
//	lthn marketplace search ollama
//	lthn marketplace install opencode
//	lthn marketplace status opencode
func cmdMarketplace(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(),
			"lthn marketplace: missing verb (list / search / install / start / stop / uninstall / status / import-coolify)\n")
		core.Print(core.Stderr(),
			"  also accepts: update BUNDLE-ID  (not yet wired — use uninstall + install to re-pull)\n")
		return 2
	}
	// import-coolify doesn't need a Core — pure file-to-file conversion.
	// Branch early so the user can pipe through `lthn marketplace
	// import-coolify` without spinning up the full service stack.
	if args[0] == "import-coolify" {
		return marketplaceImportCoolify(args[1:])
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	svc := marketplace.NewService(c)

	switch args[0] {
	case "list":
		return marketplaceList(svc, args[1:])
	case "search":
		return marketplaceSearch(svc, args[1:])
	case "install":
		return marketplaceInstall(svc, args[1:])
	case "start":
		return marketplaceStart(svc, args[1:])
	case "stop":
		return marketplaceStop(svc, args[1:])
	case "uninstall":
		return marketplaceUninstall(svc, args[1:])
	case "status":
		return marketplaceStatus(svc, args[1:])
	case "update":
		return marketplaceUpdate(svc, args[1:])
	default:
		core.Print(core.Stderr(), "lthn marketplace: unknown verb %q\n", args[0])
		return 2
	}
}

// marketplaceList prints currently-installed bundles as JSON. Empty
// list when nothing is installed yet (zero-row response, not an error).
func marketplaceList(svc *marketplace.Service, _ []string) int {
	r := svc.ListInstalled()
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace list: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceSearch browses the live catalogue. Passes args[0] (or
// empty) as the query + an optional --category flag. Output is JSON
// for machine consumption; humans get the bundle name + description
// rendered by the GUI.
func marketplaceSearch(svc *marketplace.Service, args []string) int {
	query := ""
	category := ""
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid {
			if query == "" {
				query = args[i]
			}
			continue
		}
		if k == "category" {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			category = v
		}
	}

	idxR := svc.FetchIndex("")
	if !idxR.OK {
		core.Print(core.Stderr(), "lthn marketplace search: %s\n", idxR.Error())
		return 1
	}
	idx, ok := idxR.Value.(marketplace.FetchIndexResult)
	if !ok {
		core.Print(core.Stderr(), "lthn marketplace search: unexpected index shape\n")
		return 1
	}
	results := marketplace.SearchCatalogue(idx, query, category)
	core.Println(core.JSONMarshalString(map[string]any{
		"query":    query,
		"category": category,
		"count":    len(results),
		"results":  results,
	}))
	return 0
}

// marketplaceInstall resolves the bundle-id against the catalogue,
// fetches + parses its manifest, and runs the install lifecycle.
// Env values from --env KEY=VALUE flags pass through to the manifest's
// env block. Sandbox containers spawn via pkg/sandbox.SpawnLong.
func marketplaceInstall(svc *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace install: usage: lthn marketplace install BUNDLE-ID [--env KEY=VALUE ...]\n")
		return 2
	}
	bundleID := args[0]
	env := map[string]string{}
	for i := 1; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid || k != "env" {
			continue
		}
		if v == "" && i+1 < len(args) {
			i++
			v = args[i]
		}
		// --env KEY=VALUE — split on the first =
		for j := 0; j < len(v); j++ {
			if v[j] == '=' {
				env[v[:j]] = v[j+1:]
				break
			}
		}
	}

	// Resolve bundle-id → catalogue entry → manifest source URL → manifest.
	idxR := svc.FetchIndex("")
	if !idxR.OK {
		core.Print(core.Stderr(), "lthn marketplace install: %s\n", idxR.Error())
		return 1
	}
	idx := idxR.Value.(marketplace.FetchIndexResult)
	var entry *marketplace.CatalogueEntry
	for i := range idx.Entries {
		if idx.Entries[i].Name == bundleID {
			entry = &idx.Entries[i]
			break
		}
	}
	if entry == nil {
		core.Print(core.Stderr(), "lthn marketplace install: bundle not found in catalogue: %s\n", bundleID)
		return 1
	}

	manR := svc.FetchManifest(entry.SourceURL)
	if !manR.OK {
		core.Print(core.Stderr(), "lthn marketplace install: %s\n", manR.Error())
		return 1
	}
	manifest, ok := manR.Value.(marketplace.BundleManifest)
	if !ok {
		core.Print(core.Stderr(), "lthn marketplace install: unexpected manifest shape\n")
		return 1
	}

	r := svc.Install(marketplace.InstallInput{
		Manifest: manifest,
		Env:      env,
	})
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace install: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceStart launches the named bundle's containers. Idempotent —
// re-launching a running bundle is a no-op handled by Service.Launch.
func marketplaceStart(svc *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace start: usage: lthn marketplace start BUNDLE-ID\n")
		return 2
	}
	r := svc.Launch(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace start: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceStop halts the named bundle's containers without removing
// the install record. Re-startable via `lthn marketplace start`.
func marketplaceStop(svc *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace stop: usage: lthn marketplace stop BUNDLE-ID\n")
		return 2
	}
	r := svc.Stop(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace stop: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceUninstall removes a bundle entirely — stops sandboxes,
// drops the orm record, leaves volumes (per RFC §5.4 the user opts
// in to volume removal separately).
func marketplaceUninstall(svc *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace uninstall: usage: lthn marketplace uninstall BUNDLE-ID\n")
		return 2
	}
	r := svc.Uninstall(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace uninstall: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceStatus prints one bundle's current state — the
// orm-backed Status value plus the per-image sandbox handle snapshot.
func marketplaceStatus(svc *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace status: usage: lthn marketplace status BUNDLE-ID\n")
		return 2
	}
	r := svc.Status(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace status: %s\n", r.Error())
		return 1
	}
	core.Println(core.JSONMarshalString(r.Value))
	return 0
}

// marketplaceUpdate is the v2 forward-arc verb — re-pull the manifest
// + restart sandboxes on the new image when the digest changed. v1
// emits a clear "not implemented" message rather than silently no-op.
//
// Per RFC.marketplace.md §5.3: "Likely: rolling tags update on launch;
// pinned digests update on user click only" — the design is undecided
// (open question §12). Holding the verb in the dispatch table so the
// CLI surface is forward-compatible without changing main.go later.
func marketplaceUpdate(_ *marketplace.Service, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace update: usage: lthn marketplace update BUNDLE-ID\n")
		return 2
	}
	core.Print(core.Stderr(), "lthn marketplace update: not yet implemented (v2 polish — RFC.marketplace.md §5.3 open question)\n")
	core.Print(core.Stderr(), "hint: `lthn marketplace uninstall %s && lthn marketplace install %s` re-pulls today\n", args[0], args[0])
	return 1
}

// marketplaceImportCoolify reads a Coolify template directory + emits
// the equivalent lthn-vm manifest.yml to stdout. Per RFC.marketplace
// §12: "Would slash the conversion effort for the long-tail bundles".
//
// The output goes to stdout (so `> bundles/foo/manifest.yml` captures
// it); a one-line summary goes to stderr so the redirect doesn't
// swallow the human-readable feedback.
//
// Usage example:
//
//	lthn marketplace import-coolify ./coolify-templates/n8n > bundles/n8n/manifest.yml
func marketplaceImportCoolify(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn marketplace import-coolify: usage: lthn marketplace import-coolify PATH\n")
		core.Print(core.Stderr(), "  PATH is a directory containing docker-compose.yml + optional .env.template\n")
		return 2
	}
	r := marketplace.ImportCoolify(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn marketplace import-coolify: %s\n", r.Error())
		return 1
	}
	m := r.Value.(marketplace.BundleManifest)
	out := marketplace.MarshalManifest(m)
	if !out.OK {
		core.Print(core.Stderr(), "lthn marketplace import-coolify: marshal failed: %s\n", out.Error())
		return 1
	}
	yaml := out.Value.([]byte)
	core.Print(core.Stdout(), "%s", string(yaml))
	core.Print(core.Stderr(),
		"imported: name=%s images=%d env=%d (review volumes + ports + secret-heuristic env types before installing)\n",
		m.Name, len(m.Images), len(m.Env))
	return 0
}
