// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for install.go's non-security-gate surface:
// verifyURL, fetchBinary, writePlugin, removePlugin, scanInstalled.
// (The Cerberus #1432/#1447 security gates — verifyChecksum,
// allowedLocalPath — already have their own home in
// install_security_test.go; this file is additive, not a replacement.)
//
// fetchBinary needs an HTTPS round-trip to a URL that literally starts
// "https://github.com/dappcore/..." (verifyURL's allowlist is
// hardcoded), so withLoopbackGithubTransport swaps core.DefaultHTTPClient
// for a client whose Transport dials straight into an httptest.NewTLSServer
// regardless of the target host, with InsecureSkipVerify for the
// self-signed test cert. No real network egress, no real github.com
// contact — a self-contained loopback TLS server standing in for it,
// which is squarely inside the httptest-is-allowed hermetic boundary.
// core.DefaultHTTPClient is restored via t.Cleanup so no other test
// observes the swap (tests in this package never run in parallel).

package plugin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
)

// ─── verifyURL (pure, no network) ───────────────────────────────────────

func TestInstall_VerifyURL_Good(t *core.T) {
	r := verifyURL("https://github.com/dappcore/opencode/releases/download/v1/opencode")
	core.AssertTrue(t, r.OK)
}

func TestInstall_VerifyURL_Bad_NonHTTPS(t *core.T) {
	r := verifyURL("http://github.com/dappcore/opencode/releases/download/v1/opencode")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "https required")
}

func TestInstall_VerifyURL_Bad_HostNotAllowlisted(t *core.T) {
	r := verifyURL("https://evil.example.com/dappcore/opencode/releases/download/v1/opencode")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "host not on allowlist")
}

func TestInstall_VerifyURL_Bad_PathPrefixNotDappcore(t *core.T) {
	r := verifyURL("https://github.com/someoneelse/opencode/releases/download/v1/opencode")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path must start with")
}

func TestInstall_VerifyURL_Bad_MalformedURL(t *core.T) {
	r := verifyURL("https://example.com/%")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "parse failed")
}

// ─── fetchBinary ─────────────────────────────────────────────────────────

// withLoopbackGithubTransport redirects core.DefaultHTTPClient at a local
// httptest.NewTLSServer for the test's duration, regardless of the
// request's target host — so a "https://github.com/dappcore/..." URL
// (required by verifyURL's allowlist) actually lands on the loopback
// fixture server instead of the real internet.
//
// TLS verification stays ON: the pool trusts exactly the test server's
// own generated leaf certificate (no InsecureSkipVerify — a real MITM
// window even in tests), and ServerName is pinned to that certificate's
// own DNS name so hostname verification succeeds against the loopback
// fixture's real identity rather than the request's "github.com" Host.
func withLoopbackGithubTransport(t *core.T, handler http.Handler) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)

	cert := ts.Certificate()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	serverName := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		serverName = cert.DNSNames[0]
	}

	addr := ts.Listener.Addr().String()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			ServerName: serverName,
		},
	}
	orig := core.DefaultHTTPClient
	core.DefaultHTTPClient = &http.Client{Transport: tr}
	t.Cleanup(func() { core.DefaultHTTPClient = orig })
}

const fakeGithubBinaryURL = "https://github.com/dappcore/opencode/releases/download/v1/opencode"

func TestInstall_FetchBinary_Good(t *core.T) {
	withLoopbackGithubTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello-binary"))
	}))
	r := fetchBinary(core.Background(), fakeGithubBinaryURL)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "hello-binary", string(r.Value.([]byte)))
}

// TestInstall_FetchBinary_Bad_URLRejected needs no server at all —
// verifyURL fails before any network attempt.
func TestInstall_FetchBinary_Bad_URLRejected(t *core.T) {
	r := fetchBinary(core.Background(), "http://github.com/dappcore/x")
	core.AssertFalse(t, r.OK)
}

func TestInstall_FetchBinary_Bad_NonOKStatus(t *core.T) {
	withLoopbackGithubTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	r := fetchBinary(core.Background(), fakeGithubBinaryURL)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "http status 404")
}

func TestInstall_FetchBinary_Ugly_OversizedBinaryRejected(t *core.T) {
	oversized := make([]byte, maxBinarySize+1024)
	withLoopbackGithubTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(oversized)
	}))
	r := fetchBinary(core.Background(), fakeGithubBinaryURL)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "exceeds")
}

