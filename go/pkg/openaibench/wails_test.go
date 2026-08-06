// SPDX-Licence-Identifier: EUPL-1.2

package openaibench_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/orm"

	"dappco.re/go/crypt/keys"
	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/keysvc"
	"dappco.re/lthn/desktop/pkg/openaibench"
)

// wsvcFixture wires the full slice the WailsService needs:
// config + keys (with deterministic KEK) + orm (Memium-backed) +
// benchmark.Service registered + benchmark schemas mounted. Returns
// the Core, the live benchmark.Service, and the WailsService.
//
// Mirrors storeFixture (this file) and extends with the benchmark
// substrate so List/Add/Remove can exercise the full sync path.
func wsvcFixture(t *core.T) (*core.Core, *benchmark.Service, *openaibench.WailsService) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgFile := core.PathJoin(tmp, "lthn.yaml")
	core.AssertTrue(t, core.WriteFile(cfgFile, []byte("{}\n"), 0o644).OK)

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST_OAB_WSVC",
		})),
		core.WithName("keys", keysvc.Register),
	)
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.RequireTrue(t, keysSvc != nil)
	keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x42
		}
		return k, true
	})
	keysSvc.SetKEKProvider(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x77
		}
		return k, true
	})
	keysSvc.SetCore(c)

	// orm + benchmark substrate
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range benchmark.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	bm := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, bm.Register(c).OK)

	wsvc := openaibench.NewWailsService(c, bm)
	return c, bm, wsvc
}

func TestWailsService_ServiceName(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	core.AssertEqual(t, "OpenaibenchSettings", wsvc.ServiceName())
}

func TestWailsService_ServiceStartupAndShutdownAreNoOps(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	core.AssertTrue(t, wsvc.ServiceStartup(core.Background(), nil).OK)
	core.AssertTrue(t, wsvc.ServiceShutdown().OK)
}

func TestAddEndpoint_NilBenchmarkServiceFails(t *core.T) {
	c := storeFixture(t)
	wsvc := openaibench.NewWailsService(c, nil)

	r := wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "x", URL: "https://x/v1",
	})

	core.AssertFalse(t, r.OK)
}

func TestAddEndpoint_InvalidConfigFailsBeforeRegistering(t *core.T) {
	_, bm, wsvc := wsvcFixture(t)

	r := wsvc.AddEndpoint(openaibench.EndpointConfig{URL: "https://x/v1"})

	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, 0, len(bm.ListBenchers().Value.([]benchmark.BencherInfo)))
}

func TestRemoveEndpoint_NilBenchmarkServiceFails(t *core.T) {
	c := storeFixture(t)
	wsvc := openaibench.NewWailsService(c, nil)

	r := wsvc.RemoveEndpoint("anything")

	core.AssertFalse(t, r.OK)
}

func TestRegisterPersistedEndpoints_NilCoreFails(t *core.T) {
	_, bm, _ := wsvcFixture(t)

	r := openaibench.RegisterPersistedEndpoints(nil, bm)

	core.AssertFalse(t, r.OK)
}

func TestRegisterPersistedEndpoints_NilBenchmarkServiceFails(t *core.T) {
	c, _, _ := wsvcFixture(t)

	r := openaibench.RegisterPersistedEndpoints(c, nil)

	core.AssertFalse(t, r.OK)
}

func TestListEndpoints_PropagatesLoadFailure(t *core.T) {
	wsvc := openaibench.NewWailsService(nil, nil)

	r := wsvc.ListEndpoints()

	core.AssertFalse(t, r.OK)
}

func TestRegisterPersistedEndpoints_SkipsRecordsMissingURL(t *core.T) {
	// A record persisted before URL validation existed (or corrupted
	// out-of-band) must be skipped, not crash NewBencher's nil path
	// forward into a broken registration.
	c, bm, _ := wsvcFixture(t)
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.RequireTrue(t, keysSvc.PutTier1(
		"openaibench:no-url",
		[]byte(`{"name":"no-url"}`),
	).OK)

	r := openaibench.RegisterPersistedEndpoints(c, bm)

	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, 0, len(bm.ListBenchers().Value.([]benchmark.BencherInfo)))
}

func TestRegisterPersistedEndpoints_SkipsDuplicateNameRegistration(
	t *core.T,
) {
	// Two persisted records that happen to share a Name (e.g. hand-
	// edited store) must not both land in the substrate; the second
	// RegisterBencher rejection is logged and skipped.
	c, bm, _ := wsvcFixture(t)
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.RequireTrue(t, keysSvc.PutTier1(
		"openaibench:dup-a",
		[]byte(`{"name":"dup","url":"https://a/v1"}`),
	).OK)
	core.RequireTrue(t, keysSvc.PutTier1(
		"openaibench:dup-b",
		[]byte(`{"name":"dup","url":"https://b/v1"}`),
	).OK)

	r := openaibench.RegisterPersistedEndpoints(c, bm)

	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, 1, len(bm.ListBenchers().Value.([]benchmark.BencherInfo)))
}

