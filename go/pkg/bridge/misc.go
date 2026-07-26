// SPDX-Licence-Identifier: EUPL-1.2

// misc.go — small bridge tools that don't warrant their own file:
// lang_detect / lang_list (file-extension → language name), theme
// (read system dark/light), focus_set (window focus alias).

package bridge

import (
	core "dappco.re/go"
)

// langByExt maps a file extension (lowercase, no dot) to a
// canonical language name. Mirrors core/ide's reference shape so
// agents asking the same question against either bridge get the
// same answer.
var langByExt = map[string]string{
	"go":            "go",
	"js":            "javascript",
	"jsx":           "javascript",
	"mjs":           "javascript",
	"cjs":           "javascript",
	"ts":            "typescript",
	"tsx":           "typescript",
	"py":            "python",
	"rb":            "ruby",
	"rs":            "rust",
	"java":          "java",
	"kt":            "kotlin",
	"swift":         "swift",
	"c":             "c",
	"h":             "c",
	"cpp":           "cpp",
	"cc":            "cpp",
	"cxx":           "cpp",
	"hpp":           "cpp",
	"hxx":           "cpp",
	"cs":            "csharp",
	"php":           "php",
	"sh":            "bash",
	"bash":          "bash",
	"zsh":           "bash",
	"fish":          "bash",
	"json":          "json",
	"jsonc":         "jsonc",
	"yaml":          "yaml",
	"yml":           "yaml",
	"toml":          "toml",
	"ini":           "ini",
	"xml":           "xml",
	"html":          "html",
	"htm":           "html",
	"css":           "css",
	"scss":          "scss",
	"sass":          "sass",
	"less":          "less",
	"md":            "markdown",
	"mdx":           "markdown",
	"sql":           "sql",
	"vue":           "vue",
	"svelte":        "svelte",
	"lua":           "lua",
	"r":             "r",
	"jl":            "julia",
	"dart":          "dart",
	"elm":           "elm",
	"ex":            "elixir",
	"exs":           "elixir",
	"erl":           "erlang",
	"hrl":           "erlang",
	"hs":            "haskell",
	"clj":           "clojure",
	"cljs":          "clojure",
	"scala":         "scala",
	"sc":            "scala",
	"groovy":        "groovy",
	"pl":            "perl",
	"pm":            "perl",
	"ps1":           "powershell",
	"zig":           "zig",
	"nim":           "nim",
	"d":             "d",
	"v":             "vlang",
	"dockerfile":    "dockerfile",
	"containerfile": "dockerfile",
}

// toolLangDetect maps a path's file extension to a language name.
// params: { path }
func (s *Service) toolLangDetect(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": pathParamRequired}
	}
	ext := core.Lower(core.TrimPrefix(core.PathExt(path), "."))
	if lang, ok := langByExt[ext]; ok {
		return map[string]any{"ok": true, "language": lang, "extension": ext}
	}
	// Filename-as-key fallback (Dockerfile, Containerfile, etc.).
	base := core.Lower(core.PathBase(path))
	if lang, ok := langByExt[base]; ok {
		return map[string]any{"ok": true, "language": lang, "extension": base}
	}
	return map[string]any{"ok": true, "language": "plaintext", "extension": ext}
}

// toolLangList returns the catalogue of supported language names.
func (s *Service) toolLangList() map[string]any {
	seen := map[string]bool{}
	out := make([]string, 0, len(langByExt))
	for _, lang := range langByExt {
		if !seen[lang] {
			seen[lang] = true
			out = append(out, lang)
		}
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolThemeGet returns the user's system theme (dark/light) by
// asking the WebView via prefers-color-scheme. params: { window? }
//
// Wails doesn't expose a single cross-platform theme accessor today;
// the matchMedia('(prefers-color-scheme: dark)') check is what every
// Angular window already uses for theming, so this stays consistent.
func (s *Service) toolThemeGet(ctx core.Context, params map[string]any) map[string]any {
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		`return {theme: window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'};`)
}

// toolFocusSet brings a window to the front. Alias for window_focus
// kept for parity with the core/ide tool name.
func (s *Service) toolFocusSet(params map[string]any) map[string]any {
	return s.toolWindowFocus(params)
}
