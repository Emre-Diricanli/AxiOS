import { useState, useEffect, useCallback } from "react";
import { useModelMarketplace } from "@/hooks/useModelMarketplace";
import type { InstalledModel, MarketplaceModel } from "@/types/models";
import { HostsPanel } from "@/components/Models/HostsPanel";
import { ProvidersPanel } from "@/components/Models/ProvidersPanel";

/* ── Helpers ─────────────────────────────────────────────── */

const CATEGORIES = ["all", "general", "code", "vision", "embedding"] as const;
type Category = (typeof CATEGORIES)[number];

const CATEGORY_COLORS: Record<string, { bg: string; text: string; border: string }> = {
  general: { bg: "bg-blue-500/20", text: "text-blue-400", border: "border-blue-500/30" },
  code: { bg: "bg-emerald-500/20", text: "text-emerald-400", border: "border-emerald-500/30" },
  vision: { bg: "bg-purple-500/20", text: "text-purple-400", border: "border-purple-500/30" },
  embedding: { bg: "bg-amber-500/20", text: "text-amber-400", border: "border-amber-500/30" },
};

function categoryStyle(cat: string) {
  return CATEGORY_COLORS[cat] ?? CATEGORY_COLORS.general;
}

function formatDate(dateStr: string): string {
  try {
    const d = new Date(dateStr);
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  } catch {
    return dateStr;
  }
}

/* ── Installed Model Card ────────────────────────────────── */

function InstalledModelCard({
  model,
  isActive,
  onUse,
  onDelete,
}: {
  model: InstalledModel;
  isActive: boolean;
  onUse: () => void;
  onDelete: () => void;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <div
      className={`glass rounded-xl p-5 flex flex-col gap-3 transition-all duration-200 hover:bg-accent/30 ${
        isActive ? "border-glow glow-sm" : ""
      }`}
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-bold text-foreground truncate">{model.name}</h3>
          <p className="text-[10px] text-muted-foreground mt-0.5">
            Modified {formatDate(model.modified)}
          </p>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {isActive && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-semibold uppercase tracking-wider bg-primary/15 text-primary border border-primary/25 glow-sm">
              Active
            </span>
          )}
          <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-secondary text-muted-foreground">
            {model.size_human}
          </span>
        </div>
      </div>

      {/* Info chips */}
      <div className="flex flex-wrap gap-1.5">
        {model.family && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {model.family}
          </span>
        )}
        {model.parameters && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {model.parameters}
          </span>
        )}
        {model.quantization && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {model.quantization}
          </span>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 mt-auto pt-1">
        {!isActive && (
          <button
            onClick={onUse}
            className="flex-1 px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors"
          >
            Use
          </button>
        )}
        {confirmDelete ? (
          <div className="flex items-center gap-1.5 flex-1">
            <span className="text-[10px] text-destructive whitespace-nowrap">Are you sure?</span>
            <button
              onClick={() => setConfirmDelete(false)}
              className="px-2 py-1 rounded-md text-[10px] font-medium bg-secondary text-muted-foreground hover:bg-secondary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => {
                onDelete();
                setConfirmDelete(false);
              }}
              className="px-2 py-1 rounded-md text-[10px] font-medium bg-destructive/15 text-destructive border border-destructive/25 hover:bg-destructive/25 transition-colors"
            >
              Delete
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirmDelete(true)}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-secondary text-muted-foreground hover:bg-destructive/15 hover:text-destructive hover:border-destructive/25 border border-transparent transition-colors"
          >
            Delete
          </button>
        )}
      </div>
    </div>
  );
}

/* ── Marketplace Model Card ──────────────────────────────── */

