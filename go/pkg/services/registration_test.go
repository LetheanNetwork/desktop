// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
)

func TestRegister_GoodResolvesNamedProcessAndMediumWithoutStarting(t *core.T) {
	medium := coreio.NewMemoryMedium()
	c := core.New(
		core.WithName("process", coreprocess.NewService(coreprocess.Options{})),
		core.WithName("io", func(c *core.Core) core.Result {
			return core.Ok(&coreio.Service{
				ServiceRuntime: core.NewServiceRuntime(c, coreio.IOConfig{}),
				Medium:         medium,
			})
		}),
		core.WithName("services", Register),
	)

	started := c.ServiceStartup(core.Background(), nil)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	core.RequireTrue(t, started.OK, started.Error())
	manager, ok := core.ServiceFor[*Service](c, "services")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, manager != nil)
	processes, ok := core.ServiceFor[*coreprocess.Service](c, "process")
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 0, len(processes.List()))
	view := manager.Catalogue()
	core.RequireTrue(t, view.OK, view.Error())
	snapshots := view.Value.(CatalogueView).Services
	core.RequireTrue(t, len(snapshots) == 1)
	core.AssertEqual(t, "serve", snapshots[0].Definition.ID)
	core.AssertEqual(t, StateStopped, snapshots[0].State)
	core.AssertFalse(t, medium.IsFile("desktop/services/catalogue.json"))
}

func TestRegister_BadFailsClosedWithoutNamedDependencies(t *core.T) {
	noDependencies := Register(core.New())
	core.AssertFalse(t, noDependencies.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(noDependencies))

	c := core.New(core.WithName("process", coreprocess.NewService(coreprocess.Options{})))
	noMedium := Register(c)
	core.AssertFalse(t, noMedium.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(noMedium))
}
