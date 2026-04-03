import { useState, useEffect, useRef, useCallback, useMemo } from "react";

// ── Event bus for sending messages to ChatPanel ─────────────────────────────
export function emitChatMessage(message: string) {
  window.dispatchEvent(new CustomEvent("axios-send-chat", { detail: message }));
}

// ── Types ───────────────────────────────────────────────────────────────────
interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onNavigate: (tab: string) => void;
  onToggleChat: () => void;
  onOpenChat: () => void;
  onNewChat: () => void;
}

interface Command {
  id: string;
  title: string;
  description: string;
  category: "Navigation" | "Actions" | "AI Quick Prompts";
  icon: React.ReactNode;
  shortcut?: string;
  action: () => void;
}

// ── Fuzzy match ─────────────────────────────────────────────────────────────
function fuzzyMatch(query: string, text: string): { match: boolean; indices: number[] } {
  const lower = text.toLowerCase();
  const q = query.toLowerCase();
  const indices: number[] = [];
  let qi = 0;
  for (let i = 0; i < lower.length && qi < q.length; i++) {
    if (lower[i] === q[qi]) {
      indices.push(i);
      qi++;
    }
  }
  return { match: qi === q.length, indices };
}

function highlightMatch(text: string, indices: number[]) {
  if (indices.length === 0) return <>{text}</>;
  const parts: React.ReactNode[] = [];
  let last = 0;
  const set = new Set(indices);
  for (let i = 0; i < text.length; i++) {
    if (set.has(i)) {
      if (last < i) parts.push(<span key={`p-${last}`}>{text.slice(last, i)}</span>);
      parts.push(
        <span key={`h-${i}`} className="text-primary font-semibold">
          {text[i]}
        </span>
      );
      last = i + 1;
    }
  }
  if (last < text.length) parts.push(<span key={`e-${last}`}>{text.slice(last)}</span>);
  return <>{parts}</>;
}

// ── Icons ───────────────────────────────────────────────────────────────────
const icons = {
  dashboard: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </svg>
  ),
  files: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
    </svg>
  ),
  terminal: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 17l6-5-6-5M12 19h8" />
    </svg>
  ),
  system: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="4" y="4" width="16" height="16" rx="2" />
      <rect x="9" y="9" width="6" height="6" />
      <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
    </svg>
  ),
  containers: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="2" width="20" height="8" rx="2" />
      <rect x="2" y="14" width="20" height="8" rx="2" />
      <circle cx="6" cy="6" r="1" />
      <circle cx="6" cy="18" r="1" />
    </svg>
  ),
  models: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
      <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
      <line x1="12" y1="22.08" x2="12" y2="12" />
    </svg>
  ),
  settings: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" />
    </svg>
  ),
  chat: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
    </svg>
  ),
  newChat: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
      <line x1="12" y1="8" x2="12" y2="16" />
      <line x1="8" y1="12" x2="16" y2="12" />
    </svg>
  ),
  newFile: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="12" y1="18" x2="12" y2="12" />
      <line x1="9" y1="15" x2="15" y2="15" />
    </svg>
  ),
  newFolder: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z" />
      <line x1="12" y1="11" x2="12" y2="17" />
      <line x1="9" y1="14" x2="15" y2="14" />
    </svg>
  ),
  upload: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="16 16 12 12 8 16" />
      <line x1="12" y1="12" x2="12" y2="21" />
      <path d="M20.39 18.39A5 5 0 0018 9h-1.26A8 8 0 103 16.3" />
    </svg>
  ),
  ai: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  ),
  cpu: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="4" y="4" width="16" height="16" rx="2" />
      <rect x="9" y="9" width="6" height="6" />
      <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
    </svg>
  ),
  memory: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="6" width="20" height="12" rx="2" />
      <path d="M6 12h4M14 12h4" />
      <path d="M6 2v4M10 2v4M14 2v4M18 2v4" />
    </svg>
  ),
  disk: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <ellipse cx="12" cy="5" rx="9" ry="3" />
      <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" />
      <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
    </svg>
  ),
  processes: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
    </svg>
  ),
};