function MarketplaceCard({
  model,
  installed,
  isPulling,
  pullProgress,
  onInstall,
}: {
  model: MarketplaceModel;
  installed: boolean;
  isPulling: boolean;
  pullProgress?: { status: string; percent: number };
  onInstall: (tag: string) => void;
}) {
  const [selectedTag, setSelectedTag] = useState(model.tags[0] ?? "latest");
  const cat = categoryStyle(model.category);

  return (
    <div className="glass rounded-xl p-5 flex flex-col gap-3 transition-all duration-200 hover:bg-accent/30">
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <h3 className="text-sm font-bold text-foreground">{model.name}</h3>
        <div className="flex items-center gap-1.5 shrink-0">
          {model.recommended && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-semibold uppercase tracking-wider bg-primary/15 text-primary border border-primary/25 glow-sm">
              Recommended
            </span>
          )}
        </div>
      </div>

      {/* Description */}
      <p className="text-xs text-muted-foreground leading-relaxed line-clamp-2">
        {model.description}
      </p>

      {/* Badges */}
      <div className="flex flex-wrap gap-1.5">
        <span
          className={`px-2 py-0.5 rounded-md text-[10px] font-medium border ${cat.bg} ${cat.text} ${cat.border}`}
        >
          {model.category}
        </span>
        {model.parameters && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {model.parameters}
          </span>
        )}
      </div>

      {/* Tag selector + Install */}
      <div className="flex items-center gap-2 mt-auto pt-1">
        {(model.tags ?? []).length > 1 && (
          <select
            value={selectedTag}
            onChange={(e) => setSelectedTag(e.target.value)}
            className="flex-1 min-w-0 px-2 py-1.5 rounded-lg text-[11px] font-mono bg-secondary text-foreground border border-border focus:outline-none focus:ring-1 focus:ring-primary/50 appearance-none cursor-pointer"
          >
            {(model.tags ?? []).map((tag) => (
              <option key={tag} value={tag}>
                {tag}
              </option>
            ))}
          </select>
        )}
        {(model.tags ?? []).length <= 1 && (
          <span className="flex-1 text-[11px] font-mono text-muted-foreground">
            {selectedTag}
          </span>
        )}

        {isPulling ? (
          <div className="flex-1 min-w-[120px]">
            <div className="flex items-center justify-between mb-1">
              <span className="text-[10px] text-muted-foreground truncate max-w-[100px]">
                {pullProgress?.status ?? "pulling..."}
              </span>
              <span className="text-[10px] font-mono text-primary">
                {pullProgress?.percent?.toFixed(0) ?? 0}%
              </span>
            </div>
            <div className="h-1.5 rounded-full bg-secondary overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300"
                style={{
                  width: `${pullProgress?.percent ?? 0}%`,
                  boxShadow: "0 0 10px rgba(99, 102, 241, 0.4), 0 0 20px rgba(99, 102, 241, 0.2)",
                }}
              />
            </div>
          </div>
        ) : installed ? (
          <button
            disabled
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-secondary text-muted-foreground flex items-center gap-1.5 cursor-default"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polyline points="20 6 9 17 4 12" />
            </svg>
            Installed
          </button>
        ) : (
          <button
            onClick={() => onInstall(`${model.name}:${selectedTag}`)}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors"
          >
            Install
          </button>
        )}
      </div>
    </div>
  );
}

/* ── Main Page ───────────────────────────────────────────── */

