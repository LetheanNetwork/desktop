// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/server"
)

type frontendAssetResponseRecorder struct {
	*httptest.ResponseRecorder
	closeNotify chan bool
}

func newFrontendAssetResponseRecorder() *frontendAssetResponseRecorder {
	return &frontendAssetResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      make(chan bool),
	}
}

func (r *frontendAssetResponseRecorder) CloseNotify() <-chan bool {
	return r.closeNotify
}

func newFrontendAssetTestService(frontend core.FS) (*Service, *server.Service) {
	backend := server.NewService(server.Options{})
	desktop := NewService(Options{
		Frontend: frontend,
		Server:   backend,
	})
	return desktop, backend
}

func TestFrontendAssets_Good_RegisteredBackendRouteStaysLocal(t *core.T) {
	developmentRequests := 0
	developmentServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		developmentRequests++
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("wrong-handler"))
	}))
	t.Cleanup(developmentServer.Close)
	t.Setenv("FRONTEND_DEVSERVER_URL", developmentServer.URL)

	desktop, backend := newFrontendAssetTestService(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("embedded-index")},
	})
	core.AssertTrue(t, desktop.attachSPA().OK)

	response := httptest.NewRecorder()
	backend.Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/health", nil),
	)
	core.AssertEqual(t, core.StatusOK, response.Code)
	core.AssertTrue(t, core.Contains(response.Body.String(), `"data":"healthy"`))
	core.AssertFalse(t, core.Contains(response.Body.String(), "wrong-handler"))
	core.AssertEqual(t, 0, developmentRequests)
}

func TestFrontendAssets_Ugly_EmbeddedFilesystemUsedWithoutDevelopmentURL(t *core.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")

	desktop, backend := newFrontendAssetTestService(fstest.MapFS{
		"dist/index.html":        &fstest.MapFile{Data: []byte("embedded-index")},
		"dist/media/probe.woff2": &fstest.MapFile{Data: []byte("embedded-font")},
	})
	core.AssertTrue(t, desktop.attachSPA().OK)

	response := httptest.NewRecorder()
	backend.Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/media/probe.woff2", nil),
	)
	core.AssertEqual(t, http.StatusOK, response.Code)
	core.AssertEqual(t, "embedded-font", response.Body.String())
	core.AssertEqual(t, "font/woff2", response.Header().Get("Content-Type"))
}
