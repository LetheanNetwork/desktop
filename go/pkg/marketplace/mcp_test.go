// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// TestMCP_RegisterMCPTools_Good verifies RegisterMCPTools is a no-op when
// called with nil arguments (defensive — subsystems.go may call before
// services are confirmed alive).
func TestMCP_RegisterMCPTools_Good(t *core.T) {
	// Neither panic nor error — nil mcpSvc + nil ms are safe.
	subject.RegisterMCPTools(nil, nil)
	subject.RegisterMCPTools(nil, newTestMarketplaceService())
}

// TestMCP_RegisterMCPTools_Bad verifies RegisterMCPTools with nil mcpSvc
// does not panic regardless of ms state.
func TestMCP_RegisterMCPTools_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	subject.RegisterMCPTools(nil, svc)
}

// TestMCP_RegisterMCPTools_Ugly verifies the function signature is exported
// and callable from outside the package.
func TestMCP_RegisterMCPTools_Ugly(t *core.T) {
	ref := subject.RegisterMCPTools
	core.AssertNotNil(t, ref)
}

// TestMCP_MCPListInput_Good verifies the MCPListInput struct fields.
func TestMCP_MCPListInput_Good(t *core.T) {
	in := subject.MCPListInput{
		Query:    "opencode",
		Category: "ai-agents",
	}
	core.AssertEqual(t, "opencode", in.Query)
	core.AssertEqual(t, "ai-agents", in.Category)
}

// TestMCP_MCPListInput_Bad verifies MCPListInput zero value is valid input.
func TestMCP_MCPListInput_Bad(t *core.T) {
	var in subject.MCPListInput
	core.AssertEqual(t, "", in.Query)
	core.AssertEqual(t, "", in.Category)
}

// TestMCP_MCPListInput_Ugly verifies MCPListOutput fields are exported.
func TestMCP_MCPListInput_Ugly(t *core.T) {
	out := subject.MCPListOutput{
		Entries: []subject.CatalogueEntry{},
		Total:   0,
	}
	core.AssertLen(t, out.Entries, 0)
	core.AssertEqual(t, 0, out.Total)
}

// TestMCP_MCPInstallInput_Good verifies MCPInstallInput with Name + Env.
func TestMCP_MCPInstallInput_Good(t *core.T) {
	in := subject.MCPInstallInput{
		Name:      "opencode",
		Env:       map[string]string{"SERVER_PASSWORD": "secret"},
		SourceURL: "https://marketplace.lthn.ai/v1/opencode.yml",
	}
	core.AssertEqual(t, "opencode", in.Name)
	core.AssertEqual(t, "secret", in.Env["SERVER_PASSWORD"])
}

// TestMCP_MCPInstallInput_Bad verifies MCPInstallOutput zero value is safe.
func TestMCP_MCPInstallInput_Bad(t *core.T) {
	out := subject.MCPInstallOutput{}
	core.AssertEqual(t, "", out.BundleID)
	core.AssertNil(t, out.SandboxIDs)
}

// TestMCP_MCPInstallInput_Ugly verifies MCPBundleIDInput + MCPBundleIDOutput fields.
func TestMCP_MCPInstallInput_Ugly(t *core.T) {
	in := subject.MCPBundleIDInput{Name: "opencode"}
	core.AssertEqual(t, "opencode", in.Name)

	out := subject.MCPBundleIDOutput{OK: true}
	core.AssertTrue(t, out.OK)
}
