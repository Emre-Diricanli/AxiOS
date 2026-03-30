import { useCallback, useRef, useState, useEffect } from "react";
import { ChatPanel } from "@/components/Chat/ChatPanel";
import { FileExplorer } from "@/components/Files/FileExplorer";

const MIN_LEFT_PX = 320;
const MIN_RIGHT_PX = 300;

export function Shell() {
  /** Left panel width as a fraction (0-1) */
  const [splitRatio, setSplitRatio] = useState(0.667);
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
    const totalWidth = rect.width;
    const x = e.clientX - rect.left;

    // Clamp so neither panel gets too small
    const minLeft = MIN_LEFT_PX / totalWidth;
    const maxLeft = 1 - MIN_RIGHT_PX / totalWidth;
    const ratio = Math.min(maxLeft, Math.max(minLeft, x / totalWidth));
    setSplitRatio(ratio);
  }, []);

  const onPointerUp = useCallback(() => {
    dragging.current = false;
  }, []);

  // Prevent text selection while dragging
  useEffect(() => {
    const onSelect = (e: Event) => {
      if (dragging.current) e.preventDefault();
    };
    document.addEventListener("selectstart", onSelect);
    return () => document.removeEventListener("selectstart", onSelect);
  }, []);

  return (
    <div className="h-screen flex flex-col bg-neutral-950 text-neutral-100">
      {/* Title bar */}
      <header className="flex items-center justify-between px-4 py-2 border-b border-neutral-800 bg-neutral-900 select-none shrink-0">
        <div className="flex items-center gap-3">
          {/* Traffic-light dots (decorative) */}
          <div className="flex items-center gap-1.5 mr-2">
            <div className="w-3 h-3 rounded-full bg-red-500/80" />
            <div className="w-3 h-3 rounded-full bg-yellow-500/80" />
            <div className="w-3 h-3 rounded-full bg-green-500/80" />
          </div>
          <h1 className="text-sm font-bold tracking-tight">
            Axi<span className="text-blue-500">OS</span>
          </h1>
        </div>
        <div className="flex items-center gap-4 text-xs text-neutral-500">
          <span>System</span>
          <span>Containers</span>
          <button className="hover:text-neutral-200 transition-colors">Settings</button>
        </div>
      </header>

      {/* Split view */}
      <div ref={containerRef} className="flex-1 flex min-h-0 overflow-hidden">
        {/* Left panel: File Explorer */}
        <div
          className="min-w-0 overflow-hidden"
          style={{ width: `${splitRatio * 100}%` }}
        >
          <FileExplorer />
        </div>

        {/* Resizable divider */}
        <div
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          className="w-1 shrink-0 bg-neutral-800 hover:bg-blue-500/50 active:bg-blue-500/70 cursor-col-resize transition-colors"
        />

        {/* Right panel: Chat */}
        <div
          className="min-w-0 overflow-hidden border-l border-neutral-800"
          style={{ width: `${(1 - splitRatio) * 100}%` }}
        >
          <ChatPanel />
        </div>
      </div>
    </div>
  );
}
