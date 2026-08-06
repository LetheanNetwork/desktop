// SPDX-Licence-Identifier: EUPL-1.2

// host_config_extra_test.go — remaining MergeHostConfig branches (the
// control_handlers_test.go suite already covers the Good/Conflict
// shapes end-to-end via the HTTP handler) plus direct coverage of the
// nestedString helper.

package opencode

import (
	"testing"

	core "dappco.re/go"
)

func TestNestedString_Good(t *testing.T) {
	m := map[string]any{
		"options": map[string]any{
			"baseURL": "http://localhost:8000/v1",
		},
	}
	if got := nestedString(m, "options", "baseURL"); got != "http://localhost:8000/v1" {
		t.Errorf("nestedString(found) = %q; want the baseURL", got)
	}
	if got := nestedString(m, "options", "missing"); got != "" {
		t.Errorf("nestedString(missing leaf) = %q; want empty", got)
	}
	if got := nestedString(m, "missing", "baseURL"); got != "" {
		t.Errorf("nestedString(missing branch) = %q; want empty", got)
	}
	if got := nestedString(m, "options", "baseURL", "extra"); got != "" {
		t.Errorf("nestedString(overshoot) = %q; want empty", got)
	}
	nonString := map[string]any{"options": map[string]any{"count": 3}}
	if got := nestedString(nonString, "options", "count"); got != "" {
		t.Errorf("nestedString(non-string leaf) = %q; want empty", got)
	}
}

// TestMergeHostConfig_ForceOverwritesConflict_Good — force=true
// overwrites a conflicting provider.lthn block instead of returning
// HostConfigConflict.
func TestMergeHostConfig_ForceOverwritesConflict_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	home, _ := core.UserHomeDir().Value.(string)
	path := core.PathJoin(home, hostConfigSubpath)
	if r := core.MkdirAll(core.PathDir(path), 0o700); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	existing := `{"provider":{"lthn":{"options":{"baseURL":"http://attacker.example/v1"}}}}`
	if r := core.WriteFile(path, []byte(existing), 0o600); !r.OK {
		t.Fatalf("seed existing config failed: %s", r.Error())
	}

	r := svc.MergeHostConfig(MergeHostConfigOptions{Force: true})
	if !r.OK {
		t.Fatalf("MergeHostConfig(force=true) failed: %s", r.Error())
	}
	res, _ := r.Value.(MergeHostConfigResult)
	if res.Created {
		t.Errorf("Created = true on an overwrite of an existing file; want false")
	}
	if core.Contains(res.Bytes, "attacker.example") {
		t.Errorf("merged bytes still carry the conflicting baseURL after force overwrite: %s", res.Bytes)
	}
}

// TestMergeHostConfig_MalformedExistingJSON_Bad — an existing
// opencode.json that isn't valid JSON (e.g. hand-authored JSONC with
// comments) must surface a clear error rather than silently
// clobbering the user's file.
func TestMergeHostConfig_MalformedExistingJSON_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	home, _ := core.UserHomeDir().Value.(string)
	path := core.PathJoin(home, hostConfigSubpath)
	if r := core.MkdirAll(core.PathDir(path), 0o700); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	if r := core.WriteFile(path, []byte(`{ // comment\n "provider": {} }`), 0o600); !r.OK {
		t.Fatalf("seed malformed config failed: %s", r.Error())
	}

	r := svc.MergeHostConfig(MergeHostConfigOptions{})
	if r.OK {
		t.Fatalf("MergeHostConfig against a malformed existing file returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "not valid JSON") {
		t.Errorf("error = %q; want mention of invalid JSON", r.Error())
	}
}

// TestMergeHostConfig_PreservesOtherProviders_Good — a pre-existing
// unrelated provider block (e.g. openai) survives the merge verbatim.
func TestMergeHostConfig_PreservesOtherProviders_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	home, _ := core.UserHomeDir().Value.(string)
	path := core.PathJoin(home, hostConfigSubpath)
	if r := core.MkdirAll(core.PathDir(path), 0o700); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	existing := `{"provider":{"openai":{"options":{"apiKey":"sk-user-owns-this"}}}}`
	if r := core.WriteFile(path, []byte(existing), 0o600); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}

	r := svc.MergeHostConfig(MergeHostConfigOptions{})
	if !r.OK {
		t.Fatalf("MergeHostConfig failed: %s", r.Error())
	}
	res, _ := r.Value.(MergeHostConfigResult)
	if !core.Contains(res.Bytes, "sk-user-owns-this") {
		t.Errorf("merge dropped the pre-existing openai provider block: %s", res.Bytes)
	}
	if !core.Contains(res.Bytes, "lthn") {
		t.Errorf("merge did not add the lthn provider block: %s", res.Bytes)
	}
}

// TestMergeHostConfig_UnknownProfile_Bad — GetProfile fails before any
// filesystem side effect.
func TestMergeHostConfig_UnknownProfile_Bad(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	r := svc.MergeHostConfig(MergeHostConfigOptions{Profile: "does-not-exist"})
	if r.OK {
		t.Fatalf("MergeHostConfig with an unknown profile returned OK; want Fail")
	}
}
