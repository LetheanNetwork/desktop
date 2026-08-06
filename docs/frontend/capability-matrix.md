# Frontend Capability Matrix

> Generated source evidence. Source state is not runtime maturity; promote a row to live only in an app-family plan with a passing runtime smoke.

| Route | Component | Declared contract | Source state | Evidence | Limitation |
|---|---|---|---|---|---|
| /ai/agents | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /ai/agents/activity | frontend/src/app/desktop/surfaces/agents/activity.ts | dappco.re/lthn/desktop/pkg/agents.Service.Workspaces | integrated | go/pkg/agents/cli.go#Service.Workspaces | Runtime path not certified by this source audit. |
| /ai/agents/code | frontend/src/app/desktop/surfaces/agents/code.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /ai/agents/connect | frontend/src/app/desktop/surfaces/agents/connect.ts | dappco.re/lthn/desktop/pkg/agents.Service.ClaudeConnectRecipe | integrated | go/pkg/agents/connect.go#Service.ClaudeConnectRecipe | Runtime path not certified by this source audit. |
| /ai/agents/dispatch | frontend/src/app/desktop/surfaces/agents/dispatch.ts | dappco.re/lthn/desktop/pkg/agents.Service.Dispatch | integrated | go/pkg/agents/cli.go#Service.Dispatch | Runtime path not certified by this source audit. |
| /ai/agents/flows | frontend/src/app/desktop/surfaces/agents/flows.ts | dappco.re/lthn/desktop/pkg/tools.WailsService.List | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ai/agents/scan | frontend/src/app/desktop/surfaces/agents/scan.ts | dappco.re/lthn/desktop/pkg/agents.Service.Scan | integrated | go/pkg/agents/cli.go#Service.Scan | Runtime path not certified by this source audit. |
| /ai/agents/tasks | frontend/src/app/desktop/surfaces/agents/tasks.ts | dappco.re/lthn/desktop/pkg/agents.Service.Tasks | integrated | go/pkg/agents/cli.go#Service.Tasks | Runtime path not certified by this source audit. |
| /ai/agents/terminal | frontend/src/app/desktop/surfaces/agents/terminal.ts | none declared | integrated | frontend/src/app/desktop/surfaces/agents/terminal-session.ts | Runtime path not certified by this source audit. |
| /ai/agents/workspaces | frontend/src/app/desktop/surfaces/agents/workspaces.ts | dappco.re/lthn/desktop/pkg/agents.Service.Prep | integrated | go/pkg/agents/cli.go#Service.Prep | Runtime path not certified by this source audit. |
| /ai/chat | frontend/src/app/desktop/apps/chat.app.ts | none declared | integrated | frontend/src/app/desktop/desktop-ai.service.ts | Runtime path not certified by this source audit. |
| /ai/ml-lab | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /ai/ml-lab/lora | frontend/src/app/desktop/surfaces/ml-lab/lora.ts | /v1/ml-lab/runs | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ai/ml-lab/models | frontend/src/app/desktop/surfaces/ml-lab/models.ts | /v1/ml-lab/models | unresolved | component/route only | Runtime path not certified by this source audit. |
| /ai/ml-lab/workbench | frontend/src/app/desktop/surfaces/ml-lab/ml-lab.ts | /v1/ml-lab/ask | unresolved | component/route only | Runtime path not certified by this source audit. |
| /developer/databases | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/databases/duckdb | frontend/src/app/desktop/surfaces/ml-lab/duckdb.ts | /v1/ml-lab/duckdb | unresolved | component/route only | Runtime path not certified by this source audit. |
| /developer/databases/influxdb | frontend/src/app/desktop/surfaces/ml-lab/influx.ts | /v1/ml-lab/influx | unresolved | component/route only | Runtime path not certified by this source audit. |
| /developer/ide | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/build | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/containers | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/control-panel | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/devops | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/explorer | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/forge | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/git | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/processes | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/search | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/ide/terminal | frontend/src/app/desktop/apps/terminal.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/scm | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /developer/scm/deploys | frontend/src/app/desktop/surfaces/coding/deploys.ts | dappco.re/lthn/desktop/pkg/deploys.Service.List | integrated | go/pkg/deploys/wails.go#Service.List | Runtime path not certified by this source audit. |
| /developer/scm/pull-requests | frontend/src/app/desktop/surfaces/coding/prs.ts | dappco.re/lthn/desktop/pkg/vi.Service.Activity | integrated | go/pkg/vi/service.go#Service.Activity | Runtime path not certified by this source audit. |
| /developer/scm/repositories | frontend/src/app/desktop/surfaces/coding/repos.ts | dappco.re/lthn/desktop/pkg/repos.Service.Status | integrated | go/pkg/repos/wails.go#Service.Status | Runtime path not certified by this source audit. |
| /media/games | frontend/src/app/desktop/apps/games.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /networking/lethernet | frontend/src/app/desktop/apps/lethernet.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/crm | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/crm/contacts | frontend/src/app/desktop/surfaces/sales/contacts.ts | dappco.re/lthn/desktop/pkg/sales/contacts.Service.List | integrated | go/pkg/sales/contacts/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/crm/deals | frontend/src/app/desktop/surfaces/sales/deals.ts | dappco.re/lthn/desktop/pkg/sales/deals.Service.List | integrated | go/pkg/sales/deals/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/crm/forecast | frontend/src/app/desktop/surfaces/sales/forecast.ts | dappco.re/lthn/desktop/pkg/sales/forecast.Service.Quarterly | integrated | go/pkg/sales/forecast/wails.go#Service.Quarterly | Runtime path not certified by this source audit. |
| /office/crm/pipeline | frontend/src/app/desktop/surfaces/sales/pipeline.ts | dappco.re/lthn/desktop/pkg/sales/pipeline.Service.List | integrated | go/pkg/sales/pipeline/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/documents | frontend/src/app/desktop/surfaces/office/documents.ts | dappco.re/lthn/desktop/pkg/office/documents.Service.List | integrated | go/pkg/office/documents/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/mail | frontend/src/app/desktop/surfaces/office/mail.ts | dappco.re/lthn/desktop/pkg/office/mail.Service.ListThreads | integrated | go/pkg/office/mail/wails.go#Service.ListThreads | Runtime path not certified by this source audit. |
| /office/marketing | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/marketing/analytics | frontend/src/app/desktop/surfaces/marketing/analytics.ts | dappco.re/lthn/desktop/pkg/marketing/analytics.Service.Get | integrated | go/pkg/marketing/analytics/wails.go#Service.Get | Runtime path not certified by this source audit. |
| /office/marketing/audience | frontend/src/app/desktop/surfaces/marketing/audience.ts | dappco.re/lthn/desktop/pkg/marketing/audience.Service.List | integrated | go/pkg/marketing/audience/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/marketing/campaigns | frontend/src/app/desktop/surfaces/marketing/campaigns.ts | dappco.re/lthn/desktop/pkg/marketing/campaigns.Service.List | integrated | go/pkg/marketing/campaigns/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/marketing/content | frontend/src/app/desktop/surfaces/marketing/content.ts | dappco.re/lthn/desktop/pkg/marketing/content.Service.List | integrated | go/pkg/marketing/content/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/marketing/social | frontend/src/app/desktop/surfaces/marketing/social.ts | dappco.re/lthn/desktop/pkg/marketing/social.Service.List | integrated | go/pkg/marketing/social/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/project-manager | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/project-manager/backlog | frontend/src/app/desktop/surfaces/planning/backlog.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/project-manager/board | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/project-manager/calendar | frontend/src/app/desktop/surfaces/planning/calendar.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/project-manager/issues | frontend/src/app/desktop/surfaces/coding/issues.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/project-manager/retros | frontend/src/app/desktop/surfaces/planning/retros.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/project-manager/roadmap | frontend/src/app/desktop/surfaces/planning/roadmap.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /office/project-manager/sprints | frontend/src/app/desktop/surfaces/planning/sprints.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/project-manager/today | frontend/src/app/desktop/surfaces/planning/today.ts | dappco.re/lthn/desktop/pkg/tasks.Service.List | integrated | go/pkg/tasks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /office/tenant | frontend/src/app/desktop/apps/dev-panel.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/activity | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/activity/observe | frontend/src/app/desktop/surfaces/observe/activity.ts | /v1/audit/events?limit=250 | unresolved | component/route only | Runtime path not certified by this source audit. |
| /system/activity/streams | frontend/src/app/desktop/apps/activity.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/control | frontend/src/app/desktop/apps/control.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/marketplace | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/marketplace/opencode | frontend/src/app/desktop/surfaces/extensions/opencode-shim.ts | none declared | integrated | frontend/src/app/desktop/surfaces/extensions/opencode-shim.ts | Runtime path not certified by this source audit. |
| /system/marketplace/plugin-view | frontend/src/app/desktop/surfaces/extensions/plugin-view.ts | none declared | integrated | frontend/src/app/desktop/surfaces/extensions/plugin-view-runtime.ts | Runtime path not certified by this source audit. |
| /system/marketplace/store | frontend/src/app/desktop/surfaces/extensions/marketplace.ts | none declared | integrated | frontend/src/app/desktop/surfaces/extensions/marketplace.ts | Runtime path not certified by this source audit. |
| /system/operations | frontend/src/app/desktop/apps/composite.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/operations/incidents | frontend/src/app/desktop/surfaces/operations/incidents.ts | dappco.re/lthn/desktop/pkg/incidents.Service.List | integrated | go/pkg/incidents/wails.go#Service.List | Runtime path not certified by this source audit. |
| /system/operations/runbooks | frontend/src/app/desktop/surfaces/operations/runbooks.ts | dappco.re/lthn/desktop/pkg/runbooks.Service.List | integrated | go/pkg/runbooks/wails.go#Service.List | Runtime path not certified by this source audit. |
| /system/operations/status | frontend/src/app/desktop/surfaces/operations/status.ts | dappco.re/lthn/desktop/pkg/vi.Service.Sites | integrated | go/pkg/vi/service.go#Service.Sites | Runtime path not certified by this source audit. |
| /system/settings | frontend/src/app/desktop/apps/settings.app.ts | none declared | integrated | frontend/src/app/desktop/preferences.service.ts | Runtime path not certified by this source audit. |
| /system/telemetry | frontend/src/app/desktop/apps/telemetry.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /system/welcome | frontend/src/app/desktop/apps/welcome.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /tools/files | frontend/src/app/desktop/apps/composite.app.ts | none declared | integrated | frontend/src/app/desktop/desktop-files-bridge.service.ts | Runtime path not certified by this source audit. |
| /tools/files/home | frontend/src/app/desktop/apps/files.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
| /tools/files/workspace | frontend/src/app/desktop/surfaces/office/files.ts | none declared | integrated | frontend/src/app/desktop/apps/files.app.ts<br>frontend/src/app/desktop/desktop-files-bridge.service.ts | Runtime path not certified by this source audit. |
| /tools/notepad | frontend/src/app/desktop/apps/notepad.app.ts | none declared | design-fixture | component/route only | Runtime path not certified by this source audit. |
