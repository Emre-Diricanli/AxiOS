import { useCallback, useRef, useState, useEffect, lazy, Suspense } from "react";
import {
  ChatPanel,
  type ChatContextAction,
  type ChatDisplayMode,
} from "@/components/Chat/ChatPanel";
import { FileExplorer } from "@/components/Files/FileExplorer";
import { ModelPicker } from "./ModelPicker";
import { CommandPalette } from "./CommandPalette";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { AxiosLogo } from "@/components/brand/AxiosLogo";
import type { AppSettings } from "@/lib/settings";
import { useRuntimeStatus } from "@/hooks/useRuntimeStatus";
import { OBSIDIAN_SETTINGS_EVENT } from "@/lib/obsidian";

const Terminal = lazy(() => import("@/components/Terminal/Terminal"));
const SystemDashboard = lazy(() =>
  import("@/components/System/SystemDashboard").then((m) => ({
    default: m.SystemDashboard,
  }))
);
const Dashboard = lazy(() =>
  import("@/components/Dashboard/Dashboard").then((m) => ({
    default: m.Dashboard,
  }))
);
const ContainersPage = lazy(() =>
  import("@/components/Containers/ContainersPage").then((m) => ({
    default: m.ContainersPage,
  }))
);
const ModelsPage = lazy(() =>
  import("@/components/Models/ModelsPage").then((m) => ({
    default: m.ModelsPage,
  }))
);
const NotesPage = lazy(() =>
  import("@/components/Notes/NotesPage").then((m) => ({
    default: m.NotesPage,
  }))
);
const SettingsPage = lazy(() =>
  import("@/components/Settings/SettingsPage").then((m) => ({
    default: m.SettingsPage,
  }))
);

type Tab = "dashboard" | "files" | "notes" | "terminal" | "system" | "containers" | "models" | "settings";

interface ChatLayoutState {
  open: boolean;
  mode: ChatDisplayMode;
  width: number;
}

const DEFAULT_CHAT_LAYOUT: ChatLayoutState = {
  open: true,
  mode: "docked",
  width: 400,
};

const CHAT_CONTEXT_ACTIONS: Record<Tab, ChatContextAction[]> = {
  dashboard: [
    { label: "Review this system", prompt: "Review this AxiOS system and highlight anything that needs my attention." },
    { label: "Suggest next actions", prompt: "Based on the current system, suggest the three most useful actions I should take next." },
  ],
  files: [
    { label: "Help with these files", prompt: "Help me inspect and organize the files in my AxiOS workspace." },
    { label: "Find recent changes", prompt: "Find and summarize the most recently changed files in my workspace." },
  ],
  notes: [
    { label: "Summarize this vault", prompt: "Summarize the main topics and recent work in my connected Obsidian vault." },
    { label: "Find related notes", prompt: "Search my Obsidian vault for notes related to what I am currently working on." },
  ],
  terminal: [
    { label: "Diagnose in terminal", prompt: "Help me diagnose this system using safe terminal commands. Explain each command before running it." },
    { label: "Check system logs", prompt: "Inspect the relevant system logs and summarize recent warnings or failures." },
  ],
  system: [
    { label: "Analyze system health", prompt: "Analyze this machine's CPU, memory, storage, network, and GPU health." },
    { label: "Find bottlenecks", prompt: "Look for performance bottlenecks on this machine and recommend practical improvements." },
  ],
  containers: [
    { label: "Inspect containers", prompt: "Inspect my containers and summarize their health, resource use, and any issues." },
    { label: "Review container logs", prompt: "Review recent container logs and identify failures or unusual behavior." },
  ],
  models: [
    { label: "Choose a model", prompt: "Recommend the best installed or available model for this machine and explain the tradeoffs." },
    { label: "Check model fit", prompt: "Check how well my AI models fit this machine's CPU, RAM, and GPU resources." },
  ],
  settings: [
    { label: "Review my setup", prompt: "Review my AxiOS configuration and suggest useful, security-conscious improvements." },
    { label: "Improve privacy", prompt: "Help me configure AxiOS for stronger privacy and local-model usage." },
  ],
};

