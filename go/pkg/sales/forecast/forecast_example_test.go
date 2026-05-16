// SPDX-Licence-Identifier: EUPL-1.2

package forecast_test

import (
	"dappco.re/lthn/desktop/pkg/sales/forecast"
)

// ExampleService_Quarterly shows the standard quarterly forecast invocation.
func ExampleService_Quarterly() {
	svc := forecast.NewService(nil)
	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
	if !r.OK {
		return
	}
	_ = r.Value.(forecast.QuarterlyOutput)
}
