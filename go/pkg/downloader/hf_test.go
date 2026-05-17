// SPDX-Licence-Identifier: EUPL-1.2

// hf_test.go — coverage for the Hugging Face manifest resolver per
// Mantis #1686. Three pins:
//
//   - LFSFile_GetsSHA256_Good — LFS-stored tree entry yields a real
//     sha256 in HFManifest.Digest with Unverifiable=false.
//   - NonLFSFile_DigestUnverifiable_Ugly — non-LFS tree entry returns
//     Digest="" + Unverifiable=true + emits an audit row carrying
//     reason=download.digest_unverifiable.
//   - PlumbsThroughVerify_Good — happy-path round trip from
//     ResolveHFManifest to a constructed DownloadVerified call shape:
//     the manifest's ResolveURL + FileName + Digest are the exact
//     arguments DownloadVerified expects.
//
// Tests live in `package downloader` so they can swap the package-
// private hfTreeFetcher seam without an export shim. Mirror of the
// in-package fixture pattern used by dns_test.go + audit_test.go.

package downloader

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// hfFixtureLFSEntry returns a deterministic hfTreeEntry whose LFS
// sha256 is well-shaped (64 lowercase hex). Used by the Good path test
// + the plumbing round-trip.
func hfFixtureLFSEntry(name string, size int64) hfTreeEntry {
	return hfTreeEntry{
		Type: "file",
		Path: name,
		Size: size,
		LFS: &hfLFSInfo{
			SHA256:      "a3f5b6c7d8e9f0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f6071829",
			PointerSize: 134,
			Size:        size,
		},
	}
}

// hfFixtureNonLFSEntry returns a deterministic hfTreeEntry with no
// LFS sub-object — mirrors how the HF API responds for small files
// like config.json / tokenizer.json / .gitattributes.
func hfFixtureNonLFSEntry(name string, size int64) hfTreeEntry {
	return hfTreeEntry{
		Type: "file",
		Path: name,
		Size: size,
		LFS:  nil,
	}
}

// withFixtureTree installs entries as the hfTreeFetcher response and
// returns a cleanup the test must defer. The fixture asserts the
// expected API URL shape so a future drift in hfTreeAPIBase or the
// repoID-injection format trips the test.
func withFixtureTree(t *core.T, expectAPIURL string, entries []hfTreeEntry) func() {
	t.Helper()
	prev := hfTreeFetcher
	hfTreeFetcher = func(apiURL string) ([]hfTreeEntry, error) {
		core.AssertEqual(t, expectAPIURL, apiURL)
		return entries, nil
	}
	return func() { hfTreeFetcher = prev }
}

// --- ResolveHFManifest — Good: LFS-stored file populates Digest ---

func TestDownloader_HFManifest_LFSFile_GetsSHA256_Good(t *core.T) {
	repoID := "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF"
	fileName := "qwen2.5-coder-7b-instruct-q4_k_m.gguf"
	apiURL := "https://huggingface.co/api/models/" + repoID + "/tree/main?expand=true"

	cleanup := withFixtureTree(t, apiURL, []hfTreeEntry{
		hfFixtureLFSEntry(fileName, 4_800_000_000),
		hfFixtureNonLFSEntry("README.md", 4096),
	})
	defer cleanup()

	r := ResolveHFManifest(repoID, fileName)
	core.AssertTrue(t, r.OK, "LFS-tree resolve MUST succeed")

	m, ok := r.Value.(HFManifest)
	core.AssertTrue(t, ok, "Result.Value MUST be HFManifest")
	core.AssertEqual(t, repoID, m.RepoID)
	core.AssertEqual(t, fileName, m.FileName)
	core.AssertEqual(t,
		"https://huggingface.co/"+repoID+"/resolve/main/"+fileName,
		m.ResolveURL)
	core.AssertEqual(t,
		"a3f5b6c7d8e9f0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f6071829",
		m.Digest)
	core.AssertEqual(t, int64(4_800_000_000), m.Size)
	core.AssertFalse(t, m.Unverifiable,
		"LFS file MUST NOT flag Unverifiable=true (digest present)")
}

// --- ResolveHFManifest — Ugly: non-LFS file → Unverifiable + audit ---

func TestDownloader_HFManifest_NonLFSFile_DigestUnverifiable_Ugly(t *core.T) {
	repoID := "MistralAI/Mistral-Nemo-Instruct-2407-GGUF"
	fileName := "tokenizer.json"
	apiURL := "https://huggingface.co/api/models/" + repoID + "/tree/main?expand=true"

	cleanup := withFixtureTree(t, apiURL, []hfTreeEntry{
		hfFixtureLFSEntry("mistral-nemo-instruct-2407-q4_k_m.gguf", 8_400_000_000),
		hfFixtureNonLFSEntry(fileName, 4_200_000),
	})
	defer cleanup()

	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	r := ResolveHFManifest(repoID, fileName)
	core.AssertTrue(t, r.OK,
		"non-LFS resolve MUST succeed in soft-mode (returns OK + Unverifiable=true)")

	m, ok := r.Value.(HFManifest)
	core.AssertTrue(t, ok, "Result.Value MUST be HFManifest")
	core.AssertEqual(t, repoID, m.RepoID)
	core.AssertEqual(t, fileName, m.FileName)
	core.AssertEqual(t, "", m.Digest,
		"non-LFS file MUST surface empty Digest")
	core.AssertTrue(t, m.Unverifiable,
		"non-LFS file MUST flag Unverifiable=true so the caller can route to soft-mode")

	// Audit row landed with the reserved reason literal so the
	// forensic walker can chip-filter unverifiable resolves.
	events := rec.snapshot()
	var seenUnverifiable bool
	for _, ev := range events {
		if ev.Event != audit.EventDownloaderFetchRequested {
			continue
		}
		raw, has := ev.Meta["reason"]
		if !has {
			continue
		}
		reason, isStr := raw.(string)
		if !isStr {
			continue
		}
		if reason == reasonDigestUnverifiable {
			seenUnverifiable = true
			// hf_repo_id + hf_file_name MUST also be present so a
			// forensic walker can identify which file triggered.
			gotRepo, hasRepo := ev.Meta["hf_repo_id"]
			core.AssertTrue(t, hasRepo, "audit Meta MUST carry hf_repo_id")
			core.AssertEqual(t, repoID, gotRepo)
			gotFile, hasFile := ev.Meta["hf_file_name"]
			core.AssertTrue(t, hasFile, "audit Meta MUST carry hf_file_name")
			core.AssertEqual(t, fileName, gotFile)
			break
		}
	}
	core.AssertTrue(t, seenUnverifiable,
		"non-LFS resolve MUST emit downloader.fetch.requested with reason="+reasonDigestUnverifiable)
}

