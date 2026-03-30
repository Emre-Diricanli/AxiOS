import { useCallback, useRef, useState, useEffect, lazy, Suspense } from "react";
import { ChatPanel } from "@/components/Chat/ChatPanel";
import { FileExplorer } from "@/components/Files/FileExplorer";

// Lazy load heavy components
const Terminal = lazy(() => import("@/components/Terminal/Terminal"));
const SystemDashboard = lazy(() =>
  import("@/components/System/SystemDashboard").then((m) => ({
    default: m.SystemDashboard,
  }))
);

type Tab = "files" | "terminal" | "system" | "kubernetes";

const TABS: { id: Tab; label: string; icon: string }[] = [
  { id: "files", label: "Files", icon: "M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" },
  { id: "terminal", label: "Terminal", icon: "M4 17l6-5-6-5M12 19h8" },
  { id: "system", label: "System", icon: "M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" },
  { id: "kubernetes", label: "Kubernetes", icon: "M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" },
];

const MIN_CHAT_PX = 320;

export function Shell() {
  const [activeTab, setActiveTab] = useState<Tab>("files");
  const [chatOpen, setChatOpen] = useState(true);
  const [chatWidth, setChatWidth] = useState(380);
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
    setChatWidth(Math.max(MIN_CHAT_PX, Math.min(rect.width * 0.6, newWidth)));
  }, []);

  const onPointerUp = useCallback(() => {
    dragging.current = false;
  }, []);

  useEffect(() => {
    const onSelect = (e: Event) => {
      if (dragging.current) e.preventDefault();
    };
    document.addEventListener("selectstart", onSelect);
    return () => document.removeEventListener("selectstart", onSelect);
  }, []);

  return (
    <div className="h-screen flex flex-col bg-[#0a0a0f] text-neutral-100 overflow-hidden">
      {/* Top bar */}
      <header className="flex items-center justify-between px-3 h-11 border-b border-white/[0.06] bg-[#0f0f18] select-none shrink-0">
        <div className="flex items-center gap-4">
          {/* Logo */}
          <div className="flex items-center gap-2 pr-4 border-r border-white/[0.06]">
            <div className="w-6 h-6 rounded-md bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center">
              <span className="text-[10px] font-black text-white">A</span>
            </div>
            <h1 className="text-sm font-semibold tracking-tight">
              Axi<span className="text-blue-400">OS</span>
            </h1>
          </div>

          {/* Tabs */}
          <nav className="flex items-center gap-0.5">
            {TABS.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                  activeTab === tab.id
                    ? "bg-white/[0.08] text-white shadow-sm shadow-blue-500/10"
                    : "text-neutral-500 hover:text-neutral-300 hover:bg-white/[0.04]"
                }`}
              >
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d={tab.icon} />
                </svg>
                {tab.label}
              </button>
            ))}
          </nav>
        </div>

        <div className="flex items-center gap-2">
          {/* AI Chat toggle */}
          <button
            onClick={() => setChatOpen(!chatOpen)}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
              chatOpen
                ? "bg-blue-500/20 text-blue-400 border border-blue-500/30"
                : "text-neutral-500 hover:text-neutral-300 hover:bg-white/[0.04]"
            }`}
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
            AI
          </button>

          {/* Status indicator */}
          <div className="flex items-center gap-1.5 px-2 py-1 rounded-full bg-white/[0.04]">
            <div className="w-1.5 h-1.5 rounded-full bg-emerald-400 shadow-sm shadow-emerald-400/50" />
            <span className="text-[10px] text-neutral-500 font-mono">online</span>
          </div>
        </div>
      </header>

      {/* Main content */}
      <div ref={containerRef} className="flex-1 flex min-h-0 overflow-hidden">
        {/* Main panel */}
        <div className="flex-1 min-w-0 overflow-hidden">
          <Suspense
            fallback={
              <div className="flex items-center justify-center h-full text-neutral-600">
                <div className="w-5 h-5 border-2 border-neutral-700 border-t-blue-500 rounded-full animate-spin" />
              </div>
            }
          >
            {activeTab === "files" && <FileExplorer />}
            {activeTab === "terminal" && <Terminal className="h-full" />}
            {activeTab === "system" && <SystemDashboard />}
            {activeTab === "kubernetes" && <KubernetesPlaceholder />}
          </Suspense>
        </div>

        {/* Chat panel */}
        {chatOpen && (
          <>
            {/* Resize handle */}
            <div
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              className="w-px shrink-0 bg-white/[0.06] hover:bg-blue-500/40 active:bg-blue-500/60 cursor-col-resize transition-colors"
            />

            {/* Chat */}
            <div
              className="shrink-0 min-w-0 overflow-hidden bg-[#0c0c14] border-l border-white/[0.06]"
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
        <div className="w-16 h-16 mx-auto mb-4 rounded-2xl bg-white/[0.04] border border-white/[0.06] flex items-center justify-center">
          <svg
            width="32"
            height="32"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="text-neutral-600"
          >
            <path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
          </svg>
        </div>
        <h3 className="text-sm font-medium text-neutral-400 mb-1">Kubernetes</h3>
        <p className="text-xs text-neutral-600">Container orchestration coming soon</p>
      </div>
    </div>
  );
}
