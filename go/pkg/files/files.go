// SPDX-Licence-Identifier: EUPL-1.2

// Package files is the desktop's read-only file surface for code navigation —
// the Coding/Agents views open a finding's source at its line. It resolves a
// repo's local path (the canonical workspace roots, matching pkg/repos) so a
// task carrying just a repo name + relative file can be opened, and reads the
// file via core.ReadFile (the same dev-trust, loopback boundary as the bridge's
// file tools — no production sandboxing here). Bound plainly like pkg/repos.
package files

import (
	core "dappco.re/go"
)

// maxReadBytes caps a single viewed file. 512KB is far past any source file a
// finding points at; larger reads are truncated rather than streamed.
const maxReadBytes = 512 * 1024

// Service is the Wails-bound file reader.
type Service struct {
	core *core.Core
}

// NewService returns a files service bound to the core runtime.
//
//	gui.Bind(files.NewService(c))
func NewService(c *core.Core) *Service { return &Service{core: c} }

// ServiceName identifies the Wails binding namespace.
func (s *Service) ServiceName() string { return "Files" }

// ReadInput addresses a file either absolutely (Path) or by repo + relative
// file (Repo resolves to a canonical-root path). Path wins when both are given.
type ReadInput struct {
	Repo string `json:"repo,omitempty"`
	File string `json:"file,omitempty"`
	Path string `json:"path,omitempty"`
}

// ReadOutput is the file content for the code viewer.
type ReadOutput struct {
	Path      string `json:"path"`      // the absolute path actually read
	Content   string `json:"content"`
	Lines     int    `json:"lines"`
	Truncated bool   `json:"truncated"`
}

// Read returns a file's content for the code viewer. Resolves {repo, file} to a
// path under the canonical workspace roots, or reads Path directly.
//
//	r := svc.Read(files.ReadInput{Repo: "BugSETI", File: "internal/x.go"})
//	if r.OK { out := r.Value.(files.ReadOutput); _ = out }
func (s *Service) Read(input ReadInput) core.Result {
	path := resolvePath(input)
	if path == "" {
		return core.Fail(core.E("files.Read", "could not resolve a path from input", nil))
	}
	r := core.ReadFile(path)
	if !r.OK {
		return core.Fail(core.E("files.Read", "read "+path+": "+r.Error(), nil))
	}
	b, _ := r.Value.([]byte)
	truncated := false
	if len(b) > maxReadBytes {
		b = b[:maxReadBytes]
		truncated = true
	}
	content := string(b)
	return core.Ok(ReadOutput{Path: path, Content: content, Lines: lineCount(content), Truncated: truncated})
}

// resolvePath turns a ReadInput into an absolute path. Path (or an absolute
// File) is used as-is; otherwise the repo name is resolved to its workspace
// path and joined with the relative file.
func resolvePath(input ReadInput) string {
	if abs := core.Trim(input.Path); abs != "" {
		return abs
	}
	file := core.Trim(input.File)
	if file == "" {
		return ""
	}
	if core.HasPrefix(file, "/") {
		return file // core-lint sometimes emits absolute finding paths
	}
	repoPath := resolveRepoPath(core.Trim(input.Repo))
	if repoPath == "" {
		return ""
	}
	return core.PathJoin(repoPath, file)
}

// resolveRepoPath finds a repo by its bare name under the canonical workspace
// roots (Code/{core,lthn,host-uk,lab,snider}), mirroring pkg/repos' scan. ""
// when no matching git repo is found.
func resolveRepoPath(name string) string {
	if name == "" {
		return ""
	}
	home := core.UserHomeDir()
	if !home.OK {
		return ""
	}
	homeDir, _ := home.Value.(string)
	for _, root := range []string{"core", "lthn", "host-uk", "lab", "snider"} {
		candidate := core.PathJoin(homeDir, "Code", root, name)
		if core.Stat(core.PathJoin(candidate, ".git")).OK {
			return candidate
		}
	}
	return ""
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(core.Split(s, "\n"))
}
