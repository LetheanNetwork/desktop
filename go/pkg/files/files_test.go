// SPDX-Licence-Identifier: EUPL-1.2

package files

import "testing"

func TestFiles_ResolvePath_AbsolutePathWins(t *testing.T) {
	if got := resolvePath(ReadInput{Path: "/etc/hosts", Repo: "x", File: "y"}); got != "/etc/hosts" {
		t.Errorf("explicit Path should win, got %q", got)
	}
}

func TestFiles_ResolvePath_AbsoluteFile(t *testing.T) {
	if got := resolvePath(ReadInput{File: "/abs/main.go"}); got != "/abs/main.go" {
		t.Errorf("absolute File should pass through, got %q", got)
	}
}

func TestFiles_ResolvePath_RelativeNeedsRepo(t *testing.T) {
	if got := resolvePath(ReadInput{File: "internal/x.go"}); got != "" {
		t.Errorf("relative file with no repo should not resolve, got %q", got)
	}
	if got := resolvePath(ReadInput{}); got != "" {
		t.Errorf("empty input should not resolve, got %q", got)
	}
}

func TestFiles_ResolveRepoPath_Unknown(t *testing.T) {
	if got := resolveRepoPath("definitely-not-a-real-repo-xyzzy"); got != "" {
		t.Errorf("unknown repo should resolve to empty, got %q", got)
	}
	if got := resolveRepoPath(""); got != "" {
		t.Errorf("empty name should resolve to empty, got %q", got)
	}
}

func TestFiles_LineCount(t *testing.T) {
	cases := map[string]int{"": 0, "a": 1, "a\nb\nc": 3, "a\n": 2}
	for in, want := range cases {
		if got := lineCount(in); got != want {
			t.Errorf("lineCount(%q) = %d, want %d", in, got, want)
		}
	}
}
