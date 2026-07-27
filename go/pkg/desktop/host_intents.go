// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	officefiles "dappco.re/lthn/desktop/pkg/office/files"
)

const (
	// HostIntentOpenItems opens one or more Files capabilities.
	HostIntentOpenItems = "open-items"
	// HostIntentDropItems carries Files capabilities dropped on the shell.
	HostIntentDropItems = "drop-items"
	// HostIntentItemsUnavailable reports a bounded capability resolution
	// failure without exposing the selected native path.
	HostIntentItemsUnavailable = "items-unavailable"
	// HostIntentNotification carries one allowlisted native notification
	// response back to the renderer.
	HostIntentNotification = "notification"

	maxHostTargetIDBytes       = 64
	maxHostNotificationIDBytes = 128
	maxHostCoordinate          = 32_768
)

// HostIntent is the single renderer-facing envelope for trusted native
// lifecycle input.
type HostIntent struct {
	Kind         string                     `json:"kind"`
	Items        []officefiles.HostItemView `json:"items,omitempty"`
	Navigation   map[string]string          `json:"navigation,omitempty"`
	Notification *HostNotificationIntent    `json:"notification,omitempty"`
	Target       *HostDropTarget            `json:"target,omitempty"`
	ErrorCode    string                     `json:"errorCode,omitempty"`
}

// HostNotificationIntent contains no command, URL, path, or free-form action.
// The intent and action IDs are both selected from server-owned catalogues.
type HostNotificationIntent struct {
	ID       string `json:"id"`
	IntentID string `json:"intentId"`
	Event    string `json:"event"`
	ActionID string `json:"actionId,omitempty"`
}

// HostDropTarget retains only the bounded identity and coordinates required
// to route a drop. DOM classes and attributes are intentionally discarded.
type HostDropTarget struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
}

func emitHostItemIntent(
	c *core.Core,
	kind string,
	paths []string,
	target *guiwindow.DropTarget,
	targetID string,
) core.Result {
	intent := hostItemIntent(c, kind, paths, target, targetID)
	return emitCoreEvent(c, "lthn:host:intent", intent)
}

func hostItemIntent(
	c *core.Core,
	kind string,
	paths []string,
	target *guiwindow.DropTarget,
	targetID string,
) HostIntent {
	if kind != HostIntentOpenItems && kind != HostIntentDropItems {
		return HostIntent{
			Kind:      HostIntentItemsUnavailable,
			ErrorCode: string(officefiles.ErrorInvalidInput),
		}
	}
	service, ok := core.ServiceFor[*officefiles.Service](c, "office-files")
	if !ok || service == nil {
		return HostIntent{
			Kind:      HostIntentItemsUnavailable,
			ErrorCode: string(officefiles.ErrorProviderUnavailable),
		}
	}
	resolved := officefiles.ResolveHostItems(service, paths)
	if !resolved.OK {
		return HostIntent{
			Kind:      HostIntentItemsUnavailable,
			ErrorCode: hostItemErrorCode(resolved),
		}
	}
	items, ok := resolved.Value.([]officefiles.HostItemView)
	if !ok || len(items) == 0 {
		return HostIntent{
			Kind:      HostIntentItemsUnavailable,
			ErrorCode: string(officefiles.ErrorProviderUnavailable),
		}
	}
	return HostIntent{
		Kind:       kind,
		Items:      items,
		Navigation: hostItemNavigation(items),
		Target:     boundedHostDropTarget(target, targetID),
	}
}

func hostItemErrorCode(result core.Result) string {
	if failure, ok := result.Value.(*officefiles.Failure); ok &&
		failure != nil && failure.Code != "" {
		return string(failure.Code)
	}
	return string(officefiles.ErrorProviderUnavailable)
}

func hostItemNavigation(
	items []officefiles.HostItemView,
) map[string]string {
	if len(items) == 1 &&
		core.Lower(core.PathExt(items[0].Name)) == ".lthn" {
		return map[string]string{
			"app":    "settings",
			"action": "import",
		}
	}
	return map[string]string{
		"app":    "files",
		"action": "open",
	}
}

func boundedHostDropTarget(
	target *guiwindow.DropTarget,
	fallbackID string,
) *HostDropTarget {
	if target == nil {
		if fallbackID == "" {
			return nil
		}
		target = &guiwindow.DropTarget{ID: fallbackID}
	}
	if !validHostTargetID(target.ID) ||
		target.X < 0 || target.X > maxHostCoordinate ||
		target.Y < 0 || target.Y > maxHostCoordinate {
		return nil
	}
	return &HostDropTarget{
		ID: target.ID,
		X:  target.X,
		Y:  target.Y,
	}
}

func validHostTargetID(id string) bool {
	return validHostIdentifier(id, maxHostTargetIDBytes)
}

func emitHostNotificationIntent(
	c *core.Core,
	event string,
	notificationID string,
	actionID string,
) core.Result {
	notification, ok := hostNotificationIntent(
		event,
		notificationID,
		actionID,
	)
	if !ok {
		return core.Ok(nil)
	}
	return emitCoreEvent(c, "lthn:host:intent", HostIntent{
		Kind:         HostIntentNotification,
		Notification: &notification,
	})
}

func hostNotificationIntent(
	event string,
	notificationID string,
	actionID string,
) (HostNotificationIntent, bool) {
	intentID, ok := hostNotificationIntentID(notificationID)
	if !ok {
		return HostNotificationIntent{}, false
	}
	switch event {
	case "click", "dismiss":
		if actionID != "" {
			return HostNotificationIntent{}, false
		}
	case "action":
		if actionID != "open" {
			return HostNotificationIntent{}, false
		}
	default:
		return HostNotificationIntent{}, false
	}
	return HostNotificationIntent{
		ID:       notificationID,
		IntentID: intentID,
		Event:    event,
		ActionID: actionID,
	}, true
}

func hostNotificationIntentID(notificationID string) (string, bool) {
	if !validHostIdentifier(
		notificationID,
		maxHostNotificationIDBytes,
	) {
		return "", false
	}
	for _, intentID := range []string{
		"managed-services",
		"model-runtime",
		"terminal-session",
		"files-operation",
		"desktop-update",
	} {
		prefix := "lthn." + intentID
		if notificationID == prefix ||
			core.HasPrefix(notificationID, prefix+":") {
			return intentID, true
		}
	}
	return "", false
}

func validHostIdentifier(id string, maximum int) bool {
	if id == "" || len(id) > maximum {
		return false
	}
	for _, character := range []byte(id) {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-', character == '_', character == ':',
			character == '.':
		default:
			return false
		}
	}
	return true
}
