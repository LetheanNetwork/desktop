// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func ExampleOptions() {
	options := appconfig.Options{}
	core.Println(options.Core == nil)
	// Output: true
}

func ExampleService() {
	service := appconfig.NewService(appconfig.Options{})
	core.Println(service != nil)
	// Output: true
}

func ExampleNewService() {
	service := appconfig.NewService(appconfig.Options{})
	core.Println(service != nil)
	// Output: true
}

func ExampleRegister() {
	result := appconfig.Register(nil)
	core.Println(result.OK)
	// Output: false
}

func ExampleService_Settings() {
	var method func(*appconfig.Service) core.Result = (*appconfig.Service).Settings
	core.Println(method != nil)
	// Output: true
}

func ExampleService_Set() {
	var method func(*appconfig.Service, string, any) core.Result = (*appconfig.Service).Set
	core.Println(method != nil)
	// Output: true
}

func ExampleService_SetMany() {
	var method func(*appconfig.Service, []appconfig.Change) core.Result = (*appconfig.Service).SetMany
	core.Println(method != nil)
	// Output: true
}
