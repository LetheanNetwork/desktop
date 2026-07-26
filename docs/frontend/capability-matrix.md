# Frontend Capability Matrix

> Generated source evidence. Source state is not runtime maturity; promote a row to live only in an app-family plan with a passing runtime smoke.

| Route | Component | Declared contract | Source state | Evidence | Limitation |
|---|---|---|---|---|---|
| /agents/activity | frontend-ng/src/app/desktop/surfaces/agents/activity.ts | dappco.re/lthn/desktop/pkg/agents.Service.Workspaces | integrated | go/pkg/agents/cli.go#Service.Workspaces | Runtime path not certified by this source audit. |
| /agents/code | frontend-ng/src/app/desktop/surfaces/agents/code.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /agents/connect | frontend-ng/src/app/desktop/surfaces/agents/connect.ts | dappco.re/lthn/desktop/pkg/agents.Service.ClaudeConnectRecipe | integrated | go/pkg/agents/connect.go#Service.ClaudeConnectRecipe | Runtime path not certified by this source audit. |
| /agents/dispatch | frontend-ng/src/app/desktop/surfaces/agents/dispatch.ts | dappco.re/lthn/desktop/pkg/agents.Service.Dispatch | integrated | go/pkg/agents/cli.go#Service.Dispatch | Runtime path not certified by this source audit. |
| /agents/flows | frontend-ng/src/app/desktop/surfaces/agents/flows.ts | dappco.re/lthn/desktop/pkg/tools.WailsService.List | unresolved | component/route only | Runtime path not certified by this source audit. |
| /agents/scan | frontend-ng/src/app/desktop/surfaces/agents/scan.ts | dappco.re/lthn/desktop/pkg/agents.Service.Scan | integrated | go/pkg/agents/cli.go#Service.Scan | Runtime path not certified by this source audit. |
| /agents/tasks | frontend-ng/src/app/desktop/surfaces/agents/tasks.ts | dappco.re/lthn/desktop/pkg/agents.Service.Tasks | integrated | go/pkg/agents/cli.go#Service.Tasks | Runtime path not certified by this source audit. |
| /agents/terminal | frontend-ng/src/app/desktop/surfaces/agents/terminal.ts | none declared | integrated | frontend-ng/src/app/desktop/surfaces/agents/terminal-session.ts | Runtime path not certified by this source audit. |
| /agents/workspaces | frontend-ng/src/app/desktop/surfaces/agents/workspaces.ts | dappco.re/lthn/desktop/pkg/agents.Service.Prep | integrated | go/pkg/agents/cli.go#Service.Prep | Runtime path not certified by this source audit. |
| /ai/chat | frontend-ng/src/app/desktop/apps/chat.app.ts | none declared | integrated | frontend-ng/src/app/desktop/desktop-ai.service.ts | Runtime path not certified by this source audit. |
| /coding/deploys | frontend-ng/src/app/desktop/surfaces/coding/deploys.ts | dappco.re/lthn/desktop/pkg/deploys.Service.List | integrated | go/pkg/deploys/wails.go#Service.List | Runtime path not certified by this source audit. |
| /coding/issues | frontend-ng/src/app/desktop/surfaces/coding/issues.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /coding/prs | frontend-ng/src/app/desktop/surfaces/coding/prs.ts | dappco.re/lthn/desktop/pkg/vi.Service.Activity | integrated | go/pkg/vi/service.go#Service.Activity | Runtime path not certified by this source audit. |
| /coding/repos | frontend-ng/src/app/desktop/surfaces/coding/repos.ts | dappco.re/lthn/desktop/pkg/repos.Service.Status | integrated | go/pkg/repos/wails.go#Service.Status | Runtime path not certified by this source audit. |
| /developer/build | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/containers | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/control-panel | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/devops | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/explorer | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/forge | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/git | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/marketplace | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/process | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/repos | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/search | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /extensions/marketplace | frontend-ng/src/app/desktop/surfaces/extensions/marketplace.ts | none declared | integrated | frontend-ng/src/app/desktop/surfaces/extensions/marketplace.ts | Runtime path not certified by this source audit. |
| /extensions/opencode-shim | frontend-ng/src/app/desktop/surfaces/extensions/opencode-shim.ts | none declared | integrated | frontend-ng/src/app/desktop/surfaces/extensions/opencode-shim.ts | Runtime path not certified by this source audit. |
| /extensions/plugin-view | frontend-ng/src/app/desktop/surfaces/extensions/plugin-view.ts | none declared | integrated | frontend-ng/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts | Runtime path not certified by this source audit. |
| /marketing/analytics | frontend-ng/src/app/desktop/surfaces/marketing/analytics.ts | dappco.re/lthn/desktop/pkg/marketing/analytics.Service.Get | integrated | go/pkg/marketing/analytics/wails.go#Service.Get | Runtime path not certified by this source audit. |
| /marketing/audience | frontend-ng/src/app/desktop/surfaces/marketing/audience.ts | dappco.re/lthn/desktop/pkg/marketing/audience.Service.List | integrated | go/pkg/marketing/audience/wails.go#Service.List | Runtime path not certified by this source audit. |
| /marketing/campaigns | frontend-ng/src/app/desktop/surfaces/marketing/campaigns.ts | dappco.re/lthn/desktop/pkg/marketing/campaigns.Service.List | integrated | go/pkg/marketing/campaigns/wails.go#Service.List | Runtime path not certified by this source audit. |
| /marketing/content | frontend-ng/src/app/desktop/surfaces/marketing/content.ts | dappco.re/lthn/desktop/pkg/marketing/content.Service.List | integrated | go/pkg/marketing/content/wails.go#Service.List | Runtime path not certified by this source audit. |
| /marketing/social | frontend-ng/src/app/desktop/surfaces/marketing/social.ts | dappco.re/lthn/desktop/pkg/marketing/social.Service.List | integrated | go/pkg/marketing/social/wails.go#Service.List | Runtime path not certified by this source audit. |
| /media/games | frontend-ng/src/app/desktop/apps/games.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /ml-lab/duckdb | frontend-ng/src/app/desktop/surfaces/ml-lab/duckdb.ts | /v1/ml-lab/duckdb | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ml-lab/influx | frontend-ng/src/app/desktop/surfaces/ml-lab/influx.ts | /v1/ml-lab/influx | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ml-lab/lora | frontend-ng/src/app/desktop/surfaces/ml-lab/lora.ts | /v1/ml-lab/runs | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ml-lab/ml-lab | frontend-ng/src/app/desktop/surfaces/ml-lab/ml-lab.ts | /v1/ml-lab/ask | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ml-lab/models | frontend-ng/src/app/desktop/surfaces/ml-lab/models.ts | /v1/ml-lab/models | unresolved | component/route only | Runtime path not certified by this source audit. |
| /networking/lethernet | frontend-ng/src/app/desktop/apps/lethernet.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /observe/activity | frontend-ng/src/app/desktop/surfaces/observe/activity.ts | /v1/audit/events?limit=250 | unresolved | component/route only | Runtime path not certified by this source audit. |
| /office/documents | frontend-ng/src/app/desktop/surfaces/office/documents.ts | dappco.re/lthn/desktop/pkg/office/documents.Service.List | integrated | go/pkg/office/documents/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/files | frontend-ng/src/app/desktop/surfaces/office/files.ts | none declared | integrated | frontend-ng/src/app/desktop/apps/files.app.ts<br>frontend-ng/src/app/desktop/desktop-files-bridge.service.ts | Runtime path not certified by this source audit. |
| /office/mail | frontend-ng/src/app/desktop/surfaces/office/mail.ts | dappco.re/lthn/desktop/pkg/office/mail.Service.ListThreads | integrated | go/pkg/office/mail/wails.go#Service.ListThreads | Runtime path not certified by this source audit. |
| /office/tasks | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/tenant | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /operations/incidents | frontend-ng/src/app/desktop/surfaces/operations/incidents.ts | dappco.re/lthn/desktop/pkg/incidents.Service.List | integrated | go/pkg/incidents/wails.go#Service.List | Runtime path not certified by this source audit. |
| /operations/runbooks | frontend-ng/src/app/desktop/surfaces/operations/runbooks.ts | dappco.re/lthn/desktop/pkg/runbooks.Service.List | integrated | go/pkg/runbooks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /operations/status | frontend-ng/src/app/desktop/surfaces/operations/status.ts | dappco.re/lthn/desktop/pkg/vi.Service.Sites | integrated | go/pkg/vi/service.go#Service.Sites | Runtime path not certified by this source audit. |
| /planning/backlog | frontend-ng/src/app/desktop/surfaces/planning/backlog.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /planning/calendar | frontend-ng/src/app/desktop/surfaces/planning/calendar.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /planning/retros | frontend-ng/src/app/desktop/surfaces/planning/retros.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /planning/roadmap | frontend-ng/src/app/desktop/surfaces/planning/roadmap.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /planning/sprints | frontend-ng/src/app/desktop/surfaces/planning/sprints.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /planning/today | frontend-ng/src/app/desktop/surfaces/planning/today.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /sales/contacts | frontend-ng/src/app/desktop/surfaces/sales/contacts.ts | dappco.re/lthn/desktop/pkg/sales/contacts.Service.List | integrated | go/pkg/sales/contacts/wails.go#Service.List | Runtime path not certified by this source audit. |
| /sales/deals | frontend-ng/src/app/desktop/surfaces/sales/deals.ts | dappco.re/lthn/desktop/pkg/sales/deals.Service.List | integrated | go/pkg/sales/deals/wails.go#Service.List | Runtime path not certified by this source audit. |
| /sales/forecast | frontend-ng/src/app/desktop/surfaces/sales/forecast.ts | dappco.re/lthn/desktop/pkg/sales/forecast.Service.Quarterly | integrated | go/pkg/sales/forecast/wails.go#Service.Quarterly | Runtime path not certified by this source audit. |
| /sales/pipeline | frontend-ng/src/app/desktop/surfaces/sales/pipeline.ts | dappco.re/lthn/desktop/pkg/sales/pipeline.Service.List | integrated | go/pkg/sales/pipeline/wails.go#Service.List | Runtime path not certified by this source audit. |
| /system/activity | frontend-ng/src/app/desktop/apps/activity.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/control | frontend-ng/src/app/desktop/apps/control.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/settings | frontend-ng/src/app/desktop/apps/settings.app.ts | none declared | integrated | frontend-ng/src/app/desktop/preferences.service.ts | Runtime path not certified by this source audit. |
| /system/telemetry | frontend-ng/src/app/desktop/apps/telemetry.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /tools/files | frontend-ng/src/app/desktop/apps/files.app.ts | none declared | integrated | frontend-ng/src/app/desktop/desktop-files-bridge.service.ts | Runtime path not certified by this source audit. |
| /tools/notepad | frontend-ng/src/app/desktop/apps/notepad.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /tools/terminal | frontend-ng/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
