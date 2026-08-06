// SPDX-Licence-Identifier: EUPL-1.2

// profile_validate_extra_test.go — closes the remaining
// validateProfile* branch gaps that profile_test.go's original suite
// (written against the Mantis #1603 attack walk) didn't happen to
// exercise: wrong-shape sub-values (non-map where a map is required,
// non-string where a string is required, non-array where an array is
// required), the nil/bool/number/array arms of the generic any-value
// walker, identifier length/emptiness, and the "mcp" / "agent" arms of
// defaultGuardedTouched (the original suite only drove "permission").

package opencode

import "testing"

func TestValidateProfileSchema_ProviderValueNotMap_Bad(t *testing.T) {
	p := Profile{Name: "x", Provider: map[string]any{"openai": "not-a-map"}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-map provider value to be rejected")
	}
}

func TestValidateProfileSchema_ProviderOptionsNotMap_Bad(t *testing.T) {
	p := Profile{Name: "x", Provider: map[string]any{"openai": map[string]any{"options": "not-a-map"}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-map provider.options value to be rejected")
	}
}

func TestValidateProfileSchema_ProviderBaseURLNotString_Bad(t *testing.T) {
	p := Profile{Name: "x", Provider: map[string]any{"openai": map[string]any{"options": map[string]any{"baseURL": 123}}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-string baseURL to be rejected")
	}
}

func TestValidateProfileSchema_MCPValueNotMap_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": "not-a-map"}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-map mcp value to be rejected")
	}
}

func TestValidateProfileSchema_MCPUnknownKey_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"hook": "evil"}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected unknown mcp key to be rejected")
	}
}

func TestValidateProfileSchema_MCPCommandNotString_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"command": 42}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-string mcp command to be rejected")
	}
}

func TestValidateProfileSchema_MCPArgsNotArray_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"command": "x", "args": "not-an-array"}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-array mcp args to be rejected")
	}
}

func TestValidateProfileSchema_MCPArgsElementNotString_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"command": "x", "args": []any{42}}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-string mcp args element to be rejected")
	}
}

func TestValidateProfileSchema_MCPURLNotString_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"url": 42}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-string mcp url to be rejected")
	}
}

func TestValidateProfileSchema_MCPURLInvalid_Bad(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{"url": "file:///etc/passwd"}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-http mcp url to be rejected")
	}
}

func TestValidateProfileSchema_MCPGenericKeyAccepted_Good(t *testing.T) {
	p := Profile{Name: "x", MCP: map[string]any{"srv": map[string]any{
		"command": "x", "enabled": true, "env": map[string]any{"FOO": "bar"},
	}}}
	if err := validateProfileSchema(p); err != nil {
		t.Fatalf("expected clean mcp record with enabled+env to validate, got: %v", err)
	}
}

func TestValidateProfileSchema_AgentValueNotMap_Bad(t *testing.T) {
	p := Profile{Name: "x", Agent: map[string]any{"build": "not-a-map"}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-map agent value to be rejected")
	}
}

func TestValidateProfileSchema_PermissionValueNotString_Bad(t *testing.T) {
	p := Profile{Name: "x", Permission: map[string]any{"bash": true}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected non-string permission value to be rejected")
	}
}

// --- validateProfileAnyValue arms --------------------------------------

func TestValidateProfileSchema_AnyValue_ArrayArm_Good(t *testing.T) {
	p := Profile{Name: "x", Agent: map[string]any{"build": map[string]any{
		"tools": []any{"bash", "edit"},
	}}}
	if err := validateProfileSchema(p); err != nil {
		t.Fatalf("expected array-valued agent field to validate, got: %v", err)
	}
}

