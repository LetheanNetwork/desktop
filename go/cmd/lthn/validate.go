// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/validator"
)

// cmdValidate handles `lthn validate URL` — probes a remote
// OpenAI-compatible endpoint and prints the result envelope. Used
// by the setup wizard to confirm a base_url before persisting it.
//
// Usage example:
//
//	rc := cmdValidate([]string{"http://localhost:11434/v1"})
func cmdValidate(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn validate: usage: lthn validate URL\n")
		return 2
	}
	r := validator.Endpoint(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn validate: %s\n", r.Error())
		return 1
	}
	info, ok := r.Value.(validator.EndpointInfo)
	if !ok {
		core.Print(core.Stderr(), "lthn validate: unexpected value type\n")
		return 1
	}
	jr := core.JSONMarshalIndent(info, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn validate: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	if !info.OK {
		return 1
	}
	return 0
}
