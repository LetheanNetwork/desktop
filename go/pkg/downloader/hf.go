// SPDX-Licence-Identifier: EUPL-1.2

// hf.go — Hugging Face manifest resolver for the model-browser
// Download path. Hits the public HF tree API and extracts the per-file
// metadata pkg/downloader.FetchVerified needs (sha256 digest, byte
// size). Forward-arc of Mantis #1682 Shape B + #1676 F-2: the renderer-
// reachable Download surface is fail-closed on empty digest, so the
// frontend MUST resolve a digest before invoking it.
//
// Two manifest tiers per the HF storage shape:
//
//  1. LFS-stored files (anything > ~10 MiB or marked in .gitattributes)
//     carry the canonical sha256 in the `lfs.sha256` field of the tree
//     entry. ResolveHFManifest returns it as Digest.
//  2. Small / non-LFS files (config.json, tokenizer.json) DO NOT carry
//     a sha256 in the tree API. ResolveHFManifest returns Digest="" +
//     Unverifiable=true so the caller can choose to proceed in soft-
//     mode (audit warn, no digest enforcement) or block.
//
// Per Mantis #1686 the soft-mode path is the pragmatic ship: GGUF /
// safetensors model weights are always LFS-stored (multi-GB), so the
// common Download case lands a real digest. Tokeniser / config files
// landing without verification is the documented trade-off — the
// audit row carries reason=download.digest_unverifiable so ops can
// audit-trail the path.
//
// Usage example:
//
//	r := downloader.ResolveHFManifest("Qwen/Qwen2.5-Coder-7B-Instruct-GGUF", "qwen2.5-coder-7b-instruct-q4_k_m.gguf")
//	if !r.OK { return r }
//	m := r.Value.(downloader.HFManifest)
//	dl.DownloadVerified(m.ResolveURL, m.FileName, m.Digest)

package downloader

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// hfTreeAPIBase is the canonical Hugging Face tree-listing endpoint.
// Public, no auth required for public repos. ?expand=true is what
// surfaces the lfs.sha256 + size fields on each tree entry; without
// it the response carries only path + type + size and the digest is
// absent regardless of LFS storage state.
const hfTreeAPIBase = "https://huggingface.co/api/models/"

// hfResolveBase is the user-facing GGUF download root. ResolveHFManifest
// composes the per-file URL by joining repo + file. Matches the URL
// format the marketplace + fixture rows already use.
const hfResolveBase = "https://huggingface.co/"

// hfManifestResponseCap bounds the bytes ResolveHFManifest is willing
// to read from the tree API. Even the largest public repos list well
// under 1 MiB of tree JSON; the cap is defensive against a malicious
// or compromised mirror streaming unbounded bytes through the trust
// boundary the public hostname implies.
const hfManifestResponseCap int64 = 4 << 20 // 4 MiB

// reasonDigestUnverifiable is the Meta["reason"] literal stamped on
// the audit row when ResolveHFManifest succeeds at fetching the tree
// but the requested file has no lfs.sha256 (non-LFS / small file).
// Closed-set per the downloader audit-emit contract — the value MUST
// be reserved alongside the existing reasonHostNotAllowed /
// reasonPrivateIPResolved literals so a forensic walker can chip-
// filter by reason without re-running gate logic.
const reasonDigestUnverifiable = "download.digest_unverifiable"

