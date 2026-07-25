# Lethean Desktop Native Development Host Repair Design

**Status:** Approved design

**Date:** 2026-07-25

## Purpose

Repair the native Wails development path so `wails3 task dev` provides a real
Angular HMR workflow while preserving the production contract: compiled
Angular output is embedded into the `lthn` binary and served without a
development server.

This is a focused downstream prototype in `lthn/desktop`. If the resulting
asset-routing boundary proves reusable, it can later be promoted into
`dappco.re/go/render`. The repair does not require an upstream framework
release first.

## Proven Problems

The Task 3 native smoke established three independent root causes.

### Development port collision

The Wails built-in MCP server and Lethean's configurable Wails WebSocket
transport both default to `127.0.0.1:9099`. A development binary compiled with
the `mcp` tag starts both, so the second listener fails before the application
can remain open.

### Native HMR asset requests bypass Angular

`wails3 dev` publishes `FRONTEND_DEVSERVER_URL`, but Lethean mounts its Gin
engine as the Wails asset handler. CoreGO's current middleware sends every
non-`/wails` request into that engine, whose SPA fallback serves the embedded
production filesystem.

The native origin therefore looks like
`wails://localhost:9245`, but requests such as
`/media/geist-latin-600-normal.woff2` never reach Angular's server on port
9245. The HTTP development server returns those files with `200 font/woff2`;
the native origin returns `404 text/plain`.

### Strict CSP prevents the full stylesheet from activating

Angular's production critical-CSS optimisation emits the full stylesheet as a
print-only link with an inline `onload` event that changes it to screen media.
Lethean's CSP deliberately rejects inline script and inline event handlers.

The print-only link therefore stays inactive. The remaining critical style
contains the root Geist tokens but not the later Darwin overrides or Font
Awesome pseudo-element rules. This explains both Geist remaining active on
Darwin and empty icon tiles.

## Decisions

### Development listener ownership

Development reserves:

- `127.0.0.1:9099` for Wails' built-in MCP server.
- `127.0.0.1:9199` for Lethean's configurable Wails WebSocket transport.
- `127.0.0.1:9245` for Angular's HMR development server.

`build/config.yml` supplies the 9199 listen address and public WebSocket URL to
the native primary process. Wails' generated `/wails/transport.js` therefore
publishes `ws://localhost:9199/wails/ws` inside the native development host.

This does not change the normal `pkg/connection` default. A separately served
browser GUI and non-MCP application composition may continue to use port 9099
unless explicitly configured otherwise. No access token is placed in the
generated JavaScript or URL.

### Local native-development asset handler

The embedded Angular sub-filesystem remains the single production asset
source. Lethean's desktop package will wrap it with Wails'
`application.AssetFileServerFS` when installing the Gin SPA fallback.

That Wails handler already has the required two modes:

- when `FRONTEND_DEVSERVER_URL` is set, proxy frontend requests to the Angular
  development server;
- otherwise, serve the supplied embedded filesystem.

The existing Gin router remains in front of the fallback. Registered `/v1`
and other backend routes continue to execute locally and retain their current
middleware. Only unmatched SPA and asset requests reach the Wails frontend
handler. `/wails/*` continues to be reserved for Wails through the existing
asset middleware.

This is intentionally implemented in `go/pkg/desktop/`, not by modifying the
versioned CoreGO module or the Go module cache. The downstream behaviour and
tests become the reference for a later generic CoreGO change.

### CSP-compatible Angular stylesheet output

Production Angular builds will keep optimisation and minification but disable
`inlineCritical`. The generated `index.html` must use a normal active
stylesheet link which does not depend on inline JavaScript.

The CSP remains strict. The repair must not add `'unsafe-inline'`, a broad
nonce bypass, or a post-build HTML rewrite. With the full stylesheet active:

- `[data-platform="darwin"]` applies the SF Pro/SF Mono token policy;
- Font Awesome family, weight, and pseudo-content rules participate in the
  cascade;
