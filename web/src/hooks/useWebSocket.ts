import { useCallback, useEffect, useRef, useState } from "react";
import type { ChatMessage } from "@/types/messages";

interface UseWebSocketOptions {
  onMessage: (msg: ChatMessage) => void;
}

export function useWebSocket({ onMessage }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const onMessageRef = useRef(onMessage);
  const [connected, setConnected] = useState(false);

  // Keep the callback ref up to date without re-creating the WebSocket
  onMessageRef.current = onMessage;

  useEffect(() => {
    let reconnectTimeout: ReturnType<typeof setTimeout>;
    let ws: WebSocket;

    function connect() {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

      ws.onopen = () => setConnected(true);

      ws.onclose = () => {
        setConnected(false);
        // Reconnect after 3 seconds
        reconnectTimeout = setTimeout(connect, 3000);
      };

      ws.onmessage = (event) => {
        const msg: ChatMessage = JSON.parse(event.data);
        onMessageRef.current(msg);
      };

      wsRef.current = ws;
    }

    connect();

    return () => {
      clearTimeout(reconnectTimeout);
      ws?.close();
    };
  }, []); // No dependencies — connect once

  const send = useCallback((msg: ChatMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  return { send, connected };
}
