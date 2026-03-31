import { useCallback, useRef, useState, useEffect, lazy, Suspense } from "react";
import { ChatPanel } from "@/components/Chat/ChatPanel";
import { FileExplorer } from "@/components/Files/FileExplorer";
import { ModelPicker } from "./ModelPicker";

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

type Tab = "dashboard" | "files" | "terminal" | "system" | "containers" | "models" | "kubernetes";

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
    id: "kubernetes",
    label: "K8s",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="3" />
        <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
      </svg>
    ),
  },
];

const MIN_CHAT_PX = 360;

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
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [chatOpen, setChatOpen] = useState(true);
  const [chatWidth, setChatWidth] = useState(400);
  const containerRef = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);

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

  return (
    <div className="h-screen flex bg-background text-foreground overflow-hidden">
      {/* ── Left Sidebar ─────────────────────────────────────────────── */}
      <aside className="w-[68px] shrink-0 flex flex-col items-center py-4 gap-2 glass-subtle border-r border-border z-10">
        {/* Logo */}
        <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-primary to-purple-500 flex items-center justify-center mb-4 glow-sm cursor-default select-none">
          <span className="text-xs font-black text-white tracking-tighter">Ax</span>
        </div>

        {/* Nav items */}
        <nav className="flex flex-col items-center gap-1 flex-1">
          {NAV_ITEMS.map((item) => {
            const active = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => setActiveTab(item.id)}
                title={item.label}
                className={`relative w-11 h-11 rounded-xl flex items-center justify-center transition-all duration-200 group ${
                  active
                    ? "bg-primary/15 text-primary glow-sm border-glow"
                    : "text-muted-foreground hover:text-foreground hover:bg-secondary"
                }`}
              >
                {item.icon}
                {/* Active indicator */}
                {active && (
                  <div className="absolute -left-[13px] top-1/2 -translate-y-1/2 w-[3px] h-5 rounded-r-full bg-primary shadow-[0_0_8px_rgba(99,102,241,0.6)]" />
                )}
                {/* Tooltip */}
                <span className="absolute left-full ml-3 px-2.5 py-1 rounded-md bg-popover text-popover-foreground text-[11px] font-medium whitespace-nowrap opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity shadow-xl border border-border z-50">
                  {item.label}
                </span>
              </button>
            );
          })}
        </nav>

        {/* Bottom: AI toggle */}
        <div className="flex flex-col items-center gap-2">
          <div className="w-8 h-px bg-border mb-1" />
          <button
            onClick={() => setChatOpen(!chatOpen)}
            title="AI Chat"
            className={`w-11 h-11 rounded-xl flex items-center justify-center transition-all duration-200 ${
              chatOpen
                ? "bg-primary text-primary-foreground glow-sm"
                : "text-muted-foreground hover:text-foreground hover:bg-secondary"
            }`}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
          </button>

          {/* Status dot */}
          <div className="flex items-center justify-center w-8 h-8">
            <div className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]" />
          </div>
        </div>
      </aside>

      {/* ── Main Area ────────────────────────────────────────────────── */}
      <div ref={containerRef} className="flex-1 flex min-h-0 overflow-hidden">
        {/* Content */}
        <div className="flex-1 min-w-0 overflow-hidden flex flex-col">
          {/* Page header */}
          <div className="shrink-0 px-6 py-4 flex items-center justify-between">
            <div>
              <h1 className="text-lg font-semibold tracking-tight">
                {NAV_ITEMS.find((n) => n.id === activeTab)?.label}
              </h1>
              <p className="text-xs text-muted-foreground mt-0.5">
                {activeTab === "dashboard" && "System overview and quick actions"}
                {activeTab === "files" && "Browse and manage your files"}
                {activeTab === "terminal" && "System shell access"}
                {activeTab === "system" && "Hardware and performance metrics"}
                {activeTab === "containers" && "Docker container management"}
                {activeTab === "models" && "Browse and install AI models"}
                {activeTab === "kubernetes" && "Container orchestration"}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <ModelPicker />
              <div className="glass rounded-full px-3 py-1.5 flex items-center gap-2">
                <div className="w-1.5 h-1.5 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
                <span className="text-[10px] font-mono text-muted-foreground">AxiOS v0.1</span>
              </div>
            </div>
          </div>

          {/* Tab content */}
          <div className="flex-1 min-h-0 overflow-hidden mx-4 mb-4 rounded-xl glass">
            <Suspense fallback={<LoadingSpinner />}>
              {activeTab === "dashboard" && <Dashboard />}
              {activeTab === "files" && <FileExplorer />}
              {activeTab === "terminal" && <Terminal className="h-full" />}
              {activeTab === "system" && (
                <div className="h-full overflow-y-auto scrollbar-none">
                  <SystemDashboard />
                </div>
              )}
              {activeTab === "containers" && <ContainersPage />}
              {activeTab === "models" && <ModelsPage />}
              {activeTab === "kubernetes" && <KubernetesPlaceholder />}
            </Suspense>
          </div>
        </div>

        {/* Chat panel */}
        {chatOpen && (
          <>
            {/* Resize handle */}
            <div
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              className="w-[3px] shrink-0 cursor-col-resize flex items-center justify-center group"
            >
              <div className="w-px h-full bg-border group-hover:bg-primary/40 group-active:bg-primary/60 transition-colors" />
            </div>

            {/* Chat */}
            <div
              className="shrink-0 min-w-0 overflow-hidden flex flex-col my-4 mr-4 rounded-xl glass"
              style={{ width: chatWidth }}
            >
              <ChatPanel />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function KubernetesPlaceholder() {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="text-center">
        <div className="w-20 h-20 mx-auto mb-5 rounded-2xl glass flex items-center justify-center glow-primary">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" className="text-primary">
            <circle cx="12" cy="12" r="3" />
            <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
          </svg>
        </div>
        <h3 className="text-sm font-medium text-foreground mb-1">Kubernetes</h3>
        <p className="text-xs text-muted-foreground">Container orchestration coming soon</p>
      </div>
    </div>
  );
}
