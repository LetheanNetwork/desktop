// SPDX-Licence-Identifier: EUPL-1.2

// ImportFromHost — spawns the user's *host* opencode binary in
// serve mode on a free port, drains /project + /provider, reads
// ~/.local/share/opencode/auth.json for credentials, and persists
// everything in the lthn orm so the user keeps working without
// re-authenticating + re-finding their projects.
//
// Credentials policy (per Snider, 2026-05-15): "if it has keys,
// then the keys too, so we dont break stuff". The alternative —
// definitions-only — means every imported project breaks until
// the user re-auths per provider. We trust local DuckDB to keep
// the keys local; the import is a local-to-local capture, no
// network exfil.
//
// Spawn lifecycle:
//   1. Allocate a free host port (kernel-pick + close).
//   2. Generate ephemeral OPENCODE_SERVER_PASSWORD for the run.
//   3. process.StartWithOptions("opencode", "serve", --port N).
//   4. Wait for /global/health.
//   5. Fetch /project + /provider.
//   6. Read auth.json side-channel for keys.
//   7. Kill the spawned serve (defer).
//   8. Persist rows via orm.

package opencode

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	goio "io"
	"net/http"
	"time"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/go/process"
)

// ImportSummary is the result shape returned to callers.
type ImportSummary struct {
	// Projects is the count of project rows successfully upserted.
	Projects int `json:"projects"`
	// Providers is the count of provider rows upserted.
	Providers int `json:"providers"`
	// ProvidersWithAuth is the subset of Providers that had auth
	// material in the host's auth.json (so the user can actually
	// use the provider without re-authenticating).
	ProvidersWithAuth int `json:"providers_with_auth"`
}

// ImportFromHost runs the full import cycle. Idempotent —
// re-running upserts every row (last-write-wins on ImportedAt).
//
// Usage example:
//
//	r := svc.ImportFromHost()
//	if r.OK { sum := r.Value.(opencode.ImportSummary); _ = sum }
func (s *Service) ImportFromHost() core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.ImportFromHost", "service is nil", nil))
	}
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.ImportFromHost", "process service unavailable", nil))
	}

	// 1. Free port.
	portR := allocatePort()
	if !portR.OK {
		return portR
	}
	port := portR.Value.(int)

	// 2. Ephemeral password — different from the per-install
	// OPENCODE_SERVER_PASSWORD we use for our sandboxes (this serve
	// lives for ~3 seconds, no shared-state benefit to reusing).
	pwBuf := make([]byte, 24)
	if _, err := rand.Read(pwBuf); err != nil {
		return core.Fail(core.E("opencode.ImportFromHost", "rand read failed", err))
	}
	pw := hex.EncodeToString(pwBuf)
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(serverAuthUsername+":"+pw))

	// 3. Spawn `opencode serve --port N --hostname 127.0.0.1`.
	target := core.Sprintf("http://127.0.0.1:%d", port)
	ctx, cancel := core.WithTimeout(core.Background(), 30*time.Second)
	defer cancel()
	procR := ps.StartWithOptions(ctx, process.RunOptions{
		Command: "opencode",
		Args: []string{
			"serve",
			"--port", core.Sprintf("%d", port),
			"--hostname", "127.0.0.1",
		},
		Env:     []string{"OPENCODE_SERVER_PASSWORD=" + pw},
		Timeout: 20 * time.Second,
	})
	if !procR.OK {
		return procR
	}
	proc, _ := procR.Value.(*process.ManagedProcess)
	// Ensure the temporary serve dies even on early return.
	defer func() {
		if proc != nil {
			_ = proc.Kill()
		}
	}()

	// 4. Wait for health — generous because cold-start can probe
	// the user's huge opencode.db (46MB on Snider's host).
	if r := waitHealthy(target, authHeader, 15*time.Second); !r.OK {
		return core.Fail(core.E("opencode.ImportFromHost",
			"host opencode serve never became healthy: "+r.Error(), nil))
	}

	// 5. /project + /provider.
	projects, err := importFetchJSON(target+"/project", authHeader)
	if err != nil {
		return core.Fail(core.E("opencode.ImportFromHost", "GET /project failed", err))
	}
	providers, err := importFetchJSON(target+"/provider", authHeader)
	if err != nil {
		return core.Fail(core.E("opencode.ImportFromHost", "GET /provider failed", err))
	}

	// 6. auth.json side-channel — keyed by provider id.
	authMap := readHostAuthJSON()

	// 7. Kill happens via defer.
	// 8. Persist.
	now := time.Now()
	projectsList, _ := projects.([]any)
	providersWrap, _ := providers.(map[string]any)
	providersList, _ := providersWrap["all"].([]any)

	pCount := persistProjects(s.Core(), projectsList, now)
	prCount, withAuth := persistProviders(s.Core(), providersList, authMap, now)

	return core.Ok(ImportSummary{
		Projects:          pCount,
		Providers:         prCount,
		ProvidersWithAuth: withAuth,
	})
}

// importFetchJSON GETs a JSON endpoint with Basic auth and decodes
// to any. Used for /project + /provider — caller type-asserts the
// expected shape.
func importFetchJSON(url, authHeader string) (any, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := goio.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, core.E("opencode.importFetchJSON",
			core.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	}
	var out any
	if r := core.JSONUnmarshal(body, &out); !r.OK {
		return nil, core.E("opencode.importFetchJSON", "decode: "+r.Error(), nil)
	}
	return out, nil
}

