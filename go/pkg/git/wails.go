// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for pkg/git — typed methods the WebView can
// bind directly. Lifts the porcelain-parsing logic from core/ide's
// gitbridge.go + mcp_bridge.go toolGit* into a single layer keyed
// on (repo path, args).

package git

import (
	"context"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ServiceName / Startup / Shutdown — Wails3 lifecycle.
func (s *Service) ServiceName() string { return "Git" }
func (s *Service) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *Service) ServiceShutdown() error { return nil }

// BranchInfo describes the current branch + its upstream-relative
// state. Ahead/Behind are 0 when no upstream is configured.
type BranchInfo struct {
	Branch string `json:"branch"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

// Branch returns the current branch + ahead/behind counts.
//
// Usage example (TS):
//
//	import { Branch } from "@desktop/git/service";
//	const { branch, ahead, behind } = await Branch("/path/to/repo");
func (s *Service) Branch(repo string) (BranchInfo, error) {
	out, err := s.runGit(repo, "branch", "--show-current")
	if err != nil {
		return BranchInfo{}, err
	}
	info := BranchInfo{Branch: core.Trim(out)}
	if info.Branch == "" {
		return info, nil
	}
	counts, err := s.runGit(repo, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err == nil {
		// `--count` emits "<behind>\t<ahead>" on a single line.
		parts := core.Split(core.Trim(counts), "\t")
		if len(parts) == 2 {
			if r := core.Atoi(parts[0]); r.OK {
				info.Behind = r.Value.(int)
			}
			if r := core.Atoi(parts[1]); r.OK {
				info.Ahead = r.Value.(int)
			}
		}
	}
	return info, nil
}

// StatusEntry is one row of `git status --porcelain=v1 -z`. The
// two-char status code is split (Index / Worktree) and surfaced as
// staged/unstaged/untracked booleans the WebView renders directly.
type StatusEntry struct {
	Path           string `json:"path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Staged         bool   `json:"staged"`
	Unstaged       bool   `json:"unstaged"`
	Untracked      bool   `json:"untracked"`
}

// Status returns the parsed working-tree state. Empty repo or
// nothing-to-commit returns an empty slice (not nil).
//
// Usage example (TS):
//
//	import { Status } from "@desktop/git/service";
//	const entries = await Status("/path/to/repo");
func (s *Service) Status(repo string) ([]StatusEntry, error) {
	out, err := s.runGit(repo, "status", "--porcelain=v1", "-z")
	if err != nil && out == "" {
		return nil, err
	}
	entries := []StatusEntry{}
	tokens := core.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) < 3 {
			continue
		}
		idx := string(t[0])
		wt := string(t[1])
		path := t[3:]
		// Renames are formatted "R  new\x00old" — consume the next token.
		if idx == "R" || idx == "C" {
			if i+1 < len(tokens) {
				i++
			}
		}
		entries = append(entries, StatusEntry{
			Path:           path,
			IndexStatus:    idx,
			WorktreeStatus: wt,
			Staged:         idx != " " && idx != "?",
			Unstaged:       wt != " " && wt != "?",
			Untracked:      idx == "?" && wt == "?",
		})
	}
	return entries, nil
}

// Diff returns the unified diff for `file` against the index
// (default) or HEAD (when staged=true). file="" returns the full
// repo diff.
//
// Usage example (TS):
//
//	const diff = await Diff("/path", "main.go", false);
func (s *Service) Diff(repo, file string, staged bool) (string, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--staged")
	}
	if file != "" {
		args = append(args, "--", file)
	}
	out, err := s.runGit(repo, args...)
	if err != nil && out == "" {
		return "", err
	}
	return out, nil
}

// Add stages files. When `all` is true or `files` is empty, runs
// `git add -A` (stage everything including untracked).
//
// Usage example (TS):
//
//	await Add("/path", ["main.go"], false);  // stage one file
//	await Add("/path", [],          true);   // stage all
func (s *Service) Add(repo string, files []string, all bool) error {
	args := []string{"add"}
	if all || len(files) == 0 {
		args = append(args, "-A")
	} else {
		args = append(args, "--")
		args = append(args, files...)
	}
	_, err := s.runGit(repo, args...)
	return err
}

// Unstage removes paths from the index (keeps worktree changes).
// Empty `files` unstages everything via `git restore --staged .`.
//
// Usage example (TS):
//
//	await Unstage("/path", ["accidentally-staged.go"]);
func (s *Service) Unstage(repo string, files []string) error {
	args := []string{"restore", "--staged"}
	if len(files) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, "--")
		args = append(args, files...)
	}
	_, err := s.runGit(repo, args...)
	return err
}

// CommitResult carries the SHA of the new commit so the WebView
// can surface it (paste into a Mantis note, jump to log, etc.).
type CommitResult struct {
	SHA string `json:"sha"`
}

// Commit creates a new commit with the staged changes.
//
// Usage example (TS):
//
//	const { sha } = await Commit("/path", "feat: …");
func (s *Service) Commit(repo, message string) (CommitResult, error) {
	if core.Trim(message) == "" {
		return CommitResult{}, core.E("git.Commit", "message required", nil)
	}
	_, err := s.runGit(repo, "commit", "-m", message)
	if err != nil {
		return CommitResult{}, err
	}
	out, err := s.runGit(repo, "rev-parse", "HEAD")
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{SHA: core.Trim(out)}, nil
}

// LogEntry is one parsed line of `git log --pretty=format:...`.
type LogEntry struct {
	Hash     string `json:"hash"`
	ShortSHA string `json:"short_sha"`
	Author   string `json:"author"`
	Date     string `json:"date"`
	Subject  string `json:"subject"`
}

// Log returns the most recent N commits (default 20).
//
// Usage example (TS):
//
//	const commits = await Log("/path", 50);
func (s *Service) Log(repo string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	out, err := s.runGit(repo, "log",
		core.Sprintf("--max-count=%d", limit),
		"--pretty=format:%H%x09%h%x09%an%x09%ad%x09%s",
		"--date=short",
	)
	if err != nil {
		return nil, err
	}
	commits := []LogEntry{}
	for _, line := range core.Split(out, "\n") {
		fields := core.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		commits = append(commits, LogEntry{
			Hash:     fields[0],
			ShortSHA: fields[1],
			Author:   fields[2],
			Date:     fields[3],
			Subject:  fields[4],
		})
	}
	return commits, nil
}
