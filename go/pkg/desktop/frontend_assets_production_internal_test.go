//go:build production

// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	core "dappco.re/go"
)

func TestFrontendAssets_Good_ProductionIgnoresDevelopmentServer(t *core.T) {
	developmentRequests := 0
	developmentServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		developmentRequests++
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("development-font"))
	}))
	t.Cleanup(developmentServer.Close)
	t.Setenv("FRONTEND_DEVSERVER_URL", developmentServer.URL)

	desktop, backend := newFrontendAssetTestService(fstest.MapFS{
		"dist/index.html":        &fstest.MapFile{Data: []byte("embedded-index")},
		"dist/media/probe.woff2": &fstest.MapFile{Data: []byte("embedded-font")},
	})
	core.AssertTrue(t, desktop.attachSPA().OK)

	response := newFrontendAssetResponseRecorder()
	backend.Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/media/probe.woff2", nil),
	)

	core.AssertEqual(t, http.StatusOK, response.Code)
	core.AssertEqual(t, "embedded-font", response.Body.String())
	core.AssertEqual(t, "font/woff2", response.Header().Get("Content-Type"))
	core.AssertEqual(t, 0, developmentRequests)
}