// HFManifest is the resolved per-file metadata
// pkg/downloader.FetchVerified needs to drive a Wails-bound Download.
// Returned by ResolveHFManifest. Field shapes mirror what the HF tree
// API exposes when ?expand=true is passed.
//
// Usage example:
//
//	r := downloader.ResolveHFManifest(repoID, fileName)
//	if !r.OK { return r }
//	m := r.Value.(downloader.HFManifest)
//	if m.Unverifiable {
//	    showSoftModeWarning(m.FileName)
//	}
//	wails.DownloadVerified(m.ResolveURL, m.FileName, m.Digest)
type HFManifest struct {
	// RepoID is the canonical "<org>/<repo>" Hugging Face identifier
	// the caller passed to ResolveHFManifest. Stamped on the manifest
	// so the audit trail in the frontend logs carries the same id
	// the user typed without depending on URL reverse-engineering.
	RepoID string

	// FileName is the leaf basename of the resolved file (no path
	// segments). Passed verbatim to FetchVerified so the on-disk
	// landing under ~/Lethean/conf/models/ keeps the canonical HF
	// filename.
	FileName string

	// ResolveURL is the composed public download URL — the same
	// URL pattern the HF "Download" button on the model card uses
	// (`/{repo}/resolve/main/{file}`). Tracked here so the caller
	// doesn't have to reassemble it from RepoID + FileName.
	ResolveURL string

	// Digest is the SHA-256 hex string (64 lowercase chars) the
	// downloaded bytes MUST match when verified. Empty when the
	// tree entry carried no lfs.sha256 (Unverifiable=true).
	Digest string

	// Size is the byte count the manifest claims the file takes on
	// disk. Used by the frontend's pre-flight (warn user before a
	// multi-GB Download). Zero when the tree entry didn't carry a
	// size field (some early API responses omit it for non-LFS).
	Size int64

	// Unverifiable is true when ResolveHFManifest returned without
	// a digest because the HF API tier didn't expose one for this
	// file (non-LFS, typically <10 MiB). Callers who refuse to
	// proceed without a digest MUST check this flag — Digest=""
	// alone is the same value as "field omitted by the API".
	Unverifiable bool
}

// hfTreeEntry is the JSON shape returned per file by the HF tree API
// when ?expand=true is set. Mirror of the public response — only the
// fields ResolveHFManifest actually consumes are decoded; everything
// else (rfilename / oid / blob_id / committer / etc.) is dropped on
// the floor. Decoder is lenient — missing fields don't error.
type hfTreeEntry struct {
	Type string     `json:"type"` // "file" / "directory"
	Path string     `json:"path"` // path-from-repo-root, includes subdirs
	Size int64      `json:"size"` // bytes; absent → 0
	LFS  *hfLFSInfo `json:"lfs"`  // present iff LFS-stored
}

// hfLFSInfo is the lfs sub-object on a tree entry. Only sha256 is
// load-bearing for us — the rest (pointer_size / size) is decorative.
type hfLFSInfo struct {
	SHA256      string `json:"sha256"`
	PointerSize int64  `json:"pointer_size"`
	Size        int64  `json:"size"`
}

// ResolveHFManifest hits the public Hugging Face tree API for repoID
// and returns the HFManifest entry for fileName. The tree API is
// public (no auth) for public repos; private repos error with HTTP
// 401, surfaced as core.E("downloader.hf.unauthorized", ...).
//
// Behaviours:
//
//   - LFS-stored file → Digest populated from lfs.sha256, Unverifiable=false.
//   - Non-LFS / small file → Digest="", Unverifiable=true, audit emit
//     of EventDownloaderFetchRequested with reason=download.digest_unverifiable
//     so ops can trace the soft-mode landing.
//   - File not in tree → core.E("downloader.hf.file_not_found", ...).
//   - Network / HTTP / JSON-parse failures → wrapped error result.
//
// The HF host (huggingface.co) is allowlisted in trust.go; no new
// trust-gate carve-out is required.
//
// Usage example:
//
//	r := downloader.ResolveHFManifest("Qwen/Qwen2.5-Coder-7B-Instruct-GGUF", "qwen2.5-coder-7b-instruct-q4_k_m.gguf")
//	if !r.OK { return r }
//	m := r.Value.(downloader.HFManifest)
//	if m.Unverifiable {
//	    // Soft-mode: warn user, proceed via Download() (no digest).
//	    return softModeDownload(m)
//	}
//	return wails.DownloadVerified(m.ResolveURL, m.FileName, m.Digest)
func ResolveHFManifest(repoID, fileName string) core.Result {
	if core.Trim(repoID) == "" {
		return core.Fail(core.E("downloader.hf", "repoID is required", nil))
	}
	if core.Trim(fileName) == "" {
		return core.Fail(core.E("downloader.hf", "fileName is required", nil))
	}

	apiURL := hfTreeAPIBase + repoID + "/tree/main?expand=true"
	resolveURL := hfResolveBase + repoID + "/resolve/main/" + fileName

	entries, err := fetchHFTree(apiURL)
	if err != nil {
		return core.Fail(err)
	}

	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		if e.Path != fileName {
			continue
		}
		m := HFManifest{
			RepoID:     repoID,
			FileName:   fileName,
			ResolveURL: resolveURL,
			Size:       e.Size,
		}
		if e.LFS != nil && core.Trim(e.LFS.SHA256) != "" {
			m.Digest = core.Lower(e.LFS.SHA256)
			if m.Size == 0 && e.LFS.Size > 0 {
				m.Size = e.LFS.Size
			}
			return core.Ok(m)
		}
		// LFS metadata absent or sha256 empty — the tree entry is a
		// non-LFS / small file (typically tokenizer.json / config.json
		// / .gitattributes). Soft-mode is the documented #1686 path:
		// caller decides whether to proceed without a digest; we emit
		// the audit row so the soft-mode landing is traceable.
		m.Unverifiable = true
		emitDigestUnverifiable(resolveURL, repoID, fileName)
		return core.Ok(m)
	}

	return core.Fail(core.E("downloader.hf.file_not_found",
		core.Concat("file not in HF tree: ", repoID, "/", fileName), nil))
}

