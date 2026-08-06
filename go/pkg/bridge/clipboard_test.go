// SPDX-Licence-Identifier: EUPL-1.2

// clipboard.go tests. Rather than faking the QUERY/Action wiring by
// hand, these register the REAL core/gui clipboard.Service against a
// small in-memory stub Platform (the same seam core/gui itself uses
// to keep clipboard tests off the real OS clipboard) — so the bridge
// tools are exercised against genuine service behaviour end to end.

package bridge

import (
	core "dappco.re/go"
	guiclipboard "dappco.re/go/render/display/webkit/pkg/clipboard"
)

// stubClipboardPlatform is a minimal in-memory guiclipboard.Platform.
type stubClipboardPlatform struct {
	text    string
	hasText bool
	fail    bool // when true, SetText reports failure
}

func (p *stubClipboardPlatform) Text() (string, bool) { return p.text, p.hasText }
func (p *stubClipboardPlatform) SetText(text string) bool {
	if p.fail {
		return false
	}
	p.text = text
	p.hasText = text != ""
	return true
}

func clipboardHarness(t *core.T, platform *stubClipboardPlatform) *Service {
	t.Helper()
	c := core.New()
	r := guiclipboard.Register(platform)(c)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*guiclipboard.Service)
	core.AssertTrue(t, svc.OnStartup(core.Background()).OK)
	return &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
}

// ─── toolClipboardRead ──────────────────────────────────────────────

func TestClipboard_ToolClipboardRead_Good_HasContent(t *core.T) {
	s := clipboardHarness(t, &stubClipboardPlatform{text: "hello", hasText: true})
	resp := s.toolClipboardRead()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "hello", resp["value"])
	core.AssertEqual(t, true, resp["has_content"])
}

func TestClipboard_ToolClipboardRead_Bad_Empty(t *core.T) {
	s := clipboardHarness(t, &stubClipboardPlatform{})
	resp := s.toolClipboardRead()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "", resp["value"])
	core.AssertEqual(t, false, resp["has_content"])
}

func TestClipboard_ToolClipboardRead_Ugly_NoQueryHandlerRegistered(t *core.T) {
	// Bare core: clipboard.QueryText has no handler at all.
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolClipboardRead()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolClipboardWrite ─────────────────────────────────────────────

func TestClipboard_ToolClipboardWrite_Good(t *core.T) {
	platform := &stubClipboardPlatform{}
	s := clipboardHarness(t, platform)
	resp := s.toolClipboardWrite(map[string]any{"text": "payload"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 7, resp["bytes"])
	core.AssertEqual(t, "payload", platform.text)
}

func TestClipboard_ToolClipboardWrite_Ugly_PlatformRejects(t *core.T) {
	s := clipboardHarness(t, &stubClipboardPlatform{fail: true})
	resp := s.toolClipboardWrite(map[string]any{"text": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "clipboard write failed", resp["error"])
}

func TestClipboard_ToolClipboardWrite_Bad_NoActionRegistered(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolClipboardWrite(map[string]any{"text": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolClipboardHas ───────────────────────────────────────────────

func TestClipboard_ToolClipboardHas_Good_True(t *core.T) {
	s := clipboardHarness(t, &stubClipboardPlatform{text: "x", hasText: true})
	resp := s.toolClipboardHas()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, true, resp["has_content"])
}

func TestClipboard_ToolClipboardHas_Bad_QueryUnregistered(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolClipboardHas()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolClipboardClear ─────────────────────────────────────────────

func TestClipboard_ToolClipboardClear_Good(t *core.T) {
	platform := &stubClipboardPlatform{text: "x", hasText: true}
	s := clipboardHarness(t, platform)
	resp := s.toolClipboardClear()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "clipboard", resp["cleared"])
	core.AssertEqual(t, "", platform.text)
}

func TestClipboard_ToolClipboardClear_Ugly_PlatformRejectsClear(t *core.T) {
	s := clipboardHarness(t, &stubClipboardPlatform{text: "keep", hasText: true, fail: true})
	resp := s.toolClipboardClear()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "clipboard clear failed", resp["error"])
}

func TestClipboard_ToolClipboardClear_Bad_NoActionRegistered(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolClipboardClear()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}
