import { useState, useEffect, useCallback } from "react";
import { useHuggingFaceModels, useModelMarketplace } from "@/hooks/useModelMarketplace";
import type { InstalledModel, MarketplaceModel } from "@/types/models";
import type { OllamaHost } from "@/types/hosts";
import { HostsPanel } from "@/components/Models/HostsPanel";
import { ProvidersPanel } from "@/components/Models/ProvidersPanel";
import { toastSuccess, toastError, toastInfo } from "@/hooks/useToast";
import { useSystemStats, type HostTelemetry } from "@/hooks/useSystemStats";
import { Badge } from "@/components/ui/badge";
import { useHosts } from "@/hooks/useHosts";

/* ── Helpers ─────────────────────────────────────────────── */

const CATEGORIES = ["all", "general", "code", "vision", "embedding"] as const;
type Category = (typeof CATEGORIES)[number];
type CatalogSource = "verified" | "huggingface";

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

function formatCompactNumber(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

function formatHardwareBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** index).toFixed(index > 2 ? 1 : 0)} ${units[index]}`;
}

function ActiveCompute({ telemetry, error }: { telemetry: HostTelemetry | null; error: string | null }) {
  if (!telemetry) return null;
  const stats = telemetry.system;
  const remote = telemetry.source !== "local";
  return (
    <section className="border-y border-border py-4">
      <div className="flex flex-col md:flex-row md:items-start justify-between gap-3 mb-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-base font-semibold">Active Compute · {telemetry.host.name}</h2>
            <Badge variant={remote ? "default" : "secondary"}>{remote ? "Remote" : "Local"}</Badge>
          </div>
          <p className="text-xs text-muted-foreground mt-1">
            {telemetry.host.host}:{telemetry.host.port} · {telemetry.latency_ms} ms
            {telemetry.ollama_version ? ` · Ollama ${telemetry.ollama_version}` : ""}
          </p>
        </div>
        <Badge variant="outline">{telemetry.source === "agent" ? "Full telemetry" : telemetry.source === "local" ? "Local telemetry" : "Ollama telemetry"}</Badge>
      </div>
      {stats ? (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-3">
          <div><p className="text-xs text-muted-foreground">Processor</p><p className="text-sm mt-1 truncate">{stats.cpu.model}</p><p className="text-xs text-muted-foreground">{stats.cpu.cores} cores · {stats.cpu.threads} threads</p></div>
          <div><p className="text-xs text-muted-foreground">Memory</p><p className="text-sm mt-1">{formatHardwareBytes(stats.memory.total_bytes)}</p><p className="text-xs text-muted-foreground">{formatHardwareBytes(stats.memory.available_bytes)} available</p></div>
          <div><p className="text-xs text-muted-foreground">GPU</p><p className="text-sm mt-1 truncate">{stats.gpu.length ? stats.gpu.map((gpu) => gpu.name).join(", ") : "No GPU reported"}</p><p className="text-xs text-muted-foreground">{formatHardwareBytes(stats.gpu.reduce((total, gpu) => total + gpu.memory_total_bytes, 0))} total VRAM</p></div>
          <div><p className="text-xs text-muted-foreground">Platform</p><p className="text-sm mt-1">{stats.os} / {stats.arch}</p><p className="text-xs text-muted-foreground">Up {stats.uptime}</p></div>
        </div>
      ) : (
        <div className="border-l-2 border-amber-400/50 pl-3">
          <p className="text-sm text-foreground">Full hardware details unavailable</p>
          <p className="text-xs text-muted-foreground mt-1">{error ?? telemetry.message}</p>
          {telemetry.running_models.length > 0 && (
            <div className="mt-3 space-y-1">
              {telemetry.running_models.map((model) => (
                <p key={model.name} className="text-xs text-muted-foreground">
                  Running {model.name} · {formatHardwareBytes(model.vram_bytes)} VRAM · {formatHardwareBytes(model.size_bytes)} loaded
                </p>
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function MachineTabs({ hosts, selectedHostId, onSelect }: { hosts: OllamaHost[]; selectedHostId?: string; onSelect: (host: OllamaHost) => void }) {
  const orderedHosts = [...hosts].sort((left, right) => Number(right.id === "local") - Number(left.id === "local") || left.name.localeCompare(right.name));
  return (
    <section>
      <div className="flex items-end justify-between gap-4 mb-3">
        <div>
          <h2 className="text-base font-semibold">Model Marketplaces</h2>
          <p className="text-xs text-muted-foreground mt-1">Choose the machine that will store and run the model.</p>
        </div>
      </div>
      <div className="flex items-center gap-1 overflow-x-auto border-b border-border" role="tablist" aria-label="Model marketplace machine">
        {orderedHosts.map((host) => {
          const selected = host.id === selectedHostId;
          const offline = host.status !== "online";
          return (
            <button
              key={host.id}
              type="button"
              role="tab"
              aria-selected={selected}
              disabled={offline}
              onClick={() => onSelect(host)}
              className={`min-w-fit px-4 py-3 -mb-px border-b-2 text-left transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${selected ? "border-primary text-foreground bg-primary/[0.04]" : "border-transparent text-muted-foreground hover:text-foreground"}`}
            >
              <span className="flex items-center gap-2 text-sm font-medium">
                <span className={`w-1.5 h-1.5 rounded-full ${host.status === "online" ? "bg-emerald-400" : "bg-red-400"}`} />
                {host.id === "local" ? "This machine" : host.name}
                {host.active && <Badge variant="outline">Active</Badge>}
              </span>
              <span className="block text-xs text-muted-foreground mt-1">{host.host}:{host.port} · {(host.models ?? []).length} models</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

interface HardwareEstimate {
  fit: string;
  fitTone: "good" | "warning" | "bad";
  vram: string;
  download: string;
  context: string;
  speed: string;
}

function parameterBillions(value: string): number {
  const mixture = value.match(/(\d+(?:\.\d+)?)x(\d+(?:\.\d+)?)/i);
  if (mixture) return Number(mixture[1]) * Number(mixture[2]);
  const match = value.match(/\d+(?:\.\d+)?/);
  return match ? Number(match[0]) : 7;
}

function estimateHardware(name: string, parameters: string, gpuMemory: number[], exactDownloadGB?: number): HardwareEstimate {
  const parameterCount = parameterBillions(parameters);
  const downloadGB = exactDownloadGB ?? parameterCount * 0.6;
  const estimatedVRAM = downloadGB * 1.15;
  const totalGPU = gpuMemory.reduce((total, value) => total + value, 0);
  const largestGPU = Math.max(0, ...gpuMemory);
  let fit = "CPU only";
  let fitTone: HardwareEstimate["fitTone"] = "warning";
  if (largestGPU > 0 && estimatedVRAM <= largestGPU * 0.92) {
    fit = "Excellent fit";
    fitTone = "good";
  } else if (gpuMemory.length > 1 && estimatedVRAM <= totalGPU * 0.92) {
    fit = `Uses ${gpuMemory.length === 2 ? "both" : gpuMemory.length} GPUs`;
    fitTone = "good";
  } else if (totalGPU > 0 && estimatedVRAM <= totalGPU * 1.5) {
    fit = "Needs CPU offload";
  } else if (totalGPU > 0) {
    fit = "Poor hardware fit";
    fitTone = "bad";
  }
  const normalizedName = name.toLowerCase();
  const context = normalizedName.includes("llama3.1") || normalizedName.includes("qwen") || normalizedName.includes("deepseek") || normalizedName.includes("phi3")
    ? "128K ctx"
    : "32K ctx";
  const speed = parameterCount <= 8 ? "Fast" : parameterCount <= 20 ? "Balanced" : parameterCount <= 40 ? "Moderate" : "Slow";
  return {
    fit,
    fitTone,
    vram: `~${estimatedVRAM.toFixed(1)} GB VRAM`,
    download: exactDownloadGB ? `${exactDownloadGB.toFixed(1)} GB` : `~${downloadGB.toFixed(1)} GB download`,
    context,
    speed,
  };
}

function HardwareFit({ estimate }: { estimate: HardwareEstimate }) {
  const fitClass = estimate.fitTone === "good"
    ? "border-emerald-400/25 bg-emerald-400/10 text-emerald-300"
    : estimate.fitTone === "bad"
      ? "border-red-400/25 bg-red-400/10 text-red-300"
      : "border-amber-400/25 bg-amber-400/10 text-amber-300";
  return (
    <div className="space-y-1.5">
      <Badge variant="outline" className={fitClass}>{estimate.fit}</Badge>
      <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span>{estimate.vram}</span>
        <span>{estimate.context}</span>
        <span>{estimate.speed} speed</span>
        <span>{estimate.download}</span>
      </div>
    </div>
  );
}

/* ── Installed Model Card ────────────────────────────────── */

function InstalledModelCard({
  model,
  isActive,
  onUse,
  onDelete,
  estimate,
  remote,
}: {
  model: InstalledModel;
  isActive: boolean;
  onUse: () => void;
  onDelete: () => void;
  estimate: HardwareEstimate;
  remote: boolean;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <div
      className={`px-3 py-4 grid grid-cols-1 lg:grid-cols-[minmax(180px,1fr)_minmax(220px,1.25fr)_minmax(180px,auto)] gap-4 items-center hover:bg-surface-hover ${
        isActive ? "bg-primary/[0.04]" : ""
      }`}
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground truncate">{model.name}</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            Modified {formatDate(model.modified)}
          </p>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <Badge variant={isActive ? "default" : "secondary"}>{isActive ? "Running" : "Installed"}</Badge>
          {remote && <Badge variant="outline">Remote</Badge>}
          <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-secondary text-muted-foreground">
            {model.size_human}
          </span>
        </div>
      </div>

      <HardwareFit estimate={estimate} />

      {/* Info chips */}
      <div className="flex flex-wrap gap-1.5 lg:hidden">
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
      <div className="flex items-center justify-end gap-2">
        {!isActive && (
          <button
            onClick={onUse}
            className="px-3 py-1.5 rounded-md text-xs font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
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
            className="px-3 py-1.5 rounded-md text-xs font-medium text-muted-foreground hover:bg-destructive/10 hover:text-destructive border border-border transition-colors"
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
  gpuMemory,
  targetName,
}: {
  model: MarketplaceModel;
  installed: boolean;
  isPulling: boolean;
  pullProgress?: { status: string; percent: number };
  onInstall: (tag: string) => void;
  gpuMemory: number[];
  targetName: string;
}) {
  const [selectedTag, setSelectedTag] = useState(model.tags[0] ?? "");
  const cat = categoryStyle(model.category);
  const selectedParameters = model.source === "huggingface"
    ? model.parameters
    : /\d/.test(selectedTag) ? selectedTag.toUpperCase() : model.parameters;
  const pullName = model.pull_name ?? model.name;
  const installName = selectedTag ? `${pullName}:${selectedTag}` : pullName;
  const estimate = estimateHardware(installName, selectedParameters, gpuMemory);

  return (
    <div className="px-3 py-4 grid grid-cols-1 lg:grid-cols-[minmax(180px,1fr)_minmax(220px,1.3fr)_minmax(220px,auto)] gap-4 items-center hover:bg-surface-hover">
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          {model.url ? (
            <a href={model.url} target="_blank" rel="noreferrer" className="text-sm font-medium text-foreground hover:text-primary break-all">
              {model.name}
            </a>
          ) : (
            <h3 className="text-sm font-medium text-foreground">{model.name}</h3>
          )}
          {model.source === "huggingface" && (
            <p className="text-[10px] text-muted-foreground mt-1">Community model · verify license and model card</p>
          )}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <Badge variant={installed ? "secondary" : "outline"}>{installed ? "Installed" : "Available"}</Badge>
          {model.source === "huggingface" && <Badge variant="outline">Hugging Face</Badge>}
          {model.recommended && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-semibold uppercase tracking-wider bg-primary/15 text-primary border border-primary/25 glow-sm">
              Recommended
            </span>
          )}
        </div>
      </div>

      {/* Description */}
      <div>
        <p className="text-xs text-muted-foreground leading-relaxed line-clamp-2">
          {model.description}
        </p>
        {model.source === "huggingface" && (
          <p className="text-[10px] text-muted-foreground mt-1.5">
            {formatCompactNumber(model.downloads ?? 0)} downloads · {formatCompactNumber(model.likes ?? 0)} likes · {model.license ?? "unknown"} license
          </p>
        )}
      </div>

      <HardwareFit estimate={estimate} />

      {/* Badges */}
      <div className="hidden">
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
      <div className="flex items-center justify-end gap-2">
        {(model.tags ?? []).length > 1 && (
          <div className="relative flex-1 min-w-0">
            <select
              value={selectedTag}
              onChange={(e) => setSelectedTag(e.target.value)}
              className="w-full px-2 py-1.5 pr-7 rounded-lg text-[11px] font-mono bg-secondary text-foreground border border-border focus:outline-none focus:ring-1 focus:ring-primary/50 appearance-none cursor-pointer"
            >
              {(model.tags ?? []).map((tag) => (
                <option key={tag} value={tag}>
                  {tag}
                </option>
              ))}
            </select>
            <svg className="absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <path d="M6 9l6 6 6-6" />
            </svg>
          </div>
        )}
        {(model.tags ?? []).length <= 1 && (
          <span className="flex-1 text-[11px] font-mono text-muted-foreground">
            {selectedTag || "Default quantization"}
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
            onClick={() => onInstall(installName)}
            className="px-3 py-1.5 rounded-md text-xs font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            Install on {targetName}
          </button>
        )}
      </div>
    </div>
  );
}

/* ── Main Page ───────────────────────────────────────────── */

export function ModelsPage() {
  const { hosts, activateHost } = useHosts();
  const [selectedHostId, setSelectedHostId] = useState<string>();
  const effectiveHostId = selectedHostId ?? hosts.find((host) => host.active)?.id ?? hosts.find((host) => host.status === "online")?.id;
  const selectedHost = hosts.find((host) => host.id === effectiveHostId);
  const { stats, telemetry, error: telemetryError } = useSystemStats(10000, effectiveHostId);
  const {
    installed,
    marketplace,
    pulling,
    loading,
    error,
    pullModel,
    deleteModel,
    isInstalled,
  } = useModelMarketplace(effectiveHostId, selectedHost?.name);

  const [activeCategory, setActiveCategory] = useState<Category>("all");
  const [search, setSearch] = useState("");
  const [catalogSource, setCatalogSource] = useState<CatalogSource>("verified");
  const [currentModel, setCurrentModel] = useState<string | null>(null);
  const {
    models: huggingFaceModels,
    loading: huggingFaceLoading,
    error: huggingFaceError,
  } = useHuggingFaceModels(catalogSource === "huggingface" ? search : null);
  const gpuMemory = (stats?.gpu ?? []).map((gpu) => gpu.memory_total_bytes / 1024 ** 3);
  const remoteModels = selectedHost?.id !== "local";

  useEffect(() => {
    if (!selectedHostId && effectiveHostId) setSelectedHostId(effectiveHostId);
  }, [effectiveHostId, selectedHostId]);

  // Fetch current model
  const fetchCurrentModel = useCallback(async () => {
    try {
      const res = await fetch("/api/models/current");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setCurrentModel(data.model ?? null);
    } catch {
      toastError("Model status unavailable", "Could not load the currently running model.");
    }
  }, []);

  useEffect(() => {
    fetchCurrentModel();
  }, [fetchCurrentModel]);

  const switchModel = useCallback(
    async (name: string) => {
      try {
        if (selectedHost && !selectedHost.active) {
          await activateHost(selectedHost.id);
        }
        const response = await fetch("/api/models/switch", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ model: name, backend: "local" }),
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        setCurrentModel(name);
      } catch {
        toastError("Error", "Failed to switch model");
      }
    },
    [activateHost, selectedHost]
  );

  const isActiveModel = useCallback(
    (name: string) => {
      if (!currentModel || !selectedHost?.active) return false;
      return currentModel === name || currentModel.startsWith(name.split(":")[0]);
    },
    [currentModel, selectedHost]
  );

  // Filter marketplace models
  const activeMarketplace = catalogSource === "huggingface" ? huggingFaceModels : marketplace;
  const filteredMarketplace = activeMarketplace.filter((m) => {
    if (activeCategory !== "all" && m.category !== activeCategory) return false;
    if (catalogSource === "verified" && search && !m.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  if (loading) {
    return (
      <div className="p-6 space-y-6 h-full overflow-y-auto scrollbar-none">
        <MachineTabs hosts={hosts} selectedHostId={effectiveHostId} onSelect={(host) => setSelectedHostId(host.id)} />
        <div className="divide-y divide-border border-y border-border">
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <div key={i} className="h-24 bg-secondary animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="px-6 py-5 max-[640px]:px-4 space-y-8 h-full overflow-y-auto scrollbar-none">
      <MachineTabs hosts={hosts} selectedHostId={effectiveHostId} onSelect={(host) => setSelectedHostId(host.id)} />
      {/* Error banner */}
      {(error || huggingFaceError) && (
        <div className="rounded-xl bg-destructive/10 border border-destructive/20 px-4 py-3 text-xs text-destructive">
          {error || `Hugging Face search failed: ${huggingFaceError}`}
        </div>
      )}

      <ActiveCompute telemetry={telemetry} error={telemetryError} />

      {/* ── Section 0: Compute Nodes ──────────────────────────── */}
      <div><HostsPanel /></div>

      {/* ── Section 0.5: Cloud Providers ───────────────────────── */}
      <div><ProvidersPanel /></div>

      {/* ── Section 1: Installed Models ───────────────────────── */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight text-foreground">{selectedHost?.id === "local" ? "This Machine" : selectedHost?.name ?? "Selected Host"} Models</h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              {installed.length} model{installed.length !== 1 ? "s" : ""} installed on {selectedHost?.name ?? "the selected host"}
            </p>
          </div>
        </div>

        {installed.length === 0 ? (
          <div className="border-y border-border p-8 flex flex-col items-center justify-center text-center">
            <div className="w-10 h-10 rounded-md surface-raised flex items-center justify-center mb-4">
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
          <div className="divide-y divide-border border-y border-border">
            {installed.map((model) => (
              <InstalledModelCard
                key={model.name}
                model={model}
                isActive={isActiveModel(model.name)}
                onUse={() => switchModel(model.name)}
                onDelete={async () => {
                  await deleteModel(model.name);
                  toastSuccess(`Deleted from ${selectedHost?.name ?? "selected host"}`, model.name);
                }}
                remote={remoteModels}
                estimate={estimateHardware(model.name, model.parameters, gpuMemory, model.size / 1024 ** 3)}
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
            {catalogSource === "verified"
              ? `Browse verified Ollama models and install directly on ${selectedHost?.name ?? "the selected machine"}`
              : `Search community GGUF models on Hugging Face for ${selectedHost?.name ?? "the selected machine"}`}
          </p>
        </div>

        <div className="flex items-center gap-1 border-b border-border mb-4" role="tablist" aria-label="Model catalog source">
          {([
            ["verified", "Verified"],
            ["huggingface", "Hugging Face GGUF"],
          ] as const).map(([source, label]) => (
            <button
              key={source}
              type="button"
              role="tab"
              aria-selected={catalogSource === source}
              onClick={() => {
                setCatalogSource(source);
                setActiveCategory("all");
              }}
              className={`px-3 py-2 text-xs font-medium transition-colors border-b-2 -mb-px ${
                catalogSource === source
                  ? "text-primary border-primary"
                  : "text-muted-foreground hover:text-foreground border-transparent"
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        {/* Filters */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 mb-4">
          {/* Category tabs */}
          <div className="flex items-center gap-1 border-b border-border">
            {CATEGORIES.map((cat) => (
              <button
                key={cat}
                onClick={() => setActiveCategory(cat)}
                className={`px-3 py-2 text-xs font-medium transition-colors capitalize border-b-2 -mb-px ${
                  activeCategory === cat
                    ? "text-primary border-primary"
                    : "text-muted-foreground hover:text-foreground border-transparent"
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
              placeholder={catalogSource === "huggingface" ? "Search Hugging Face..." : "Search models..."}
              className="w-full pl-9 pr-3 py-2 rounded-md text-sm field-control placeholder:text-muted-foreground"
            />
          </div>
        </div>

        {/* Model grid */}
        {catalogSource === "huggingface" && huggingFaceLoading ? (
          <div className="border-y border-border p-8 flex items-center justify-center text-xs text-muted-foreground">
            Searching Hugging Face…
          </div>
        ) : filteredMarketplace.length === 0 ? (
          <div className="border-y border-border p-8 flex flex-col items-center justify-center text-center">
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
          <div className="divide-y divide-border border-y border-border">
            {filteredMarketplace.map((model) => {
              const modelPullName = model.pull_name ?? model.name;
              const pullKey = Array.from(pulling.keys()).find(
                (k) => k.startsWith(modelPullName + ":") || k === modelPullName
              );
              const progress = pullKey ? pulling.get(pullKey) : undefined;

              return (
                <div key={`${model.source ?? "ollama"}:${model.name}`}>
                  <MarketplaceCard
                    model={model}
                    installed={isInstalled(modelPullName)}
                    isPulling={!!progress}
                    pullProgress={progress}
                    onInstall={(tag) => {
                      pullModel(tag);
                      toastInfo(`Downloading to ${selectedHost?.name ?? "selected host"}`, tag);
                    }}
                    gpuMemory={gpuMemory}
                    targetName={selectedHost?.id === "local" ? "this machine" : selectedHost?.name ?? "host"}
                  />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
