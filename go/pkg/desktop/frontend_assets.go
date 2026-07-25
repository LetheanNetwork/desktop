// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// frontendAssetHandler serves the embedded Angular filesystem in compiled
// builds and proxies it to FRONTEND_DEVSERVER_URL during Wails development.
func frontendAssetHandler(assets core.FS) core.Handler {
	return application.AssetFileServerFS(assets)
}
