// SPDX-Licence-Identifier: EUPL-1.2

// Package desktop exposes build identity shared by the lthn CLI,
// Wails services, HTTP branding, and update integration.
package desktop

// Version is the lthn binary release tag. Production builds stamp
// this with:
//
//	go build -ldflags "-X dappco.re/lthn/desktop.Version=<tag>"
//
// Usage example:
//
//	core.Println("lthn", desktop.Version)
var Version = "dev"