const NAV_ITEMS: { id: Tab; label: string; icon: React.ReactNode }[] = [
  {
    id: "dashboard",
    label: "Dashboard",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
      </svg>
    ),
  },
  {
    id: "files",
    label: "Files",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
      </svg>
    ),
  },
  {
    id: "terminal",
    label: "Terminal",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M4 17l6-5-6-5M12 19h8" />
      </svg>
    ),
  },
  {
    id: "notes",
    label: "Notes",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
        <polyline points="14 2 14 8 20 8" />
        <path d="M8 13h8M8 17h6" />
      </svg>
    ),
  },
  {
    id: "system",
    label: "System",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <rect x="4" y="4" width="16" height="16" rx="2" />
        <rect x="9" y="9" width="6" height="6" />
        <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
      </svg>
    ),
  },
  {
    id: "containers",
    label: "Containers",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <rect x="2" y="2" width="20" height="8" rx="2" />
        <rect x="2" y="14" width="20" height="8" rx="2" />
        <circle cx="6" cy="6" r="1" />
        <circle cx="6" cy="18" r="1" />
      </svg>
    ),
  },
  {
    id: "models",
    label: "Models",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
        <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
        <line x1="12" y1="22.08" x2="12" y2="12" />
      </svg>
    ),
  },
  {
    id: "settings",
    label: "Settings",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" />
      </svg>
    ),
  },
];

const NAV_GROUPS: { label: string; items: Tab[] }[] = [
  { label: "Workspace", items: ["dashboard", "files", "notes", "terminal"] },
  { label: "Infrastructure", items: ["system", "containers"] },
  { label: "Intelligence", items: ["models"] },
];

function navItem(id: Tab) {
  return NAV_ITEMS.find((item) => item.id === id)!;
}

const MIN_CHAT_PX = 360;

function loadChatLayout(): ChatLayoutState {
  try {
    const saved = JSON.parse(localStorage.getItem("axios-chat-layout") ?? "{}") as Partial<ChatLayoutState>;
    const settings = JSON.parse(localStorage.getItem("axios-settings") ?? "{}") as { chatPanelWidth?: number };
    const preferredWidth = typeof saved.width === "number"
      ? saved.width
      : typeof settings.chatPanelWidth === "number"
        ? settings.chatPanelWidth
        : DEFAULT_CHAT_LAYOUT.width;
    return {
      open: typeof saved.open === "boolean" ? saved.open : DEFAULT_CHAT_LAYOUT.open,
      mode: saved.mode === "overlay" || saved.mode === "fullscreen" ? saved.mode : "docked",
      width: Math.max(MIN_CHAT_PX, Math.min(720, preferredWidth)),
    };
  } catch {
    return DEFAULT_CHAT_LAYOUT;
  }
}

function LoadingSpinner() {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="relative w-8 h-8">
        <div className="absolute inset-0 rounded-full border-2 border-primary/20" />
        <div className="absolute inset-0 rounded-full border-2 border-transparent border-t-primary animate-spin" />
      </div>
    </div>
  );
}

