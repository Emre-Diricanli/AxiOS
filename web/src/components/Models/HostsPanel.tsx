import { useState } from "react";
import { useHosts } from "@/hooks/useHosts";
import type { OllamaHost } from "@/types/hosts";

/* ── Status dot colors ──────────────────────────────────── */

function StatusDot({ status }: { status: OllamaHost["status"] }) {
  const colors = {
    online: "bg-green-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]",
    offline: "bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]",
    checking: "bg-yellow-400 shadow-[0_0_8px_rgba(250,204,21,0.6)] animate-pulse",
  };

  return <span className={`inline-block w-2 h-2 rounded-full shrink-0 ${colors[status]}`} />;
}

/* ── Host Card ──────────────────────────────────────────── */

function HostCard({
  host,
  onActivate,
  onRemove,
}: {
  host: OllamaHost;
  onActivate: () => void;
  onRemove: () => void;
}) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const isOffline = host.status === "offline";

  const borderColor = host.active
    ? "border-primary/30 glow-sm"
    : host.status === "online"
      ? "border-l-green-500/40"
      : "border-l-red-500/40";

  return (
    <div
      className={`glass rounded-xl p-4 flex flex-col gap-2.5 transition-all duration-200 border-l-[3px] ${borderColor} ${
        isOffline ? "opacity-60" : ""
      }`}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <StatusDot status={host.status} />
          <h4 className="text-sm font-bold text-foreground truncate">{host.name}</h4>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {host.active && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-semibold uppercase tracking-wider bg-primary/15 text-primary border border-primary/25 glow-sm">
              Active
            </span>
          )}
          <span
            className={`px-2 py-0.5 rounded-full text-[9px] font-medium capitalize ${
              host.status === "online"
                ? "bg-green-500/15 text-green-400 border border-green-500/25"
                : host.status === "offline"
                  ? "bg-red-500/15 text-red-400 border border-red-500/25"
                  : "bg-yellow-500/15 text-yellow-400 border border-yellow-500/25"
            }`}
          >
            {host.status}
          </span>
        </div>
      </div>

      {/* Info */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="text-[11px] font-mono text-muted-foreground">
          {host.host}:{host.port}
        </span>
        <span className="text-[10px] text-muted-foreground">
          {host.models.length} model{host.models.length !== 1 ? "s" : ""}
        </span>
        {host.gpu_info && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {host.gpu_info}
          </span>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 mt-auto pt-1">
        {!host.active && host.status === "online" && (
          <button
            onClick={onActivate}
            className="flex-1 px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors"
          >
            Use
          </button>
        )}
        {confirmRemove ? (
          <div className="flex items-center gap-1.5 flex-1">
            <span className="text-[10px] text-destructive whitespace-nowrap">Remove?</span>
            <button
              onClick={() => setConfirmRemove(false)}
              className="px-2 py-1 rounded-md text-[10px] font-medium bg-secondary text-muted-foreground hover:bg-secondary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => {
                onRemove();
                setConfirmRemove(false);
              }}
              className="px-2 py-1 rounded-md text-[10px] font-medium bg-destructive/15 text-destructive border border-destructive/25 hover:bg-destructive/25 transition-colors"
            >
              Remove
            </button>
          </div>
        ) : (
          <button
            onClick={() => {
              if (host.active) return;
              setConfirmRemove(true);
            }}
            disabled={host.active}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              host.active
                ? "bg-secondary text-muted-foreground/40 cursor-not-allowed"
                : "bg-secondary text-muted-foreground hover:bg-destructive/15 hover:text-destructive hover:border-destructive/25 border border-transparent"
            }`}
          >
            Remove
          </button>
        )}
      </div>
    </div>
  );
}

/* ── Add Host Form ──────────────────────────────────────── */

function AddHostForm({
  onAdd,
  onCancel,
}: {
  onAdd: (name: string, host: string, port: number) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("11434");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!name.trim() || !host.trim()) {
      setFormError("Name and host are required");
      return;
    }
    const portNum = parseInt(port, 10);
    if (isNaN(portNum) || portNum < 1 || portNum > 65535) {
      setFormError("Port must be between 1 and 65535");
      return;
    }
    setSubmitting(true);
    setFormError(null);
    try {
      await onAdd(name.trim(), host.trim(), portNum);
      onCancel();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Failed to add host");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="glass rounded-xl p-4 border-glow">
      <div className="flex items-center gap-2 mb-3">
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="text-primary"
        >
          <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
          <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
          <line x1="6" y1="6" x2="6.01" y2="6" />
          <line x1="6" y1="18" x2="6.01" y2="18" />
        </svg>
        <h4 className="text-sm font-semibold text-foreground">Connect New Host</h4>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto] gap-2 mb-3">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Name (e.g. Jetson Nano)"
          className="px-3 py-2 rounded-lg text-xs glass-subtle text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
        />
        <input
          type="text"
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="Host/IP (e.g. 192.168.1.50)"
          className="px-3 py-2 rounded-lg text-xs font-mono glass-subtle text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
        />
        <input
          type="text"
          value={port}
          onChange={(e) => setPort(e.target.value)}
          placeholder="Port"
          className="px-3 py-2 rounded-lg text-xs font-mono glass-subtle text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50 w-24"
        />
      </div>

      {formError && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/20 px-3 py-2 text-[11px] text-destructive mb-3">
          {formError}
        </div>
      )}

      <div className="flex items-center gap-2">
        <button
          onClick={handleSubmit}
          disabled={submitting}
          className="px-4 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors disabled:opacity-50"
        >
          {submitting ? "Connecting..." : "Connect"}
        </button>
        <button
          onClick={onCancel}
          className="px-4 py-1.5 rounded-lg text-xs font-medium bg-secondary text-muted-foreground hover:bg-secondary/80 transition-colors"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

/* ── Hosts Panel ────────────────────────────────────────── */

export function HostsPanel() {
  const { hosts, loading, error, addHost, removeHost, activateHost, checkHealth } = useHosts();
  const [showAddForm, setShowAddForm] = useState(false);
  const [checking, setChecking] = useState(false);

  const handleCheckHealth = async () => {
    setChecking(true);
    try {
      await checkHealth();
    } finally {
      setChecking(false);
    }
  };

  if (loading) {
    return (
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight text-foreground">Compute Nodes</h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">Loading hosts...</p>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2].map((i) => (
            <div key={i} className="glass rounded-xl p-4 h-32 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold tracking-tight text-foreground">
                Compute Nodes
              </h2>
              <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-secondary text-muted-foreground">
                {hosts.length}
              </span>
            </div>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              Remote Ollama hosts for distributed inference
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleCheckHealth}
            disabled={checking}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-secondary text-muted-foreground hover:bg-secondary/80 transition-colors disabled:opacity-50 flex items-center gap-1.5"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={checking ? "animate-spin" : ""}
            >
              <polyline points="23 4 23 10 17 10" />
              <polyline points="1 20 1 14 7 14" />
              <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
            </svg>
            {checking ? "Checking..." : "Check All"}
          </button>
          <button
            onClick={() => setShowAddForm(!showAddForm)}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors flex items-center gap-1.5"
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
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Add Host
          </button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="rounded-xl bg-destructive/10 border border-destructive/20 px-4 py-3 text-xs text-destructive mb-4">
          {error}
        </div>
      )}

      {/* Add Host Form */}
      {showAddForm && (
        <div className="mb-4">
          <AddHostForm onAdd={addHost} onCancel={() => setShowAddForm(false)} />
        </div>
      )}

      {/* Host Cards */}
      {hosts.length === 0 ? (
        <div className="glass rounded-xl p-6 flex flex-col items-center justify-center text-center">
          <div className="w-12 h-12 rounded-2xl glass flex items-center justify-center mb-3 glow-primary">
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="text-primary"
            >
              <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
              <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
              <line x1="6" y1="6" x2="6.01" y2="6" />
              <line x1="6" y1="18" x2="6.01" y2="18" />
            </svg>
          </div>
          <h3 className="text-sm font-medium text-foreground mb-1">No compute nodes connected</h3>
          <p className="text-xs text-muted-foreground">
            Add a remote Ollama host to distribute your inference workloads
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {hosts.map((host) => (
            <HostCard
              key={host.id}
              host={host}
              onActivate={() => activateHost(host.id)}
              onRemove={() => removeHost(host.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
