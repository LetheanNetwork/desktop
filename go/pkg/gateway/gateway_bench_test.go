// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the gateway's per-request dispatch — Service.Handle
// runs once for EVERY community-plugin call into host data/services
// (RFC.marketplace.md §7a): body-cap wrap, bundle-identity resolution,
// scope lookup, CheckPermission (an orm query + JSON-decode of the
// bundle's permission set), the scope Handler itself, and the
// response JSON encode. Companion load paths: pkg/runner's
// BenchmarkWChatStream_TokenLoop_* (per-token, not per-request) and
// pkg/connection's BenchmarkHandleRequest (the Wails binding sibling
// of this HTTP dispatch).
//
// Run:
//
//	go test -run='^$' -bench=. -benchmem -benchtime=20x ./pkg/gateway/
package gateway_test

import (
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// benchNoopRecorder discards every audit event with zero allocation.
// Service.Handle fires an audit.Default().Record on every branch
// (requested/succeeded/failed/rejected — pkg/gateway/audit_emit.go),
// and the production Default() is a real NDJSON file sink with an
// fsync per call — installing this keeps the bench about the
// gateway's own per-request cost, not disk latency, and stops the
// benchmark writing real files to ~/Lethean/audit/ on every run.
type benchNoopRecorder struct{}

func (benchNoopRecorder) Record(audit.Event) core.Result { return core.Ok(nil) }

// benchGatewayCore mirrors newGatewayCore (gateway_test.go) but takes
// *core.B — newGatewayCore is typed to *core.T, which a *core.B cannot
// satisfy positionally even though both implement the TB helpers used
// inside it.
func benchGatewayCore(b *core.B) *core.Core {
	b.Helper()
	c := core.New()
	core.RequireTrue(b, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(b, orm.Mount(c, "default", mem).OK)
	schema := marketplace.InstalledBundle{}.Schema()
	core.RequireTrue(b, orm.RegisterSchema(c, schema).OK)
	mem.RegisterTable(schema.Name, schema)
	core.RequireTrue(b, c.ServiceStartup(core.Background(), nil).OK)
	b.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// benchSeedBundle mirrors seedBundle (gateway_test.go), *core.B typed.
func benchSeedBundle(b *core.B, c *core.Core, id string, perms []marketplace.Permission) {
	b.Helper()
	rec := marketplace.InstalledBundle{
		BundleID:       id,
		Display:        id,
		ManifestSchema: "lthn-vm/v1",
		Status:         "running",
		ConfigPath:     "/tmp/" + id,
		InstalledAt:    core.Now(),
	}
	if len(perms) > 0 {
		rec.Permissions = core.JSONMarshalString(perms)
	}
	core.RequireTrue(b, orm.Save(c, &rec).OK)
}

// BenchmarkGatewayHandle_ProjectMetadataRead measures the full
// per-request dispatch for the no-body GET-shaped scope
// (project.metadata:read) — the cheapest real Handle call: bundle
// resolution + CheckPermission (orm.Find + DecodePermissions +
// linear scan) + the handler's own orm.Get + JSON response encode.
func BenchmarkGatewayHandle_ProjectMetadataRead(b *core.B) {
	audit.SetDefault(benchNoopRecorder{})
	b.Cleanup(func() { audit.SetDefault(nil) })
	gin.SetMode(gin.TestMode)
	c := benchGatewayCore(b)
	benchSeedBundle(b, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})
	svc := gateway.NewService(c)
	r := gin.New()
	r.POST("/v1/api/gateway/:scope/:mode", svc.Handle)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
		req.Header.Set("Bundle-ID", "opencode")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != core.StatusOK {
			b.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkGatewayHandle_ServiceNotifyInvoke measures the
// body-decoding dispatch shape (service.notify:invoke) —
// ShouldBindJSON + the byte-cap validation gates + dispatch through
// c.Action("notification.send"), the heaviest of the two built-in
// scopes and representative of a real plugin write-call.
func BenchmarkGatewayHandle_ServiceNotifyInvoke(b *core.B) {
	audit.SetDefault(benchNoopRecorder{})
	b.Cleanup(func() { audit.SetDefault(nil) })
	gin.SetMode(gin.TestMode)
	c := benchGatewayCore(b)
	benchSeedBundle(b, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	c.Action("notification.send", func(core.Context, core.Options) core.Result {
		return core.Ok(nil)
	})
	svc := gateway.NewService(c)
	r := gin.New()
	r.POST("/v1/api/gateway/:scope/:mode", svc.Handle)
	const payload = `{"title":"hi","message":"there"}`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(
			"POST", "/v1/api/gateway/service.notify/invoke", strings.NewReader(payload))
		req.Header.Set("Bundle-ID", "opencode")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != core.StatusOK {
			b.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkGatewayCheckPermission isolates the permission-lookup step
// alone (orm.Find + orm.Detail cast + DecodePermissions JSON-decode +
// linear scan) — called once per Handle dispatch, so this is the
// per-request floor before the scope Handler runs at all.
func BenchmarkGatewayCheckPermission(b *core.B) {
	c := benchGatewayCore(b)
	benchSeedBundle(b, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
		{Scope: "service.notify", Mode: "invoke"},
		{Scope: "project.files", Mode: "read"},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := gateway.CheckPermission(c, "opencode", "project.metadata", "read")
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}
