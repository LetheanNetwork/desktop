// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
)

const deepLinkNavigateEvent = "navigate"

// parseDeepLink converts the external URL into the stable frontend event
// envelope. Core's URLParse is the repository wrapper around net/url.Parse.
func parseDeepLink(rawURL string) core.Result {
	parsed := core.URLParse(rawURL)
	if !parsed.OK {
		return core.Fail(core.E("desktop.parseDeepLink", "parse URL", parsed.Err()))
	}

	targetURL := parsed.Value.(*core.URL)
	if !core.EqualFold(targetURL.Scheme, "lthn") {
		return core.Fail(core.E("desktop.parseDeepLink", "unsupported URL scheme", nil))
	}

	action := core.Lower(core.Trim(targetURL.Host))
	if action == "" {
		return core.Fail(core.E("desktop.parseDeepLink", "URL action is required", nil))
	}

	resource := ""
	pathID := ""
	path := core.TrimCutset(targetURL.Path, "/")
	if path != "" {
		segments := core.Split(path, "/")
		resource = core.Lower(core.Trim(segments[0]))
		if len(segments) > 1 {
			pathID = core.Trim(segments[1])
		}
	}

	id := core.Trim(targetURL.Query().Get("id"))
	if id == "" {
		id = pathID
	}

	return core.Ok(map[string]string{
		"action":   action,
		"resource": resource,
		"id":       id,
	})
}

// handleDeepLink restores the Angular shell and publishes the parsed target.
// gui.OpenWindow is the CoreGUI Show+Focus primitive: it restores, shows, and
// focuses the registered window in that order.
func handleDeepLink(c *core.Core, rawURL string) core.Result {
	target := parseDeepLink(rawURL)
	if !target.OK {
		return target
	}
	if !gui.OpenWindow(c, mainWindowName) {
		return core.Fail(core.E("desktop.handleDeepLink", "main window is unavailable", nil))
	}
	return emitCoreEvent(c, deepLinkNavigateEvent, target.Value)
}
