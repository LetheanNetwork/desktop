// SPDX-Licence-Identifier: EUPL-1.2

package bridge_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/bridge"
)

func ExampleRegisterWebMCPTools() {
	ref := subject.RegisterWebMCPTools
	_ = core.Sprintf("%T", ref)
}