export function ModelsPage() {
  const {
    installed,
    marketplace,
    pulling,
    loading,
    error,
    pullModel,
    deleteModel,
    isInstalled,
  } = useModelMarketplace();

  const [activeCategory, setActiveCategory] = useState<Category>("all");
  const [search, setSearch] = useState("");
  const [currentModel, setCurrentModel] = useState<string | null>(null);

  // Fetch current model
  const fetchCurrentModel = useCallback(async () => {
    try {
      const res = await fetch("/api/models/current");
      if (!res.ok) return;
      const data = await res.json();
      setCurrentModel(data.model ?? null);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    fetchCurrentModel();
  }, [fetchCurrentModel]);

  const switchModel = useCallback(
    async (name: string) => {
      try {
        await fetch("/api/models/switch", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ model: name, backend: "local" }),
        });
        setCurrentModel(name);
      } catch {
        // ignore
      }
    },
    []
  );

  const isActiveModel = useCallback(
    (name: string) => {
      if (!currentModel) return false;
      return currentModel === name || currentModel.startsWith(name.split(":")[0]);
    },
    [currentModel]
  );

  // Filter marketplace models
  const filteredMarketplace = marketplace.filter((m) => {
    if (activeCategory !== "all" && m.category !== activeCategory) return false;
    if (search && !m.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  if (loading) {
    return (
      <div className="p-6 space-y-6 h-full overflow-y-auto scrollbar-none">
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <div key={i} className="glass rounded-xl p-5 h-44 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-8 h-full overflow-y-auto scrollbar-none">
      {/* Error banner */}
      {error && (
        <div className="rounded-xl bg-destructive/10 border border-destructive/20 px-4 py-3 text-xs text-destructive">
          {error}
        </div>
      )}

      {/* ── Section 0: Compute Nodes ──────────────────────────── */}
      <HostsPanel />

      {/* ── Section 0.5: Cloud Providers ───────────────────────── */}
      <ProvidersPanel />

      {/* ── Section 1: Installed Models ───────────────────────── */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight text-foreground">Installed Models</h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              {installed.length} model{installed.length !== 1 ? "s" : ""} installed locally via Ollama
            </p>
          </div>
        </div>

        {installed.length === 0 ? (
          <div className="glass rounded-xl p-8 flex flex-col items-center justify-center text-center">
            <div className="w-14 h-14 rounded-2xl glass flex items-center justify-center mb-4 glow-primary">
              <svg
                width="24"
                height="24"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
                className="text-primary"
              >
                <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
                <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
                <line x1="12" y1="22.08" x2="12" y2="12" />
              </svg>
            </div>
            <h3 className="text-sm font-medium text-foreground mb-1">No models installed</h3>
            <p className="text-xs text-muted-foreground">
              Browse the model store below to install your first model
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
            {installed.map((model) => (
              <InstalledModelCard
                key={model.name}
                model={model}
                isActive={isActiveModel(model.name)}
                onUse={() => switchModel(model.name)}
                onDelete={() => deleteModel(model.name)}
              />
            ))}
          </div>
        )}
      </div>

      {/* ── Section 2: Model Store ────────────────────────────── */}
      <div>
        <div className="mb-4">
          <h2 className="text-base font-semibold tracking-tight text-foreground">Model Store</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">
            Browse and install AI models from the Ollama registry
          </p>
        </div>

        {/* Filters */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 mb-4">
          {/* Category tabs */}
          <div className="flex items-center gap-1 bg-secondary/50 rounded-lg p-1">
            {CATEGORIES.map((cat) => (
              <button
                key={cat}
                onClick={() => setActiveCategory(cat)}
                className={`px-3 py-1.5 rounded-md text-[11px] font-medium transition-all duration-200 capitalize ${
                  activeCategory === cat
                    ? "bg-primary/15 text-primary border-glow"
                    : "text-muted-foreground hover:text-foreground hover:bg-secondary"
                }`}
              >
                {cat}
              </button>
            ))}
          </div>

          {/* Search */}
          <div className="relative flex-1 max-w-xs">
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            >
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search models..."
              className="w-full pl-9 pr-3 py-2 rounded-lg text-xs glass-subtle text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
            />
          </div>
        </div>

        {/* Model grid */}
        {filteredMarketplace.length === 0 ? (
          <div className="glass rounded-xl p-8 flex flex-col items-center justify-center text-center">
            <svg
              width="24"
              height="24"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="text-muted-foreground mb-3"
            >
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
            <h3 className="text-sm font-medium text-foreground mb-1">No models found</h3>
            <p className="text-xs text-muted-foreground">
              {search
                ? `No models matching "${search}"`
                : "No models available in this category"}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
            {filteredMarketplace.map((model) => {
              const pullKey = Array.from(pulling.keys()).find(
                (k) => k.startsWith(model.name + ":") || k === model.name
              );
              const progress = pullKey ? pulling.get(pullKey) : undefined;

              return (
                <MarketplaceCard
                  key={model.name}
                  model={model}
                  installed={isInstalled(model.name)}
                  isPulling={!!progress}
                  pullProgress={progress}
                  onInstall={(tag) => pullModel(tag)}
                />
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
