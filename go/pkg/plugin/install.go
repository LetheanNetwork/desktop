// SPDX-Licence-Identifier: EUPL-1.2

// Install paths — fetch a plugin's binary from a github.com/dappcore
// release URL, verify against the manifest checksum, and write to
// ~/Lethean/conf/plugins/<code>/. The Phase 1 host allowlist is
// hardcoded to github.com/dappcore/* releases.

package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"

	core "dappco.re/go"
)

// allowedHosts are the binary-hosting domains the Phase 1 fetch
// will accept. github.com is the canonical source for now; an
// expanded allowlist (forge.lthn.sh releases) can land later
// without changing callers.
var allowedHosts = []string{"github.com"}

// allowedPathPrefix narrows the github.com allowance to
// /dappcore/<repo>/releases/download/... — random URLs on github
// are not acceptable. Anchored on /dappcore/ so forks can't
// trivially substitute.
const allowedPathPrefix = "/dappcore/"

// maxBinarySize bounds the download to 32 MB. Most plugin
// binaries are 5-15 MB; 32 MB is a generous cushion that still
// catches "someone tried to ship a model file as a plugin"
// mistakes.
const maxBinarySize = 32 << 20

// downloadTimeout caps the fetch end-to-end. Slow connections
// can still complete a 32 MB transfer in 60 s comfortably.
const downloadTimeout = 60 * core.Second

// verifyURL checks the URL against the host allowlist + path
// prefix. Returns the parsed URL on success.
func verifyURL(rawURL string) core.Result {
	u, err := url.Parse(rawURL)
	if err != nil {
		return core.Fail(core.E(verifyURLOp, "parse failed: "+err.Error(), nil))
	}
	if u.Scheme != "https" {
		return core.Fail(core.E(verifyURLOp, "https required, got "+u.Scheme, nil))
	}
	hostAllowed := false
	for _, h := range allowedHosts {
		if u.Host == h {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return core.Fail(core.E(verifyURLOp, "host not on allowlist: "+u.Host, nil))
	}
	if !core.HasPrefix(u.Path, allowedPathPrefix) {
		return core.Fail(core.E(verifyURLOp,
			"path must start with "+allowedPathPrefix+", got "+u.Path, nil))
	}
	return core.Ok(u)
}

// fetchBinary downloads `rawURL` (after allowlist check) up to
// maxBinarySize bytes. Returns the raw bytes for verifyChecksum
// to inspect before they hit disk.
func fetchBinary(ctx core.Context, rawURL string) core.Result {
	if res := verifyURL(rawURL); !res.OK {
		return res
	}
	cctx, cancel := core.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return core.Fail(core.E(fetchBinaryOp, "build request: "+err.Error(), nil))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return core.Fail(core.E(fetchBinaryOp, "http: "+err.Error(), nil))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return core.Fail(core.E(fetchBinaryOp,
			"http status "+core.Itoa(resp.StatusCode)+" for "+rawURL, nil))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		return core.Fail(core.E(fetchBinaryOp, "read body: "+err.Error(), nil))
	}
	if len(body) > maxBinarySize {
		return core.Fail(core.E(fetchBinaryOp,
			"binary exceeds "+core.Itoa(maxBinarySize)+" bytes", nil))
	}
	return core.Ok(body)
}

// verifyChecksum compares the SHA-256 of `data` against the
// manifest's checksum string. Format: "sha256:<hex>" (lowercase
// hex). Manifest without a checksum is allowed — the verify
// becomes a no-op so dev plugins can ship without a published
// hash. Production plugins should always declare one.
func verifyChecksum(data []byte, checksum string) core.Result {
	checksum = core.Trim(checksum)
	if checksum == "" {
		return core.Ok(nil)
	}
	const prefix = "sha256:"
	if !core.HasPrefix(checksum, prefix) {
		return core.Fail(core.E("plugin.verifyChecksum",
			"unsupported algorithm in "+checksum+" (need sha256:)", nil))
	}
	want := core.Lower(checksum[len(prefix):])
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return core.Fail(core.E("plugin.verifyChecksum",
			"checksum mismatch: want "+want+" got "+got, nil))
	}
	return core.Ok(nil)
}

// writePlugin lays out a fresh plugin on disk:
//
//	~/Lethean/conf/plugins/<code>/
//	├── plugin.json
//	├── bin/<binary>
//	└── data/         (created empty; plugin writes here)
//
// Returns the install directory path on success.
func writePlugin(m Manifest, binary []byte) core.Result {
	dirR := pluginDir(m.Code)
	if !dirR.OK {
		return dirR
	}
	dir, _ := dirR.Value.(string)
	if r := core.MkdirAll(core.PathJoin(dir, "bin"), 0o755); !r.OK {
		return core.Fail(core.E(writePluginOp, "mkdir bin: "+r.Error(), nil))
	}
	if r := core.MkdirAll(core.PathJoin(dir, "data"), 0o755); !r.OK {
		return core.Fail(core.E(writePluginOp, "mkdir data: "+r.Error(), nil))
	}
	binPath := core.PathJoin(dir, m.Binary)
	if r := core.WriteFile(binPath, binary, 0o755); !r.OK {
		return core.Fail(core.E(writePluginOp, "write binary: "+r.Error(), nil))
	}
	if r := saveManifest(dir, m); !r.OK {
		return core.Fail(core.E(writePluginOp, "write manifest: "+r.Error(), nil))
	}
	return core.Ok(dir)
}

// removePlugin deletes the plugin's install directory + every
// file under it. Caller must Stop the plugin first.
func removePlugin(code string) core.Result {
	dirR := pluginDir(code)
	if !dirR.OK {
		return dirR
	}
	installDir, _ := dirR.Value.(string)
	if r := core.RemoveAll(installDir); !r.OK {
		return core.Fail(core.E("plugin.removePlugin", "remove "+installDir+": "+r.Error(), nil))
	}
	return core.Ok(nil)
}

// scanInstalled walks the install root looking for valid
// plugin.json files and returns one InstalledPlugin per entry.
// Plugins with corrupt manifests are skipped (not surfaced as
// errors) so a single bad install doesn't break the whole list.
func (s *Service) scanInstalled() []InstalledPlugin {
	rootR := installRoot()
	if !rootR.OK {
		return nil
	}
	root, _ := rootR.Value.(string)
	listing := core.ReadDir(core.DirFS(root), ".")
	if !listing.OK {
		return nil
	}
	entries, _ := listing.Value.([]core.FsDirEntry)
	out := make([]InstalledPlugin, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		code := entry.Name()
		dir := core.PathJoin(root, code)
		mr := loadManifest(core.PathJoin(dir, "plugin.json"))
		if !mr.OK {
			continue
		}
		m, _ := mr.Value.(Manifest)
		s.mu.RLock()
		status := s.statusFor(code)
		s.mu.RUnlock()
		out = append(out, InstalledPlugin{
			Code:      m.Code,
			Name:      m.Name,
			Version:   m.Version,
			Namespace: m.Namespace,
			Dir:       dir,
			Status:    status,
		})
	}
	return out
}
