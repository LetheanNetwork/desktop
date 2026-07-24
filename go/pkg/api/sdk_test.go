// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the SDK generation helpers. ExportSpec is exercised with
// a real Core boot + temp-dir output so the full chain (Engine →
// SpecBuilder → file write) is verified. GenerateSDK is NOT covered
// here — it shells out to openapi-generator-cli + a JDK, which only
// the Taskfile / CI runs; we trust core/api's own SDKGenerator tests.

package api_test

import (
	core "dappco.re/go"
	coreapi "dappco.re/go/api"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/runner"
)

func TestSDK_DefaultSpecInfo_HasStableTitleAndVersion(t *core.T) {
	info := lthnapi.DefaultSpecInfo()
	core.AssertEqual(t, "lthn API", info.Title)
	if info.Version == "" {
		t.Fatalf("DefaultSpecInfo.Version must be non-empty")
	}
	if info.Description == "" {
		t.Fatalf("DefaultSpecInfo.Description must be non-empty")
	}
}

// newSpecCore builds a Core with just the api service registered —
// enough surface for ExportSpec to walk Engine.Groups().
func newSpecCore() *core.Core {
	c := core.New(
		core.WithName("api", coreapi.NewService(coreapi.ApiConfig{})),
	)
	_ = c.ServiceStartup(core.Background(), nil)
	return c
}

func TestSDK_ExportSpec_NilCoreFails(t *core.T) {
	r := lthnapi.ExportSpec(nil, "yaml", "/tmp/x.yaml", lthnapi.DefaultSpecInfo())
	core.AssertEqual(t, false, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSDK_ExportSpec_EmptyPathFails(t *core.T) {
	c := newSpecCore()
	defer c.ServiceShutdown(core.Background())
	r := lthnapi.ExportSpec(c, "yaml", "", lthnapi.DefaultSpecInfo())
	core.AssertEqual(t, false, r.OK)
}

func TestSDK_ExportSpec_InvalidFormatFails(t *core.T) {
	c := newSpecCore()
	defer c.ServiceShutdown(core.Background())
	r := lthnapi.ExportSpec(c, "xml", "/tmp/x.yaml", lthnapi.DefaultSpecInfo())
	core.AssertEqual(t, false, r.OK)
}

func TestSDK_ExportSpec_RunnerRoutesAppearInYAML(t *core.T) {
	c := newSpecCore()
	defer c.ServiceShutdown(core.Background())

	if r := lthnapi.Register(c, runner.NewService(runner.Options{})); !r.OK {
		t.Fatalf("register groups: %s", r.Error())
	}

	tmpR := c.Fs().TempDir("lthn-api-test-")
	core.RequireTrue(t, tmpR.OK)
	tmp := tmpR.Value.(string)
	out := core.Path(tmp, "openapi.yaml")
	r := lthnapi.ExportSpec(c, "yaml", out, lthnapi.DefaultSpecInfo())
	if !r.OK {
		t.Fatalf("export: %s", r.Error())
	}

	read := c.Fs().NewUnrestricted().Read(out)
	if !read.OK {
		t.Fatalf("read spec: %s", read.Error())
	}
	body, _ := read.Value.(string)
	for _, want := range []string{"/v1/runner/models", "/v1/runner/generate", "/v1/runner/chat"} {
		if !core.Contains(body, want) {
			t.Fatalf("exported spec missing %s — got:\n%s", want, body)
		}
	}
}

func TestSDK_GenerateSDK_RequiredArgsValidated(t *core.T) {
	ctx := core.Background()
	for _, c := range []struct {
		name      string
		spec, dir string
		lang      string
	}{
		{"missing spec", "", "/tmp/out", "typescript-fetch"},
		{"missing out", "/tmp/spec.yaml", "", "typescript-fetch"},
		{"missing lang", "/tmp/spec.yaml", "/tmp/out", ""},
	} {
		r := lthnapi.GenerateSDK(ctx, c.spec, c.dir, c.lang, "lthn-api")
		if r.OK {
			t.Fatalf("%s: expected failure", c.name)
		}
	}
}
