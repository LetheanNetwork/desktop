// SPDX-Licence-Identifier: EUPL-1.2

export interface ProcessEvent {
  type: string;
  channel?: string;
  data?: any;
  timestamp?: string;
}

/**
 * Connects to a WebSocket endpoint and dispatches process events to a handler.
 * Returns the WebSocket instance for lifecycle management.
 */
export function connectProcessEvents(
  wsUrl: string,
  handler: (event: ProcessEvent) => void,
): WebSocket {
  const ws = new WebSocket(wsUrl);

  ws.onmessage = (e: MessageEvent) => {
    try {
      const event: ProcessEvent = JSON.parse(e.data);
      if (event.type?.startsWith?.('process.') || event.channel?.startsWith?.('process.')) {
        handler(event);
      }
    } catch {
      // Ignore malformed messages
    }
  };

  return ws;
}
