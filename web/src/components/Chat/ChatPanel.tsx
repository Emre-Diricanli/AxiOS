import { useCallback, useRef, useState, useEffect, type MutableRefObject } from "react";
import { useWebSocket } from "@/hooks/useWebSocket";
import { MessageBubble } from "./MessageBubble";
import { ToolBlock } from "./ToolBlock";
import { ApprovalCard } from "./ApprovalCard";
import { InputBar } from "./InputBar";
import { DiffBlock, type DiffFile } from "./DiffBlock";
import { TasksDrawer } from "./TasksDrawer";
import type { ApprovalStatus, ChatMessage } from "@/types/messages";
import { recordActivity } from "@/lib/activity";

export type ChatDisplayMode = "docked" | "overlay" | "fullscreen";

export interface ChatContextAction {
  label: string;
  prompt: string;
}

interface ChatPanelProps {
  newChatRef?: MutableRefObject<(() => void) | null>;
  mode: ChatDisplayMode;
  canDock: boolean;
  contextActions: ChatContextAction[];
  promptToSend?: string | null;
  onPromptSent?: () => void;
  onModeChange: (mode: ChatDisplayMode) => void;
  onClose: () => void;
}

interface DisplayMessage {
  id: string;
  role: "user" | "assistant" | "error" | "tool_use" | "tool_result" | "approval_request" | "diff";
  content: string;
  thinking?: string; // model reasoning, rendered collapsed and dim
  diffFiles?: DiffFile[]; // role "diff": file changes from a code turn
  model?: string;
  provider?: string;
  toolName?: string;
  approvalId?: string;
  approvalStatus?: ApprovalStatus;
}

// Marks still-pending approval cards as expired. Called when a tool_result
// or error arrives for the flow without the user having responded — the
// daemon has already resolved the request server-side (timeout = deny).
function expirePendingApprovals(msgs: DisplayMessage[]): DisplayMessage[] {
  if (!msgs.some((m) => m.role === "approval_request" && m.approvalStatus === "pending")) {
    return msgs;
  }
  return msgs.map((m) =>
    m.role === "approval_request" && m.approvalStatus === "pending"
      ? { ...m, approvalStatus: "expired" as const }
      : m
  );
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

const CHAT_MODES: { mode: ChatDisplayMode; label: string }[] = [
  { mode: "docked", label: "Dock" },
  { mode: "overlay", label: "Overlay" },
  { mode: "fullscreen", label: "Focus" },
];

const EMPTY_STATE_ACTIONS: ChatContextAction[] = [
  { label: "Inspect system", prompt: "Inspect this system and summarize its current health, resources, and anything requiring attention." },
  { label: "Summarize logs", prompt: "Review recent system and service logs, then summarize warnings, failures, and likely causes." },
  { label: "Diagnose storage", prompt: "Diagnose storage usage, identify what is consuming space, and recommend safe cleanup options." },
  { label: "Start coding task", prompt: "Help me choose a useful coding task in my workspace and create a safe implementation plan." },
];

function ChatModeIcon({ mode }: { mode: ChatDisplayMode }) {
  if (mode === "docked") {
    return (
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <path d="M15 4v16" />
      </svg>
    );
  }
  if (mode === "overlay") {
    return (
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <rect x="11" y="7" width="7" height="10" rx="1" />
      </svg>
    );
  }
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
      <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
    </svg>
  );
}