func TestListEndpoints_EmptyReturnsEmpty(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	r := wsvc.ListEndpoints()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]openaibench.PublicEndpoint)))
}

func TestAddEndpoint_PersistsAndRegisters(t *core.T) {
	_, bm, wsvc := wsvcFixture(t)
	r := wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name:   "openai-compat:test",
		URL:    "https://api.example/v1",
		Bearer: "sk-test",
	})
	core.RequireTrue(t, r.OK)
	added, ok := r.Value.(openaibench.PublicEndpoint)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "openai-compat:test", added.Name)
	core.AssertEqual(t, true, added.AuthSet)

	// Persisted: ListEndpoints surfaces it.
	list := wsvc.ListEndpoints().Value.([]openaibench.PublicEndpoint)
	core.AssertEqual(t, 1, len(list))
	core.AssertEqual(t, "openai-compat:test", list[0].Name)

	// Registered: benchmark.Service.ListBenchers shows it.
	infos := bm.ListBenchers().Value.([]benchmark.BencherInfo)
	core.AssertEqual(t, 1, len(infos))
	core.AssertEqual(t, "openai-compat:test", infos[0].Name)
}

func TestAddEndpoint_OverwriteUnregistersFirst(t *core.T) {
	_, bm, wsvc := wsvcFixture(t)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "x", URL: "https://old/v1",
	}).OK)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "x", URL: "https://new/v1",
	}).OK)
	// Substrate still has exactly one — overwrite path took.
	infos := bm.ListBenchers().Value.([]benchmark.BencherInfo)
	core.AssertEqual(t, 1, len(infos))
	core.AssertEqual(t, "x", infos[0].Name)
}

func TestAddEndpoint_AuthSetFlagsBearerPresence(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "with-key", URL: "https://x/v1", Bearer: "sk-x",
	}).OK)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "no-key", URL: "https://y/v1",
	}).OK)
	list := wsvc.ListEndpoints().Value.([]openaibench.PublicEndpoint)
	// Two entries; assert their AuthSet by name (order not guaranteed).
	authByName := map[string]bool{}
	for _, e := range list {
		authByName[e.Name] = e.AuthSet
	}
	core.AssertEqual(t, true, authByName["with-key"])
	core.AssertEqual(t, false, authByName["no-key"])
}

func TestRemoveEndpoint_DropsBothStoreAndSubstrate(t *core.T) {
	_, bm, wsvc := wsvcFixture(t)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "doomed", URL: "https://x/v1", Bearer: "sk-x",
	}).OK)
	core.RequireTrue(t, wsvc.RemoveEndpoint("doomed").OK)

	core.AssertEqual(t, 0, len(wsvc.ListEndpoints().Value.([]openaibench.PublicEndpoint)))
	core.AssertEqual(t, 0, len(bm.ListBenchers().Value.([]benchmark.BencherInfo)))
}

func TestRemoveEndpoint_EmptyNameFails(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	core.AssertFalse(t, wsvc.RemoveEndpoint("").OK)
}

func TestRemoveEndpoint_UnknownIdempotent(t *core.T) {
	_, _, wsvc := wsvcFixture(t)
	// Remove of a never-existed name returns OK.
	core.RequireTrue(t, wsvc.RemoveEndpoint("never-existed").OK)
}

func TestRegisterPersistedEndpoints_RestoresAcrossBoot(t *core.T) {
	// Save endpoints via one wsvc, simulate "restart" by building a
	// fresh benchmark.Service from the SAME Core (and so the SAME
	// pkg/keys store), then call RegisterPersistedEndpoints. Expect
	// the saved endpoints to land back in the substrate.
	c, _, wsvc := wsvcFixture(t)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "boot-a", URL: "https://a/v1", Bearer: "sk-a",
	}).OK)
	core.RequireTrue(t, wsvc.AddEndpoint(openaibench.EndpointConfig{
		Name: "boot-b", URL: "https://b/v1",
	}).OK)

	// Fresh substrate (sim restart). Note: this writes to a NEW orm
	// table for the simulated reboot; pkg/keys persistence is shared.
	freshBM := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, freshBM.Register(c).OK)
	core.AssertEqual(t, 0, len(freshBM.ListBenchers().Value.([]benchmark.BencherInfo)))

	core.RequireTrue(t, openaibench.RegisterPersistedEndpoints(c, freshBM).OK)
	infos := freshBM.ListBenchers().Value.([]benchmark.BencherInfo)
	core.AssertEqual(t, 2, len(infos))
}
