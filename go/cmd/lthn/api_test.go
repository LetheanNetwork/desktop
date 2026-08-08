// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// api_test.go — coverage for the `lthn api` verb surface. The pure
// helpers and pre-boot usage errors run without any Core; the spec
// happy path boots the real composed app Core against an isolated
// HOME, per the app_test.go pattern, and asserts the artefact on
// disk rather than captured output.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestApi_CmdAPI_Bad — missing and unknown verbs are usage errors
// decided before any Core boots.
func TestApi_CmdAPI_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdAPI(nil))
	core.AssertEqual(t, 2, cmdAPI([]string{"nonsense"}))
}

// TestApi_ApiSpec_Bad — an unsupported format is rejected before
// boot.
func TestApi_ApiSpec_Bad(t *testing.T) {
	core.AssertEqual(t, 2, apiSpec([]string{"--format", "toml"}))
}

// TestApi_ApiSpec_Good — the full path: boot the composed app Core
// against an isolated HOME and write a JSON spec to an explicit
// output. The receipt is the file on disk.
func TestApi_ApiSpec_Good(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	out := core.PathJoin(tmp, "openapi.json")
	core.AssertEqual(t, 0, apiSpec([]string{"--format", "json", "--out", out}))
	core.AssertTrue(t, core.Stat(out).OK, "spec file must exist at --out")
}

// TestApi_ApiSDK_Bad — a missing language argument is a usage error
// before boot.
func TestApi_ApiSDK_Bad(t *testing.T) {
	core.AssertEqual(t, 2, apiSDK(nil))
}

// TestApi_ApiFlagValues_Good — both --k=v and --k v forms parse, and
// only allowed keys are collected.
func TestApi_ApiFlagValues_Good(t *testing.T) {
	got := apiFlagValues(
		[]string{"--format=json", "--out", "spec.json", "--rogue=x"},
		map[string]bool{"format": true, "out": true},
	)
	core.AssertEqual(t, 2, len(got))
	core.AssertEqual(t, "json", got["format"])
	core.AssertEqual(t, "spec.json", got["out"])
}

// TestApi_ApiFlagValues_Bad — non-flag tokens and disallowed keys
// yield an empty map, never an error.
func TestApi_ApiFlagValues_Bad(t *testing.T) {
	got := apiFlagValues([]string{"spec", "plain", "--rogue", "v"},
		map[string]bool{"format": true})
	core.AssertEqual(t, 0, len(got))
}

// TestApi_ApiFlagValues_Ugly — a trailing separated flag with no
// value keeps the empty value rather than reading past the slice.
func TestApi_ApiFlagValues_Ugly(t *testing.T) {
	got := apiFlagValues([]string{"--out"}, map[string]bool{"out": true})
	core.AssertEqual(t, "", got["out"])
}

// TestApi_DefaultSpecOutput_Good — the format decides the default
// filename.
func TestApi_DefaultSpecOutput_Good(t *testing.T) {
	core.AssertEqual(t, "openapi.json", defaultSpecOutput("json"))
	core.AssertEqual(t, "openapi.yaml", defaultSpecOutput("yaml"))
}