// ─── writePlugin ─────────────────────────────────────────────────────────

func TestInstall_WritePlugin_Good(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	r := writePlugin(m, []byte("fake-binary-bytes"))
	core.RequireTrue(t, r.OK)
	dir := r.Value.(string)
	core.AssertEqual(t, tmp+"/Lethean/conf/plugins/opencode", dir)

	bin := core.ReadFile(core.PathJoin(dir, "bin", "opencode"))
	core.RequireTrue(t, bin.OK)
	core.AssertEqual(t, "fake-binary-bytes", string(bin.Value.([]byte)))

	manifestFile := core.ReadFile(core.PathJoin(dir, "plugin.json"))
	core.AssertTrue(t, manifestFile.OK)
}

// TestInstall_WritePlugin_Bad_MkdirBlockedByExistingFile is real fault
// injection: the plugin dir slot is already occupied by a plain file
// (not a directory), so MkdirAll("<dir>/bin") fails with ENOTDIR.
func TestInstall_WritePlugin_Bad_MkdirBlockedByExistingFile(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := tmp + "/Lethean/conf/plugins"
	core.RequireTrue(t, core.MkdirAll(root, 0o755).OK)
	// "opencode" exists as a FILE, not a directory.
	core.RequireTrue(t, core.WriteFile(root+"/opencode", []byte("x"), 0o644).OK)

	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	r := writePlugin(m, []byte("bytes"))
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "mkdir bin")
}

func TestInstall_WritePlugin_Bad_PluginDirUnresolvable(t *core.T) {
	t.Setenv("HOME", "")
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	r := writePlugin(m, []byte("bytes"))
	core.AssertFalse(t, r.OK)
}

// ─── removePlugin ────────────────────────────────────────────────────────

func TestInstall_RemovePlugin_Good(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/plugin.json", []byte("{}"), 0o644).OK)

	r := removePlugin("opencode")
	core.RequireTrue(t, r.OK)
	listing := core.ReadDir(core.DirFS(tmp+"/Lethean/conf/plugins"), ".")
	if listing.OK {
		entries := listing.Value.([]core.FsDirEntry)
		for _, e := range entries {
			core.AssertNotEqual(t, "opencode", e.Name())
		}
	}
}

func TestInstall_RemovePlugin_Ugly_NeverInstalledIsNoOp(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := removePlugin("never-installed")
	core.AssertTrue(t, r.OK, "os.RemoveAll on a missing path is not an error")
}

func TestInstall_RemovePlugin_Bad_PluginDirUnresolvable(t *core.T) {
	t.Setenv("HOME", "")
	r := removePlugin("opencode")
	core.AssertFalse(t, r.OK)
}

// ─── scanInstalled ───────────────────────────────────────────────────────

func TestInstall_ScanInstalled_Good_InstallRootMissingReturnsEmpty(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp) // Lethean/conf/plugins never created
	svc := newTestService(t, core.New())
	core.AssertEqual(t, 0, len(svc.scanInstalled()))
}

func TestInstall_ScanInstalled_Bad_HomeUnresolvable(t *core.T) {
	t.Setenv("HOME", "")
	svc := newTestService(t, core.New())
	core.AssertNil(t, svc.scanInstalled())
}

func TestInstall_ScanInstalled_Ugly_SkipsNonDirsAndCorruptManifests(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := tmp + "/Lethean/conf/plugins"
	good := root + "/opencode"
	broken := root + "/broken"
	core.RequireTrue(t, core.MkdirAll(good, 0o755).OK)
	core.RequireTrue(t, core.MkdirAll(broken, 0o755).OK)
	core.RequireTrue(t, saveManifest(good, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)
	core.RequireTrue(t, core.WriteFile(broken+"/plugin.json", []byte("not json"), 0o644).OK)
	// A stray file directly under the install root — not a dir, must be
	// skipped rather than treated as a plugin.
	core.RequireTrue(t, core.WriteFile(root+"/README.txt", []byte("x"), 0o644).OK)

	svc := newTestService(t, core.New())
	installed := svc.scanInstalled()
	core.RequireTrue(t, len(installed) == 1)
	core.AssertEqual(t, "opencode", installed[0].Code)
	core.AssertEqual(t, "stopped", installed[0].Status.State, "untracked plugin reports stopped")
}