// ── Component ───────────────────────────────────────────────────────────────
export function CommandPalette({
  isOpen,
  onClose,
  onNavigate,
  onToggleChat,
  onOpenChat,
  onNewChat,
}: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Build commands list
  const commands: Command[] = useMemo(
    () => [
      // Navigation
      {
        id: "nav-dashboard",
        title: "Dashboard",
        description: "System overview and quick actions",
        category: "Navigation",
        icon: icons.dashboard,
        shortcut: "1",
        action: () => { onNavigate("dashboard"); onClose(); },
      },
      {
        id: "nav-files",
        title: "Files",
        description: "Browse and manage your files",
        category: "Navigation",
        icon: icons.files,
        shortcut: "2",
        action: () => { onNavigate("files"); onClose(); },
      },
      {
        id: "nav-terminal",
        title: "Terminal",
        description: "System shell access",
        category: "Navigation",
        icon: icons.terminal,
        shortcut: "3",
        action: () => { onNavigate("terminal"); onClose(); },
      },
      {
        id: "nav-system",
        title: "System",
        description: "Hardware and performance metrics",
        category: "Navigation",
        icon: icons.system,
        shortcut: "4",
        action: () => { onNavigate("system"); onClose(); },
      },
      {
        id: "nav-containers",
        title: "Containers",
        description: "Docker container management",
        category: "Navigation",
        icon: icons.containers,
        shortcut: "5",
        action: () => { onNavigate("containers"); onClose(); },
      },
      {
        id: "nav-models",
        title: "Models",
        description: "Browse and install AI models",
        category: "Navigation",
        icon: icons.models,
        shortcut: "6",
        action: () => { onNavigate("models"); onClose(); },
      },
      {
        id: "nav-settings",
        title: "Settings",
        description: "System preferences and configuration",
        category: "Navigation",
        icon: icons.settings,
        shortcut: "7",
        action: () => { onNavigate("settings"); onClose(); },
      },
      // Actions
      {
        id: "act-new-chat",
        title: "New Chat",
        description: "Create a new chat session",
        category: "Actions",
        icon: icons.newChat,
        action: () => { onNewChat(); onClose(); },
      },
      {
        id: "act-toggle-chat",
        title: "Toggle AI Chat",
        description: "Show or hide the chat panel",
        category: "Actions",
        icon: icons.chat,
        action: () => { onToggleChat(); onClose(); },
      },
      {
        id: "act-new-file",
        title: "New File",
        description: "Create a new file (coming soon)",
        category: "Actions",
        icon: icons.newFile,
        action: () => { onClose(); },
      },
      {
        id: "act-new-folder",
        title: "New Folder",
        description: "Create a new folder (coming soon)",
        category: "Actions",
        icon: icons.newFolder,
        action: () => { onClose(); },
      },
      {
        id: "act-upload",
        title: "Upload Files",
        description: "Upload files to the server (coming soon)",
        category: "Actions",
        icon: icons.upload,
        action: () => { onClose(); },
      },
      // AI Quick Prompts
      {
        id: "ai-ask",
        title: "Ask AI...",
        description: "Open chat panel and focus input",
        category: "AI Quick Prompts",
        icon: icons.ai,
        action: () => { onOpenChat(); onClose(); },
      },
      {
        id: "ai-sysinfo",
        title: "System Info",
        description: "Ask AI about your system",
        category: "AI Quick Prompts",
        icon: icons.cpu,
        action: () => { onOpenChat(); emitChatMessage("Tell me about my system"); onClose(); },
      },
      {
        id: "ai-memory",
        title: "Memory Usage",
        description: "Ask AI what's using your memory",
        category: "AI Quick Prompts",
        icon: icons.memory,
        action: () => { onOpenChat(); emitChatMessage("What's using my memory?"); onClose(); },
      },
      {
        id: "ai-disk",
        title: "Disk Usage",
        description: "Ask AI about disk space",
        category: "AI Quick Prompts",
        icon: icons.disk,
        action: () => { onOpenChat(); emitChatMessage("How's my disk space?"); onClose(); },
      },
      {
        id: "ai-processes",
        title: "Running Processes",
        description: "Ask AI about running processes",
        category: "AI Quick Prompts",
        icon: icons.processes,
        action: () => { onOpenChat(); emitChatMessage("Show me running processes"); onClose(); },
      },
    ],
    [onClose, onNavigate, onToggleChat, onOpenChat, onNewChat]
  );

  // Filter commands by fuzzy match
  const filtered = useMemo(() => {
    if (!query.trim()) return commands.map((cmd) => ({ cmd, titleIndices: [] as number[], descIndices: [] as number[] }));
    const results: { cmd: Command; titleIndices: number[]; descIndices: number[]; score: number }[] = [];
    for (const cmd of commands) {
      const titleMatch = fuzzyMatch(query, cmd.title);
      const descMatch = fuzzyMatch(query, cmd.description);
      if (titleMatch.match || descMatch.match) {
        // Title match scores higher
        const score = titleMatch.match ? titleMatch.indices.length * 2 : descMatch.indices.length;
        results.push({
          cmd,
          titleIndices: titleMatch.match ? titleMatch.indices : [],
          descIndices: descMatch.match ? descMatch.indices : [],
          score,
        });
      }
    }
    results.sort((a, b) => b.score - a.score);
    return results.map(({ cmd, titleIndices, descIndices }) => ({ cmd, titleIndices, descIndices }));
  }, [query, commands]);

  // Group by category preserving order
  const grouped = useMemo(() => {
    const map = new Map<string, typeof filtered>();
    for (const item of filtered) {
      const cat = item.cmd.category;
      if (!map.has(cat)) map.set(cat, []);
      map.get(cat)!.push(item);
    }
    return map;
  }, [filtered]);

  // Reset selection when filter changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Auto-focus input on open
  useEffect(() => {
    if (isOpen) {
      setQuery("");
      setSelectedIndex(0);
      // Small timeout to let the DOM render
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [isOpen]);

  // Scroll selected item into view
  useEffect(() => {
    if (!listRef.current) return;
    const items = listRef.current.querySelectorAll("[data-command-item]");
    const target = items[selectedIndex] as HTMLElement | undefined;
    target?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((prev) => Math.min(prev + 1, filtered.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((prev) => Math.max(prev - 1, 0));
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (filtered[selectedIndex]) {
          filtered[selectedIndex].cmd.action();
        }
      } else if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    },
    [filtered, selectedIndex, onClose]
  );

  if (!isOpen) return null;

  // Flatten for index tracking
  let flatIndex = 0;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh] animate-in fade-in duration-150"
      onClick={onClose}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />

      {/* Panel */}
      <div
        className="relative w-full max-w-lg mx-4 rounded-2xl shadow-2xl glass border border-glass-border overflow-hidden animate-in zoom-in-95 fade-in duration-150"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search Input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border">
          {/* Magnifying glass icon */}
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="text-muted-foreground shrink-0"
          >
            <circle cx="11" cy="11" r="8" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command..."
            className="flex-1 bg-transparent text-base text-foreground placeholder:text-muted-foreground outline-none"
            autoComplete="off"
            spellCheck={false}
          />
          {/* ESC badge */}
          <kbd className="shrink-0 glass-subtle rounded px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
            ESC
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-80 overflow-y-auto scrollbar-none py-2">
          {filtered.length === 0 ? (
            <div className="px-4 py-8 text-center">
              <p className="text-sm text-muted-foreground">No results found</p>
              <p className="text-xs text-muted-foreground/60 mt-1">Try a different search term</p>
            </div>
          ) : (
            Array.from(grouped.entries()).map(([category, items]) => (
              <div key={category}>
                {/* Section header */}
                <div className="px-4 pt-2 pb-1">
                  <span className="text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
                    {category}
                  </span>
                </div>
                {/* Items */}
                {items.map((item) => {
                  const idx = flatIndex++;
                  const isActive = idx === selectedIndex;
                  return (
                    <button
                      key={item.cmd.id}
                      data-command-item
                      onClick={() => item.cmd.action()}
                      onMouseEnter={() => setSelectedIndex(idx)}
                      className={`w-full flex items-center gap-3 py-2 px-3 mx-1 rounded-lg transition-colors text-left ${
                        isActive
                          ? "bg-accent border-l-2 border-primary"
                          : "border-l-2 border-transparent hover:bg-accent/50"
                      }`}
                    >
                      {/* Icon */}
                      <div
                        className={`shrink-0 w-8 h-8 rounded-lg flex items-center justify-center ${
                          isActive
                            ? "bg-primary/15 text-primary"
                            : "bg-secondary text-muted-foreground"
                        }`}
                      >
                        {item.cmd.icon}
                      </div>
                      {/* Text */}
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium text-foreground truncate">
                          {highlightMatch(item.cmd.title, item.titleIndices)}
                        </div>
                        <div className="text-xs text-muted-foreground truncate">
                          {highlightMatch(item.cmd.description, item.descIndices)}
                        </div>
                      </div>
                      {/* Shortcut badge */}
                      {item.cmd.shortcut && (
                        <kbd className="shrink-0 glass-subtle rounded px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                          {item.cmd.shortcut}
                        </kbd>
                      )}
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        {/* Footer */}
        <div className="px-4 py-2 border-t border-border flex items-center gap-4 text-[10px] text-muted-foreground">
          <span className="flex items-center gap-1">
            <kbd className="glass-subtle rounded px-1 py-0.5 font-mono">&uarr;&darr;</kbd>
            navigate
          </span>
          <span className="flex items-center gap-1">
            <kbd className="glass-subtle rounded px-1 py-0.5 font-mono">&crarr;</kbd>
            select
          </span>
          <span className="flex items-center gap-1">
            <kbd className="glass-subtle rounded px-1 py-0.5 font-mono">esc</kbd>
            close
          </span>
        </div>
      </div>
    </div>
  );
}