- browser/web presentation continues to use Geist and Geist Mono;
- Instrument Serif remains available for editorial text.

## Development and Production Data Flow

### Native development

```text
wails3 task dev
  -> Angular HMR server on 127.0.0.1:9245
  -> Wails MCP on 127.0.0.1:9099
  -> Lethean Wails transport on 127.0.0.1:9199
  -> native WebView loads wails://localhost:9245/#/
       /wails/*       -> Wails runtime/transport
       registered API -> Gin/CoreGO services
       other paths    -> Wails frontend handler -> Angular HMR server
```

### Compiled application

```text
production build
  -> frontend-ng writes go/cmd/lthn/dist/
  -> go/cmd/lthn/embed.go embeds dist/
  -> native WebView requests
       /wails/*       -> Wails runtime/transport
       registered API -> Gin/CoreGO services
       other paths    -> Wails frontend handler -> embedded dist/
```

No production process depends on port 9245, a local Node process, or the
development proxy environment.

## Error Behaviour

- An unavailable Angular HMR server should produce the Wails proxy's explicit
  development error rather than silently falling back to a stale embedded
  asset.
- A missing production asset remains an asset-server failure and must not be
  rewritten to `index.html`.
- Backend API routes must never be proxied to Angular.
- `/wails/transport.js` must remain Wails-generated in a native host and must
  not expose the connection token.
- A collision between the configured MCP and transport listeners is a failing
  development contract.

## Test Contract

### Development command contract

Extend the existing behavioural config test to prove the primary native
process receives:

```text
LTHN_DEV=1
LTHN_WAILS_WS_LISTEN=127.0.0.1:9199
LTHN_WAILS_WS_URL=ws://localhost:9199/wails/ws
```

The test must also prove the MCP-tagged build command still receives
`EXTRA_TAGS=mcp`.

### Go asset-routing contract

Use an `httptest` Angular server and the real frontend handler with
`FRONTEND_DEVSERVER_URL` set:

- `/media/probe.woff2` reaches the development server and returns
  `font/woff2`;
- an unmatched Angular route reaches the development server;
- registered backend routes still execute in Gin;
- with no development URL, the same handler serves the embedded filesystem.

The test should derive its request from the real handler boundary rather than
asserting a configuration string.

### Build/CSP contract

Build production Angular output and verify:

- the full stylesheet link is active for screen media;
- the link does not use inline `onload`;
- no required font reference is missing;
- Geist, Geist Mono, Instrument Serif, and Font Awesome remain declared.

### Native acceptance smoke

Run `wails3 task dev` without a smoke-only environment override and verify:

1. the normal native Lethean Desktop window remains open;
2. `data-platform` is `darwin` on macOS;
3. Wails MCP listens on 9099 and the Lethean transport listens on 9199;
4. `/wails/transport.js` publishes the frozen 9199 URL and no token;
5. Angular HMR changes become visible without rebuilding the embedded bundle;
6. requested native-origin font and icon assets return `200` with non-HTML
   MIME types;
7. Darwin sans/mono variables resolve to the SF policy;
8. Instrument Serif loads explicitly;
9. a real Font Awesome icon has the expected family, weight, and non-empty
   pseudo-content;
10. all development processes stop cleanly.

The same production build must then pass the existing font verifier, proving
the embedded application remains intact.

## Scope Boundaries

This repair does not:

- weaken CSP;
- change the production `pkg/connection` default;
- add a new transport provider;
- modify the external Lethean Design Pack;
- redesign application surfaces;
- retire archives before the native smoke is green;
- publish or modify `dappco.re/go/render`.

An upstream CoreGO change may follow only after the local handler and its tests
demonstrate a reusable contract.

## Success Criteria

The repair is complete when `wails3 task dev` provides native Angular HMR with
the 9099/9199 listener split, native assets load through the development
server, strict CSP remains unchanged, Darwin typography and Font Awesome work,
and production still serves the embedded Angular build without a development
server.
