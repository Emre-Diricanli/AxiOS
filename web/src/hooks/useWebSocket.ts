import { useCallback, useEffect, useRef, useState } from "react";
import type { ChatMessage } from "@/types/messages";

interface UseWebSocketOptions {
  onMessage: (msg: ChatMessage) => void;
}

export function useWebSocket({ onMessage }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

    ws.onopen = () => setConnected(true);
    ws.onclose = () => {
      setConnected(false);
      // Reconnect after 2 seconds
      setTimeout(() => {
        wsRef.current = null;
      }, 2000);
    };
    ws.onmessage = (event) => {
      const msg: ChatMessage = JSON.parse(event.data);
      onMessage(msg);
    };

    wsRef.current = ws;

    return () => {
      ws.close();
    };
  }, [onMessage]);

  const send = useCallback((msg: ChatMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  return { send, connected };
}
