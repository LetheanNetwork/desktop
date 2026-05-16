// SPDX-Licence-Identifier: EUPL-1.2

package social_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/social"
)

// TestList_Empty_Good — empty dir → empty posts, zero scheduled.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(out.Posts))
	}
	if out.ScheduledCount != 0 {
		t.Fatalf("expected ScheduledCount=0, got %d", out.ScheduledCount)
	}
}

// TestCreate_Defaults_Good — Create with ch+text defaults to state=draft.
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	r := svc.Create(social.CreateInput{
		Ch:   []string{"mastodon", "x"},
		When: "today · 16:00",
		Text: "Lethean v0.2 is out.",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	if p.State != "draft" {
		t.Fatalf("expected state=draft, got %q", p.State)
	}
	if p.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
	if len(p.Ch) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(p.Ch))
	}
}

// TestCreate_NoChannels_Bad — empty Ch returns core.Fail.
func TestCreate_NoChannels_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	r := svc.Create(social.CreateInput{Ch: nil, When: "today", Text: "Hello"})
	if r.OK {
		t.Fatalf("expected failure for empty channels, got OK")
	}
}

// TestCreate_NoText_Bad — empty text returns core.Fail.
func TestCreate_NoText_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	r := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, When: "today", Text: ""})
	if r.OK {
		t.Fatalf("expected failure for empty text, got OK")
	}
}

// TestMarkSent_Good — MarkSent transitions state to "sent".
func TestMarkSent_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	cr := svc.Create(social.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "today · 09:00",
		Text:  "Just shipped.",
		State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID

	r := svc.MarkSent(id)
	if !r.OK {
		t.Fatalf("MarkSent failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	if p.State != "sent" {
		t.Fatalf("expected state=sent, got %q", p.State)
	}
}

// TestList_ChannelFilter_Good — List with Channel filter returns only matching posts.
func TestList_ChannelFilter_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	svc.Create(social.CreateInput{Ch: []string{"mastodon", "x"}, When: "today", Text: "A"})
	svc.Create(social.CreateInput{Ch: []string{"linkedin"}, When: "tomorrow", Text: "B"})

	r := svc.List(social.ListInput{Channel: "linkedin"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected 1 linkedin post, got %d", len(out.Posts))
	}
	if out.Posts[0].Text != "B" {
		t.Fatalf("expected text=B, got %q", out.Posts[0].Text)
	}
}

// TestServiceName_Good — ServiceName returns "Social".
func TestServiceName_Good(t *testing.T) {
	svc := social.NewService(nil)
	if svc.ServiceName() != "Social" {
		t.Fatalf("expected Social, got %q", svc.ServiceName())
	}
}
