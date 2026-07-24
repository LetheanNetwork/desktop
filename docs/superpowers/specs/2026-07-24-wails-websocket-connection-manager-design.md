<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Wails WebSocket Connection Manager Design

## Goal

Expose the existing Wails v3 binding and event surface through a production
connection-manager service. The native Angular WebView, a separately served
browser, and a mobile Angular client must be able to use the same generated
bindings over a configurable WebSocket URL. The default endpoint is
`ws://localhost:9099/wails/ws`; deployments may publish it through an
authenticating TLS reverse proxy as `wss://...`.

## Selected architecture

The connection manager implements Wails' `application.Transport`,
`application.AssetServerTransport`, and `application.WailsEventListener`
contracts. Wails continues to own binding registration and its
`MessageProcessor`; the manager owns the TCP listener, WebSocket upgrades,
client lifecycle, request correlation, bounded concurrency, event fan-out,
and shutdown. `pkg/desktop` receives the manager as a dependency and passes
its stable transport into `application.Options`.

Angular installs one root `ConnectionManagerService` as the
`@wailsio/runtime` transport before any generated binding consumer starts.
The service derives its URL from an explicit injected option, an intentional
page override, backend-served public configuration, a previously selected
URL, or the current origin. It converts same-origin HTTPS deployments to
WSS, rejects insecure non-loopback WS URLs, reconnects with bounded
exponential delay, correlates responses, and forwards Wails events into the
normal runtime dispatcher.

This approach was selected over:

1. A second REST/RPC facade, which would duplicate Wails' binding metadata,
   cancellation, error, and event semantics.
2. A GUI-owned ad-hoc socket, which would keep lifecycle and configuration
   coupled to one window and would not form a reusable Core service.
3. A separate headless binding registry, which would duplicate the desktop's
   large Wails service catalogue and make native window-only bindings
   ambiguous. A browser or mobile frontend can already live in another
   process or machine while the desktop backend remains authoritative.

## Configuration and proxy contract

Backend environment:

- `LTHN_WAILS_WS_LISTEN`: listen address; default `127.0.0.1:9099`.
- `LTHN_WAILS_WS_PATH`: upgrade path; default `/wails/ws`.
- `LTHN_WAILS_WS_URL`: public client URL or root-relative proxy path; default
  `ws://localhost:9099/wails/ws`.
- `LTHN_WAILS_WS_ORIGINS`: comma-separated exact browser Origin allow-list.
- `LTHN_WAILS_WS_TOKEN`: optional access token required for the upgrade.
- `LTHN_WAILS_WS_TRUST_PROXY`: permits a non-loopback listener without a
  service token only when an authenticating proxy is the sole route.

Remote plaintext listeners and remote plaintext client URLs fail closed.
The normal deployment keeps the manager on loopback and terminates TLS and
user authentication at a reverse proxy. If the manager itself listens on a
non-loopback address, it requires either its own token or the explicit
trusted-proxy acknowledgement.

The backend-provided JavaScript may publish the public URL but must never
publish the token. Browser clients can receive a short-lived token through
an intentional client-side configuration channel; non-browser clients may
also send `Authorization: Bearer ...`. Secrets must travel only over WSS
outside loopback.

## Protocol and lifecycle

Client request:

```json
{
  "id": "client-id-1",
  "type": "request",
  "request": {
    "object": 0,
    "method": 12,
    "args": {},
    "webviewWindowName": "app",
    "clientId": "client-id"
  }
}
```

Server responses use HTTP-equivalent status codes in a `response` envelope.
Server-pushed Wails events use an `event` envelope. Request IDs, message
size, connected clients, per-client in-flight calls, and outbound queues are
bounded. One writer goroutine owns each Gorilla WebSocket connection.
Heartbeat deadlines evict dead peers, and shutdown closes clients before
stopping the HTTP server.

The Core registration factory returns the constructed `*connection.Service`
so `core.WithName("connection", connection.Register)` is canonical.
`connection.status` returns only non-secret listener state.

## Verification

Go tests cover canonical registration, default and environment
configuration, invalid remote/insecure configuration, asset serving,
authenticated upgrades, Origin rejection, binding request envelopes,
event fan-out, client limits, protocol limits, heartbeat-safe lifecycle,
and idempotent shutdown. Desktop tests pin that the supplied transport is
placed in `application.Options`.

Angular Vitest tests cover installation order, default and same-origin URLs,
secure proxy/token handling, unsafe URL rejection, correct Wails request
field names, successful and failed calls, malformed messages, request
timeouts, reconnect backoff, explicit disconnect/destroy, persistence of
non-secret URLs, and pending-request limits.

The final gate is the repository's workspace-mode Go and frontend build,
test, vet, formatting, and compliance sequence from `AGENTS.md`.
