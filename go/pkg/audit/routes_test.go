// SPDX-Licence-Identifier: EUPL-1.2

// routes_test.go — handler-level tests for GET /v1/audit/events.
// Pairs with query_test.go which pins Service.Query directly; this
// file exercises the gin handler shape:
//
//   - Cerberus #29 Q3 STRONGLY (RFC.stage-e-audit-viewer v2 §5.3) —
//     outcome closed-set enforcement: unknown values 400 with
//     audit.query.invalid_param + canonical values pass + missing
//     value passes.
//   - Cerberus #29 ADD-MED-3 (§5.7) — deadline path maps to HTTP 504
//     with error.code = audit.query.deadline; partial events + resume
//     cursor still ride along in the response body.

package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "dappco.re/go"

	"github.com/gin-gonic/gin"
)

// newRoutesEngine builds a bare gin engine mounting the audit
// RouteGroup at its canonical base path. No auth middleware — the
// session-tier gate lives in pkg/server's bootstrap_auth chain which
// is pinned by pkg/server's own tests.
func newRoutesEngine(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	groups := svc.RouteGroups()
	if len(groups) != 1 {
		t.Fatalf("expected exactly one RouteGroup, got %d", len(groups))
	}
	g := groups[0]
	g.RegisterRoutes(r.Group(g.BasePath()))
	return r
}

func doGET(eng *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	eng.ServeHTTP(rr, req)
	return rr
}

// TestRoutes_OutcomeInvalidRejects_Bad — outcome=banana → 400 with the
// audit.query.invalid_param code and the canonical error message
// pointing the operator at the closed set.
func TestRoutes_OutcomeInvalidRejects_Bad(t *testing.T) {
	svc := newTestService(t)
	eng := newRoutesEngine(t, svc)

	rr := doGET(eng, "/v1/audit/events?outcome=banana")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if r := core.JSONUnmarshal(rr.Body.Bytes(), &env); !r.OK {
		t.Fatalf("decode body: %s", r.Error())
	}
	if env.Success {
		t.Fatal("expected success=false")
	}
	if env.Error.Code != codeAuditQueryInvalidParam {
		t.Fatalf("want code %q got %q", codeAuditQueryInvalidParam, env.Error.Code)
	}
	if !core.Contains(env.Error.Message, "outcome must be one of") {
		t.Fatalf("error message should name the closed set; got %q", env.Error.Message)
	}
}

// TestRoutes_OutcomeClosedSetAccepts_Good — each of ok|failed|denied|error
// reaches the handler successfully (200) without 400-rejecting at the
// validation gate. Empty result-set is fine — we only assert the gate
// doesn't reject the canonical literals.
func TestRoutes_OutcomeClosedSetAccepts_Good(t *testing.T) {
	svc := newTestService(t)
	eng := newRoutesEngine(t, svc)

	for _, outcome := range []string{
		OutcomeOK, OutcomeFailed, OutcomeDenied, OutcomeError,
	} {
		rr := doGET(eng, "/v1/audit/events?outcome="+outcome)
		if rr.Code != http.StatusOK {
			t.Fatalf("outcome=%s MUST be accepted; got status=%d body=%s",
				outcome, rr.Code, rr.Body.String())
		}
	}
}

// TestRoutes_OutcomeEmptyPasses_Good — missing outcome= falls through
// to no-filter behaviour. The handler MUST 200 even when the param is
// absent.
func TestRoutes_OutcomeEmptyPasses_Good(t *testing.T) {
	svc := newTestService(t)
	eng := newRoutesEngine(t, svc)

	rr := doGET(eng, "/v1/audit/events")
	if rr.Code != http.StatusOK {
		t.Fatalf("missing outcome MUST be accepted; got status=%d body=%s",
			rr.Code, rr.Body.String())
	}

	rr2 := doGET(eng, "/v1/audit/events?outcome=")
	if rr2.Code != http.StatusOK {
		t.Fatalf("empty outcome= MUST be accepted; got status=%d body=%s",
			rr2.Code, rr2.Body.String())
	}
}

// TestRoutes_DeadlineMapsTo504_Bad — Cerberus #29 ADD-MED-3. Force the
// deadline override so the very first walker iteration trips. Handler
// MUST map QueryOutput.DeadlineExceeded → HTTP 504 with error.code =
// audit.query.deadline. Partial Events + resume NextCursor still ride
// along in the response body so the client can rehydrate state.
func TestRoutes_DeadlineMapsTo504_Bad(t *testing.T) {
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	// Seed a recent day so the walker has at least one file in scope
	// — otherwise it short-circuits before reaching the deadline check.
	writeDayFileRaw(t, svc, dateStem(now), []Event{{
		Event:   EventAuthSessionIssued,
		TS:      now,
		Outcome: OutcomeOK,
	}})

	prev := queryDeadlineOverride
	queryDeadlineOverride = 1 * time.Nanosecond
	defer func() { queryDeadlineOverride = prev }()

	eng := newRoutesEngine(t, svc)
	rr := doGET(eng, "/v1/audit/events")
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Events           []Event `json:"events"`
			NextCursor       string  `json:"next_cursor"`
			DeadlineExceeded bool    `json:"deadline_exceeded"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if r := core.JSONUnmarshal(rr.Body.Bytes(), &env); !r.OK {
		t.Fatalf("decode body: %s", r.Error())
	}
	if env.Success {
		t.Fatal("deadline path MUST report success=false")
	}
	if env.Error.Code != codeAuditQueryDeadline {
		t.Fatalf("want code %q got %q", codeAuditQueryDeadline, env.Error.Code)
	}
	if env.Data.NextCursor == "" {
		t.Fatal("deadline response body MUST carry a resume next_cursor")
	}
	if !env.Data.DeadlineExceeded {
		t.Fatal("deadline response body MUST carry deadline_exceeded=true")
	}
}
