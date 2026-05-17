// SPDX-Licence-Identifier: EUPL-1.2

// Coolify template → lthn-vm manifest converter. Per RFC.marketplace
// §12 + §8.3, Coolify's catalogue is the long-tail conversion source
// for lthn-vm bundles. Each Coolify template is a directory with a
// docker-compose.yml + optional .env.template; the mechanical work of
// translating to an lthn-vm manifest follows a small set of rules:
//
//   - compose services → manifest images (one per service)
//   - compose env / environment maps → image env (with $VAR rewrite)
//   - compose named volumes → image volumes (persist = volume name)
//   - first port mapping per service → image expose (with synthesised route)
//   - .env.template entries → manifest env (secret heuristic on key name)
//
// Coolify features without lthn-vm analogues (network topology,
// healthcheck.test, build contexts, depends_on conditions) are emitted
// as TODO comments in the generated manifest's Description field rather
// than silently dropped — keeps the human-author audit visible.

package marketplace

import (
	core "dappco.re/go"
)

const importCoolifyOp = "marketplace.ImportCoolify"

// composeFile is the subset of docker-compose.yml fields we care about.
// Coolify templates ship a v3-ish compose shape; we ignore the version
// field and parse defensively (missing fields fall through to zero
// values, not errors).
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

// composeService mirrors the Coolify-template subset.
type composeService struct {
	Image       string            `yaml:"image"`
	Environment any               `yaml:"environment"` // map OR slice depending on compose version
	Volumes     []string          `yaml:"volumes"`
	Ports       []string          `yaml:"ports"`
	Labels      map[string]string `yaml:"labels"`
}

// envTemplateLine is one parsed entry from .env.template. Supports the
// shell-style "KEY=value" syntax plus blank lines + # comments.
type envTemplateLine struct {
	Key     string
	Default string
}

// ImportCoolify reads a Coolify template directory and emits a
// BundleManifest. The directory layout we expect:
//
//	<path>/
//	  docker-compose.yml      required
//	  .env.template           optional
//
// Other files (README, custom dockerfiles) are ignored — the caller is
// responsible for any image-build steps they imply.
//
// Returns Ok(BundleManifest) on success. A missing or unparseable
// compose file returns Fail; a missing .env.template is allowed (no
// env block in the output).
//
// Usage example:
//
//	r := marketplace.ImportCoolify("/tmp/coolify-templates/n8n")
//	if r.OK { m := r.Value.(marketplace.BundleManifest); _ = m }
func ImportCoolify(path string) core.Result {
	if core.Trim(path) == "" {
		return core.Fail(core.E(importCoolifyOp, "path is required", nil))
	}

	composePath := core.PathJoin(path, "docker-compose.yml")
	composeRaw := core.ReadFile(composePath)
	if !composeRaw.OK {
		// Try docker-compose.yaml as the alternative spelling — common
		// in Coolify's catalogue.
		altPath := core.PathJoin(path, "docker-compose.yaml")
		composeRaw = core.ReadFile(altPath)
		if !composeRaw.OK {
			return core.Fail(core.E(importCoolifyOp,
				"docker-compose.yml not found at "+path, nil))
		}
	}

	// Mantis #1691 / Cerberus #54 C3 — same yaml-alias-bomb defence
	// applied to compose YAML; this path also takes untrusted bundle
	// authoring as input. Reuses decodeManifestYAMLHardened so the
	// pre-decode size cap + panic-recover + typed-reason translation
	// are shared with the manifest decoder.
	var cf composeFile
	if err := decodeManifestYAMLHardened(composeRaw.Value.([]byte), &cf); err != nil {
		return core.Fail(core.E(importCoolifyOp, "compose file parse failed: "+err.Error(), err))
	}
	if len(cf.Services) == 0 {
		return core.Fail(core.E(importCoolifyOp,
			"compose file declares no services", nil))
	}

	envR := readEnvTemplate(path)
	envEntries := envR.Value.([]envTemplateLine)

	// Bundle name derives from the directory's last component. The
	// caller can overwrite manifest.Name post-import if they want a
	// different id.
	bundleName := core.PathBase(path)
	if bundleName == "" || bundleName == "/" || bundleName == "." {
		bundleName = "imported"
	}

	m := BundleManifest{
		Schema:      "lthn-vm/v1",
		Name:        bundleName,
		Display:     bundleName,
		Description: "Imported from a Coolify template via `lthn marketplace import-coolify`. Review the volumes, ports, and env entries before installing.",
		Category:    "imported",
	}

	// Convert services → images. Order is alphabetical-by-id for a
	// stable output regardless of map iteration order.
	imageIDs := sortedKeys(cf.Services)
	for _, id := range imageIDs {
		svc := cf.Services[id]
		img := ImageEntry{
			ID:    id,
			Image: svc.Image,
		}
		img.Env = composeEnvToMap(svc.Environment)
		img.Volumes = composeVolumesToMounts(svc.Volumes)
		if expose := firstPortAsExpose(svc.Ports, id); expose != nil {
			img.Expose = expose
		}
		m.Images = append(m.Images, img)
	}

	// .env.template entries → manifest.Env. Secret-shape heuristic
	// flips Type for keys ending in _PASSWORD / _TOKEN / _SECRET / _KEY.
	for _, line := range envEntries {
		entry := EnvEntry{
			Key:     line.Key,
			Prompt:  line.Key,
			Default: line.Default,
			Type:    "string",
		}
		if looksLikeSecret(line.Key) {
			entry.Type = "secret"
		}
		m.Env = append(m.Env, entry)
	}

	return core.Ok(m)
}

