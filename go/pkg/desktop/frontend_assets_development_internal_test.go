//go:build !production

// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	core "dappco.re/go"
)

func TestFrontendAssets_Good_DevelopmentServerHandlesAssetsAndRoutes(t *core.T) {
	var observedPaths []string
	developmentServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observedPaths = append(observedPaths, request.URL.Path)
		switch request.URL.Path {
		case "/media/probe.woff2":
			writer.Header().Set("Content-Type", "font/woff2")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("development-font"))
		case "/system/telemetry":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("development-route"))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(developmentServer.Close)
	t.Setenv("FRONTEND_DEVSERVER_URL", developmentServer.URL)

	desktop, backend := newFrontendAssetTestService(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("embedded-index")},
	})
	core.AssertTrue(t, desktop.attachSPA().OK)

	fontResponse := newFrontendAssetResponseRecorder()
	backend.Handler().ServeHTTP(
		fontResponse,
		httptest.NewRequest(http.MethodGet, "/media/probe.woff2", nil),
	)
	core.AssertEqual(t, http.StatusOK, fontResponse.Code)
	core.AssertEqual(t, "development-font", fontResponse.Body.String())
	core.AssertEqual(t, "font/woff2", fontResponse.Header().Get("Content-Type"))

	routeResponse := newFrontendAssetResponseRecorder()
	backend.Handler().ServeHTTP(
		routeResponse,
		httptest.NewRequest(http.MethodGet, "/system/telemetry", nil),
	)
	core.AssertEqual(t, http.StatusOK, routeResponse.Code)
	core.AssertEqual(t, "development-route", routeResponse.Body.String())

	core.AssertEqual(t, 2, len(observedPaths))
	if len(observedPaths) != 2 {
		return
	}
	core.AssertEqual(t, "/media/probe.woff2", observedPaths[0])
	core.AssertEqual(t, "/system/telemetry", observedPaths[1])
}

func TestFrontendAssetHandler_Bad_InvalidDevelopmentURLFailsExplicitly(t *core.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "://invalid")

	handler := frontendAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("embedded-index")},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/index.html", nil),
	)

	core.AssertEqual(t, http.StatusInternalServerError, response.Code)
	core.AssertFalse(t, core.Contains(response.Body.String(), "embedded-index"))
}
