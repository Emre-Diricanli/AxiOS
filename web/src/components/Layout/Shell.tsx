import { ChatPanel } from "@/components/Chat/ChatPanel";

export function Shell() {
  return (
    <div className="h-screen flex flex-col bg-neutral-950">
      {/* Header */}
      <header className="flex items-center justify-between px-4 py-3 border-b border-neutral-800">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold tracking-tight">
            Axi<span className="text-blue-500">OS</span>
          </h1>
        </div>
        <div className="flex items-center gap-4 text-sm text-neutral-400">
          <span>System</span>
          <span>Containers</span>
          <button className="hover:text-neutral-200">Settings</button>
        </div>
      </header>

      {/* Main content — chat takes full space for Phase 1 */}
      <main className="flex-1 overflow-hidden">
        <ChatPanel />
      </main>
    </div>
  );
}