func TestValidateProfileSchema_AnyValue_ArrayArmRejectsBadElement_Bad(t *testing.T) {
	big := make([]any, 0, 1)
	big = append(big, string(make([]byte, profileMaxStringLen+1)))
	p := Profile{Name: "x", Agent: map[string]any{"build": map[string]any{
		"tools": big,
	}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected over-long string inside an array value to be rejected")
	}
}

func TestValidateProfileSchema_AnyValue_ScalarArms_Good(t *testing.T) {
	p := Profile{Name: "x", Agent: map[string]any{"build": map[string]any{
		"temperature": 0.7,
		"mode":        true,
		"description": nil,
	}}}
	if err := validateProfileSchema(p); err != nil {
		t.Fatalf("expected nil/bool/float64 scalar agent fields to validate, got: %v", err)
	}
}

func TestValidateProfileSchema_AnyValue_UnsupportedType_Bad(t *testing.T) {
	// A channel is not a JSON-representable shape; any non-JSON Go
	// caller path (or a future non-JSON entry point) must be rejected
	// rather than silently accepted.
	p := Profile{Name: "x", Provider: map[string]any{"openai": map[string]any{
		"name": make(chan int),
	}}}
	if err := validateProfileSchema(p); err == nil {
		t.Fatal("expected an unsupported Go value type to be rejected")
	}
}

// --- identifier shape ----------------------------------------------------

func TestValidateProfileIdentifier_Empty_Bad(t *testing.T) {
	if err := validateProfileIdentifier("mcp", ""); err == nil {
		t.Fatal("expected empty identifier to be rejected")
	}
}

func TestValidateProfileIdentifier_TooLong_Bad(t *testing.T) {
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if err := validateProfileIdentifier("mcp", string(long)); err == nil {
		t.Fatal("expected a 65-byte identifier to be rejected")
	}
}

// --- defaultGuardedTouched: mcp / agent arms ------------------------------

func TestDefaultGuardedTouched_MCPArm_Good(t *testing.T) {
	p := Profile{Name: DefaultProfile, MCP: map[string]any{"srv": map[string]any{"command": "x"}}}
	touched := defaultGuardedTouched(p)
	if len(touched) != 1 || touched[0] != "mcp" {
		t.Fatalf("touched = %v; want [mcp]", touched)
	}
}

func TestDefaultGuardedTouched_AgentArm_Good(t *testing.T) {
	p := Profile{Name: DefaultProfile, Agent: map[string]any{"build": map[string]any{"model": "x"}}}
	touched := defaultGuardedTouched(p)
	if len(touched) != 1 || touched[0] != "agent" {
		t.Fatalf("touched = %v; want [agent]", touched)
	}
}

func TestDefaultGuardedTouched_AllThree_Good(t *testing.T) {
	p := Profile{
		Name:       DefaultProfile,
		MCP:        map[string]any{"srv": map[string]any{"command": "x"}},
		Agent:      map[string]any{"build": map[string]any{"model": "x"}},
		Permission: map[string]any{"bash": "deny"},
	}
	touched := defaultGuardedTouched(p)
	if len(touched) != 3 {
		t.Fatalf("touched = %v; want all 3 guarded fields", touched)
	}
}

// --- Service-level kv() Bad paths for ListProfiles / DeleteProfile /
// GetProfile ------------------------------------------------------------

func TestListProfiles_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.ListProfiles()
	if r.OK {
		t.Fatalf("ListProfiles with kv unavailable returned OK; want Fail")
	}
}

func TestDeleteProfile_EmptyName_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.DeleteProfile("  ")
	if r.OK {
		t.Fatalf("DeleteProfile('') returned OK; want Fail")
	}
}

func TestDeleteProfile_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.DeleteProfile("some-profile")
	if r.OK {
		t.Fatalf("DeleteProfile with kv unavailable returned OK; want Fail")
	}
}

func TestGetProfile_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.GetProfile("default")
	if r.OK {
		t.Fatalf("GetProfile with kv unavailable returned OK; want Fail")
	}
}

// TestGetProfile_CorruptStoredJSON_Ugly — a profile row that somehow
// landed non-JSON in the store (defensive: SaveProfile always writes
// valid JSON, but a future direct-KV write or migration bug shouldn't
// panic the read path) surfaces as a Fail rather than a panic.
func TestGetProfile_CorruptStoredJSON_Ugly(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	st, r := kv()
	if !r.OK {
		t.Fatalf("kv() failed: %s", r.Error())
	}
	if err := st.Set(profileStoreGroup, "corrupt", "{not-json"); err != nil {
		t.Fatalf("seed corrupt profile failed: %v", err)
	}
	got := svc.GetProfile("corrupt")
	if got.OK {
		t.Fatalf("GetProfile against corrupt stored JSON returned OK; want Fail")
	}
}

// TestSaveProfile_KVUnavailable_Bad — validation passes but the kv()
// persistence layer is unavailable.
func TestSaveProfile_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.SaveProfile(Profile{Name: "x"})
	if r.OK {
		t.Fatalf("SaveProfile with kv unavailable returned OK; want Fail")
	}
}

func TestSaveProfile_EmptyName_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.SaveProfile(Profile{})
	if r.OK {
		t.Fatalf("SaveProfile with empty name returned OK; want Fail")
	}
}

// --- upgradeGateCode direct coverage of the digest_mismatch arm --------

func TestUpgradeGateCode_DigestMismatch_Good(t *testing.T) {
	if got := upgradeGateCode("opencode.Upgrade: upgrade.digest_mismatch: registry served a different digest"); got != "upgrade.digest_mismatch" {
		t.Errorf("upgradeGateCode(digest_mismatch) = %q; want upgrade.digest_mismatch", got)
	}
}

func TestUpgradeGateCode_NoMatch_Good(t *testing.T) {
	if got := upgradeGateCode("some unrelated substrate failure"); got != "" {
		t.Errorf("upgradeGateCode(unrelated) = %q; want empty", got)
	}
}
