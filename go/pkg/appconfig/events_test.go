// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func TestSubscribe_GoodReceivesCommittedChange(t *core.T) {
	c, _ := newMediumConfigFixture(t, coreio.NewMemoryMedium())
	svc := appconfig.NewService(appconfig.Options{Core: c})
	events := []appconfig.Event{}
	appconfig.Subscribe(c, func(_ *core.Core, event appconfig.Event) {
		events = append(events, event)
	})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.theme.interface", Value: "light"},
		{Key: "desktop.shell.show_widgets", Value: false},
	})

	core.RequireTrue(t, result.OK, result.Error())
	snapshot, ok := result.Value.(map[string]any)
	core.RequireTrue(t, ok)
	revision, ok := snapshot["revision"].(string)
	core.RequireTrue(t, ok)
	core.AssertNotEqual(t, "", revision)
	core.RequireTrue(t, len(events) == 1)
	core.AssertEqual(t, revision, events[0].Revision)
	core.AssertEqual(t, []string{
		"desktop.theme.interface",
		"desktop.shell.show_widgets",
	}, events[0].Keys)
	core.AssertNotEqual(t, "", events[0].At)
}

func TestSubscribe_BadRejectedBatchEmitsNothing(t *core.T) {
	c, _ := newMediumConfigFixture(t, coreio.NewMemoryMedium())
	svc := appconfig.NewService(appconfig.Options{Core: c})
	events := []appconfig.Event{}
	appconfig.Subscribe(c, func(_ *core.Core, event appconfig.Event) {
		events = append(events, event)
	})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.theme.interface", Value: "light"},
		{Key: "desktop.theme.interface", Value: "dark"},
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, 0, len(events))
}

func TestSubscribe_UglyFailedCommitEmitsNothing(t *core.T) {
	base := coreio.NewMemoryMedium()
	fault := &configFaultMedium{Medium: base}
	c, _ := newMediumConfigFixture(t, fault)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	core.RequireTrue(t, svc.Set("desktop.theme.interface", "dark").OK)
	events := []appconfig.Event{}
	appconfig.Subscribe(c, func(_ *core.Core, event appconfig.Event) {
		events = append(events, event)
	})
	fault.failNextCommit = true

	result := svc.Set("desktop.theme.interface", "light")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, 0, len(events))
}

func TestSubscribe_BadNilCore(t *core.T) {
	appconfig.Subscribe(nil, func(_ *core.Core, _ appconfig.Event) {})
}

func TestSubscribe_UglyNilListener(t *core.T) {
	appconfig.Subscribe(core.New(), nil)
}
