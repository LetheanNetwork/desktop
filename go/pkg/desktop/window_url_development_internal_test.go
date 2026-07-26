//go:build !production

// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

func TestFrontendWindowURL_Good_LoadsDirectHMRServerWithSocketBoundary(t *core.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "http://localhost:9245")
	t.Setenv("LTHN_WAILS_WS_URL", "ws://localhost:9199/wails/ws")

	core.AssertEqual(
		t,
		"http://localhost:9245/?lthn-ws=ws%3A%2F%2Flocalhost%3A9199%2Fwails%2Fws#/tools/files",
		frontendWindowURL("/#/tools/files"),
	)
}

func TestFrontendWindowURL_Bad_InvalidDevelopmentServerRetainsWailsRoute(t *core.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "://invalid")
	t.Setenv("LTHN_WAILS_WS_URL", "ws://localhost:9199/wails/ws")

	core.AssertEqual(t, "/#/tools/files", frontendWindowURL("/#/tools/files"))
}

func TestFrontendWindowURL_Ugly_EmptyDevelopmentServerRetainsWailsRoute(t *core.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	t.Setenv("LTHN_WAILS_WS_URL", "ws://localhost:9199/wails/ws")

	core.AssertEqual(t, "/#/tools/files", frontendWindowURL("/#/tools/files"))
}
