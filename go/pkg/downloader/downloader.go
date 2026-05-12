// SPDX-Licence-Identifier: EUPL-1.2

// Package downloader fetches model files into ~/Lethean/conf/models/
// via the dappco.re/go HTTP wrappers. HuggingFace and any
// direct-link source work as long as the URL is reachable.
//
// The minimum-viable shape: take a URL + a destination name,
// stream the body into the file. Progress reporting / resume /
// checksum verification are follow-ons; today the call is blocking
// and writes the bytes through core.NewService(io) so the file
// lives under the runner's sandbox root.
//
// TODO sovereign-rootFS: when the integration lands, route writes
// through Snider/Borg/pkg/datanode + Enchantrix so model files live
// as content-addressed lthnHash blobs under
// ~/Lethean/drive/{lthnHash(folder)}/{lthnHash(file)} per the
// triadic Borg+Enchantrix+Poindexter shape. Today: plain HTTP→file.
//
// Usage example:
//
//	r := downloader.Fetch(
//	    "https://huggingface.co/.../resolve/main/model.gguf",
//	    "gemma-4-e2b.gguf",
//	)
//	if !r.OK { return r }
//	dest := r.Value.(string)
package downloader

import (
	"io"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Fetch downloads url and writes it to ~/Lethean/conf/models/<name>.
// Overwrites any existing file at the destination. Returns the
// absolute destination path on success.
//
// Usage example:
//
//	r := downloader.Fetch("https://example.com/model.gguf", "model.gguf")
//	if r.OK { dest := r.Value.(string); _ = dest }
func Fetch(url, name string) core.Result {
	if url == "" {
		return core.Fail(core.E("downloader.Fetch", "url is required", nil))
	}
	if name == "" {
		return core.Fail(core.E("downloader.Fetch", "name is required", nil))
	}
	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dest := core.PathJoin(dirR.Value.(string), name)

	getR := core.HTTPGet(url)
	if !getR.OK {
		return getR
	}
	resp := getR.Value.(*core.Response)
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return core.Fail(core.E("downloader.Fetch",
			core.Concat("HTTP ", core.Sprintf("%d", resp.StatusCode), " from ", url),
			nil))
	}

	createR := core.Create(dest)
	if !createR.OK {
		return createR
	}
	file := createR.Value.(*core.OSFile)
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return core.Fail(core.E("downloader.Fetch", "stream copy failed", err))
	}
	return core.Ok(dest)
}
