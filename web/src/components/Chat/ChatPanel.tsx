import { useCallback, useRef, useState, useEffect } from "react";
import { useWebSocket } from "@/hooks/useWebSocket";
import { MessageBubble } from "./MessageBubble";
import { InputBar } from "./InputBar";
import type { ChatMessage } from "@/types/messages";

interface DisplayMessage {
  id: string;
  role: "user" | "assistant" | "error";
  content: string;
  model?: string;
}

const SESSION_ID = "default";

export function ChatPanel() {
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const streamBufferRef = useRef("");

  const onMessage = useCallback((msg: ChatMessage) => {
    if (msg.type === "assistant") {
      streamBufferRef.current += msg.content;
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.role === "assistant" && last.id === "streaming") {
          return [
            ...prev.slice(0, -1),
            { ...last, content: streamBufferRef.current, model: msg.model },
          ];
        }
        return [
          ...prev,
          {
            id: "streaming",
            role: "assistant",
            content: streamBufferRef.current,
            model: msg.model,
          },
        ];
      });
    } else if (msg.type === "error") {
      setMessages((prev) => [
        ...prev,
        { id: crypto.randomUUID(), role: "error", content: msg.content },
      ]);
      setStreaming(false);
    }
  }, []);

  const { send, connected } = useWebSocket({ onMessage });

  const handleSend = useCallback(
    (content: string) => {
      const userMsg: DisplayMessage = {
        id: crypto.randomUUID(),
        role: "user",
        content,
      };
      setMessages((prev) => [...prev, userMsg]);
      setStreaming(true);
      streamBufferRef.current = "";

      send({
        type: "user",
        content,
        sessionId: SESSION_ID,
      });
    },
    [send]
  );

  // Auto-scroll to bottom
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div className="flex flex-col h-full">
      {/* Connection status */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-neutral-800 text-sm">
        <div
          className={`w-2 h-2 rounded-full ${
            connected ? "bg-green-500" : "bg-red-500"
          }`}
        />
        <span className="text-neutral-400">
          {connected ? "Connected" : "Disconnected"}
        </span>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="flex items-center justify-center h-full text-neutral-600">
            <div className="text-center">
              <h2 className="text-2xl font-bold text-neutral-300 mb-2">
                AxiOS
              </h2>
              <p>Your AI-native operating system is ready.</p>
              <p className="mt-1">Ask me anything about your system.</p>
            </div>
          </div>
        )}
        {messages.map((msg) => (
          <MessageBubble
            key={msg.id}
            role={msg.role}
            content={msg.content}
            model={msg.model}
          />
        ))}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <InputBar onSend={handleSend} disabled={!connected || streaming} />
    </div>
  );
}
