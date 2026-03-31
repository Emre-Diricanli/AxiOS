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
  const [activeModel, setActiveModel] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const streamBufferRef = useRef("");

  const onMessage = useCallback((msg: ChatMessage) => {
    if (msg.type === "assistant") {
      if (msg.model) setActiveModel(msg.model);
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
          { id: "streaming", role: "assistant", content: streamBufferRef.current, model: msg.model },
        ];
      });
    } else if (msg.type === "tool_use") {
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        if (last && last.id === "streaming") {
          updated[updated.length - 1] = { ...last, id: crypto.randomUUID() };
        }
        updated.push({ id: crypto.randomUUID(), role: "tool_use", content: msg.content, toolName: msg.toolName });
        return updated;
      });
      streamBufferRef.current = "";
    } else if (msg.type === "tool_result") {
      setMessages((prev) => [
        ...prev,
        { id: crypto.randomUUID(), role: "tool_result", content: msg.content, toolName: msg.toolName },
      ]);
    } else if (msg.type === "error") {
      setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "error", content: msg.content }]);
      setStreaming(false);
    } else if (msg.type === "status" && msg.content === "done") {
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.id === "streaming") {
          return [...prev.slice(0, -1), { ...last, id: crypto.randomUUID() }];
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
      setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "user", content }]);
      setStreaming(true);
      streamBufferRef.current = "";
      send({ type: "user", content, sessionId: SESSION_ID });
    },
    [send]
  );

  // Fetch active model on mount
  useEffect(() => {
    fetch("/api/models/current")
      .then((r) => r.json())
      .then((d) => { if (d.model) setActiveModel(d.model); })
      .catch(() => {});
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-primary to-purple-500 flex items-center justify-center shadow-[0_0_12px_rgba(99,102,241,0.3)]">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
          </div>
          <div>
            <span className="text-sm font-semibold">Axi<span className="text-primary">OS</span></span>
            <p className="text-[10px] text-muted-foreground">{activeModel ?? "System Intelligence"}</p>
          </div>
        </div>
        <div className="flex items-center gap-1.5 px-2 py-1 rounded-full glass-subtle">
          <div className={`w-1.5 h-1.5 rounded-full ${connected ? "bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" : "bg-red-400 shadow-[0_0_6px_rgba(248,113,113,0.5)]"}`} />
          <span className="text-[10px] font-mono text-muted-foreground">{connected ? "live" : "offline"}</span>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3 min-h-0 scrollbar-none">
        {messages.length === 0 && (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <div className="w-14 h-14 mx-auto mb-4 rounded-2xl glass flex items-center justify-center glow-primary">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-primary">
                  <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
                </svg>
              </div>
              <p className="text-sm font-medium text-foreground/70 mb-1">Ask Claude anything</p>
              <p className="text-xs text-muted-foreground">System commands, file management, and more</p>
            </div>
          </div>
        )}
        {messages.map((msg) => {
          if (msg.role === "tool_use" || msg.role === "tool_result") {
            return <ToolBlock key={msg.id} type={msg.role} toolName={msg.toolName ?? "unknown"} content={msg.content} />;
          }
          return <MessageBubble key={msg.id} role={msg.role} content={msg.content} model={msg.model} />;
        })}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <InputBar onSend={handleSend} disabled={!connected || streaming} modelName={activeModel ?? undefined} />
    </div>
  );
}
