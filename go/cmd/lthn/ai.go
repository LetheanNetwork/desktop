// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/runner"
)

// aiChat handles `lthn ai chat MESSAGE`. Sends a single user message
// to the runner and prints the assistant reply. One-shot — no REPL
// loop today; interactive chat is a window-level concern.
//
// Usage example:
//
//	rc := aiChat([]string{"hello"})
func aiChat(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn ai chat: usage: lthn ai chat MESSAGE\n")
		return 2
	}
	r := newRunner()
	reply := r.Chat([]inference.Message{
		{Role: "user", Content: args[0]},
	})
	return printReply(reply, "ai chat")
}

// aiGenerate handles `lthn ai generate PROMPT`. One-shot completion.
//
// Usage example:
//
//	rc := aiGenerate([]string{"write a haiku"})
func aiGenerate(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn ai generate: usage: lthn ai generate PROMPT\n")
		return 2
	}
	r := newRunner()
	reply := r.Generate(args[0])
	return printReply(reply, "ai generate")
}

// aiModels handles `lthn ai models`. Lists the configured routes.
//
// Usage example:
//
//	rc := aiModels(nil)
func aiModels(args []string) int {
	r := newRunner()
	models := r.Models()
	if !models.OK {
		core.Print(core.Stderr(), "lthn ai models: %s\n", models.Error())
		return 1
	}
	ids, ok := models.Value.([]string)
	if !ok {
		core.Print(core.Stderr(), "lthn ai models: unexpected value type\n")
		return 1
	}
	if len(ids) == 0 {
		core.Println("(no routes configured — run `lthn config set routes.NAME.kind openai` etc.)")
		return 0
	}
	for _, id := range ids {
		core.Println(id)
	}
	return 0
}

// newRunner constructs the talk-surface runner with routes loaded
// from ~/Lethean/conf/lthn.yaml via the config service. Routes are
// dotted-key entries under `routes.<name>.{kind,base_url,api_key,model}`.
// When no routes are configured the runner serves the echo stub.
func newRunner() *runner.Service {
	c := newAppCore()
	if c == nil {
		return runner.NewService(runner.Options{})
	}
	return runner.NewServiceFromCore(c)
}

// printReply formats a Result from runner.Generate / Chat to stdout
// (string assistant reply) or stderr (failure with op prefix).
func printReply(r core.Result, op string) int {
	if !r.OK {
		core.Print(core.Stderr(), "lthn %s: %s\n", op, r.Error())
		return 1
	}
	if s, ok := r.Value.(string); ok {
		core.Println(s)
	}
	return 0
}
