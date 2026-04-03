import { useState, useEffect, useCallback } from "react";

interface Settings {
  systemName: string;
  routingMode: "auto" | "cloud" | "local";
  systemPrompt: string;
  chatPanelWidth: number;
  fileExplorerView: "grid" | "list";
}

const DEFAULT_SETTINGS: Settings = {
  systemName: "AxiOS",
  routingMode: "auto",
  systemPrompt: "",
  chatPanelWidth: 400,
  fileExplorerView: "grid",
};

function loadSettings(): Settings {
  try {
    const raw = localStorage.getItem("axios-settings");
    if (raw) return { ...DEFAULT_SETTINGS, ...JSON.parse(raw) };
  } catch {
    // ignore
  }
  return { ...DEFAULT_SETTINGS };
}

function saveSettings(s: Settings) {
  localStorage.setItem("axios-settings", JSON.stringify(s));
}

export function SettingsPage() {
  const [settings, setSettings] = useState<Settings>(loadSettings);
  const [saveFlash, setSaveFlash] = useState<string | null>(null);
  const [clearConfirm, setClearConfirm] = useState<"history" | "all" | null>(null);

  useEffect(() => {
    saveSettings(settings);
  }, [settings]);

  const update = useCallback(<K extends keyof Settings>(key: K, value: Settings[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  }, []);

  const flash = useCallback((msg: string) => {
    setSaveFlash(msg);
    setTimeout(() => setSaveFlash(null), 2000);
  }, []);

  const handleClearHistory = useCallback(async () => {
    if (clearConfirm !== "history") {
      setClearConfirm("history");
      return;
    }
    try {
      const res = await fetch("/api/chat/sessions");
      const data = await res.json();
      if (data.sessions) {
        for (const s of data.sessions) {
          await fetch(`/api/chat/sessions?id=${s.id}`, { method: "DELETE" });
        }
      }
      flash("Chat history cleared");
    } catch {
      flash("Failed to clear history");
    }
    setClearConfirm(null);
  }, [clearConfirm, flash]);

  const handleClearAll = useCallback(() => {
    if (clearConfirm !== "all") {
      setClearConfirm("all");
      return;
    }
    localStorage.clear();
    setSettings({ ...DEFAULT_SETTINGS });
    handleClearHistory();
    setClearConfirm(null);
  }, [clearConfirm, handleClearHistory]);

  return (
    <div className="h-full overflow-y-auto scrollbar-none p-6">
      <div className="max-w-2xl mx-auto space-y-6">
        {/* Flash notification */}
        {saveFlash && (
          <div className="fixed top-6 right-6 z-50 px-4 py-2 rounded-lg bg-emerald-500/20 border border-emerald-500/30 text-emerald-300 text-sm font-medium animate-in fade-in slide-in-from-top-2">
            {saveFlash}
          </div>
        )}

        {/* General */}
        <section className="glass rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-primary">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" />
            </svg>
            General
          </h2>

          <div className="space-y-3">
            <div>
              <label className="text-xs text-muted-foreground block mb-1.5">System Name</label>
              <input
                type="text"
                value={settings.systemName}
                onChange={(e) => update("systemName", e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-secondary/50 border border-border text-sm text-foreground focus:outline-none focus:border-primary/50 transition-colors"
              />
            </div>

            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Version</span>
              <span className="text-xs font-mono text-foreground/70">AxiOS v0.1</span>
            </div>
          </div>
        </section>

        {/* AI */}
        <section className="glass rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-purple-400">
              <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
              <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
              <line x1="12" y1="22.08" x2="12" y2="12" />
            </svg>
            AI
          </h2>

          <div className="space-y-3">
            <div>
              <label className="text-xs text-muted-foreground block mb-1.5">Default Routing Mode</label>
              <select
                value={settings.routingMode}
                onChange={(e) => update("routingMode", e.target.value as Settings["routingMode"])}
                className="w-full px-3 py-2 rounded-lg bg-secondary/50 border border-border text-sm text-foreground focus:outline-none focus:border-primary/50 transition-colors appearance-none cursor-pointer"
              >
                <option value="auto">Auto</option>
                <option value="cloud">Cloud Only</option>
                <option value="local">Local Only</option>
              </select>
            </div>

            <div>
              <label className="text-xs text-muted-foreground block mb-1.5">System Prompt Override</label>
              <textarea
                value={settings.systemPrompt}
                onChange={(e) => update("systemPrompt", e.target.value)}
                placeholder="Leave empty to use default system prompt..."
                rows={4}
                className="w-full px-3 py-2 rounded-lg bg-secondary/50 border border-border text-sm text-foreground focus:outline-none focus:border-primary/50 transition-colors resize-none font-mono text-xs"
              />
            </div>
          </div>
        </section>

        {/* Appearance */}
        <section className="glass rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-cyan-400">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 2a7 7 0 017 7 4 4 0 01-4 4H9a4 4 0 01-4-4 7 7 0 017-7z" />
            </svg>
            Appearance
          </h2>

          <div className="space-y-4">
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-xs text-muted-foreground">Chat Panel Width</label>
                <span className="text-xs font-mono text-foreground/70">{settings.chatPanelWidth}px</span>
              </div>
              <input
                type="range"
                min={300}
                max={600}
                step={10}
                value={settings.chatPanelWidth}
                onChange={(e) => update("chatPanelWidth", Number(e.target.value))}
                className="w-full h-1.5 rounded-full appearance-none bg-secondary cursor-pointer accent-primary"
              />
              <div className="flex justify-between text-[10px] text-muted-foreground mt-1">
                <span>300px</span>
                <span>600px</span>
              </div>
            </div>

            <div>
              <label className="text-xs text-muted-foreground block mb-2">File Explorer Default View</label>
              <div className="flex gap-2">
                {(["grid", "list"] as const).map((view) => (
                  <button
                    key={view}
                    onClick={() => update("fileExplorerView", view)}
                    className={`flex-1 px-3 py-2 rounded-lg text-xs font-medium transition-all ${
                      settings.fileExplorerView === view
                        ? "bg-primary/20 text-primary border border-primary/30"
                        : "bg-secondary/50 text-muted-foreground border border-border hover:text-foreground"
                    }`}
                  >
                    {view === "grid" ? "Grid" : "List"}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* Data */}
        <section className="glass rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-amber-400">
              <path d="M21 5H3a2 2 0 00-2 2v10a2 2 0 002 2h18a2 2 0 002-2V7a2 2 0 00-2-2z" />
              <path d="M7 15h0M2 10h20" />
            </svg>
            Data
          </h2>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Data Directory</span>
              <span className="text-xs font-mono text-foreground/70 bg-secondary/50 px-2 py-1 rounded">~/.axios/</span>
            </div>

            <div className="flex gap-2">
              <button
                onClick={handleClearHistory}
                className={`flex-1 px-3 py-2 rounded-lg text-xs font-medium transition-all border ${
                  clearConfirm === "history"
                    ? "bg-red-500/20 text-red-400 border-red-500/30"
                    : "bg-secondary/50 text-muted-foreground border-border hover:text-foreground hover:border-border"
                }`}
              >
                {clearConfirm === "history" ? "Confirm Clear History?" : "Clear Chat History"}
              </button>
              <button
                onClick={handleClearAll}
                className={`flex-1 px-3 py-2 rounded-lg text-xs font-medium transition-all border ${
                  clearConfirm === "all"
                    ? "bg-red-500/20 text-red-400 border-red-500/30"
                    : "bg-secondary/50 text-muted-foreground border-border hover:text-foreground hover:border-border"
                }`}
              >
                {clearConfirm === "all" ? "Confirm Clear All?" : "Clear All Data"}
              </button>
            </div>
          </div>
        </section>

        {/* About */}
        <section className="glass rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-blue-400">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 16v-4M12 8h0" />
            </svg>
            About
          </h2>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Version</span>
              <span className="text-xs font-mono text-foreground/70">AxiOS v0.1</span>
            </div>

            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Built with</span>
              <div className="flex gap-1.5">
                {["Go", "React", "Ollama", "MCP"].map((tech) => (
                  <span key={tech} className="text-[10px] font-mono text-foreground/60 bg-secondary/50 px-1.5 py-0.5 rounded">
                    {tech}
                  </span>
                ))}
              </div>
            </div>

            <div className="flex gap-2 pt-1">
              <a
                href="https://github.com/axios-os/axios"
                target="_blank"
                rel="noopener noreferrer"
                className="flex-1 px-3 py-2 rounded-lg bg-secondary/50 border border-border text-xs text-muted-foreground hover:text-foreground transition-colors text-center"
              >
                GitHub
              </a>
              <a
                href="https://github.com/axios-os/axios#readme"
                target="_blank"
                rel="noopener noreferrer"
                className="flex-1 px-3 py-2 rounded-lg bg-secondary/50 border border-border text-xs text-muted-foreground hover:text-foreground transition-colors text-center"
              >
                Documentation
              </a>
            </div>
          </div>
        </section>

        {/* Bottom padding */}
        <div className="h-4" />
      </div>
    </div>
  );
}
