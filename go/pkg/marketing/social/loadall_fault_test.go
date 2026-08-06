// SPDX-Licence-Identifier: EUPL-1.2

// loadall_fault_test.go — fault injection for loadPosts' per-entry skip
// branches (RFC.stage-e-encrypt-at-rest v2 §4.1: "do NOT abort whole
// List on one bad file") plus the List/Get/MarkSent/Create wails.go
// branches that only trigger with a populated, multi-post queue
// (Channel/State filters, ScheduledCount, not-found forwarding,
// session-locked-after-atrest forwarding). Posts are seeded directly on
// disk with fixed IDs rather than via two back-to-back Create() calls —
// Create's ID is core.Now().UTC().Unix()-derived, so two fast calls in
// the same wall-clock second collide on id and silently overwrite one
// another, masking the very branches this file targets. Mirrors
// pkg/sales/deals/loadall_fault_test.go's precedent.

package social_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/social"
)

// TestSocial_LoadPosts_CorruptLthn_SkippedNotAborted_Ugly — a .lthn
// file too short/malformed to survive loadHeaderOnly is skipped; a
// healthy .md sibling still surfaces via List.
func TestSocial_LoadPosts_CorruptLthn_SkippedNotAborted_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "corrupt-entry.lthn"), []byte("not-a-real-envelope"), 0o600); !w.OK {
		t.Fatalf("seed corrupt .lthn: %s", w.Error())
	}

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Healthy encrypted post"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate one corrupt .lthn entry: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected exactly the healthy record (corrupt one skipped), got %d", len(out.Posts))
	}
}

// TestSocial_LoadPosts_UnreadableMd_SkippedNotAborted_Bad — a .md file
// with permissions denying read hits loadPosts' `if !raw.OK { continue
// }` branch.
func TestSocial_LoadPosts_UnreadableMd_SkippedNotAborted_Bad(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	unreadable := core.PathJoin(dir, "unreadable-post.md")
	legacy := []byte("---\nid: unreadable-post\nch: mastodon\nwhen: \"\"\nstate: draft\nattach: \"\"\n---\n")
	if w := core.WriteFile(unreadable, legacy, 0o600); !w.OK {
		t.Fatalf("seed .md: %s", w.Error())
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Healthy legacy post"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate an unreadable .md entry: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected exactly the healthy record (unreadable one skipped), got %d", len(out.Posts))
	}
}

// TestSocial_LoadPosts_MalformedMdYaml_SkippedNotAborted_Bad — a .md
// file whose frontmatter fails yaml.Unmarshal hits loadPosts'
// parsePost-error `continue` branch.
func TestSocial_LoadPosts_MalformedMdYaml_SkippedNotAborted_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "malformed-post.md")
	if w := core.WriteFile(bad, []byte("---\n[not: valid: yaml\n---\nbody"), 0o600); !w.OK {
		t.Fatalf("seed malformed .md: %s", w.Error())
	}

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Healthy legacy post two"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate a malformed .md entry: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected exactly the healthy record (malformed YAML skipped), got %d", len(out.Posts))
	}
}

// TestSocial_LoadPosts_SubdirEntry_Ignored_Good — a stray subdirectory
// inside the social dir must be ignored by both loadPosts passes.
func TestSocial_LoadPosts_SubdirEntry_Ignored_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(core.PathJoin(dir, "stray-subdir"), 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Post beside a stray subdir"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List must ignore a stray subdirectory: %s", r.Error())
	}
}

// seedLegacyPost writes a legacy plaintext .md post directly to disk
// with a fixed id, sidestepping Create()'s Unix-second id collision
// risk when a test needs two or more distinct posts fast.
func seedLegacyPost(t *testing.T, dir, id, ch, when, state string) {
	t.Helper()
	body := "---\nid: " + id + "\nch: " + ch + "\nwhen: \"" + when + "\"\nstate: " + state + "\nattach: \"\"\n---\n"
	if w := core.WriteFile(core.PathJoin(dir, id+".md"), []byte(body), 0o600); !w.OK {
		t.Fatalf("seedLegacyPost WriteFile: %s", w.Error())
	}
}

// TestList_ChannelFilter_ExcludesNonMatching_Good — with two DISTINCT
// posts on disk, filtering by a channel present on only one exercises
// the `continue` branch (post A skipped) as well as the keep branch
// (post B retained).
func TestList_ChannelFilter_ExcludesNonMatching_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	seedLegacyPost(t, dir, "post-a", "mastodon,x", "today", "draft")
	seedLegacyPost(t, dir, "post-b", "linkedin", "tomorrow", "draft")

	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.List(social.ListInput{Channel: "linkedin"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected 1 linkedin post, got %d", len(out.Posts))
	}
	if out.Posts[0].ID != "post-b" {
		t.Fatalf("expected post-b, got %q", out.Posts[0].ID)
	}
}

// TestList_StateFilter_ExcludesNonMatching_Good — State filter's
// `continue` branch (49-50) is symmetric to Channel's.
func TestList_StateFilter_ExcludesNonMatching_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	seedLegacyPost(t, dir, "post-draft", "mastodon", "today", "draft")
	seedLegacyPost(t, dir, "post-sched", "mastodon", "tomorrow", "scheduled")

	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.List(social.ListInput{State: "scheduled"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 || out.Posts[0].ID != "post-sched" {
		t.Fatalf("expected exactly post-sched, got %+v", out.Posts)
	}
	if out.ScheduledCount != 1 {
		t.Fatalf("expected ScheduledCount=1 (computed pre-filter), got %d", out.ScheduledCount)
	}
}

// TestMarkSent_NotFound_Bad — MarkSent on a nonexistent ID forwards
// loadOne's not-found error.
func TestMarkSent_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.MarkSent("does-not-exist-anywhere")
	if r.OK {
		t.Fatal("MarkSent on a nonexistent id must fail")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %s", r.Error())
	}
}

// TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad — an .lthn
// record exists on disk; the gate is then swapped to a NARROW
// stubSessionGate (satisfies SessionGate but not accountKeyProvider).
// loadOne's atrestWriterFor check fails closed with the typed
// "social.session.locked" code, which Get forwards verbatim.
func TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Narrow-gate probe"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID

	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on an .lthn record with a narrow gate must fail")
	}
	if !core.Contains(g.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked, got %s", g.Error())
	}
}

// TestMarkSent_SessionLockedViaNarrowGateAfterAtRestWrite_Bad —
// MarkSent's own session.locked forwarding branch, symmetric to Get's.
func TestMarkSent_SessionLockedViaNarrowGateAfterAtRestWrite_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Narrow-gate MarkSent probe"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID

	// Swap to a narrow gate that still reports unlocked (assertUnlocked
	// passes) but does not satisfy accountKeyProvider.
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.MarkSent(id)
	if r.OK {
		t.Fatal("MarkSent on an .lthn record with a narrow gate must fail")
	}
	if !core.Contains(r.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked, got %s", r.Error())
	}
}

// TestCreate_SocialDirFails_Bad — Create forwards socialDir's failure
// when $HOME is unavailable.
func TestCreate_SocialDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	r := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, Text: "Homeless post"})
	if r.OK {
		t.Fatal("Create must fail when socialDir() cannot resolve $HOME")
	}
}
