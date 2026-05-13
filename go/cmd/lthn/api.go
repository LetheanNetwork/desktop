// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	"context"

	core "dappco.re/go"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/runner"
)

// cmdAPI handles `lthn api <verb> [args...]`. Sub-verbs:
//
//	spec [--format yaml|json] [--out PATH]
//	  Build the OpenAPI 3.1 spec describing every registered
//	  RouteGroup. Default format: yaml. Default out:
//	  ./openapi.yaml (or .json).
//
//	sdk <language> [--out DIR] [--package NAME] [--spec PATH]
//	  Run openapi-generator-cli to produce an SDK in <language>
//	  under <out>/<language>/. Default out: ./build/sdk.
//	  Default package name: lthn-api. When --spec is omitted the
//	  CLI generates a fresh openapi.yaml in a temp dir first.
//
// Usage example:
//
//	lthn api spec --format yaml --out openapi.yaml
//	lthn api sdk typescript-fetch --out build/sdk --package lthn-api
func cmdAPI(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn api: missing verb (spec / sdk)\n")
		core.Print(core.Stderr(), "run `lthn help api` for usage\n")
		return 2
	}
	switch args[0] {
	case "spec":
		return apiSpec(args[1:])
	case "sdk":
		return apiSDK(args[1:])
	default:
		core.Print(core.Stderr(), "lthn api: unknown verb %q (try: spec, sdk)\n", args[0])
		return 2
	}
}

// apiSpec handles `lthn api spec [--format ...] [--out ...]`.
func apiSpec(args []string) int {
	format := "yaml"
	out := ""
	for k, v := range apiFlagValues(args, map[string]bool{"format": true, "out": true}) {
		switch k {
		case "format":
			format = v
		case "out":
			out = v
		}
	}
	if format != "yaml" && format != "json" {
		core.Print(core.Stderr(), "lthn api spec: --format must be yaml or json\n")
		return 2
	}
	if out == "" {
		out = defaultSpecOutput(format)
	}

	c, runner := bootAPICore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	if r := lthnapi.Register(c, runner); !r.OK {
		core.Print(core.Stderr(), "lthn api spec: register groups: %s\n", r.Error())
		return 1
	}

	r := lthnapi.ExportSpec(c, format, out, lthnapi.DefaultSpecInfo())
	if !r.OK {
		core.Print(core.Stderr(), "lthn api spec: %s\n", r.Error())
		return 1
	}
	core.Println("wrote", out)
	return 0
}

// apiSDK handles `lthn api sdk LANGUAGE [--out DIR] [--package NAME]
// [--spec PATH]`.
func apiSDK(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn api sdk: missing language (try: typescript-fetch)\n")
		return 2
	}
	language := args[0]
	rest := args[1:]

	out := "build/sdk"
	pkg := "lthn-api"
	specPath := ""
	for k, v := range apiFlagValues(rest, map[string]bool{"out": true, "package": true, "spec": true}) {
		switch k {
		case "out":
			out = v
		case "package":
			pkg = v
		case "spec":
			specPath = v
		}
	}

	c, runner := bootAPICore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	if r := lthnapi.Register(c, runner); !r.OK {
		core.Print(core.Stderr(), "lthn api sdk: register groups: %s\n", r.Error())
		return 1
	}

	// Materialise a spec if the caller didn't hand us one. Lands in
	// the same directory the SDK output ends up under so subsequent
	// runs can be reproduced without re-invoking spec.
	if specPath == "" {
		tmp := c.Fs().TempDir("lthn-api-")
		specPath = core.Path(tmp, "openapi.yaml")
		if r := lthnapi.ExportSpec(c, "yaml", specPath, lthnapi.DefaultSpecInfo()); !r.OK {
			core.Print(core.Stderr(), "lthn api sdk: build spec: %s\n", r.Error())
			return 1
		}
	}

	if r := lthnapi.GenerateSDK(context.Background(), specPath, out, language, pkg); !r.OK {
		core.Print(core.Stderr(), "lthn api sdk: %s\n", r.Error())
		return 1
	}
	core.Println("generated", language, "SDK at", out)
	return 0
}

func apiFlagValues(args []string, allowed map[string]bool) map[string]string {
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid || !allowed[k] {
			continue
		}
		if v == "" && i+1 < len(args) {
			i++
			v = args[i]
		}
		values[k] = v
	}
	return values
}

func defaultSpecOutput(format string) string {
	if format == "json" {
		return "openapi.json"
	}
	return "openapi.yaml"
}

// bootAPICore builds the *core.Core and a runner.Service the API
// verbs need to register RouteGroups. Returns (nil, nil) on failure
// after printing the error to stderr — caller should return 1.
func bootAPICore() (*core.Core, *runner.Service) {
	c := newAppCore()
	if c == nil {
		return nil, nil
	}
	r := runner.NewServiceFromCore(c)
	return c, r
}
