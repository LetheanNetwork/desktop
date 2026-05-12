# frontend/BINDINGS.md — Lit ↔ Go bridge

How Lit components consume the Wails v3 bindings shipped from `go/pkg/desktop/`.

## Layout

```
frontend/
├── bindings/                                — generated, do NOT edit
│   └── dappco.re/lthn/desktop/pkg/desktop/  — one .ts per service + models.ts + index.ts
├── src/
│   ├── lit/                                  — Lit elements (Lethean-5 ports + app surfaces)
│   ├── main.js                               — surface router (?surface=chat etc.)
│   └── tokens.css                            — Lethean design tokens
├── tsconfig.json                             — strict TS, allowJs for legacy lit-*.js
└── package.json                              — lit + @wailsio/runtime + typescript
```

Regenerate after Go-side service changes:

```
task generate:bindings
# or directly:
cd go && wails3 generate bindings -ts -d ../frontend/bindings -clean=true ./pkg/desktop/...
```

The `-ts` flag is canonical — Lit + Vite consumes `.ts` natively via esbuild; `npm run typecheck` (`tsc --noEmit`) gates the binding surface across the whole frontend.

## Calling Go from Lit

Every exported method on a registered service becomes an async TS function. Errors thrown by Go are JS exceptions; return values arrive as typed promises.

```ts
import { ClipboardService, EnvService, DialogService } from "../bindings/dappco.re/lthn/desktop/pkg/desktop";

// Copy
await ClipboardService.SetText("hello");

// Read
const text = await ClipboardService.GetText();

// Show a dialog (returns when dismissed)
await DialogService.Info("Saved", "Wallet exported to ~/Lethean/wallets/");

// Open file picker — returns []string, empty if cancelled
const files = await DialogService.OpenFile({
  title: "Import model",
  filters: [{ name: "GGUF", pattern: "*.gguf" }],
});
```

## Per-service vs namespace import

Both shapes are valid:

```ts
// Namespace (recommended for components that touch many services)
import { ClipboardService, DialogService } from "../bindings/dappco.re/lthn/desktop/pkg/desktop";
await ClipboardService.SetText(value);

// Per-method (recommended for tree-shaking when only one method is used)
import { SetText } from "../bindings/dappco.re/lthn/desktop/pkg/desktop/clipboardservice";
await SetText(value);
```

Vite tree-shakes either way; the namespace form is just clearer at the call site.

## The `lthn:*` event bus

Wails-internal events (`events.Common.*`) get re-broadcast as `lthn:*` strings by `go/pkg/desktop/sysevents.go`. The frontend never imports Wails event constants — it subscribes by string.

```ts
import { Events } from "@wailsio/runtime";

const off = Events.On("lthn:window:files-dropped", (e) => {
  const { files, target } = e.data;
  // target?.id, target?.classList route the drop
});

// Cleanup in disconnectedCallback / a controller dispose
off();
```

The full event surface is in `go/pkg/desktop/sysevents.go` + `contextmenus.go` + `keybindings.go`. Today's catalogue:

| Event | Payload | Source |
|-------|---------|--------|
| `lthn:app:started` | `null` | `events.Common.ApplicationStarted` |
| `lthn:app:opened-file` | `string` (path) | `events.Common.ApplicationOpenedWithFile` |
| `lthn:app:opened-url` | `string` (URL) | `events.Common.ApplicationLaunchedWithUrl` (lthn:// scheme) |
| `lthn:theme` | `"dark"` \| `"light"` | system appearance change |
| `lthn:window:ready` | `{ window }` | window runtime ready (bindings safe) |
| `lthn:window:focus` \| `:blur` | `{ window }` | focus state |
| `lthn:window:hide` \| `:show` | `{ window }` | visibility |
| `lthn:window:resize` | `{ window }` | window resized |
| `lthn:window:files-dropped` | `{ window, payload: { files: string[], target: { id, classList, x, y, attributes } } }` | OS file drop; requires `EnableFileDrop` on the window spec |
| `lthn:context:<menu>:<action>` | `ContextMenuData` | right-click via `data-contextmenu="lthn-<menu>"` |
| `lthn:notification:response` | response payload | user clicked / actioned a system notification |

## Lit component pattern

The canonical shape — call bindings from event handlers, subscribe to `lthn:*` in `connectedCallback`, dispose in `disconnectedCallback`:

```ts
import { LitElement, html } from "lit";
import { customElement, state } from "lit/decorators.js";
import { Events } from "@wailsio/runtime";
import { ClipboardService } from "../../bindings/dappco.re/lthn/desktop/pkg/desktop";

@customElement("lthn-copy-demo")
export class LthnCopyDemo extends LitElement {
  @state() private status = "";

  private offFilesDropped?: () => void;

  connectedCallback() {
    super.connectedCallback();
    this.offFilesDropped = Events.On("lthn:window:files-dropped", (e) => {
      const { files } = (e.data as { files: string[] });
      this.status = `dropped ${files.length} files`;
    });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.offFilesDropped?.();
  }

  private async copy() {
    await ClipboardService.SetText("lethean");
    this.status = "copied";
  }

  render() {
    return html`
      <button @click=${this.copy}>copy</button>
      <span>${this.status}</span>
    `;
  }
}
```

## Models

Go structs returned by service methods arrive as plain typed objects — class-shaped on the TS side when generated under `models.ts`. They auto-deserialise; `time.Time` → `Date`, `[]byte` → `Uint8Array`, pointers transparent, JSON tags control field naming.

```ts
import type * as $models from "../bindings/dappco.re/lthn/desktop/pkg/desktop/models";
import { ModelsService } from "../bindings/dappco.re/lthn/desktop/pkg/desktop";

const items: $models.ModelInfo[] = await ModelsService.List();
for (const m of items) console.log(m.id, m.size);
```

## Error handling

Go `error` returns become JS exceptions. Always wrap binding calls in try/catch when a failure is meaningful for UI:

```ts
try {
  await SessionsService.Delete(id);
} catch (err) {
  console.error("delete failed:", err);
  this.toast(`Could not delete: ${(err as Error).message}`);
}
```

## Performance notes

Binding calls are in-process IPC over Wails' message bus — ~sub-millisecond, faster than HTTP. Two patterns to keep in mind:

- **Batch where possible.** One `RunnerService.GenerateBatch(prompts)` beats N `Generate(prompt)` calls for any loop ≥3.
- **Stream via events.** Long Go operations should emit progress to `lthn:*` rather than blocking the binding call. `RunnerService.Generate` is async-by-design — the streaming token surface is event-driven (see `pkg/runner/routes.go`).

## Regeneration triggers

The TS surface regenerates whenever:

- A new service is registered in `go/pkg/desktop/desktop.go`
- A method is added/renamed/removed on an existing service
- A struct field type changes (alters `models.ts`)
- An enum type or its constants change

`task generate:bindings` is idempotent — `clean=true` resets the bindings tree. `wails3 dev` watches Go sources and regenerates inline.

## Conventions

- Service struct method comments → TS JSDoc on the binding function (AX-2: comments as usage examples; the generator preserves them verbatim).
- Avoid `interface{}` / `any` in service signatures — the generator emits `any` and you lose type safety; use a concrete shape or a sum-type enum instead.
- Don't return channels or function values from bindings — not representable; use the event bus.
- Keep method args primitive-or-struct, never pointer-to-stdlib (`*os.File` etc.); use an ID-handle pattern when a Go-side resource needs to live across calls.
