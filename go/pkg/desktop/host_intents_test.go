// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	gui "dappco.re/go/render/display/webkit"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	guilifecycle "dappco.re/go/render/display/webkit/pkg/lifecycle"
	guinotification "dappco.re/go/render/display/webkit/pkg/notification"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	officefiles "dappco.re/lthn/desktop/pkg/office/files"
)

type hostIntentRuntime struct{}

func (hostIntentRuntime) Load() (officefiles.RuntimeSnapshot, error) {
	return officefiles.RuntimeSnapshot{Version: 1}, nil
}

func (hostIntentRuntime) Save(officefiles.RuntimeSnapshot) error {
	return nil
}

func hostIntentFixture(
	t *core.T,
) (*core.Core, *[]guievents.TaskEmit, string, coreio.Medium) {
	t.Helper()
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	filesService := officefiles.NewService(officefiles.Options{
		Mounts: []officefiles.Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Kind:               "local",
			LocalRoot:          root,
			Capabilities:       officefiles.ReadWriteCapabilities(),
			Medium:             medium,
			Owned:              true,
			ContainmentAudited: true,
		}},
		Runtime: hostIntentRuntime{},
	})
	registered := filesService.Register(nil)
	core.RequireTrue(t, registered.OK, registered.Error())

	c := core.New()
	core.RequireTrue(t, c.RegisterService("office-files", filesService).OK)
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})
	registerSystemEvents(c)
	return c, &emitted, root, medium
}

