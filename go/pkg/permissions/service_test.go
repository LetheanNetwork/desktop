// SPDX-License-Identifier: EUPL-1.2

package permissions

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

type permissionHostProbe struct {
	states       map[ID]HostState
	statusCalls  []ID
	requestCalls []ID
}

func (p *permissionHostProbe) Status(id ID) (HostState, error) {
	p.statusCalls = append(p.statusCalls, id)
	return p.states[id], nil
}

func (p *permissionHostProbe) Request(id ID) (HostState, error) {
	p.requestCalls = append(p.requestCalls, id)
	return p.states[id], nil
}

func permissionServiceFixture(
	t *core.T,
	host Host,
) (*core.Core, *Service) {
	t.Helper()
	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{
			Path: core.PathJoin(t.TempDir(), "lthn.yaml"),
		}),
	))
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	service := NewService(Options{Core: c, Host: host})
	core.RequireTrue(t, service.Register(c).OK)
	return c, service
}

func TestService_Status_GoodSeparatesPolicyAndHostState(t *core.T) {
	host := &permissionHostProbe{states: map[ID]HostState{
		Notifications: HostGranted,
		Camera:        HostUnsupported,
	}}
	c, service := permissionServiceFixture(t, host)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("desktop.permissions.notifications", "deny").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.camera", "allow").OK)

	result := service.Status()

	core.RequireTrue(t, result.OK, result.Error())
	snapshots, ok := result.Value.([]Snapshot)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, Snapshot{
		ID:     Notifications,
		Policy: PolicyDeny,
		Host:   HostGranted,
	}, snapshotFor(t, snapshots, Notifications))
	core.AssertEqual(t, Snapshot{
		ID:     Camera,
		Policy: PolicyAllow,
		Host:   HostUnsupported,
	}, snapshotFor(t, snapshots, Camera))
	core.AssertEqual(t, 0, len(host.requestCalls))
}

func TestService_Request_GoodPromptsOnlyAfterExplicitKnownRequest(t *core.T) {
	host := &permissionHostProbe{states: map[ID]HostState{
		Notifications: HostGranted,
	}}
	_, service := permissionServiceFixture(t, host)

	status := service.Status()
	core.RequireTrue(t, status.OK, status.Error())
	core.AssertEqual(t, 0, len(host.requestCalls))

	result := service.Request(string(Notifications))

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []ID{Notifications}, host.requestCalls)
	core.AssertEqual(t, HostGranted, result.Value.(Snapshot).Host)
}

func TestService_Request_BadRejectsUnknownPermissionWithoutPrompt(t *core.T) {
	host := &permissionHostProbe{states: map[ID]HostState{}}
	_, service := permissionServiceFixture(t, host)

	result := service.Request("filesystem-root")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, 0, len(host.requestCalls))
}

func TestService_Status_UglyUnavailableHostReportsUnsupported(t *core.T) {
	_, service := permissionServiceFixture(t, nil)

	result := service.Status()

	core.RequireTrue(t, result.OK, result.Error())
	snapshots := result.Value.([]Snapshot)
	for _, snapshot := range snapshots {
		core.AssertEqual(t, HostUnsupported, snapshot.Host)
	}
}

func snapshotFor(t *core.T, snapshots []Snapshot, id ID) Snapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			return snapshot
		}
	}
	t.Fatalf("missing permission snapshot %q", id)
	return Snapshot{}
}
