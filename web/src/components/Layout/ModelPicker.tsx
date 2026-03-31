import { useState, useEffect, useRef, useCallback } from "react";

interface PickerModel {
  name: string;
  provider: string;
  providerName: string;
  backend: "cloud" | "local";
  active: boolean;
}

export function ModelPicker() {
  const [models, setModels] = useState<PickerModel[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const fetchAll = useCallback(async () => {
    try {
      const [providersRes, installedRes, currentRes] = await Promise.all([
        fetch("/api/providers"),
        fetch("/api/models/installed"),
        fetch("/api/models/current"),
      ]);

      const all: PickerModel[] = [];
      let currentModel = "";
      let currentBackend = "";

      if (currentRes.ok) {
        const cur = await currentRes.json();
        currentModel = cur.model ?? "";
        currentBackend = cur.backend ?? "";
      }

      // Cloud providers
      if (providersRes.ok) {
        const pData = await providersRes.json();
        for (const p of pData.providers ?? []) {
          if (!p.has_key) continue;
          for (const m of p.models ?? []) {
            all.push({
              name: m,
              provider: p.id,
              providerName: p.name,
              backend: "cloud",
              active: currentBackend === "cloud" && p.active && m === currentModel,
            });
          }
        }
      }

      // Local Ollama models
      if (installedRes.ok) {
        const iData = await installedRes.json();
        for (const m of iData.models ?? []) {
          all.push({
            name: m.name,
            provider: "ollama",
            providerName: "Ollama (local)",
            backend: "local",
            active: currentBackend === "local" && m.name === currentModel,
          });
        }
      }

      setModels(all);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const switchModel = async (model: PickerModel) => {
    setLoading(true);
    try {
      if (model.backend === "local") {
        await fetch("/api/models/switch", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ model: model.name, backend: "local" }),
        });
      } else {
        await fetch("/api/providers/activate", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ provider: model.provider, model: model.name }),
        });
      }
      await fetchAll();
    } catch {
      // ignore
    }
    setLoading(false);
    setOpen(false);
  };

  const active = models.find((m) => m.active);
  const displayName = active?.name ?? "No model";
  const isCloud = active?.backend === "cloud";

  // Group by provider
  const grouped = new Map<string, PickerModel[]>();
  for (const m of models) {
    const key = m.providerName;
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key)!.push(m);
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => { setOpen(!open); if (!open) fetchAll(); }}
        className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg glass-subtle hover:bg-accent transition-colors"
      >
        <div className={`w-4 h-4 rounded-md flex items-center justify-center text-[8px] font-bold ${
          isCloud
            ? "bg-blue-500/20 text-blue-400 border border-blue-500/30"
            : "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
        }`}>
          {isCloud ? "C" : "L"}
        </div>
        <span className="text-[11px] font-mono text-foreground/80 max-w-[120px] truncate">
          {displayName}
        </span>
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-muted-foreground shrink-0">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>

      {open && (
        <div className="absolute top-full right-0 mt-2 w-72 rounded-xl glass border border-border shadow-2xl z-50 overflow-hidden">
          <div className="max-h-80 overflow-y-auto scrollbar-none">
            {[...grouped.entries()].map(([providerName, providerModels]) => (
              <div key={providerName}>
                <div className="px-3 py-1.5 border-b border-border sticky top-0 bg-popover/90 backdrop-blur-sm">
                  <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{providerName}</p>
                </div>
                {providerModels.map((m) => (
                  <button
                    key={`${m.provider}-${m.name}`}
                    onClick={() => switchModel(m)}
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
                    <p className="text-xs font-mono truncate flex-1">{m.name}</p>
                    {m.active && (
                      <div className="w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_6px_rgba(99,102,241,0.5)] shrink-0" />
                    )}
                  </button>
                ))}
              </div>
            ))}
            {models.length === 0 && (
              <p className="px-3 py-4 text-xs text-muted-foreground text-center">No models configured</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
