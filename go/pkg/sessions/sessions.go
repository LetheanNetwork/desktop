// SPDX-Licence-Identifier: EUPL-1.2

// Package sessions persists chat sessions through the registered
// dappco.re/go/store service. Each session is a JSON-encoded record
// under group "sessions" (per-session message log) plus a manifest
// entry under group "sessions:manifest" (id → title + timestamps).
//
// The package is a thin typed accessor — it does no protocol work
// of its own; it composes go-store actions through the Core bus.
//
// Schema:
//
//	store group "sessions"          key = session id      value = JSON([]Message)
//	store group "sessions:manifest" key = session id      value = JSON(SessionInfo)
//
// Usage example:
//
//	id := sessions.Create(c, "first chat")
//	sessions.Append(c, id, "user", "hello")
//	sessions.Append(c, id, "assistant", "hi")
//	msgs := sessions.Read(c, id)
package sessions

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/inference"
)

// SessionInfo is the manifest entry for a session — what `lthn
// sessions list` prints.
type SessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Messages  int    `json:"messages"`
}

const (
	groupMessages = "sessions"
	groupManifest = "sessions:manifest"
)

// Create starts a new session with the given title. Returns the new
// session id (random hex) on success.
//
// Usage example:
//
//	r := sessions.Create(c, "first chat")
//	if r.OK { id := r.Value.(string); _ = id }
func Create(c *core.Core, title string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Create", "core is nil", nil))
	}
	idResult := core.RandomString(16)
	if !idResult.OK {
		return idResult
	}
	id := idResult.Value.(string)
	now := core.UnixNow()
	info := SessionInfo{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  0,
	}
	if r := writeManifest(c, info); !r.OK {
		return r
	}
	if r := writeMessages(c, id, []inference.Message{}); !r.OK {
		return r
	}
	return core.Ok(id)
}

// Append adds a message to the session, refreshes UpdatedAt and the
// message count in the manifest.
//
// Usage example:
//
//	sessions.Append(c, id, "user", "ping")
func Append(c *core.Core, id, role, content string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Append", "core is nil", nil))
	}
	msgs, r := readMessages(c, id)
	if !r.OK {
		return r
	}
	msgs = append(msgs, inference.Message{Role: role, Content: content})
	if r := writeMessages(c, id, msgs); !r.OK {
		return r
	}
	info, r := readManifest(c, id)
	if !r.OK {
		return r
	}
	info.UpdatedAt = core.UnixNow()
	info.Messages = len(msgs)
	return writeManifest(c, info)
}

// Read returns the full message log for the session.
//
// Usage example:
//
//	r := sessions.Read(c, id)
//	if r.OK { msgs := r.Value.([]inference.Message); _ = msgs }
func Read(c *core.Core, id string) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.Read", "core is nil", nil))
	}
	msgs, r := readMessages(c, id)
	if !r.OK {
		return r
	}
	return core.Ok(msgs)
}

// List returns the manifest entries for every session. Order is
// store-dependent; callers wanting recency-sorted output sort by
// UpdatedAt themselves.
//
// Usage example:
//
//	r := sessions.List(c)
//	if r.OK { infos := r.Value.([]SessionInfo); _ = infos }
func List(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("sessions.List", "core is nil", nil))
	}
	r := c.Action("store.get_all").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
	))
	if !r.OK {
		return r
	}
	raw, ok := r.Value.(map[string]string)
	if !ok {
		return core.Ok([]SessionInfo{})
	}
	out := make([]SessionInfo, 0, len(raw))
	for _, encoded := range raw {
		var info SessionInfo
		if ur := core.JSONUnmarshalString(encoded, &info); !ur.OK {
			continue
		}
		out = append(out, info)
	}
	return core.Ok(out)
}

func writeMessages(c *core.Core, id string, msgs []inference.Message) core.Result {
	encoded := core.JSONMarshal(msgs)
	if !encoded.OK {
		return encoded
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return core.Fail(core.E("sessions.writeMessages", "encode failed", nil))
	}
	return c.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupMessages},
		core.Option{Key: "key", Value: id},
		core.Option{Key: "value", Value: string(bytes)},
	))
}

func readMessages(c *core.Core, id string) ([]inference.Message, core.Result) {
	r := c.Action("store.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupMessages},
		core.Option{Key: "key", Value: id},
	))
	if !r.OK {
		return nil, r
	}
	encoded, ok := r.Value.(string)
	if !ok || encoded == "" {
		return []inference.Message{}, core.Ok(nil)
	}
	var msgs []inference.Message
	if ur := core.JSONUnmarshalString(encoded, &msgs); !ur.OK {
		return nil, ur
	}
	return msgs, core.Ok(nil)
}

func writeManifest(c *core.Core, info SessionInfo) core.Result {
	encoded := core.JSONMarshal(info)
	if !encoded.OK {
		return encoded
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return core.Fail(core.E("sessions.writeManifest", "encode failed", nil))
	}
	return c.Action("store.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
		core.Option{Key: "key", Value: info.ID},
		core.Option{Key: "value", Value: string(bytes)},
	))
}

func readManifest(c *core.Core, id string) (SessionInfo, core.Result) {
	r := c.Action("store.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "group", Value: groupManifest},
		core.Option{Key: "key", Value: id},
	))
	if !r.OK {
		return SessionInfo{}, r
	}
	encoded, ok := r.Value.(string)
	if !ok || encoded == "" {
		return SessionInfo{}, core.Fail(core.E("sessions.readManifest", "not found", nil))
	}
	var info SessionInfo
	if ur := core.JSONUnmarshalString(encoded, &info); !ur.OK {
		return SessionInfo{}, ur
	}
	return info, core.Ok(nil)
}
