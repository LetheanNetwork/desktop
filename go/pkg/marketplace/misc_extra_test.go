// SPDX-Licence-Identifier: EUPL-1.2

// misc_extra_test.go — small pure-function / low-effort gaps across
// marketplace.go (search/findByCode), views.go (IframePorts/
// LoopbackOriginFor), manifest.go (ParseManifest), and three
// audit_emit.go helpers only ever fired on a success path that no
// existing or new test reaches end-to-end (Install/Launch success
// needs a real container runtime; FetchManifest success is covered
// elsewhere and already exercises emitFetchManifestSucceeded, but
// emitInstallSucceeded / a couple of install.go's own emit helpers
// remain unreachable without a real docker daemon — direct-called
// here since they're pure "build an audit.Event, call Record" bodies
// with no branching logic worth a scenario test).

package marketplace

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
)

// --- marketplace.go: search / findByCode (unexported, backing Search/Info) ---

func TestSearch_QueryMatchesDescription_Good(t *testing.T) {
	got := search("local runner", "")
	found := false
	for _, p := range got {
		if p.Code == "lemma-runner" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a description-text match to surface lemma-runner, got %+v", got)
	}
}

func TestSearch_CategoryOnly_Good(t *testing.T) {
	got := search("", "agents")
	if len(got) == 0 {
		t.Fatal("expected at least one agents-category fixture entry")
	}
	for _, p := range got {
		if p.Category != "agents" {
			t.Errorf("unexpected category %q in agents-filtered results", p.Category)
		}
	}
}

func TestSearch_NoMatch_Bad(t *testing.T) {
	got := search("zzz-nonexistent-zzz", "")
	if len(got) != 0 {
		t.Errorf("expected zero matches, got %d", len(got))
	}
}

func TestFindByCode_CaseInsensitive_Good(t *testing.T) {
	p, ok := findByCode("CoreAgent")
	if !ok {
		t.Fatal("expected case-insensitive lookup to find coreagent")
	}
	if p.Code != "coreagent" {
		t.Errorf("got code %q", p.Code)
	}
}

func TestFindByCode_NotFound_Bad(t *testing.T) {
	_, ok := findByCode("nope")
	if ok {
		t.Error("expected findByCode to report not-found")
	}
}

// --- views.go: IframePorts / LoopbackOriginFor ---

func TestViewRegistry_IframePorts_Empty_Good(t *testing.T) {
	r := newPluginViewRegistry()
	if got := r.IframePorts(); got != nil {
		t.Errorf("expected nil for an empty registry, got %v", got)
	}
}

func TestViewRegistry_IframePorts_DedupedAcrossPlugins_Good(t *testing.T) {
	r := newPluginViewRegistry()
	if res := r.Add("plugin-a", PluginViewDescriptor{
		ID: "a", Kind: PluginViewKindIframe, LoopbackPort: 4096,
	}); !res.OK {
		t.Fatalf("Add plugin-a: %s", res.Error())
	}
	if res := r.Add("plugin-b", PluginViewDescriptor{
		ID: "b", Kind: PluginViewKindIframe, LoopbackPort: 4097,
	}); !res.OK {
		t.Fatalf("Add plugin-b: %s", res.Error())
	}
	ports := r.IframePorts()
	if len(ports) != 2 {
		t.Fatalf("expected 2 distinct ports, got %v", ports)
	}
}

func TestViewRegistry_LoopbackOriginFor_Good(t *testing.T) {
	r := newPluginViewRegistry()
	if res := r.Add("plugin-a", PluginViewDescriptor{
		ID: "a", Kind: PluginViewKindIframe, LoopbackPort: 5000,
	}); !res.OK {
		t.Fatalf("Add: %s", res.Error())
	}
	origin, ok := r.LoopbackOriginFor("plugin-a")
	if !ok {
		t.Fatal("expected LoopbackOriginFor to find plugin-a")
	}
	if origin != "http://127.0.0.1:5000" {
		t.Errorf("got %q", origin)
	}
}

func TestViewRegistry_LoopbackOriginFor_UnknownAndEmpty_Bad(t *testing.T) {
	r := newPluginViewRegistry()
	if _, ok := r.LoopbackOriginFor(""); ok {
		t.Error("expected empty code to report not-found")
	}
	if _, ok := r.LoopbackOriginFor("ghost"); ok {
		t.Error("expected an unregistered code to report not-found")
	}
}

// --- manifest.go: ParseManifest (file-backed wrapper around parseManifestBytes) ---

func TestParseManifest_ValidFile_Good(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yml")
	yaml := "schema: lthn-vm/v1\nname: test-bundle\ndisplay: Test\nimages:\n  - id: app\n    image: alpine:3.21\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}
	r := ParseManifest(path)
	if !r.OK {
		t.Fatalf("ParseManifest: %s", r.Error())
	}
	m := r.Value.(BundleManifest)
	if m.Name != "test-bundle" {
		t.Errorf("got name %q", m.Name)
	}
}

func TestParseManifest_MissingFile_Bad(t *testing.T) {
	r := ParseManifest("/nonexistent/path/manifest.yml")
	if r.OK {
		t.Fatal("expected ParseManifest to fail for a missing file")
	}
	if !core.Contains(r.Error(), "read failed") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// --- audit_emit.go / install.go emit helpers unreachable without a
// real container runtime: direct-called for their (branch-free)
// Record-construction bodies. ---

func TestEmitHelpers_DoNotPanic_Ugly(t *testing.T) {
	assertNoPanic := func(name string, fn func()) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s panicked: %v", name, r)
			}
		}()
		fn()
	}
	assertNoPanic("emitInstallSucceeded", func() { emitInstallSucceeded("bundle-x", "code-x", 2) })
	assertNoPanic("emitManifestVersionBumpTransition", func() {
		emitManifestVersionBumpTransition("bundle-x", "sha256:old", "sha256:new", "confirmed")
	})
	assertNoPanic("emitImageDigestMismatch", func() {
		emitImageDigestMismatch("bundle-x", "alpine@sha256:aaa", "sha256:aaa", "sha256:bbb")
	})
	assertNoPanic("emitImageDigestUnverifiable", func() {
		emitImageDigestUnverifiable("bundle-x", "alpine@sha256:aaa", "docker unavailable")
	})
	assertNoPanic("emitPermissionDiffRequiresReConsent", func() {
		emitPermissionDiffRequiresReConsent("bundle-x", []string{"files:read"})
	})
}