// --- ResolveHFManifest — Good: plumb through to DownloadVerified shape ---

// TestDownloader_HFResolve_PlumbsThroughVerify_Good pins the end-to-
// end shape contract — a successful ResolveHFManifest returns the
// three string fields DownloadVerified expects (url, name, digest)
// in a directly-usable form. This test does NOT invoke the network
// (DownloadVerified would need a real HF server); it asserts the
// manifest fields satisfy the DownloadVerified pre-conditions:
//
//  1. ResolveURL is a non-empty, well-formed HTTPS URL.
//  2. FileName is a non-empty leaf basename (no path separators).
//  3. Digest is 64 lowercase hex chars (SHA-256 hex shape).
//
// A regression here means the resolver yields a manifest that can't
// be handed straight to the Wails Download surface — the seam between
// model-browser-window.ts and pkg/downloader.WailsService is broken.
func TestDownloader_HFResolve_PlumbsThroughVerify_Good(t *core.T) {
	repoID := "bartowski/gemma-3-27b-it-GGUF"
	fileName := "gemma-3-27b-it-Q4_K_M.gguf"
	apiURL := "https://huggingface.co/api/models/" + repoID + "/tree/main?expand=true"

	cleanup := withFixtureTree(t, apiURL, []hfTreeEntry{
		hfFixtureLFSEntry(fileName, 16_000_000_000),
	})
	defer cleanup()

	r := ResolveHFManifest(repoID, fileName)
	core.AssertTrue(t, r.OK, "resolve MUST succeed")
	m := r.Value.(HFManifest)

	// Pre-conditions for DownloadVerified(url, name, digest):
	core.AssertTrue(t, core.HasPrefix(m.ResolveURL, "https://huggingface.co/"),
		"ResolveURL MUST start with the HF resolve root so trust.go's AllowedSource passes")
	core.AssertTrue(t, core.Contains(m.ResolveURL, "/resolve/main/"),
		"ResolveURL MUST use the canonical /resolve/main/ form")
	core.AssertTrue(t, core.Contains(m.ResolveURL, fileName),
		"ResolveURL MUST embed the file basename")

	core.AssertFalse(t, core.Contains(m.FileName, "/"),
		"FileName MUST be a leaf basename — no path separators")
	core.AssertFalse(t, core.Contains(m.FileName, "\\"),
		"FileName MUST be a leaf basename — no backslash separators")
	core.AssertTrue(t, len(m.FileName) > 0, "FileName MUST be non-empty")

	core.AssertEqual(t, 64, len(m.Digest),
		"Digest MUST be 64 hex chars (SHA-256)")
	for i := 0; i < len(m.Digest); i++ {
		c := m.Digest[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		core.AssertTrue(t, isHex,
			"Digest MUST be lowercase hex — invalid char at offset "+core.Sprintf("%d", i))
	}

	core.AssertFalse(t, m.Unverifiable,
		"happy-path Plumb test runs against an LFS file → MUST NOT be unverifiable")
}

// --- ResolveHFManifest — Bad: missing inputs ---

func TestDownloader_HFManifest_EmptyRepoID_Bad(t *core.T) {
	r := ResolveHFManifest("", "x.gguf")
	core.AssertFalse(t, r.OK, "empty repoID MUST fail")
	core.AssertTrue(t, core.Contains(r.Error(), "repoID"))
}

func TestDownloader_HFManifest_EmptyFileName_Bad(t *core.T) {
	r := ResolveHFManifest("foo/bar", "")
	core.AssertFalse(t, r.OK, "empty fileName MUST fail")
	core.AssertTrue(t, core.Contains(r.Error(), "fileName"))
}

// --- ResolveHFManifest — Bad: file not in tree ---

func TestDownloader_HFManifest_FileNotInTree_Bad(t *core.T) {
	repoID := "foo/bar"
	apiURL := "https://huggingface.co/api/models/" + repoID + "/tree/main?expand=true"
	cleanup := withFixtureTree(t, apiURL, []hfTreeEntry{
		hfFixtureLFSEntry("model.gguf", 1024),
	})
	defer cleanup()

	r := ResolveHFManifest(repoID, "missing.gguf")
	core.AssertFalse(t, r.OK,
		"file missing from tree MUST return failure")
	core.AssertTrue(t, core.Contains(r.Error(), "downloader.hf.file_not_found"))
}
