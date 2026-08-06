// SPDX-Licence-Identifier: EUPL-1.2

// misc.go tests — lang_detect / lang_list are pure lookups; theme_get
// and focus_set delegate onward (eval / toolWindowFocus) and are
// exercised via the shared eval harness in webview_extra_test.go.

package bridge

import (
	core "dappco.re/go"
)

func TestMisc_ToolLangDetect_Good_KnownExtension(t *core.T) {
	s := &Service{}
	resp := s.toolLangDetect(map[string]any{"path": "main.go"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "go", resp["language"])
	core.AssertEqual(t, "go", resp["extension"])
}

func TestMisc_ToolLangDetect_Good_FilenameFallback(t *core.T) {
	s := &Service{}
	resp := s.toolLangDetect(map[string]any{"path": "/srv/app/Dockerfile"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "dockerfile", resp["language"])
}

func TestMisc_ToolLangDetect_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolLangDetect(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestMisc_ToolLangDetect_Ugly_UnknownExtensionFallsBackToPlaintext(t *core.T) {
	s := &Service{}
	resp := s.toolLangDetect(map[string]any{"path": "file.xyzzy"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "plaintext", resp["language"])
}

func TestMisc_ToolLangList_Good(t *core.T) {
	s := &Service{}
	resp := s.toolLangList()
	core.AssertEqual(t, true, resp["ok"])
	langs, ok := resp["value"].([]string)
	core.AssertTrue(t, ok)
	core.AssertGreater(t, len(langs), 0)
	// De-duped: langByExt maps many extensions ("js","jsx",...) onto the
	// same "javascript" name — the output list must not repeat it.
	seen := map[string]int{}
	for _, l := range langs {
		seen[l]++
	}
	core.AssertEqual(t, 1, seen["javascript"], "lang_list must de-dupe repeated language names")
}

func TestMisc_ToolFocusSet_Bad_NoWindowService(t *core.T) {
	// toolFocusSet is a pure alias for toolWindowFocus — with a bare
	// Core (no window service registered) the underlying task action
	// is unregistered, so this drives the error branch end to end.
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolFocusSet(map[string]any{"name": "tray"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}
