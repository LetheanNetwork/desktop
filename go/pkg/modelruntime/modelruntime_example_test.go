// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime_test

import (
	"fmt"

	"dappco.re/lthn/desktop/pkg/modelruntime"
)

func ExampleNewService() {
	runtime := modelruntime.NewService(modelruntime.Options{})
	result := runtime.Snapshot()
	fmt.Println(modelruntime.ErrorCodeOf(result))
	// Output: runtime_unavailable
}

func ExampleNewWailsService() {
	binding := modelruntime.NewWailsService(nil)
	fmt.Println(binding.ServiceName())
	// Output: ModelRuntime
}
