// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	"strings"
	"testing"
)

func TestUpdates_ArrowLines(t *testing.T) {
	out := "\n" + // blank
		"go: downloading something\n" + // tool chatter, no arrow
		"dappco.re/go: v0.10.0 → v0.10.3\n" +
		"   golang.org/x/sys: v0.1.0 → v0.2.0   \n" + // indented + trailing space
		"\n"
	got := arrowLines(out)
	if len(got) != 2 {
		t.Fatalf("arrowLines = %v (want 2 update lines)", got)
	}
	if got[0] != "dappco.re/go: v0.10.0 → v0.10.3" || got[1] != "golang.org/x/sys: v0.1.0 → v0.2.0" {
		t.Errorf("arrowLines content = %q", got)
	}
}

func TestUpdates_ArrowLines_NoneOutdated(t *testing.T) {
	// `go list -f` emits a blank line per up-to-date module — all dropped.
	if got := arrowLines("\n\n\n"); len(got) != 0 {
		t.Errorf("arrowLines(blanks) = %v, want empty", got)
	}
}

func TestUpdates_Description(t *testing.T) {
	d := updateDescription("Go", []string{"foo: v1 → v2", "bar: v3 → v4"})
	for _, want := range []string{"2 outdated Go", "Package Update template", "- foo: v1 → v2", "- bar: v3 → v4"} {
		if !strings.Contains(d, want) {
			t.Errorf("updateDescription missing %q in:\n%s", want, d)
		}
	}
}
