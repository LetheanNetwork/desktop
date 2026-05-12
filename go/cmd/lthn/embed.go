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
// the OS handles dark/light-mode tinting automatically.
//
// File: 256×256 tight-cropped hoplite (copied verbatim from
// core/ide/icons/apptray.png — the proven asset Snider built for
// core-ide's systray, same SetTemplateIcon pipeline).
// tray-black.png is kept as a fallback if the template renders
// invisible on some future macOS version.
//
//go:embed assets/tray.png
var trayIcon []byte

// appIcon is the embedded application icon used by application.Options.Icon
// — Wails renders it in the default About dialog. The macOS Dock /
// Launchpad / Finder bundle icon comes separately from
// build/darwin/icons.icns (regenerated from build/appicon.png by
// `task common:generate:icons`); both files are derived from the
// same gradient hoplite master so they stay visually consistent.
//
//go:embed assets/app-icon.png
var appIcon []byte
