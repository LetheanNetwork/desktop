// SPDX-Licence-Identifier: EUPL-1.2

package api

import (
	"context"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
)

// SpecInfo describes the OpenAPI document we generate from registered
// RouteGroups. Static today — bumped manually when the API surface
// breaks compatibility.
type SpecInfo struct {
	Title       string
	Version     string
	Description string
}

// DefaultSpecInfo returns the canonical metadata used by `lthn api
// spec`. The npm package name is fixed at @lthn/api so generated SDKs
// stay consistent across releases.
//
// Usage example:
//
//	info := api.DefaultSpecInfo()
//	if r := api.ExportSpec(c, "yaml", "/tmp/openapi.yaml", info); !r.OK {
//	    return r
//	}
func DefaultSpecInfo() SpecInfo {
	return SpecInfo{
		Title:       "lthn API",
		Version:     "0.1.0",
		Description: "Lethean Desktop HTTP gateway — same surface the WebView reaches via Wails bindings, exposed to external clients and the plugin market.",
	}
}

// ExportSpec writes an OpenAPI document describing every RouteGroup
// registered on the api Service to path. format must be "yaml" or
// "json".
//
// Caller MUST have called api.Register(c, runner) first; an empty
// Engine yields an empty (but valid) spec.
//
// Usage example:
//
//	if r := api.ExportSpec(c, "yaml", "openapi.yaml", api.DefaultSpecInfo()); !r.OK {
//	    return r
//	}
func ExportSpec(c *core.Core, format, path string, info SpecInfo) core.Result {
	if c == nil {
		return core.Fail(core.E("api.ExportSpec", "core is nil", nil))
	}
	if path == "" {
		return core.Fail(core.E("api.ExportSpec", "path is required", nil))
	}
	if format != "yaml" && format != "json" {
		return core.Fail(core.E("api.ExportSpec", "format must be yaml or json", nil))
	}

	apiSvc, ok := core.ServiceFor[*coreapi.Service](c, "api")
	if !ok || apiSvc == nil || apiSvc.Engine == nil {
		return core.Fail(core.E("api.ExportSpec", "api service not registered on core", nil))
	}

	builder := &coreapi.SpecBuilder{
		Title:       info.Title,
		Version:     info.Version,
		Description: info.Description,
	}

	if err := coreapi.ExportSpecToFile(path, format, builder, apiSvc.Engine.Groups()); err != nil {
		return core.Fail(core.E("api.ExportSpec", "export failed", err))
	}
	return core.Ok(path)
}

// GenerateSDK runs openapi-generator-cli over the spec at specPath and
// writes the result under outputDir/<language>/. Requires the
// openapi-generator-cli binary on PATH — installed via `npm i -g
// @openapitools/openapi-generator-cli` or `brew install
// openapi-generator`.
//
// Supported languages come from coreapi.SupportedLanguages() —
// typescript-fetch is the default lane for @lthn/api on npm.
//
// Usage example:
//
//	if r := api.GenerateSDK(context.Background(), "openapi.yaml", "build/sdk", "typescript-fetch", "lthn-api"); !r.OK {
//	    return r
//	}
func GenerateSDK(ctx context.Context, specPath, outputDir, language, packageName string) core.Result {
	if specPath == "" {
		return core.Fail(core.E("api.GenerateSDK", "specPath is required", nil))
	}
	if outputDir == "" {
		return core.Fail(core.E("api.GenerateSDK", "outputDir is required", nil))
	}
	if language == "" {
		return core.Fail(core.E("api.GenerateSDK", "language is required", nil))
	}

	gen := &coreapi.SDKGenerator{
		SpecPath:    specPath,
		OutputDir:   outputDir,
		PackageName: packageName,
	}
	if !gen.Available() {
		return core.Fail(core.E("api.GenerateSDK",
			"openapi-generator-cli not on PATH — install via `npm i -g @openapitools/openapi-generator-cli`",
			nil))
	}
	if err := gen.Generate(ctx, language); err != nil {
		return core.Fail(core.E("api.GenerateSDK", "generation failed", err))
	}
	return core.Ok(outputDir)
}