export function ChatPanel({
  newChatRef,
  mode,
  canDock,
  contextActions,
  promptToSend,
  onPromptSent,
  onModeChange,
  onClose,
}: ChatPanelProps) {
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [codeMode, setCodeMode] = useState(false);
  const [codeDir, setCodeDir] = useState("");
  const [activeModel, setActiveModel] = useState<string | null>(null);
  const [activeBackend, setActiveBackend] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [tasksOpen, setTasksOpen] = useState(false);
  const [panelError, setPanelError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const streamBufferRef = useRef("");
  const thinkingBufferRef = useRef("");
  const fetchSessionsRef = useRef<() => void>(() => {});
  // Refs mirror state the WS callback needs without re-subscribing.
  const codeActiveRef = useRef(false);
  const sessionIdRef = useRef<string | null>(null);
  const onMessage = useCallback((msg: ChatMessage) => {
    if (msg.type === "assistant" || msg.type === "thinking") {
      if (msg.type === "thinking") {
        thinkingBufferRef.current += msg.content ?? "";
      } else {
        streamBufferRef.current += msg.content ?? "";
      }
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        const patch = {
          content: streamBufferRef.current,
          thinking: thinkingBufferRef.current || undefined,
          model: msg.model,
          provider: msg.provider,
        };
        if (last && last.role === "assistant" && last.id === "streaming") {
          return [...prev.slice(0, -1), { ...last, ...patch }];
        }
        return [...prev, { id: "streaming", role: "assistant", ...patch }];
      });
    } else if (msg.type === "tool_use") {
      recordActivity({ kind: "tool", title: msg.toolName ?? "Tool call", detail: msg.content, status: "pending" });
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        if (last && last.id === "streaming") {
          updated[updated.length - 1] = { ...last, id: crypto.randomUUID() };
        }
        updated.push({ id: crypto.randomUUID(), role: "tool_use", content: msg.content ?? "", toolName: msg.toolName });
        return updated;
      });
      streamBufferRef.current = "";
      thinkingBufferRef.current = "";
    } else if (msg.type === "approval_request") {
      recordActivity({ kind: "approval", title: `Approval requested · ${msg.tool ?? "unknown tool"}`, detail: JSON.stringify(msg.params ?? {}), status: "pending" });
      setMessages((prev) => [
        ...prev,
        {
          id: crypto.randomUUID(),
          role: "approval_request",
          content: msg.params !== undefined ? JSON.stringify(msg.params) : "{}",
          toolName: msg.tool,
          approvalId: msg.id,
          approvalStatus: "pending",
        },
      ]);
    } else if (msg.type === "tool_result") {
      setMessages((prev) => [
        ...expirePendingApprovals(prev),
        { id: crypto.randomUUID(), role: "tool_result", content: msg.content ?? "", toolName: msg.toolName },
      ]);
    } else if (msg.type === "error") {
      setMessages((prev) => [
        ...expirePendingApprovals(prev),
        { id: crypto.randomUUID(), role: "error", content: msg.content ?? "" },
      ]);
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
      thinkingBufferRef.current = "";
      setStreaming(false);
      // Refresh session list after a response completes
      fetchSessionsRef.current();
      // Code turns: surface the file changes the session accumulated.
      if (codeActiveRef.current) {
        const sid = sessionIdRef.current || "default";
        fetch(`/api/code/chat-diff?session=${encodeURIComponent(sid)}`)
          .then((r) => (r.ok ? r.json() : null))
          .then((data: { files?: DiffFile[] } | null) => {
            const files = data?.files ?? [];
            if (files.length === 0) return;
            recordActivity({
              kind: "files",
              title: `${files.length} file${files.length === 1 ? "" : "s"} changed`,
              detail: files.map((file) => file.file ?? "unknown file").join(", "),
              status: "success",
            });
            setMessages((prev) => {
              // Replace a previous diff card so the latest state shows once.
              const withoutOldDiff = prev.filter((m) => m.role !== "diff");
              return [...withoutOldDiff, { id: crypto.randomUUID(), role: "diff", content: "", diffFiles: files }];
            });
          })
          .catch(() => {});
      }
    }
  }, []);

  const { send, connected } = useWebSocket({ onMessage });

  // Answer a pending approval request: notify the daemon and persist the
  // verdict on the rendered card for the rest of the session.
  const respondToApproval = useCallback(
    (approvalId: string, approve: boolean) => {
      send({ type: "approval_response", id: approvalId, approve });
      recordActivity({
        kind: "approval",
        title: approve ? "Approval granted" : "Approval denied",
        detail: approvalId,
        status: approve ? "success" : "denied",
      });
      setMessages((prev) =>
        prev.map((m) =>
          m.role === "approval_request" && m.approvalId === approvalId && m.approvalStatus === "pending"
            ? { ...m, approvalStatus: approve ? ("approved" as const) : ("denied" as const) }
            : m
        )
      );
    },
    [send]
  );

  // Fetch sessions list
  const fetchSessions = useCallback(async () => {
    try {
      const res = await fetch("/api/chat/sessions");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      if (data.sessions) {
        setSessions(data.sessions);
      }
      setPanelError(null);
    } catch {
      setPanelError("Chat history is currently unavailable.");
    }
  }, []);

  // Keep the ref up to date
  fetchSessionsRef.current = fetchSessions;

  // Create a new session
  const createSession = useCallback(async () => {
    try {
      const res = await fetch("/api/chat/sessions", { method: "POST" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      if (data.id) {
        setSessionId(data.id);
        setMessages([]);
        streamBufferRef.current = "";
        thinkingBufferRef.current = "";
        await fetchSessions();
      }
    } catch {
      setPanelError("Could not create a new chat session.");
    }
  }, [fetchSessions]);

  // Switch to a session
  const switchSession = useCallback(async (id: string) => {
    setSessionId(id);
    setMessages([]);
    streamBufferRef.current = "";
    thinkingBufferRef.current = "";
    setHistoryOpen(false);
    try {
      const res = await fetch(`/api/chat/sessions/messages?id=${id}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
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
      setPanelError("Could not load that conversation.");
    }
  }, []);

  // Delete a session
  const deleteSession = useCallback(async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const response = await fetch(`/api/chat/sessions?id=${id}`, { method: "DELETE" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      await fetchSessions();
      if (id === sessionId) {
        setSessionId(null);
        setMessages([]);
      }
    } catch {
      setPanelError("Could not delete that conversation.");
    }
  }, [fetchSessions, sessionId]);

  const codeActive = codeMode || activeBackend === "supergrok";
  codeActiveRef.current = codeActive;
  sessionIdRef.current = sessionId;

  const handleSend = useCallback(
    (content: string) => {
      const currentSessionId = sessionId || "default";
      const mode = codeMode ? ("code" as const) : undefined;
      const directory = (codeActive && codeDir.trim()) || undefined;
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
              thinkingBufferRef.current = "";
              send({ type: "user", content, sessionId: data.id, mode, directory });
            }
          })
          .catch(() => setPanelError("Could not start the chat request."));
        return;
      }
      setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "user", content }]);
      setStreaming(true);
      streamBufferRef.current = "";
      thinkingBufferRef.current = "";
      send({ type: "user", content, sessionId: currentSessionId, mode, directory });
    },
    [send, sessionId, codeMode, codeActive, codeDir]
  );

  // Stop the in-flight code turn; the daemon answers with the usual done.
  const abortCodeTurn = useCallback(() => {
    send({ type: "abort", sessionId: sessionId || "default" });
  }, [send, sessionId]);

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

  useEffect(() => {
    if (!promptToSend || !connected || streaming) return;
    handleSend(promptToSend);
    onPromptSent?.();
  }, [connected, handleSend, onPromptSent, promptToSend, streaming]);

  // Fetch active model on mount + poll every 3s for changes
  useEffect(() => {
    const fetchModel = () => {
      fetch("/api/models/current")
        .then((r) => r.json())
        .then((d) => {
          if (d.model) setActiveModel(d.model);
          if (d.backend) setActiveBackend(d.backend);
        })
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
      <div className="h-12 flex items-center justify-between px-3 border-b border-border shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="w-6 h-6 rounded bg-secondary flex items-center justify-center">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
          </div>
          <div>
            <span className="text-sm font-medium">Assistant</span>
            <p key={activeModel ?? "none"} className="text-xs text-muted-foreground max-w-36 truncate">
              {activeModel ?? "System Intelligence"}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-1">
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
          {/* Code tasks toggle */}
          <button
            onClick={() => setTasksOpen(!tasksOpen)}
            title="Code Tasks"
            className={`w-7 h-7 rounded-lg flex items-center justify-center transition-colors ${
              tasksOpen
                ? "text-primary bg-primary/10"
                : "text-muted-foreground hover:text-foreground hover:bg-secondary/80"
            }`}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="8" y1="6" x2="21" y2="6" />
              <line x1="8" y1="12" x2="21" y2="12" />
              <line x1="8" y1="18" x2="21" y2="18" />
              <line x1="3" y1="6" x2="3.01" y2="6" />
              <line x1="3" y1="12" x2="3.01" y2="12" />
              <line x1="3" y1="18" x2="3.01" y2="18" />
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
          <div className="flex items-center gap-1.5 px-1.5 py-1">
            <div className={`w-1.5 h-1.5 rounded-full ${connected ? "bg-emerald-400" : "bg-red-400"}`} />
            <span className="text-[10px] text-muted-foreground">{connected ? "live" : "offline"}</span>
          </div>
          <button
            onClick={onClose}
            title="Close Chat"
            aria-label="Close chat"
            className="w-7 h-7 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-secondary/80 transition-colors"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>

      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        {contextActions[0] && (
          <button
            type="button"
            onClick={() => handleSend(contextActions[0].prompt)}
            disabled={!connected || streaming}
            className="flex-1 min-w-0 flex items-center gap-2 px-2.5 py-1.5 rounded-md text-left text-xs font-medium text-foreground border border-border hover:bg-secondary disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            title={contextActions[0].prompt}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true" className="shrink-0">
              <path d="M12 3v18M3 12h18" />
              <path d="m5 5 14 14M19 5 5 19" opacity=".35" />
            </svg>
            <span className="truncate">{contextActions[0].label}</span>
          </button>
        )}
        <div className="flex items-center p-0.5 rounded-md bg-secondary border border-border" aria-label="Chat layout">
          {CHAT_MODES.map((item) => {
            const disabled = item.mode === "docked" && !canDock;
            return (
              <button
                key={item.mode}
                type="button"
                onClick={() => onModeChange(item.mode)}
                disabled={disabled}
                aria-label={`${item.label} chat`}
                aria-pressed={mode === item.mode}
                title={disabled ? "Docking needs a wider window" : `${item.label} chat`}
                className={`h-6 px-2 rounded-md flex items-center gap-1.5 text-[10px] transition-colors disabled:opacity-25 disabled:cursor-not-allowed ${
                  mode === item.mode
                    ? "bg-surface-raised text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-white/[0.04]"
                }`}
              >
                <ChatModeIcon mode={item.mode} />
                <span className={mode === "fullscreen" ? "inline" : "hidden min-[520px]:inline"}>{item.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Code Tasks Panel */}
      {tasksOpen && <TasksDrawer />}

      {panelError && (
        <div role="alert" className="flex items-center justify-between gap-3 border-b border-red-400/20 bg-red-400/[0.08] px-4 py-2 text-xs text-red-300">
          <span>{panelError}</span>
          <button type="button" onClick={() => setPanelError(null)} className="text-red-200 hover:text-white" aria-label="Dismiss error">×</button>
        </div>
      )}

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
          <div className="flex items-center justify-center h-full px-4">
            <div className="text-left max-w-sm w-full">
              <p className="text-sm font-medium text-foreground mb-1">Start with the system</p>
              <p className="text-xs text-muted-foreground">Use {activeModel ?? "the active model"} to inspect, operate, or build.</p>
              <div className="divide-y divide-border border-y border-border mt-5">
                {[...contextActions.slice(0, 1), ...EMPTY_STATE_ACTIONS].map((action) => (
                  <button
                    key={action.label}
                    type="button"
                    onClick={() => handleSend(action.prompt)}
                    disabled={!connected || streaming}
                    className="w-full px-1 py-2.5 text-left text-xs text-foreground/75 hover:text-primary disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                  >
                    {action.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}
        {messages.map((msg) => {
          if (msg.role === "tool_use" || msg.role === "tool_result") {
            return <div key={msg.id}><ToolBlock type={msg.role} toolName={msg.toolName ?? "unknown"} content={msg.content} /></div>;
          }
          if (msg.role === "diff") {
            return <div key={msg.id}><DiffBlock files={msg.diffFiles ?? []} /></div>;
          }
          if (msg.role === "approval_request") {
            return (
              <div key={msg.id}>
                <ApprovalCard
                  toolName={msg.toolName ?? "unknown"}
                  params={msg.content}
                  status={msg.approvalStatus ?? "expired"}
                  onRespond={(approve) => msg.approvalId && respondToApproval(msg.approvalId, approve)}
                />
              </div>
            );
          }
          return <div key={msg.id}><MessageBubble role={msg.role} content={msg.content} thinking={msg.thinking} model={msg.model} provider={msg.provider} /></div>;
        })}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div>
        <InputBar
          onSend={handleSend}
          disabled={!connected || streaming}
          modelName={codeActive ? activeModel ?? "opencode" : activeModel ?? undefined}
          codeMode={codeMode}
          onToggleCodeMode={() => setCodeMode((v) => !v)}
          codeActive={codeActive}
          codeDir={codeDir}
          onCodeDirChange={setCodeDir}
          streaming={streaming}
          onAbort={abortCodeTurn}
        />
      </div>
    </div>
  );
}