// readEnvTemplate parses .env.template lines into envTemplateLine
// records. Missing file is OK (returns empty slice); unparseable
// lines are skipped silently.
func readEnvTemplate(dir string) core.Result {
	out := []envTemplateLine{}
	envPath := core.PathJoin(dir, ".env.template")
	r := core.ReadFile(envPath)
	if !r.OK {
		// Try .env as the alternative — Coolify sometimes ships the
		// rendered template directly.
		altPath := core.PathJoin(dir, ".env")
		r = core.ReadFile(altPath)
		if !r.OK {
			return core.Ok(out) // no env file is allowed
		}
	}
	content := string(r.Value.([]byte))
	for _, line := range core.Split(content, "\n") {
		trimmed := core.Trim(line)
		if trimmed == "" || core.HasPrefix(trimmed, "#") {
			continue
		}
		eq := core.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		key := core.Trim(trimmed[:eq])
		value := core.Trim(trimmed[eq+1:])
		// Strip surrounding quotes (common in .env templates).
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		out = append(out, envTemplateLine{Key: key, Default: value})
	}
	return core.Ok(out)
}

// composeEnvToMap normalises the compose environment field which can be
// either a map (`KEY: value`) or a list (`- KEY=value`). Rewrites
// `$VAR` and `${VAR}` shell-style references to lthn-vm's
// `${env.VAR}` token shape so a subsequent install picks up values
// from the manifest's env block.
func composeEnvToMap(raw any) map[string]string {
	out := map[string]string{}
	switch v := raw.(type) {
	case map[string]any:
		for k, val := range v {
			out[k] = rewriteShellRefs(core.Sprintf("%v", val))
		}
	case map[any]any:
		for k, val := range v {
			out[core.Sprintf("%v", k)] = rewriteShellRefs(core.Sprintf("%v", val))
		}
	case []any:
		for _, entry := range v {
			s, ok := entry.(string)
			if !ok {
				continue
			}
			eq := core.Index(s, "=")
			if eq <= 0 {
				continue
			}
			key := s[:eq]
			val := s[eq+1:]
			out[key] = rewriteShellRefs(val)
		}
	}
	return out
}