// fetchHFTree performs the bounded GET + JSON decode for the tree API
// response. Pulled out of ResolveHFManifest so the HTTP / JSON
// boundary stays mockable in tests via hfTreeFetcher (see below).
//
// Response cap is hfManifestResponseCap (4 MiB) — a malicious or
// compromised mirror cannot stream unbounded bytes through this
// trust boundary even though hostname allowlisting passed.
func fetchHFTree(apiURL string) ([]hfTreeEntry, error) {
	if fetcher := hfTreeFetcher; fetcher != nil {
		// Test seam — the production fetcher is nil, the live path
		// runs through httpClient. Tests set hfTreeFetcher to a
		// fixture so the call doesn't reach the network.
		return fetcher(apiURL)
	}

	reqR := core.NewHTTPRequest("GET", apiURL, nil)
	if !reqR.OK {
		return nil, core.E("downloader.hf", "constructing tree request failed", nil)
	}
	req := reqR.Value.(*core.Request)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, core.E("downloader.hf", "tree GET failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, core.E("downloader.hf.unauthorized",
			core.Concat("HTTP ", core.Sprintf("%d", resp.StatusCode),
				" — private repo or token required"), nil)
	}
	if resp.StatusCode >= 400 {
		return nil, core.E("downloader.hf",
			core.Concat("HTTP ", core.Sprintf("%d", resp.StatusCode), " from tree API"), nil)
	}

	bounded := core.LimitReader(resp.Body, hfManifestResponseCap)
	bodyR := core.ReadAll(bounded)
	if !bodyR.OK {
		return nil, core.E("downloader.hf", "tree body read failed",
			bodyR.Value.(error))
	}
	body, _ := bodyR.Value.([]byte)

	var entries []hfTreeEntry
	if r := core.JSONUnmarshal(body, &entries); !r.OK {
		return nil, core.E("downloader.hf",
			"tree JSON decode failed — HF API contract drift?", nil)
	}
	return entries, nil
}

// hfTreeFetcher is the test seam ResolveHFManifest uses to bypass the
// httpClient when set. Production paths leave it nil — fetchHFTree
// falls through to the live httpClient.Do path. Tests assign a fixture
// returning canned hfTreeEntry slices.
//
// Usage example (in *_test.go):
//
//	downloader.SetHFTreeFetcherForTest(func(url string) ([]hfTreeEntry, error) {
//	    return []hfTreeEntry{ /* fixture */ }, nil
//	})
//	defer downloader.SetHFTreeFetcherForTest(nil)
var hfTreeFetcher func(apiURL string) ([]hfTreeEntry, error)

// emitDigestUnverifiable fires an audit row that carries reason=
// download.digest_unverifiable in Meta so ops can trace which HF
// resolves landed without a sha256 — the soft-mode trail. Routed
// through the same EventDownloaderFetchRequested family as the
// FetchVerified primary so a single chip-filter (event=
// downloader.fetch.requested) surfaces both verified + unverifiable
// resolves in the same panel.
//
// Folded into the existing audit event-set rather than minting a new
// audit.EventDownloaderDigestUnverifiable constant — pkg/audit is
// outside this ticket's path allowlist, and the Meta["reason"] field
// is the documented degree-of-freedom for sub-classifying within an
// event family per the downloader audit-emit contract.
func emitDigestUnverifiable(resolveURL, repoID, fileName string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDownloaderFetchRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   downloaderScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"source_host":     domainOnly(resolveURL),
			"sha256_expected": "",
			"reason":          reasonDigestUnverifiable,
			"hf_repo_id":      repoID,
			"hf_file_name":    fileName,
		},
	})
}
