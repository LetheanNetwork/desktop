//go:build !production

// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

// frontendWindowURL loads the Angular development server directly so WebKit
// treats its WebSocket connection as an ordinary loopback HTTP origin. Wails'
// secure custom scheme remains the production path.
func frontendWindowURL(route string) string {
	rawServer := core.Trim(core.Getenv("FRONTEND_DEVSERVER_URL"))
	if rawServer == "" {
		return route
	}
	serverResult := core.URLParse(rawServer)
	if !serverResult.OK {
		return route
	}
	server, ok := serverResult.Value.(*core.URL)
	if !ok || server.Host == "" || (server.Scheme != "http" && server.Scheme != "https") {
		return route
	}

	routeResult := core.URLParse(route)
	if !routeResult.OK {
		return route
	}
	routeURL, ok := routeResult.Value.(*core.URL)
	if !ok {
		return route
	}
	server.Path = routeURL.Path
	server.Fragment = routeURL.Fragment

	rawSocket := core.Trim(core.Getenv("LTHN_WAILS_WS_URL"))
	if rawSocket != "" {
		socketResult := core.URLParse(rawSocket)
		if !socketResult.OK {
			return route
		}
		socketURL, socketOK := socketResult.Value.(*core.URL)
		if !socketOK || (socketURL.Scheme != "ws" && socketURL.Scheme != "wss") {
			return route
		}
		query := server.Query()
		query.Set("lthn-ws", socketURL.String())
		server.RawQuery = query.Encode()
	}
	return server.String()
}