func TestHostIntent_OpenedWithFile_GoodEmitsOpaqueCapability(t *core.T) {
	c, emitted, root, medium := hostIntentFixture(t)
	core.RequireNoError(t, medium.Write("profile.lthn", "version: 1"))

	result := c.ACTION(guilifecycle.ActionOpenedWithFile{
		Path: core.PathJoin(root, "profile.lthn"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:host:intent", (*emitted)[0].Name)
	intent, ok := (*emitted)[0].Data.(HostIntent)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, HostIntentOpenItems, intent.Kind)
	core.RequireTrue(t, len(intent.Items) == 1)
	core.AssertEqual(t, "documents", intent.Items[0].MountID)
	core.AssertEqual(t, "profile.lthn", intent.Items[0].Path)
	core.AssertEqual(t, map[string]string{
		"app":    "settings",
		"action": "import",
	}, intent.Navigation)
	assertHostIntentRedacted(t, (*emitted)[0].Data, root)
}

func TestHostIntent_FilesDropped_GoodRetainsOnlyBoundedTarget(t *core.T) {
	c, emitted, root, medium := hostIntentFixture(t)
	core.RequireNoError(t, medium.Write("notes.txt", "hello"))

	result := c.ACTION(guiwindow.ActionFilesDropped{
		Name:  "app",
		Paths: []string{core.PathJoin(root, "notes.txt")},
		Target: &guiwindow.DropTarget{
			ID:        "files-drop-zone",
			X:         180,
			Y:         240,
			ClassList: []string{"drop", "/Users/private"},
			Attributes: map[string]string{
				"data-secret": `C:\Users\private`,
				"data-share":  `\\server\private`,
			},
		},
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:host:intent", (*emitted)[0].Name)
	intent := (*emitted)[0].Data.(HostIntent)
	core.AssertEqual(t, HostIntentDropItems, intent.Kind)
	core.AssertEqual(t, &HostDropTarget{
		ID: "files-drop-zone",
		X:  180,
		Y:  240,
	}, intent.Target)
	assertHostIntentRedacted(t, intent, root)
	serialised := core.JSONMarshalString(intent)
	core.AssertNotContains(t, serialised, "classList")
	core.AssertNotContains(t, serialised, "attributes")
}

func TestHostIntent_FilesDropped_BadInvalidTargetIsOmitted(t *core.T) {
	c, emitted, root, medium := hostIntentFixture(t)
	core.RequireNoError(t, medium.Write("notes.txt", "hello"))

	result := c.ACTION(guiwindow.ActionFilesDropped{
		Name:  "app",
		Paths: []string{core.PathJoin(root, "notes.txt")},
		Target: &guiwindow.DropTarget{
			ID: core.Repeat("x", maxHostTargetIDBytes+1),
			X:  maxHostCoordinate + 1,
			Y:  -1,
		},
	})

	core.RequireTrue(t, result.OK)
	core.RequireTrue(t, len(*emitted) == 1)
	intent := (*emitted)[0].Data.(HostIntent)
	core.AssertNil(t, intent.Target)
}

func TestHostIntent_OpenedWithFile_UglyFailureDoesNotLeakPath(t *core.T) {
	c, emitted, root, _ := hostIntentFixture(t)
	missing := core.PathJoin(root, "private", "missing.txt")

	result := c.ACTION(guilifecycle.ActionOpenedWithFile{Path: missing})

	core.RequireTrue(t, result.OK)
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:host:intent", (*emitted)[0].Name)
	intent := (*emitted)[0].Data.(HostIntent)
	core.AssertEqual(t, HostIntentItemsUnavailable, intent.Kind)
	core.AssertNotEmpty(t, intent.ErrorCode)
	assertHostIntentRedacted(t, intent, root)
	core.AssertNotContains(t, core.JSONMarshalString(intent), missing)
}

func TestHostIntent_NotificationClicked_GoodRoutesKnownServerIntent(t *core.T) {
	c, emitted, _, _ := hostIntentFixture(t)

	result := c.ACTION(guinotification.ActionNotificationClicked{
		ID: "lthn.model-runtime:load-complete",
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:host:intent", (*emitted)[0].Name)
	intent, ok := (*emitted)[0].Data.(HostIntent)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, HostIntentNotification, intent.Kind)
	core.AssertEqual(t, &HostNotificationIntent{
		ID:       "lthn.model-runtime:load-complete",
		IntentID: "model-runtime",
		Event:    "click",
	}, intent.Notification)
}

func TestHostIntent_NotificationAction_GoodRoutesAllowlistedOpen(t *core.T) {
	c, emitted, _, _ := hostIntentFixture(t)

	result := c.ACTION(guinotification.ActionNotificationActionTriggered{
		NotificationID: "lthn.managed-services:runner",
		ActionID:       "open",
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	intent := (*emitted)[0].Data.(HostIntent)
	core.AssertEqual(t, &HostNotificationIntent{
		ID:       "lthn.managed-services:runner",
		IntentID: "managed-services",
		Event:    "action",
		ActionID: "open",
	}, intent.Notification)
}

func TestHostIntent_NotificationAction_BadRejectsUnknownAction(t *core.T) {
	c, emitted, _, _ := hostIntentFixture(t)

	result := c.ACTION(guinotification.ActionNotificationActionTriggered{
		NotificationID: "lthn.managed-services:runner",
		ActionID:       "execute",
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, 0, len(*emitted))
}

func TestHostIntent_NotificationDismissed_GoodRoutesKnownIntent(t *core.T) {
	c, emitted, _, _ := hostIntentFixture(t)

	result := c.ACTION(guinotification.ActionNotificationDismissed{
		ID: "lthn.files-operation:copy-complete",
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	intent := (*emitted)[0].Data.(HostIntent)
	core.AssertEqual(t, &HostNotificationIntent{
		ID:       "lthn.files-operation:copy-complete",
		IntentID: "files-operation",
		Event:    "dismiss",
	}, intent.Notification)
}

func TestHostIntent_NotificationDismissed_UglyRejectsUnknownIntent(t *core.T) {
	c, emitted, _, _ := hostIntentFixture(t)

	result := c.ACTION(guinotification.ActionNotificationDismissed{
		ID: "third-party:anything",
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, 0, len(*emitted))
}

func TestHostIntent_SecondInstanceFile_GoodUsesSameOpaqueCapability(t *core.T) {
	c, emitted, root, medium := hostIntentFixture(t)
	core.RequireNoError(t, medium.Write("profile.lthn", "version: 1"))

	handleSecondInstanceLaunch(c, gui.SecondInstanceData{
		Args: []string{
			"lthn",
			core.PathJoin(root, "profile.lthn"),
		},
	})

	core.RequireTrue(t, len(*emitted) >= 2)
	var found bool
	for _, event := range *emitted {
		if event.Name != "lthn:host:intent" {
			continue
		}
		intent, ok := event.Data.(HostIntent)
		core.RequireTrue(t, ok)
		core.AssertEqual(t, HostIntentOpenItems, intent.Kind)
		assertHostIntentRedacted(t, intent, root)
		found = true
	}
	core.AssertTrue(t, found)
}

func assertHostIntentRedacted(t *core.T, payload any, actualRoot string) {
	t.Helper()
	serialised := core.JSONMarshalString(payload)
	core.AssertNotContains(t, serialised, actualRoot)
	core.AssertNotContains(t, serialised, "/Users/")
	core.AssertNotContains(t, serialised, `C:\\`)
	core.AssertNotContains(t, serialised, `\\\\server`)
	core.AssertNotContains(t, serialised, `"files":`)
}
