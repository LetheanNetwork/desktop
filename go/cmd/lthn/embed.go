// SPDX-Licence-Identifier: EUPL-1.2

package main

import "embed"

// frontendDist is the embedded Vite-built frontend. The Taskfile
// `frontend` task copies frontend/dist → go/cmd/lthn/dist before
// the Go build so go:embed has something to bundle.
//
// Path is relative to this source file (cmd/lthn/). The dist
// directory is .gitignored — it's rebuilt on every Taskfile
// `build` invocation.
//
//go:embed all:dist
var frontendDist embed.FS

// trayIcon is the embedded hoplite-helmet systray icon. On macOS
// it's used as a template icon — only the alpha channel matters,
// the OS handles dark/light-mode tinting automatically. The white
// variant is the canonical default; tray-black.png is available
// as a fallback if the white template ever renders invisible on
// a future macOS version.
//
//go:embed assets/tray.png
var trayIcon []byte
