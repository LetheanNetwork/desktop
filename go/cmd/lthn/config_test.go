// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// config_test.go — hermetic coverage for the `lthn config` verb
// handlers. Each test composes a Core with only the config service,
// backed by a TempDir yaml file, and drives the handler exactly as
// cmdConfig dispatches it — asserting exit codes and the on-disk /
// in-service effects rather than captured stdout, per the
// main_test.go pattern.

package main

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// newConfigCore composes the minimal Core the config verbs need:
// the config service on a TempDir-backed yaml path, started and
// cleaned up with the test.
func newConfigCore(t *testing.T) *core.Core {
	t.Helper()
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      path,
			EnvPrefix: "LTHN",
		})),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestConfig_ConfigSet_Good — set persists the key and commits to
// disk in one call (the CLI flow), observable through the get action
// and the file existing at the service's path.
func TestConfig_ConfigSet_Good(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 0, configSet(c, []string{"transport.port", "8000"}))

	r := c.Action("config.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "key", Value: "transport.port"},
	))
	core.RequireTrue(t, r.OK)

	pathR := c.Action("config.path").Run(core.Background(), core.NewOptions())
	core.RequireTrue(t, pathR.OK)
	core.AssertTrue(t, core.Stat(pathR.Value.(string)).OK,
		"config set must commit the file to disk")
}

// TestConfig_ConfigSet_Bad — fewer than two arguments is a usage
// error before any service work happens.
func TestConfig_ConfigSet_Bad(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 2, configSet(c, []string{"only-a-key"}))
	core.AssertEqual(t, 2, configSet(c, nil))
}

// TestConfig_ConfigGet_Good — a set key reads back with exit 0.
func TestConfig_ConfigGet_Good(t *testing.T) {
	c := newConfigCore(t)
	core.RequireTrue(t, 0 == configSet(c, []string{"ui.theme", "dark"}))

	core.AssertEqual(t, 0, configGet(c, []string{"ui.theme"}))
}

// TestConfig_ConfigGet_Bad — no key argument is a usage error.
func TestConfig_ConfigGet_Bad(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 2, configGet(c, nil))
}

// TestConfig_ConfigAll_Good — listing succeeds on a fresh service
// and after a write.
func TestConfig_ConfigAll_Good(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 0, configAll(c, nil))
	core.RequireTrue(t, 0 == configSet(c, []string{"a.b", "1"}))
	core.AssertEqual(t, 0, configAll(c, nil))
}

// TestConfig_ConfigCommit_Good — an explicit commit succeeds and
// leaves the file on disk.
func TestConfig_ConfigCommit_Good(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 0, configCommit(c, nil))

	pathR := c.Action("config.path").Run(core.Background(), core.NewOptions())
	core.RequireTrue(t, pathR.OK)
	core.AssertTrue(t, core.Stat(pathR.Value.(string)).OK,
		"commit must write the config file")
}

// TestConfig_ConfigPath_Good — path reports the service's file with
// exit 0.
func TestConfig_ConfigPath_Good(t *testing.T) {
	c := newConfigCore(t)

	core.AssertEqual(t, 0, configPath(c, nil))
}

// TestConfig_CmdConfig_Bad — the dispatcher rejects a missing verb
// before booting any Core; unknown verbs boot the hermetic app Core
// (HOME isolated) and are rejected after dispatch.
func TestConfig_CmdConfig_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdConfig(nil))
}
