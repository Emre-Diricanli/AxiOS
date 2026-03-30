import { useCallback, useRef, useState, useEffect } from "react";
import { useWebSocket } from "@/hooks/useWebSocket";
import { MessageBubble } from "./MessageBubble";
import { ToolBlock } from "./ToolBlock";
import { InputBar } from "./InputBar";
import type { ChatMessage } from "@/types/messages";

interface DisplayMessage {
  id: string;
  role: "user" | "assistant" | "error" | "tool_use" | "tool_result";
  content: string;
  model?: string;
  toolName?: string;
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
    } else if (msg.type === "tool_use") {
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        if (last && last.id === "streaming") {
          updated[updated.length - 1] = { ...last, id: crypto.randomUUID() };
        }
        updated.push({
          id: crypto.randomUUID(),
          role: "tool_use",
          content: msg.content,
          toolName: msg.toolName,
        });
        return updated;
      });
      streamBufferRef.current = "";
    } else if (msg.type === "tool_result") {
      setMessages((prev) => [
        ...prev,
        {
          id: crypto.randomUUID(),
          role: "tool_result",
          content: msg.content,
          toolName: msg.toolName,
        },
      ]);
    } else if (msg.type === "error") {
      setMessages((prev) => [
        ...prev,
        { id: crypto.randomUUID(), role: "error", content: msg.content },
      ]);
      setStreaming(false);
    } else if (msg.type === "status" && msg.content === "done") {
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.id === "streaming") {
          return [
            ...prev.slice(0, -1),
            { ...last, id: crypto.randomUUID() },
          ];
        }
        return prev;
      });
      streamBufferRef.current = "";
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

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div className="flex flex-col h-full">
      {/* Chat header */}
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06] shrink-0">
        <div className="flex items-center gap-2">
          <div className="w-5 h-5 rounded-full bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
          </div>
          <span className="text-xs font-semibold text-neutral-300">Claude</span>
        </div>
        <div className="flex items-center gap-1.5">
          <div className={`w-1.5 h-1.5 rounded-full ${connected ? "bg-emerald-400" : "bg-red-400"}`} />
          <span className="text-[10px] text-neutral-600">{connected ? "connected" : "offline"}</span>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-3 space-y-3 min-h-0">
        {messages.length === 0 && (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <div className="w-10 h-10 mx-auto mb-3 rounded-xl bg-gradient-to-br from-blue-500/20 to-blue-600/20 border border-blue-500/20 flex items-center justify-center">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-blue-400">
                  <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
                </svg>
              </div>
              <p className="text-xs text-neutral-500">Ask Claude anything about your system</p>
            </div>
          </div>
        )}
        {messages.map((msg) => {
          if (msg.role === "tool_use" || msg.role === "tool_result") {
            return (
              <ToolBlock
                key={msg.id}
                type={msg.role}
                toolName={msg.toolName ?? "unknown"}
                content={msg.content}
              />
            );
          }
          return (
            <MessageBubble
              key={msg.id}
              role={msg.role}
              content={msg.content}
              model={msg.model}
            />
          );
        })}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <InputBar onSend={handleSend} disabled={!connected || streaming} />
    </div>
  );
}