export function Shell() {
  const initialChatLayout = useRef(loadChatLayout()).current;
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [chatOpen, setChatOpen] = useState(initialChatLayout.open);
  const [chatMode, setChatMode] = useState<ChatDisplayMode>(initialChatLayout.mode);
  const [chatWidth, setChatWidth] = useState(initialChatLayout.width);
  const [canDock, setCanDock] = useState(() => window.matchMedia("(min-width: 901px)").matches);
  const [isMobile, setIsMobile] = useState(() => window.matchMedia("(max-width: 640px)").matches);
  const navExpanded = true;
  const [pendingChatPrompt, setPendingChatPrompt] = useState<string | null>(null);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);
  const newChatRef = useRef<(() => void) | null>(null);
  const { daemonOnline, status, activeHost } = useRuntimeStatus(5000);
  const effectiveChatMode: ChatDisplayMode = chatMode === "docked" && !canDock ? "overlay" : chatMode;

  const onPointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    dragging.current = true;
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }, []);

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    if (!dragging.current || !containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    const newWidth = rect.right - e.clientX;
    setChatWidth(Math.max(MIN_CHAT_PX, Math.min(rect.width * 0.5, newWidth)));
  }, []);

  const onPointerUp = useCallback(() => {
    dragging.current = false;
  }, []);

  useEffect(() => {
    const handler = (e: Event) => {
      if (dragging.current) e.preventDefault();
    };
    document.addEventListener("selectstart", handler);
    return () => document.removeEventListener("selectstart", handler);
  }, []);

  useEffect(() => {
    const media = window.matchMedia("(min-width: 901px)");
    const updateDocking = () => setCanDock(media.matches);
    media.addEventListener("change", updateDocking);
    return () => media.removeEventListener("change", updateDocking);
  }, []);

  useEffect(() => {
    const media = window.matchMedia("(max-width: 640px)");
    const updateMobile = () => setIsMobile(media.matches);
    media.addEventListener("change", updateMobile);
    return () => media.removeEventListener("change", updateMobile);
  }, []);

  useEffect(() => {
    localStorage.setItem(
      "axios-chat-layout",
      JSON.stringify({ open: chatOpen, mode: chatMode, width: chatWidth })
    );
  }, [chatOpen, chatMode, chatWidth]);

  useEffect(() => {
    const applySettings = (event: Event) => {
      setChatWidth((event as CustomEvent<AppSettings>).detail.chatPanelWidth);
    };
    window.addEventListener("axios-settings-changed", applySettings);
    return () => window.removeEventListener("axios-settings-changed", applySettings);
  }, []);

  useEffect(() => {
    const openChat = (event: Event) => {
      const detail = (event as CustomEvent<{ prompt?: string; mode?: ChatDisplayMode }>).detail;
      setChatOpen(true);
      if (detail?.mode) setChatMode(detail.mode);
      if (detail?.prompt) setPendingChatPrompt(detail.prompt);
    };
    const navigate = (event: Event) => {
      const tab = (event as CustomEvent<Tab>).detail;
      if (NAV_ITEMS.some((item) => item.id === tab)) setActiveTab(tab);
    };
    const openObsidianSettings = () => {
      window.location.hash = "settings-obsidian";
      setActiveTab("settings");
    };
    window.addEventListener("axios-open-chat", openChat);
    window.addEventListener("axios-navigate", navigate);
    window.addEventListener(OBSIDIAN_SETTINGS_EVENT, openObsidianSettings);
    return () => {
      window.removeEventListener("axios-open-chat", openChat);
      window.removeEventListener("axios-navigate", navigate);
      window.removeEventListener(OBSIDIAN_SETTINGS_EVENT, openObsidianSettings);
    };
  }, []);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || !chatOpen) return;
      if (effectiveChatMode === "fullscreen") {
        setChatMode(canDock ? "docked" : "overlay");
      } else if (effectiveChatMode === "overlay") {
        setChatOpen(false);
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [canDock, chatOpen, effectiveChatMode]);

  // Cmd+K / Ctrl+K to open command palette
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen((prev) => !prev);
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);

  const handlePaletteNavigate = useCallback((tab: string) => {
    setActiveTab(tab as Tab);
  }, []);

  const handlePaletteToggleChat = useCallback(() => {
    setChatOpen((prev) => !prev);
  }, []);

  const handlePaletteOpenChat = useCallback(() => {
    setChatOpen(true);
  }, []);

  const handlePaletteNewChat = useCallback(() => {
    setChatOpen(true);
    // Trigger new chat creation via the ref set by ChatPanel
    newChatRef.current?.();
  }, []);

  const activeItem = navItem(activeTab);
  const activeGroup = NAV_GROUPS.find((group) => group.items.includes(activeTab))?.label ?? "System";
  const routeLabel = status?.routing?.replace(/_/g, " ") ?? status?.backend ?? "unknown route";
  const remoteHost = Boolean(activeHost && !["localhost", "127.0.0.1", "::1"].includes(activeHost.host));

  return (
    <div className="h-screen flex bg-background text-foreground overflow-hidden">
      {/* ── Left Sidebar ─────────────────────────────────────────────── */}
      <aside className="w-[224px] max-[900px]:w-[64px] max-[640px]:hidden shrink-0 flex flex-col bg-sidebar border-r border-border z-40 overflow-hidden">
        <div className="h-12 flex items-center px-3 gap-2.5 border-b border-border shrink-0 max-[900px]:justify-center">
          <AxiosLogo
            className="max-[900px]:gap-0"
            wordmarkClassName="max-[900px]:hidden"
          />
        </div>

        <TooltipProvider delay={250}>
        <nav className="flex-1 overflow-y-auto scrollbar-none px-2 py-3 max-[900px]:px-2" aria-label="Primary navigation">
          {NAV_GROUPS.map((group, groupIndex) => (
            <div key={group.label} className="mb-4">
              {navExpanded ? (
                <p className="px-2 mb-1.5 text-[11px] font-medium text-muted-foreground/70 max-[900px]:hidden">
                  {group.label}
                </p>
              ) : (
                <div className={`h-px bg-border mx-2 mb-2 ${groupIndex === 0 ? "hidden" : ""}`} />
              )}
              <div className="space-y-1">
                {group.items.map((id) => {
                  const item = navItem(id);
                  const active = activeTab === item.id;
                  return (
                    <Tooltip key={item.id}>
                      <TooltipTrigger
                        onClick={() => setActiveTab(item.id)}
                        aria-current={active ? "page" : undefined}
                        className={`relative h-9 rounded-md flex items-center transition-colors ${
                          "w-full px-2.5 gap-2.5 max-[900px]:justify-center"
                        } ${
                          active
                            ? "bg-sidebar-accent text-foreground"
                            : "text-muted-foreground hover:text-foreground hover:bg-sidebar-accent/70"
                        }`}
                      >
                        <span className="shrink-0">{item.icon}</span>
                        <span className="text-sm truncate max-[900px]:hidden">{item.label}</span>
                      </TooltipTrigger>
                      <TooltipContent side="right" className="hidden max-[900px]:flex">{item.label}</TooltipContent>
                    </Tooltip>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>
        </TooltipProvider>

        <div className="px-2 py-3 space-y-1 shrink-0 border-t border-border">
          <div className="h-px bg-border mx-2 mb-2" />
          <button
            onClick={() => setActiveTab("settings")}
            className={`h-9 rounded-md flex items-center w-full px-2.5 gap-2.5 max-[900px]:justify-center transition-colors ${
              activeTab === "settings" ? "bg-sidebar-accent text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-sidebar-accent/70"
            }`}
            aria-label="Settings"
          >
            {navItem("settings").icon}
            <span className="text-sm max-[900px]:hidden">Settings</span>
          </button>
          <button
            onClick={() => setChatOpen(!chatOpen)}
            className={`h-9 rounded-md flex items-center w-full px-2.5 gap-2.5 max-[900px]:justify-center transition-colors ${
              chatOpen ? "bg-sidebar-accent text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-sidebar-accent/70"
            }`}
            aria-label="Toggle assistant"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
            <span className="text-sm max-[900px]:hidden">Assistant</span>
          </button>
        </div>
      </aside>

      {/* ── Main Area ────────────────────────────────────────────────── */}
      <div className="flex-1 min-w-0 flex flex-col max-[640px]:pb-16">
      <div ref={containerRef} className="relative flex-1 flex min-h-0 overflow-hidden">
        {/* Content */}
        <div className="flex-1 min-w-0 overflow-hidden flex flex-col">
          <header className="h-12 shrink-0 border-b border-border bg-background px-4 max-[640px]:px-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 min-w-0 text-xs">
              <span className="text-muted-foreground max-[640px]:hidden">{activeGroup}</span>
              <span className="text-border max-[640px]:hidden">/</span>
              <h1 className="font-medium text-foreground truncate">{activeItem.label}</h1>
            </div>
            <div className="flex items-center gap-2 min-w-0">
              <button
                type="button"
                onClick={() => setPaletteOpen(true)}
                className="h-7 min-w-[180px] max-[1100px]:min-w-0 px-2.5 border border-border bg-surface text-muted-foreground hover:text-foreground flex items-center gap-2 rounded-md max-[700px]:hidden"
                aria-label="Open command palette"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
                  <circle cx="11" cy="11" r="7" />
                  <path d="m20 20-3.5-3.5" />
                </svg>
                <span className="text-xs flex-1 text-left max-[1100px]:hidden">Search or run a command</span>
                <kbd className="text-[10px] font-mono max-[1100px]:hidden">⌘K</kbd>
              </button>
              <div className="h-5 w-px bg-border max-[760px]:hidden" />
              <div className="max-[900px]:hidden text-right leading-tight min-w-0">
                <p className="text-xs text-foreground truncate max-w-36">{remoteHost ? "Remote · " : ""}{activeHost?.name ?? "This host"}</p>
                <p className="text-[10px] text-muted-foreground capitalize truncate max-w-36">{routeLabel}</p>
              </div>
              <ModelPicker />
              <div className="h-7 px-2 flex items-center gap-1.5 border-l border-border max-[640px]:border-0" title={daemonOnline ? "AxiOS daemon connected" : "AxiOS daemon offline"}>
                <span className={`w-1.5 h-1.5 rounded-full ${daemonOnline ? "bg-emerald-400" : "bg-red-400"}`} />
                <span className="text-[10px] text-muted-foreground max-[640px]:hidden">{daemonOnline ? "Connected" : "Offline"}</span>
              </div>
            </div>
          </header>

          {/* Tab content */}
          <main className="flex-1 min-h-0 overflow-hidden bg-workspace">
            <Suspense fallback={<LoadingSpinner />}>
              {activeTab === "dashboard" && <div key="dashboard" className="h-full page-enter"><Dashboard /></div>}
              {activeTab === "files" && <div key="files" className="h-full page-enter"><FileExplorer /></div>}
              {activeTab === "notes" && <div key="notes" className="h-full page-enter"><NotesPage /></div>}
              {activeTab === "terminal" && <div key="terminal" className="h-full page-enter"><Terminal className="h-full" /></div>}
              {activeTab === "system" && (
                <div key="system" className="h-full overflow-y-auto scrollbar-none page-enter">
                  <SystemDashboard />
                </div>
              )}
              {activeTab === "containers" && <div key="containers" className="h-full page-enter"><ContainersPage /></div>}
              {activeTab === "models" && <div key="models" className="h-full page-enter"><ModelsPage /></div>}
              {activeTab === "settings" && <div key="settings" className="h-full page-enter"><SettingsPage /></div>}
            </Suspense>
          </main>
        </div>

        {/* Chat panel */}
        {chatOpen && (
          <>
            {/* Resize handle */}
            {effectiveChatMode === "docked" && (
              <div
                onPointerDown={onPointerDown}
                onPointerMove={onPointerMove}
                onPointerUp={onPointerUp}
                onPointerCancel={onPointerUp}
                className="w-[3px] shrink-0 cursor-col-resize flex items-center justify-center group"
                role="separator"
                aria-label="Resize chat panel"
                aria-orientation="vertical"
              >
                <div className="w-px h-full bg-border group-hover:bg-primary/40 group-active:bg-primary/60 transition-colors" />
              </div>
            )}

            {effectiveChatMode === "overlay" && (
              <button
                type="button"
                aria-label="Close chat overlay"
                onClick={() => setChatOpen(false)}
                className="absolute inset-0 z-20 bg-background/50 backdrop-blur-[2px] animate-fade-in"
              />
            )}

            {/* Chat */}
            <div
              key="adaptive-chat-panel"
              className={`min-w-0 overflow-hidden flex flex-col transition-[width,transform,opacity] duration-300 ${
                effectiveChatMode === "docked"
                  ? "shrink-0 border-l border-border bg-surface"
                  : effectiveChatMode === "overlay"
                    ? `absolute z-30 border border-border bg-surface shadow-[-16px_0_48px_rgba(0,0,0,0.42)] ${
                        isMobile ? "left-0 right-0 top-12 bottom-0" : canDock ? "top-3 right-3 bottom-3 rounded-lg" : "inset-2 rounded-lg"
                      }`
                    : "absolute inset-0 z-30 bg-background"
              }`}
              style={effectiveChatMode === "docked" || (effectiveChatMode === "overlay" && canDock)
                ? { width: effectiveChatMode === "overlay" ? `min(${chatWidth}px, calc(100% - 2rem))` : chatWidth }
                : undefined}
            >
              <ChatPanel
                newChatRef={newChatRef}
                mode={effectiveChatMode}
                canDock={canDock}
                contextActions={CHAT_CONTEXT_ACTIONS[activeTab]}
                promptToSend={pendingChatPrompt}
                onPromptSent={() => setPendingChatPrompt(null)}
                onModeChange={(mode) => {
                  setChatMode(mode);
                  setChatOpen(true);
                }}
                onClose={() => setChatOpen(false)}
              />
            </div>
          </>
        )}
      </div>
      </div>

      <nav className="hidden max-[640px]:flex fixed inset-x-0 bottom-0 z-50 h-16 items-center justify-around border-t border-border bg-sidebar/95 backdrop-blur-xl px-1" aria-label="Mobile navigation">
        {(["dashboard", "files", "notes", "models", "settings"] as Tab[]).map((id) => {
          const item = navItem(id);
          return (
            <button
              key={id}
              type="button"
              onClick={() => setActiveTab(id)}
              aria-label={item.label}
              aria-current={activeTab === id ? "page" : undefined}
              className={`w-11 h-11 rounded-md flex items-center justify-center ${activeTab === id ? "bg-sidebar-accent text-primary" : "text-muted-foreground"}`}
            >
              {item.icon}
            </button>
          );
        })}
        <button
          type="button"
          onClick={() => setChatOpen((open) => !open)}
          aria-label="AI assistant"
          aria-expanded={chatOpen}
          className={`w-11 h-11 rounded-md flex items-center justify-center ${chatOpen ? "bg-sidebar-accent text-primary" : "text-muted-foreground"}`}
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
            <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
          </svg>
        </button>
      </nav>

      {/* Command Palette */}
      <CommandPalette
        isOpen={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        onNavigate={handlePaletteNavigate}
        onToggleChat={handlePaletteToggleChat}
        onOpenChat={handlePaletteOpenChat}
        onNewChat={handlePaletteNewChat}
      />
    </div>
  );
}
