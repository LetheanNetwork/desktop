// SPDX-Licence-Identifier: EUPL-1.2

package main

import core "dappco.re/go"

func readPackagingFixture(t *core.T, path string) string {
	t.Helper()
	result := core.ReadFile(path)
	core.AssertTrue(t, result.OK)
	body, ok := result.Value.([]byte)
	core.AssertTrue(t, ok)
	return string(body)
}

func TestWailsPackagingConfig_Good_CompleteDesktopMetadata(t *core.T) {
	config := readPackagingFixture(t, "../../../build/config.yml")

	for _, expected := range []string{
		`cfBundleIconName: "appicon"`,
		`minIOSVersion: "15.0"`,
		"backgroundModes: []",
		"ext: lthn",
		"name: Lethean Desktop Configuration",
		"description: Lethean Desktop configuration file",
		"iconName: icons",
		"role: Editor",
		"mimeType: application/x-lethean",
		"scheme: lthn",
	} {
		core.AssertContains(t, config, expected)
	}
}

func TestWailsPackagingConfig_Bad_PlaceholderMetadataRemoved(t *core.T) {
	config := readPackagingFixture(t, "../../../build/config.yml")

	core.AssertFalse(t, core.Contains(config, "My Other Data"))
	core.AssertFalse(t, core.Contains(config, "fileAssociations: []"))
}

func TestWailsPackagingConfig_Ugly_WindowsInputsAreExplicit(t *core.T) {
	taskfile := readPackagingFixture(t, "../../../build/windows/Taskfile.yml")

	core.AssertContains(t, taskfile, `PUBLISHER: "CN=Lethean"`)
	core.AssertContains(t, taskfile, `MSIX_PROCESSOR_ARCHITECTURE: "x64"`)
	core.AssertContains(t, taskfile, `CERT_PATH: ""`)
	core.AssertContains(t, taskfile, `SIGN_CERTIFICATE: ""`)
}
