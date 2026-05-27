// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/downloader"
	"dappco.re/lthn/desktop/pkg/models"
)

// cmdModels dispatches `lthn models <verb>` — local model snapshot
// management. Today supports `list` (alias `ls`) and `pull URL NAME`
// via pkg/downloader. `remove` is on the roadmap once the deletion
// flow gets the same digest-verification gate that pull has.
//
// Usage example:
//
//	rc := cmdModels([]string{"list"})
//	rc := cmdModels([]string{"pull", "https://example.com/m.gguf", "my-model"})
func cmdModels(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn models: missing verb (list / pull)\n")
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return modelsList(args[1:])
	case "pull":
		return modelsPull(args[1:])
	default:
		core.Print(core.Stderr(), "lthn models: unknown verb %q\n", args[0])
		return 2
	}
}

func modelsPull(args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn models pull: usage: lthn models pull URL NAME\n")
		return 2
	}
	r := downloader.Fetch(args[0], args[1])
	if !r.OK {
		core.Print(core.Stderr(), "lthn models pull: %s\n", r.Error())
		return 1
	}
	if s, ok := r.Value.(string); ok {
		core.Println(s)
	}
	return 0
}

func modelsList(_ []string) int {
	r := models.List()
	if !r.OK {
		core.Print(core.Stderr(), "lthn models list: %s\n", r.Error())
		return 1
	}
	jr := core.JSONMarshalIndent(r.Value, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn models list: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}
