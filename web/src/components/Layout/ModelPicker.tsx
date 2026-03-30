import { useState, useEffect, useRef, useCallback } from "react";

interface ModelInfo {
  name: string;
  backend: "cloud" | "local";
  active: boolean;
}

export function ModelPicker() {
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [current, setCurrent] = useState<{ model: string; backend: string } | null>(null);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const fetchModels = useCallback(async () => {
    try {
      const [modelsRes, currentRes] = await Promise.all([
        fetch("/api/models"),
        fetch("/api/models/current"),
      ]);
      const modelsData = await modelsRes.json();
      const currentData = await currentRes.json();
      setModels(modelsData.models ?? []);
      setCurrent(currentData);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    fetchModels();
  }, [fetchModels]);

  // Close on click outside
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const switchModel = async (model: string, backend: string) => {
    setLoading(true);
    try {
      await fetch("/api/models/switch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model, backend }),
      });
      await fetchModels();
    } catch {
      // ignore
    }
    setLoading(false);
    setOpen(false);
  };

  const activeModel = current?.model ?? "...";
  const activeBackend = current?.backend ?? "local";
  const isCloud = activeBackend === "cloud";

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => { setOpen(!open); if (!open) fetchModels(); }}
        className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg glass-subtle hover:bg-accent transition-colors"
      >
        {/* Backend indicator */}
        <div className={`w-4 h-4 rounded-md flex items-center justify-center text-[8px] font-bold ${
          isCloud
            ? "bg-blue-500/20 text-blue-400 border border-blue-500/30"
            : "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
        }`}>
          {isCloud ? "C" : "L"}
        </div>
        <span className="text-[11px] font-mono text-foreground/80 max-w-[100px] truncate">
          {activeModel.split(":")[0]}
        </span>
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-muted-foreground">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>

      {open && (
        <div className="absolute top-full right-0 mt-2 w-64 rounded-xl glass border border-border shadow-2xl z-50 overflow-hidden">
          <div className="px-3 py-2 border-b border-border">
            <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Models</p>
          </div>
          <div className="max-h-64 overflow-y-auto scrollbar-none py-1">
            {models.length === 0 && (
              <p className="px-3 py-2 text-xs text-muted-foreground">No models available</p>
            )}
            {models.map((m) => (
              <button
                key={`${m.backend}-${m.name}`}
                onClick={() => switchModel(m.name, m.backend)}
                disabled={loading}
                className={`w-full flex items-center gap-2.5 px-3 py-2 text-left transition-colors ${
                  m.active
                    ? "bg-primary/10 text-foreground"
                    : "text-foreground/70 hover:bg-accent hover:text-foreground"
                }`}
              >
                <div className={`w-5 h-5 rounded-md flex items-center justify-center text-[8px] font-bold shrink-0 ${
                  m.backend === "cloud"
                    ? "bg-blue-500/20 text-blue-400 border border-blue-500/30"
                    : "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
                }`}>
                  {m.backend === "cloud" ? "C" : "L"}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-mono truncate">{m.name}</p>
                  <p className="text-[10px] text-muted-foreground">
                    {m.backend === "cloud" ? "Anthropic API" : "Ollama (local)"}
                  </p>
                </div>
                {m.active && (
                  <div className="w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_6px_rgba(99,102,241,0.5)] shrink-0" />
                )}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
