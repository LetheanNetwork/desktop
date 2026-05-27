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
// Sub-verbs (`ls`, `pull NAME`) were promised by older help text but
// were never wired here — `ls` was a duplicate of the flat list and
// `pull` belongs to the Lemma admin download surface, not the CLI.
// When a sub-verb is passed today, the user gets a clear hint to the
// real surface rather than a silent ignore.
//
// Usage example:
//
//	rc := aiModels(nil)
func aiModels(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "ls":
			// Legacy verb — flat list is what this command already does;
			// keep going so existing scripts still work, just print the
			// note so the next reader sees the new shape.
			core.Print(core.Stderr(),
				"lthn ai models: `ls` is implicit — `lthn ai models` already lists.\n")
		case "pull":
			repo := ""
			if len(args) > 1 {
				repo = args[1]
			}
			core.Print(core.Stderr(),
				"lthn ai models pull: download lives in Lemma admin — run `lthn gui` and\n  open the Lemma window's Download form%s.\n",
				func() string {
					if repo != "" {
						return " (HF repo: " + repo + ")"
					}
					return ""
				}())
			return 2
		default:
			core.Print(core.Stderr(),
				"lthn ai models: unknown verb %q — `lthn ai models` lists configured routes\n",
				args[0])
			return 2
		}
	}
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

// newRunner returns the Core-registered runner Service. The runner is
// constructed in app.go via core.WithName("runner", runner.Register)
// which reads routes from ~/Lethean/conf/lthn.yaml via the config
// service. Falls back to an empty Service if Core boot fails (echo
// stub behaviour).
func newRunner() *runner.Service {
	c := newAppCore()
	if c == nil {
		return runner.NewService(runner.Options{})
	}
	r, _ := core.ServiceFor[*runner.Service](c, "runner")
	if r == nil {
		return runner.NewService(runner.Options{})
	}
	return r
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
