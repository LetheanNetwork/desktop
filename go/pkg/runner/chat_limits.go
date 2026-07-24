// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
)

// validateChatMessages applies the Cerberus #1426 message-array caps shared by
// ChatCtx, WChat, and WChatStream. Keeping this at the common ingress boundary
// prevents provider selection or the optional welfare pass from creating an
// uncapped chat path.
func validateChatMessages(op string, messages []inference.Message) core.Result {
	if len(messages) > maxChatMessages {
		return core.Fail(core.E(op,
			core.Sprintf("message count exceeds %d (got %d)",
				maxChatMessages, len(messages)), nil))
	}

	total := 0
	for i, message := range messages {
		size := len(message.Role) + len(message.Content)
		if size > maxPromptBytes {
			return core.Fail(core.E(op,
				core.Sprintf("message[%d] size %d exceeds %d byte cap",
					i, size, maxPromptBytes), nil))
		}
		total += size
		if total > maxChatTotalBytes {
			return core.Fail(core.E(op,
				core.Sprintf("cumulative message size at index %d exceeds %d byte cap",
					i, maxChatTotalBytes), nil))
		}
	}
	return core.Ok(nil)
}
