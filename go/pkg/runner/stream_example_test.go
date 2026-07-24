// SPDX-Licence-Identifier: EUPL-1.2

package runner_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runner"
)

func ExampleService_WChatStream() {
	ref := (*subject.Service).WChatStream
	_ = core.Sprintf("%T", ref)
}