// readHostAuthJSON loads ~/.local/share/opencode/auth.json into a
// {providerID → {type,key,...}} map. Missing file / parse errors
// fall back to an empty map — auth-less providers still import.
func readHostAuthJSON() map[string]map[string]any {
	out := map[string]map[string]any{}
	homeR := core.UserHomeDir()
	if !homeR.OK {
		return out
	}
	home, _ := homeR.Value.(string)
	path := core.PathJoin(home, ".local/share/opencode/auth.json")
	r := core.ReadFile(path)
	if !r.OK {
		return out
	}
	data, _ := r.Value.([]byte)
	if len(data) == 0 {
		return out
	}
	if r := core.JSONUnmarshal(data, &out); !r.OK {
		return map[string]map[string]any{}
	}
	return out
}

// persistProjects upserts ImportedProject rows from the /project
// JSON array. Returns count of rows successfully written.
func persistProjects(c *core.Core, projects []any, now time.Time) int {
	count := 0
	for _, raw := range projects {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sourceID := stringFrom(p, "id")
		if sourceID == "" {
			continue
		}
		worktree := stringFrom(p, "worktree")
		name := projectNameFrom(worktree, sourceID)

		sandboxesJSON := ""
		if sandboxes, ok := p["sandboxes"]; ok && sandboxes != nil {
			sandboxesJSON = core.JSONMarshalString(sandboxes)
		}

		iconColor, iconURL := "", ""
		if icon, ok := p["icon"].(map[string]any); ok {
			iconColor, _ = icon["color"].(string)
			iconURL, _ = icon["url"].(string)
		}

		var uCreated, uUpdated time.Time
		if t, ok := p["time"].(map[string]any); ok {
			uCreated = unixMillis(t["created"])
			uUpdated = unixMillis(t["updated"])
		}

		rec := ImportedProject{
			ID:                SourceOpenCodeHost + ":" + sourceID,
			Source:            SourceOpenCodeHost,
			SourceID:          sourceID,
			Name:              name,
			Worktree:          worktree,
			VCS:               stringFrom(p, "vcs"),
			IconColor:         iconColor,
			IconDataURL:       iconURL,
			SandboxesJSON:     sandboxesJSON,
			UpstreamCreatedAt: uCreated,
			UpstreamUpdatedAt: uUpdated,
			ImportedAt:        now,
		}
		if r := orm.Of[ImportedProject](c).Save(&rec); r.OK {
			count++
		}
	}
	return count
}

// persistProviders upserts ImportedProvider rows, looking up auth
// material per-provider-id in authMap. Returns (count, withAuth).
func persistProviders(c *core.Core, providers []any, authMap map[string]map[string]any, now time.Time) (int, int) {
	count, withAuth := 0, 0
	for _, raw := range providers {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		pid := stringFrom(p, "id")
		if pid == "" {
			continue
		}

		optsJSON := ""
		if opts, ok := p["options"]; ok && opts != nil {
			optsJSON = core.JSONMarshalString(opts)
		}

		authType, authKey := "", ""
		if entry, ok := authMap[pid]; ok {
			authType, _ = entry["type"].(string)
			authKey, _ = entry["key"].(string)
		}
		hasAuth := authKey != ""
		if hasAuth {
			withAuth++
		}

		rec := ImportedProvider{
			ID:          SourceOpenCodeHost + ":" + pid,
			Source:      SourceOpenCodeHost,
			ProviderID:  pid,
			Name:        stringFrom(p, "name"),
			NPM:         stringFrom(p, "npm"),
			OptionsJSON: optsJSON,
			AuthType:    authType,
			AuthKey:     authKey,
			HasAuth:     hasAuth,
			ImportedAt:  now,
		}
		if r := orm.Of[ImportedProvider](c).Save(&rec); r.OK {
			count++
		}
	}
	return count, withAuth
}

// stringFrom safely fetches a string field from a map[string]any.
func stringFrom(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// unixMillis converts a JSON-decoded numeric field (opencode uses
// float64 for unix-ms) into a time.Time. Zero on absent / non-numeric.
func unixMillis(v any) time.Time {
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(int64(n))
	case int64:
		return time.UnixMilli(n)
	}
	return time.Time{}
}

// projectNameFrom picks a human-readable label from the worktree
// path, falling back to the upstream source id when the worktree
// is virtual (opencode's "/" pseudo-projects).
func projectNameFrom(worktree, fallback string) string {
	wt := core.Trim(worktree)
	if wt == "" || wt == "/" {
		return fallback
	}
	return core.PathBase(wt)
}

// ListImports returns every ImportedProject ordered by most-
// recently-imported first. Used by `lthn opencode imports`.
//
// Usage example:
//
//	r := svc.ListImports()
//	if r.OK { rows := r.Value.([]opencode.ImportedProject); _ = rows }
func (s *Service) ListImports() core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.ListImports", "service is nil", nil))
	}
	return orm.Of[ImportedProject](s.Core()).
		Order("imported_at", "desc").
		Get()
}

// ListImportedProviders returns every ImportedProvider row. Auth
// key included — caller is responsible for not leaking it to
// untrusted contexts (the surface today is local-only so this is
// fine).
//
// Usage example:
//
//	r := svc.ListImportedProviders()
//	if r.OK { rows := r.Value.([]opencode.ImportedProvider); _ = rows }
func (s *Service) ListImportedProviders() core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.ListImportedProviders", "service is nil", nil))
	}
	return orm.Of[ImportedProvider](s.Core()).
		Order("imported_at", "desc").
		Get()
}
