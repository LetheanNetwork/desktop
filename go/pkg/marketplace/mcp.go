// SPDX-Licence-Identifier: EUPL-1.2

// MCP tool registrations for the marketplace service.
//
// RegisterMCPTools wires five tools into the MCP server:
//   - marketplace_list      — list catalogue entries, optionally filtered
//   - marketplace_install   — install a bundle by name
//   - marketplace_launch    — start a stopped bundle
//   - marketplace_stop      — stop a running bundle
//   - marketplace_uninstall — remove a bundle
//
// Called from pkg/desktop/subsystems.go after both mcp and marketplace
// services are confirmed alive on the Core container.
//
// Spec: plans/project/lthn/desktop/RFC.marketplace.md §10 (mcp.go row).
package marketplace

import (
	"context"

	core "dappco.re/go"
	coremcp "dappco.re/go/mcp/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpGroup = "marketplace"

// RegisterMCPTools wires the five marketplace tools into the MCP server.
// Safe to call with a nil svc or mcpSvc — missing services are no-ops.
//
// Usage example:
//
//	marketplace.RegisterMCPTools(mcpSvc, marketplaceSvc)
func RegisterMCPTools(mcpSvc *coremcp.Service, ms *Service) {
	if mcpSvc == nil || ms == nil {
		return
	}
	server := mcpSvc.Server()
	if server == nil {
		return
	}

	coremcp.AddToolRecorded(mcpSvc, server, mcpGroup, &mcp.Tool{
		Name:        "marketplace_list",
		Description: "List marketplace catalogue entries. Optionally filter by query text and/or category.",
	}, ms.mcpList)

	coremcp.AddToolRecorded(mcpSvc, server, mcpGroup, &mcp.Tool{
		Name:        "marketplace_install",
		Description: "Install a bundle from the marketplace by name. Downloads the manifest, prompts for env vars if needed, and starts the containers.",
	}, ms.mcpInstall)

	coremcp.AddToolRecorded(mcpSvc, server, mcpGroup, &mcp.Tool{
		Name:        "marketplace_launch",
		Description: "Start a bundle that is installed but currently stopped.",
	}, ms.mcpLaunch)

	coremcp.AddToolRecorded(mcpSvc, server, mcpGroup, &mcp.Tool{
		Name:        "marketplace_stop",
		Description: "Stop all running containers for an installed bundle.",
	}, ms.mcpStop)

	coremcp.AddToolRecorded(mcpSvc, server, mcpGroup, &mcp.Tool{
		Name:        "marketplace_uninstall",
		Description: "Remove a bundle. Stops containers and deletes the install record. Does not delete persistent volumes.",
	}, ms.mcpUninstall)
}

// MCPListInput is the typed input for marketplace_list.
type MCPListInput struct {
	// Query is a free-text search string matched against name/display/category/description.
	Query string `json:"query,omitempty"`
	// Category is an exact category filter (e.g. "ai-agents", "databases").
	Category string `json:"category,omitempty"`
}

// MCPListOutput is what marketplace_list returns.
type MCPListOutput struct {
	Entries []CatalogueEntry `json:"entries"`
	Total   int              `json:"total"`
}

// MCPInstallInput is the typed input for marketplace_install.
type MCPInstallInput struct {
	// Name is the bundle name (e.g. "opencode").
	Name string `json:"name"`
	// Env holds user-supplied env values for install-time prompts.
	// Keys must match the manifest env[].key field names.
	Env map[string]string `json:"env,omitempty"`
	// SourceURL overrides the catalogue source URL. When empty the
	// catalogue index is fetched first to resolve the source URL.
	SourceURL string `json:"source_url,omitempty"`
}

// MCPInstallOutput is what marketplace_install returns.
type MCPInstallOutput struct {
	BundleID   string            `json:"bundle_id"`
	SandboxIDs map[string]string `json:"sandbox_ids"`
}

// MCPBundleIDInput is the typed input for operations that take a bundle ID.
type MCPBundleIDInput struct {
	// Name is the bundle name (= manifest.Name, e.g. "opencode").
	Name string `json:"name"`
}

// MCPBundleIDOutput is the generic success response for stop/launch/uninstall.
type MCPBundleIDOutput struct {
	OK bool `json:"ok"`
}

func (s *Service) mcpList(_ context.Context, _ *mcp.CallToolRequest, input MCPListInput) (
	*mcp.CallToolResult,
	MCPListOutput,
	error,
) {
	indexR := s.FetchIndex("")
	if !indexR.OK {
		// Fall back to empty catalogue — agent gets a useful response
		// even when the remote registry is unreachable.
		return nil, MCPListOutput{Entries: []CatalogueEntry{}, Total: 0}, nil
	}
	index := indexR.Value.(FetchIndexResult)
	results := SearchCatalogue(index, input.Query, input.Category)
	return nil, MCPListOutput{Entries: results, Total: len(results)}, nil
}

func (s *Service) mcpInstall(_ context.Context, _ *mcp.CallToolRequest, input MCPInstallInput) (
	*mcp.CallToolResult,
	MCPInstallOutput,
	error,
) {
	if core.Trim(input.Name) == "" {
		return nil, MCPInstallOutput{}, mcpErr("name is required")
	}

	sourceURL := core.Trim(input.SourceURL)
	if sourceURL == "" {
		// Resolve from catalogue.
		indexR := s.FetchIndex("")
		if !indexR.OK {
			return nil, MCPInstallOutput{}, mcpErr("catalogue fetch failed: " + indexR.Error())
		}
		index := indexR.Value.(FetchIndexResult)
		results := SearchCatalogue(index, input.Name, "")
		if len(results) == 0 {
			return nil, MCPInstallOutput{}, mcpErr("bundle not found in catalogue: " + input.Name)
		}
		sourceURL = results[0].SourceURL
	}

	mR := s.FetchManifest(sourceURL)
	if !mR.OK {
		return nil, MCPInstallOutput{}, mcpErr("manifest fetch failed: " + mR.Error())
	}
	manifest := mR.Value.(BundleManifest)

	installR := s.Install(InstallInput{
		Manifest: manifest,
		Env:      input.Env,
	})
	if !installR.OK {
		return nil, MCPInstallOutput{}, mcpErr("install failed: " + installR.Error())
	}
	out := installR.Value.(InstallOutput)

	return nil, MCPInstallOutput{
		BundleID:   out.BundleID,
		SandboxIDs: out.SandboxIDs,
	}, nil
}

func (s *Service) mcpLaunch(_ context.Context, _ *mcp.CallToolRequest, input MCPBundleIDInput) (
	*mcp.CallToolResult,
	MCPBundleIDOutput,
	error,
) {
	if core.Trim(input.Name) == "" {
		return nil, MCPBundleIDOutput{}, mcpErr("name is required")
	}
	r := s.Launch(input.Name)
	if !r.OK {
		return nil, MCPBundleIDOutput{}, mcpErr(r.Error())
	}
	return nil, MCPBundleIDOutput{OK: true}, nil
}

func (s *Service) mcpStop(_ context.Context, _ *mcp.CallToolRequest, input MCPBundleIDInput) (
	*mcp.CallToolResult,
	MCPBundleIDOutput,
	error,
) {
	if core.Trim(input.Name) == "" {
		return nil, MCPBundleIDOutput{}, mcpErr("name is required")
	}
	r := s.Stop(input.Name)
	if !r.OK {
		return nil, MCPBundleIDOutput{}, mcpErr(r.Error())
	}
	return nil, MCPBundleIDOutput{OK: true}, nil
}

func (s *Service) mcpUninstall(_ context.Context, _ *mcp.CallToolRequest, input MCPBundleIDInput) (
	*mcp.CallToolResult,
	MCPBundleIDOutput,
	error,
) {
	if core.Trim(input.Name) == "" {
		return nil, MCPBundleIDOutput{}, mcpErr("name is required")
	}
	r := s.Uninstall(input.Name)
	if !r.OK {
		return nil, MCPBundleIDOutput{}, mcpErr(r.Error())
	}
	return nil, MCPBundleIDOutput{OK: true}, nil
}

// mcpErr wraps a string error for MCP handler return.
func mcpErr(msg string) error {
	return core.E("marketplace.mcp", msg, nil)
}
