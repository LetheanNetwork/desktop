// SPDX-Licence-Identifier: EUPL-1.2

// routes.go — REST surface for pkg/audit per RFC.stage-f.md §5 +
// §3.1. Implements coreapi.RouteGroup so pkg/server's
// auto-discovery loop mounts the handler via the RoutesProvider
// contract — same pattern as pkg/account / pkg/marketplace.
//
// Endpoints (RFC §5):
//
//   - GET /v1/audit/events — paginated forensic query. Filters via
//     query-string params (since, until, event, account_id,
//     outcome, limit, cursor). Returns QueryOutput { events,
//     next_cursor } via coreapi.Response envelope.
//
// Auth: session-tier per RouteTiers entry — see pkg/server's
// service.go for the registration. Bootstrap-tokens cannot satisfy
// the gate (RFC §5.3 + Cerberus #1711 scope-tier discipline).

package audit

import (
	"net/http"
	"time"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"github.com/gin-gonic/gin"
)

// APIBasePath is the canonical mount for the audit REST surface.
// Lives under /v1/audit/ to match the pkg/server.RouteTiers entry
// the boot path registers. Any change here MUST be made in
// lockstep with the RouteTiers map.
const APIBasePath = "/v1/audit"

// GroupName is the RouteGroup identifier the auto-discovery loop
// publishes. Stable + grep-able.
const GroupName = "lthn-audit"

// routesProvider satisfies coreapi.RouteGroup. Held private — callers
// consume it through Service.RouteGroups() per the pkg/server
// RoutesProvider contract.
type routesProvider struct {
	svc *Service
}

// Name satisfies coreapi.RouteGroup.
func (g *routesProvider) Name() string { return GroupName }

// BasePath satisfies coreapi.RouteGroup. All audit routes mount under
// /v1/audit/.
func (g *routesProvider) BasePath() string { return APIBasePath }

// RegisterRoutes satisfies coreapi.RouteGroup. Phase 2 ships the
// paginated query endpoint; the SSE tail endpoint slips to Phase 3.
//
// Usage example (inside server.NewService):
//
//	for _, g := range provider.RouteGroups() {
//	    engine.Register(g)
//	}
func (g *routesProvider) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/events", g.handleEvents)
}

// RouteGroups satisfies pkg/server.RoutesProvider. Returns the single
// REST group this Service publishes.
func (s *Service) RouteGroups() []coreapi.RouteGroup {
	return []coreapi.RouteGroup{&routesProvider{svc: s}}
}

// handleEvents is the gin handler for GET /v1/audit/events. Reads
// the QueryInput shape from query-string params, delegates to
// Service.Query, and shapes the HTTP response.
//
// Auth: session-tier enforced by the upstream bearer-auth middleware
// based on the RouteTiers entry. By the time this handler runs the
// caller has an unlocked session token.
//
// HTTP status mapping:
//
//   - audit.query.invalid_cursor → 400 Bad Request
//   - audit.query.invalid_param  → 400 Bad Request
//   - audit.query.deadline       → 504 Gateway Timeout (partial result + cursor)
//   - any other (unexpected)     → 500 Internal Server Error
//   - success                    → 200 OK
//
// Query-string params:
//
//   - since=<unix-seconds>       — inclusive lower bound (optional)
//   - until=<unix-seconds>       — exclusive upper bound (optional)
//   - event=<dotted.name>        — exact event-name match
//   - account_id=<raw>           — hashed before compare (RFC §6.4)
//   - outcome=<ok|failed|denied|error> — closed set per Cerberus #29
//     Q3 STRONGLY; unknown values 400 with audit.query.invalid_param.
//   - limit=<1..5000>            — default 200, cap 5000
//   - cursor=<opaque>            — resume from prior call
func (g *routesProvider) handleEvents(c *gin.Context) {
	in, perr, ok := parseEventsQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAuditQueryInvalidParam, perr))
		return
	}
	r := g.svc.Query(in)
	if r.OK {
		out, _ := r.Value.(QueryOutput)
		// Cerberus #29 ADD-MED-3 — Service.Query signals a wall-clock
		// deadline hit via QueryOutput.DeadlineExceeded; map to HTTP 504
		// with the audit.query.deadline error code. The partial Events
		// + resume NextCursor still ride along in the response body so
		// the client can rehydrate state and resume from the cursor.
		if out.DeadlineExceeded {
			c.JSON(http.StatusGatewayTimeout, coreapi.Response[QueryOutput]{
				Success: false,
				Data:    out,
				Error: &coreapi.Error{
					Code:    codeAuditQueryDeadline,
					Message: "audit query deadline exceeded; resume with next_cursor",
				},
			})
			return
		}
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}
	code := r.Code()
	switch code {
	case codeAuditQueryInvalidCursor, codeAuditQueryInvalidParam:
		c.JSON(http.StatusBadRequest, coreapi.Fail(code, r.Error()))
	default:
		if code == "" {
			code = codeAuditQueryUnexpected
		}
		c.JSON(http.StatusInternalServerError, coreapi.Fail(code, r.Error()))
	}
}

// parseEventsQuery extracts QueryInput from the gin context's
// query-string. Returns (in, "", false) when ANY supplied param fails
// validation — caller returns 400 with codeAuditQueryInvalidParam and
// the explanation string.
//
// Missing/blank fields fall through to QueryInput zero-values which
// Service.Query interprets as "no filter on this field."
//
// Cerberus #29 Q3 STRONGLY (RFC.stage-e-audit-viewer v2 §5.3): the
// outcome param is validated against the closed enum {ok, failed,
// denied, error}. Empty/missing → no filter. Unknown values fail with
// an explicit explanation so a probe like outcome=banana can't
// fingerprint scan-shape via the response timing/shape gap that a
// free-string passthrough would expose.
func parseEventsQuery(c *gin.Context) (QueryInput, string, bool) {
	in := QueryInput{
		Event:     c.Query("event"),
		AccountID: c.Query("account_id"),
		Outcome:   c.Query("outcome"),
		Cursor:    c.Query("cursor"),
	}
	if in.Outcome != "" && !isValidOutcome(in.Outcome) {
		return in, "outcome must be one of ok|failed|denied|error", false
	}
	if v := c.Query("since"); v != "" {
		r := core.ParseInt(v, 10, 64)
		if !r.OK {
			return in, "since must be a unix-seconds integer", false
		}
		i, _ := r.Value.(int64)
		in.Since = time.Unix(i, 0).UTC()
	}
	if v := c.Query("until"); v != "" {
		r := core.ParseInt(v, 10, 64)
		if !r.OK {
			return in, "until must be a unix-seconds integer", false
		}
		i, _ := r.Value.(int64)
		in.Until = time.Unix(i, 0).UTC()
	}
	if v := c.Query("limit"); v != "" {
		r := core.Atoi(v)
		if !r.OK {
			return in, "limit must be an integer", false
		}
		n, _ := r.Value.(int)
		in.Limit = n
	}
	return in, "", true
}

// Error codes Query path emits via coreapi.Response envelope.
const (
	codeAuditQueryInvalidParam = "audit.query.invalid_param"
	codeAuditQueryUnexpected   = "audit.query.unexpected"
)
