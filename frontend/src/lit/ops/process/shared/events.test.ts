// SPDX-Licence-Identifier: EUPL-1.2
//
// Unit tests for connectProcessEvents — the WebSocket → handler bridge
// for process.* events. happy-dom ships a WebSocket constructor; we
// stub it with a minimal fake so we can drive onmessage synchronously
// without a live server.

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { connectProcessEvents } from "./events";
import type { ProcessEvent } from "./events";

// Minimal WebSocket double — captures the assigned onmessage handler so
// the test can fire messages, and records the URL it was opened with.
class FakeWebSocket {
  static last: FakeWebSocket | null = null;
  url: string;
  onmessage: ((e: MessageEvent) => void) | null = null;
  constructor(url: string) {
    this.url = url;
    FakeWebSocket.last = this;
  }
  // Helper for tests: simulate a server frame.
  emit(data: unknown) {
    this.onmessage?.({ data: typeof data === "string" ? data : JSON.stringify(data) } as MessageEvent);
  }
}

let originalWS: typeof WebSocket;

beforeEach(() => {
  originalWS = globalThis.WebSocket;
  (globalThis as unknown as { WebSocket: unknown }).WebSocket = FakeWebSocket;
  FakeWebSocket.last = null;
});

afterEach(() => {
  (globalThis as unknown as { WebSocket: unknown }).WebSocket = originalWS;
});

describe("connectProcessEvents", () => {
  it("opens a WebSocket against the supplied URL and returns it", () => {
    const ws = connectProcessEvents("ws://127.0.0.1:9000/events", () => {});
    expect((ws as unknown as FakeWebSocket).url).toBe("ws://127.0.0.1:9000/events");
    expect(FakeWebSocket.last).not.toBeNull();
  });

  it("dispatches events whose type starts with 'process.'", () => {
    const handler = vi.fn();
    connectProcessEvents("ws://x", handler);
    const evt: ProcessEvent = { type: "process.started", data: { id: "p1" } };
    FakeWebSocket.last!.emit(evt);
    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith(evt);
  });

  it("dispatches events whose channel starts with 'process.' even when type does not", () => {
    const handler = vi.fn();
    connectProcessEvents("ws://x", handler);
    const evt: ProcessEvent = { type: "broadcast", channel: "process.output", data: "line" };
    FakeWebSocket.last!.emit(evt);
    expect(handler).toHaveBeenCalledWith(evt);
  });

  it("ignores events that are neither process-typed nor process-channelled", () => {
    const handler = vi.fn();
    connectProcessEvents("ws://x", handler);
    FakeWebSocket.last!.emit({ type: "agent.tick", channel: "agent.heartbeat" });
    expect(handler).not.toHaveBeenCalled();
  });

  it("swallows malformed JSON frames without throwing or dispatching", () => {
    const handler = vi.fn();
    connectProcessEvents("ws://x", handler);
    expect(() => FakeWebSocket.last!.emit("not-json{")).not.toThrow();
    expect(handler).not.toHaveBeenCalled();
  });

  it("tolerates a frame with no type or channel (optional-chaining guards)", () => {
    const handler = vi.fn();
    connectProcessEvents("ws://x", handler);
    expect(() => FakeWebSocket.last!.emit({ data: "orphan" })).not.toThrow();
    expect(handler).not.toHaveBeenCalled();
  });
});
