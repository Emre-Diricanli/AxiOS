import { useCallback, useRef, useState, useEffect, type MutableRefObject } from "react";
import { useWebSocket } from "@/hooks/useWebSocket";
import { MessageBubble } from "./MessageBubble";
import { ToolBlock } from "./ToolBlock";
import { InputBar } from "./InputBar";
import type { ChatMessage } from "@/types/messages";

interface ChatPanelProps {
  newChatRef?: MutableRefObject<(() => void) | null>;
}

interface DisplayMessage {
  id: string;
  role: "user" | "assistant" | "error" | "tool_use" | "tool_result";
  content: string;
  model?: string;
  toolName?: string;
}

interface SessionMeta {
  id: string;
  title: string;
  message_count: number;
  updated_at: string;
}

function relativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days === 1) return "yesterday";
  if (days < 7) return `${days}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

export function ChatPanel({ newChatRef }: ChatPanelProps) {
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [activeModel, setActiveModel] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const streamBufferRef = useRef("");
  const fetchSessionsRef = useRef<() => void>(() => {});
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
      // Refresh session list after a response completes
      fetchSessionsRef.current();
    }
  }, []);

  const { send, connected } = useWebSocket({ onMessage });

  // Fetch sessions list
  const fetchSessions = useCallback(async () => {
    try {
      const res = await fetch("/api/chat/sessions");
      const data = await res.json();
      if (data.sessions) {
        setSessions(data.sessions);
      }
    } catch {
      // ignore
    }
  }, []);

  // Keep the ref up to date
  fetchSessionsRef.current = fetchSessions;

  // Create a new session
  const createSession = useCallback(async () => {
    try {
      const res = await fetch("/api/chat/sessions", { method: "POST" });
      const data = await res.json();
      if (data.id) {
        setSessionId(data.id);
        setMessages([]);
        streamBufferRef.current = "";
        await fetchSessions();
      }
    } catch {
      // ignore
    }
  }, [fetchSessions]);

  // Switch to a session
  const switchSession = useCallback(async (id: string) => {
    setSessionId(id);
    setMessages([]);
    streamBufferRef.current = "";
    setHistoryOpen(false);
    try {
      const res = await fetch(`/api/chat/sessions/messages?id=${id}`);
      const data = await res.json();
      if (data.messages) {
        const displayMsgs: DisplayMessage[] = [];
        for (const m of data.messages) {
          const content = typeof m.content === "string" ? m.content : JSON.stringify(m.content);
          if (m.role === "user") {
            displayMsgs.push({ id: crypto.randomUUID(), role: "user", content });
          } else if (m.role === "assistant") {
            // Assistant messages from storage may have content blocks
            if (Array.isArray(m.content)) {
              for (const block of m.content) {
                if (block.type === "text" && block.text) {
                  displayMsgs.push({ id: crypto.randomUUID(), role: "assistant", content: block.text });
                } else if (block.type === "tool_use") {
                  const inputStr = block.input ? JSON.stringify(block.input) : "";
                  displayMsgs.push({ id: crypto.randomUUID(), role: "tool_use", content: inputStr, toolName: block.name });
                }
              }
            } else {
              displayMsgs.push({ id: crypto.randomUUID(), role: "assistant", content });
            }
          }
        }
        setMessages(displayMsgs);
      }
    } catch {
      // ignore
    }
  }, []);

  // Delete a session
  const deleteSession = useCallback(async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await fetch(`/api/chat/sessions?id=${id}`, { method: "DELETE" });
      await fetchSessions();
      if (id === sessionId) {
        setSessionId(null);
        setMessages([]);
      }
    } catch {
      // ignore
    }
  }, [fetchSessions, sessionId]);

  const handleSend = useCallback(
    (content: string) => {
      const currentSessionId = sessionId || "default";
      if (!sessionId) {
        // Auto-create a session if none active
        fetch("/api/chat/sessions", { method: "POST" })
          .then((r) => r.json())
          .then((data) => {
            if (data.id) {
              setSessionId(data.id);
              setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "user", content }]);
              setStreaming(true);
              streamBufferRef.current = "";
              send({ type: "user", content, sessionId: data.id });
            }
          });
        return;
      }
      setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "user", content }]);
      setStreaming(true);
      streamBufferRef.current = "";
      send({ type: "user", content, sessionId: currentSessionId });
    },
    [send, sessionId]
  );

  // Expose createSession to parent via ref (for Command Palette "New Chat")
  useEffect(() => {
    if (newChatRef) {
      newChatRef.current = createSession;
    }
    return () => {
      if (newChatRef) {
        newChatRef.current = null;
      }
    };
  }, [newChatRef, createSession]);

  // Listen for chat messages from command palette
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<string>).detail;
      if (detail) {
        handleSend(detail);
      }
    };
    window.addEventListener("axios-send-chat", handler);
    return () => window.removeEventListener("axios-send-chat", handler);
  }, [handleSend]);

  // Fetch active model on mount + poll every 3s for changes
  useEffect(() => {
    const fetchModel = () => {
      fetch("/api/models/current")
        .then((r) => r.json())
        .then((d) => { if (d.model) setActiveModel(d.model); })
        .catch(() => {});
    };
    fetchModel();
    const id = setInterval(fetchModel, 3000);
    return () => clearInterval(id);
  }, []);

  // Fetch sessions on mount
  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

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
        <div className="flex items-center gap-2">
          {/* New Chat button */}
          <button
            onClick={createSession}
            title="New Chat"
            className="w-7 h-7 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-secondary/80 transition-colors"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </button>
          {/* History toggle */}
          <button
            onClick={() => setHistoryOpen(!historyOpen)}
            title="Chat History"
            className={`w-7 h-7 rounded-lg flex items-center justify-center transition-colors ${
              historyOpen
                ? "text-primary bg-primary/10"
                : "text-muted-foreground hover:text-foreground hover:bg-secondary/80"
            }`}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </button>
          {/* Connection status */}
          <div className="flex items-center gap-1.5 px-2 py-1 rounded-full glass-subtle">
            <div className={`w-1.5 h-1.5 rounded-full ${connected ? "bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" : "bg-red-400 shadow-[0_0_6px_rgba(248,113,113,0.5)]"}`} />
            <span className="text-[10px] font-mono text-muted-foreground">{connected ? "live" : "offline"}</span>
          </div>
        </div>
      </div>

      {/* Session History Panel */}
      {historyOpen && (
        <div className="border-b border-border bg-background/50 max-h-52 overflow-y-auto scrollbar-none">
          {sessions.length === 0 ? (
            <div className="px-4 py-3 text-xs text-muted-foreground text-center">No chat history yet</div>
          ) : (
            <div className="py-1">
              {sessions.map((s) => (
                <button
                  key={s.id}
                  onClick={() => switchSession(s.id)}
                  className={`w-full flex items-center justify-between px-4 py-2 text-left group transition-colors ${
                    s.id === sessionId
                      ? "bg-primary/10 text-primary"
                      : "text-foreground/80 hover:bg-secondary/50"
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-xs font-medium truncate">{s.title}</p>
                    <p className="text-[10px] text-muted-foreground mt-0.5">
                      {s.message_count} messages &middot; {relativeTime(s.updated_at)}
                    </p>
                  </div>
                  <button
                    onClick={(e) => deleteSession(s.id, e)}
                    className="ml-2 w-5 h-5 rounded flex items-center justify-center opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-400 hover:bg-red-400/10 transition-all shrink-0"
                    title="Delete session"
                  >
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                      <line x1="18" y1="6" x2="6" y2="18" />
                      <line x1="6" y1="6" x2="18" y2="18" />
                    </svg>
                  </button>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

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
              <p className="text-sm font-medium text-foreground/70 mb-1">Ask {activeModel ?? "AxiOS"} anything</p>
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