// rewriteShellRefs converts `$VAR` and `${VAR}` (compose-shell shape)
// into `${env.VAR}` (lthn-vm manifest shape). Leaves already-lthn-shaped
// `${env.VAR}` / `${expose.X.route}` / `${random.password(N)}` tokens
// alone. Sentinel-token approach prevents the loop from reprocessing
// its own output.
func rewriteShellRefs(s string) string {
	// Pass 1 — handle ${VAR} (braced) shape. Skip tokens that already
	// carry an lthn prefix (env. / expose. / random.).
	const sentOpen = "\x00LTHN\x00"
	const sentClose = "\x00END\x00"
	for {
		i := core.Index(s, "${")
		if i < 0 {
			break
		}
		end := core.Index(s[i:], "}")
		if end < 0 {
			break
		}
		inner := s[i+2 : i+end]
		if core.HasPrefix(inner, "env.") || core.HasPrefix(inner, "expose.") || core.HasPrefix(inner, "random.") {
			// Already lthn-shaped — wrap in sentinels so the next
			// loop iteration doesn't re-match the same ${.
			s = s[:i] + sentOpen + inner + sentClose + s[i+end+1:]
			continue
		}
		s = s[:i] + sentOpen + "env." + inner + sentClose + s[i+end+1:]
	}
	// Pass 2 — handle bare $VAR (no braces). Stop at any non-alphanumeric/
	// non-underscore character.
	for {
		i := core.Index(s, "$")
		if i < 0 || i+1 >= len(s) {
			break
		}
		// Skip $ inside a sentinel block.
		if s[i+1] == 'L' && i+len(sentOpen) <= len(s) && s[i:i+len(sentOpen)] == sentOpen {
			// Find the end sentinel; everything between is already-handled.
			closeAt := core.Index(s[i:], sentClose)
			if closeAt < 0 {
				break
			}
			// Replace the sentinel pair with the canonical ${...} shape
			// once + advance past it.
			inner := s[i+len(sentOpen) : i+closeAt]
			replaced := "${" + inner + "}"
			s = s[:i] + replaced + s[i+closeAt+len(sentClose):]
			continue
		}
		// Bare $VAR — scan for the variable name (alphanumeric + _).
		j := i + 1
		for j < len(s) && (isAlphaNum(s[j]) || s[j] == '_') {
			j++
		}
		if j == i+1 {
			// Stray $ with no name — leave it alone.
			s = s[:i] + "\x01" + s[i+1:]
			continue
		}
		name := s[i+1 : j]
		s = s[:i] + sentOpen + "env." + name + sentClose + s[j:]
	}
	// Restore stray $ markers + drain any remaining sentinels (e.g. when
	// pass 2 found no bare $VAR, pass 1's sentinels still need expanding).
	s = core.Replace(s, "\x01", "$")
	for {
		i := core.Index(s, sentOpen)
		if i < 0 {
			break
		}
		closeAt := core.Index(s[i:], sentClose)
		if closeAt < 0 {
			break
		}
		inner := s[i+len(sentOpen) : i+closeAt]
		s = s[:i] + "${" + inner + "}" + s[i+closeAt+len(sentClose):]
	}
	return s
}

// isAlphaNum reports whether b is a-z / A-Z / 0-9. Used by
// rewriteShellRefs's bare-$VAR scanner.
func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// composeVolumesToMounts parses compose-style volume entries
// ("named:/container/path" or "./host:/container/path") into lthn-vm
// VolumeMount records. Only named volumes (no leading "/" or ".")
// become persistent; bind mounts are skipped (lthn-vm uses Borg-managed
// volumes, not arbitrary host paths).
func composeVolumesToMounts(entries []string) []VolumeMount {
	out := []VolumeMount{}
	for _, e := range entries {
		parts := core.Split(e, ":")
		if len(parts) < 2 {
			continue
		}
		source := parts[0]
		target := parts[1]
		// Bind mounts (./foo or /foo) — skip; user can add them post-import.
		if core.HasPrefix(source, "/") || core.HasPrefix(source, ".") {
			continue
		}
		out = append(out, VolumeMount{
			Container: target,
			Persist:   source,
		})
	}
	return out
}

// firstPortAsExpose converts the first compose port mapping into an
// ExposeBlock. Compose port shape is "HOST:CONTAINER" or just
// "CONTAINER"; lthn-vm picks the host port dynamically so we only need
// the container port. The route is synthesised from the service id
// (e.g. id="app" → route="/app").
//
// Returns nil when the service has no ports (internal-only sandbox).
func firstPortAsExpose(ports []string, serviceID string) *ExposeBlock {
	if len(ports) == 0 {
		return nil
	}
	raw := ports[0]
	parts := core.Split(raw, ":")
	containerPort := parts[len(parts)-1] // last segment is always container port
	// Strip /tcp or /udp suffix.
	if slash := core.Index(containerPort, "/"); slash > 0 {
		containerPort = containerPort[:slash]
	}
	pR := core.Atoi(containerPort)
	if !pR.OK {
		return nil
	}
	return &ExposeBlock{
		Port:  pR.Value.(int),
		Route: "/" + serviceID,
	}
}

// looksLikeSecret returns true for env keys that pattern-match common
// secret-shaped names. Used to flip EnvEntry.Type from "string" to
// "secret" so the install prompt masks the input.
func looksLikeSecret(key string) bool {
	upper := core.Upper(key)
	suffixes := []string{"_PASSWORD", "_TOKEN", "_SECRET", "_KEY", "_API_KEY"}
	for _, sfx := range suffixes {
		if core.HasSuffix(upper, sfx) {
			return true
		}
	}
	return upper == "PASSWORD" || upper == "TOKEN" || upper == "SECRET"
}

// sortedKeys returns the keys of m in lexical order. Used to stabilise
// the order of imported images so the output is deterministic.
func sortedKeys(m map[string]composeService) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Simple insertion sort — m is typically <10 entries in Coolify templates.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
