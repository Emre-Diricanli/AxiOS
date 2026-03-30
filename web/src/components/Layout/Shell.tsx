import { useCallback, useRef, useState, useEffect, lazy, Suspense } from "react";
import { ChatPanel } from "@/components/Chat/ChatPanel";
import { FileExplorer } from "@/components/Files/FileExplorer";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const Terminal = lazy(() => import("@/components/Terminal/Terminal"));
const SystemDashboard = lazy(() =>
  import("@/components/System/SystemDashboard").then((m) => ({
    default: m.SystemDashboard,
  }))
);

const MIN_CHAT_PX = 340;

function LoadingSpinner() {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="w-5 h-5 border-2 border-muted border-t-primary rounded-full animate-spin" />
    </div>
  );
}

export function Shell() {
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
    setChatWidth(Math.max(MIN_CHAT_PX, Math.min(rect.width * 0.55, newWidth)));
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
    <div className="h-screen flex flex-col bg-background text-foreground overflow-hidden">
      <Tabs defaultValue="files" className="flex-1 flex flex-col min-h-0 gap-0">
        {/* Top bar */}
        <header className="flex items-center justify-between px-4 h-12 border-b bg-card/60 backdrop-blur-md select-none shrink-0">
          <div className="flex items-center gap-4">
            {/* Logo */}
            <div className="flex items-center gap-2.5 pr-4">
              <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-primary/90 to-primary shadow-md shadow-primary/20 flex items-center justify-center">
                <span className="text-[11px] font-black text-primary-foreground tracking-tight">Ax</span>
              </div>
              <span className="text-sm font-bold tracking-tight">
                Axi<span className="text-primary">OS</span>
              </span>
            </div>

            <Separator orientation="vertical" className="h-5" />

            {/* Tabs */}
            <TabsList variant="line" className="h-8">
              <TabsTrigger value="files" className="gap-1.5 text-xs">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                </svg>
                Files
              </TabsTrigger>
              <TabsTrigger value="terminal" className="gap-1.5 text-xs">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M4 17l6-5-6-5M12 19h8" />
                </svg>
                Terminal
              </TabsTrigger>
              <TabsTrigger value="system" className="gap-1.5 text-xs">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                </svg>
                System
              </TabsTrigger>
              <TabsTrigger value="kubernetes" className="gap-1.5 text-xs">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
                </svg>
                Kubernetes
              </TabsTrigger>
            </TabsList>
          </div>

          <div className="flex items-center gap-2">
            {/* AI Chat toggle */}
            <Button
              variant={chatOpen ? "default" : "ghost"}
              size="sm"
              onClick={() => setChatOpen(!chatOpen)}
              className={`gap-1.5 text-xs h-7 ${chatOpen ? "shadow-sm shadow-primary/20" : ""}`}
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
              </svg>
              AI
            </Button>

            <Separator orientation="vertical" className="h-5" />

            {/* Status */}
            <Badge variant="outline" className="h-6 gap-1.5 text-[10px] font-mono text-muted-foreground">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 shadow-sm shadow-emerald-400/50" />
              online
            </Badge>
          </div>
        </header>

        {/* Main content area */}
        <div ref={containerRef} className="flex-1 flex min-h-0 overflow-hidden">
          {/* Main panel */}
          <div className="flex-1 min-w-0 overflow-hidden">
            <Suspense fallback={<LoadingSpinner />}>
              <TabsContent value="files" className="h-full">
                <FileExplorer />
              </TabsContent>
              <TabsContent value="terminal" className="h-full">
                <Terminal className="h-full" />
              </TabsContent>
              <TabsContent value="system" className="h-full overflow-y-auto">
                <SystemDashboard />
              </TabsContent>
              <TabsContent value="kubernetes" className="h-full">
                <KubernetesPlaceholder />
              </TabsContent>
            </Suspense>
          </div>

          {/* Chat panel */}
          {chatOpen && (
            <>
              <div
                onPointerDown={onPointerDown}
                onPointerMove={onPointerMove}
                onPointerUp={onPointerUp}
                className="w-px shrink-0 bg-border hover:bg-primary/40 active:bg-primary/60 cursor-col-resize transition-colors"
              />
              <div
                className="shrink-0 min-w-0 overflow-hidden bg-card/40 backdrop-blur-sm border-l"
                style={{ width: chatWidth }}
              >
                <ChatPanel />
              </div>
            </>
          )}
        </div>
      </Tabs>
    </div>
  );
}

function KubernetesPlaceholder() {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="text-center">
        <div className="w-16 h-16 mx-auto mb-4 rounded-2xl bg-muted/50 border flex items-center justify-center">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground">
            <path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
          </svg>
        </div>
        <h3 className="text-sm font-medium text-foreground/60 mb-1">Kubernetes</h3>
        <p className="text-xs text-muted-foreground">Container orchestration coming soon</p>
      </div>
    </div>
  );
}
