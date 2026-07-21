import { useState } from "react";
import { useHosts } from "@/hooks/useHosts";
import type { OllamaHost } from "@/types/hosts";
import { toastSuccess, toastInfo, toastError } from "@/hooks/useToast";

/* ── Status dot colors ──────────────────────────────────── */

function StatusDot({ status }: { status: OllamaHost["status"] }) {
  const colors = {
    online: "bg-green-400",
    offline: "bg-red-400",
    checking: "bg-yellow-400 animate-pulse",
  };

  return <span className={`inline-block w-2 h-2 rounded-full shrink-0 ${colors[status]}`} />;
}

/* ── Host Card ──────────────────────────────────────────── */

function HostCard({
  host,
  onActivate,
  onRemove,
  onTelemetryChange,
}: {
  host: OllamaHost;
  onActivate: () => void;
  onRemove: () => void;
  onTelemetryChange: (port: number, token?: string) => Promise<void>;
}) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [editingTelemetry, setEditingTelemetry] = useState(false);
  const [telemetryPort, setTelemetryPort] = useState(String(host.telemetry_port || 3000));
  const [telemetryToken, setTelemetryToken] = useState("");
  const isOffline = host.status === "offline";
  const isLocal = host.id === "local";

  const borderColor = host.active
    ? "border-primary/30 glow-sm"
    : host.status === "online"
      ? "border-l-green-500/40"
      : "border-l-red-500/40";

  return (
    <div
      className={`surface-panel rounded-xl p-4 flex flex-col gap-2.5 transition-all duration-200 border-l-[3px] ${borderColor} ${
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

      {editingTelemetry && (
        <div className="grid grid-cols-1 sm:grid-cols-[auto_88px_minmax(140px,1fr)_auto_auto] items-center gap-2 border-t border-border pt-2">
          <label className="text-xs text-muted-foreground" htmlFor={`telemetry-${host.id}`}>AxiOS port</label>
          <input
            id={`telemetry-${host.id}`}
            value={telemetryPort}
            onChange={(event) => setTelemetryPort(event.target.value)}
            className="w-24 rounded-md field-control px-2 py-1 text-xs font-mono"
          />
          <input
            type="password"
            value={telemetryToken}
            onChange={(event) => setTelemetryToken(event.target.value)}
            placeholder={host.has_telemetry_token ? "New token (optional)" : "Telemetry token"}
            aria-label="Telemetry bearer token"
            autoComplete="new-password"
            className="min-w-0 rounded-md field-control px-2 py-1 text-xs font-mono"
          />
          <button
            type="button"
            onClick={async () => {
              const port = Number(telemetryPort);
              if (!Number.isInteger(port) || port < 1 || port > 65535) return;
              await onTelemetryChange(port, telemetryToken || undefined);
              setTelemetryToken("");
              setEditingTelemetry(false);
            }}
            className="rounded-md bg-primary px-2.5 py-1 text-xs text-primary-foreground"
          >
            Save
          </button>
          <button type="button" onClick={() => setEditingTelemetry(false)} className="text-xs text-muted-foreground hover:text-foreground">Cancel</button>
        </div>
      )}

      {/* Info */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="text-[11px] font-mono text-muted-foreground">
          {host.host}:{host.port}
        </span>
        <span className="text-xs text-muted-foreground">
          telemetry :{host.telemetry_port || 3000}
        </span>
        <span className={`text-xs ${isLocal || host.has_telemetry_token ? "text-emerald-400" : "text-amber-300"}`}>
          {isLocal ? "local telemetry" : host.has_telemetry_token ? "authenticated" : "token required"}
        </span>
        <span className="text-[10px] text-muted-foreground">
          {(host.models ?? []).length} model{(host.models ?? []).length !== 1 ? "s" : ""}
        </span>
        {host.gpu_info && (
          <span className="px-2 py-0.5 rounded-md text-[10px] bg-secondary text-muted-foreground">
            {host.gpu_info}
          </span>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 mt-auto pt-1">
        {!isLocal && (
          <button
            type="button"
            onClick={() => setEditingTelemetry((current) => !current)}
            className="px-3 py-1.5 rounded-md text-xs font-medium border border-border text-muted-foreground hover:text-foreground hover:bg-secondary"
          >
            Telemetry
          </button>
        )}
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
  onAdd: (name: string, host: string, port: number, telemetryPort?: number, telemetryToken?: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("11434");
  const [telemetryPort, setTelemetryPort] = useState("3000");
  const [telemetryToken, setTelemetryToken] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!name.trim() || !host.trim()) {
      setFormError("Name and host are required");
      return;
    }
    const portNum = parseInt(port, 10);
    if (isNaN(portNum) || portNum < 1 || portNum > 65535) {
      setFormError("Ollama port must be between 1 and 65535");
      return;
    }
    const telemetryPortNum = parseInt(telemetryPort, 10);
    if (isNaN(telemetryPortNum) || telemetryPortNum < 1 || telemetryPortNum > 65535) {
      setFormError("Telemetry port must be between 1 and 65535");
      return;
    }
    setSubmitting(true);
    setFormError(null);
    try {
      await onAdd(name.trim(), host.trim(), portNum, telemetryPortNum, telemetryToken.trim());
      toastSuccess("Connected", `${name.trim()} at ${host.trim()}:${portNum}`);
      onCancel();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to add host";
      toastError("Error", msg);
      setFormError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="surface-panel rounded-xl p-4 border-glow">
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

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mb-2">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Name (e.g. Jetson Nano)"
          className="px-3 py-2 rounded-lg text-sm field-control placeholder:text-muted-foreground"
        />
        <input
          type="text"
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="Host/IP (e.g. 192.168.1.50)"
          className="px-3 py-2 rounded-lg text-xs font-mono field-control placeholder:text-muted-foreground"
        />
        <input
          type="text"
          value={port}
          onChange={(e) => setPort(e.target.value)}
          placeholder="Ollama port"
          aria-label="Ollama port"
          className="px-3 py-2 rounded-md text-xs font-mono field-control placeholder:text-muted-foreground"
        />
        <input
          type="text"
          value={telemetryPort}
          onChange={(e) => setTelemetryPort(e.target.value)}
          placeholder="AxiOS telemetry port"
          aria-label="AxiOS telemetry port"
          className="px-3 py-2 rounded-md text-xs font-mono field-control placeholder:text-muted-foreground"
        />
        <input
          type="password"
          value={telemetryToken}
          onChange={(e) => setTelemetryToken(e.target.value)}
          placeholder="Telemetry token (optional)"
          aria-label="Telemetry bearer token"
          autoComplete="new-password"
          className="px-3 py-2 rounded-md text-xs font-mono field-control placeholder:text-muted-foreground sm:col-span-2"
        />
      </div>
      <p className="text-xs text-muted-foreground mb-3">
        Full hardware details require the same token configured on the remote AxiOS telemetry agent.
      </p>

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
  const { hosts, loading, error, addHost, removeHost, activateHost, updateTelemetryPort, checkHealth } = useHosts();
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
            <div key={i} className="surface-panel rounded-xl p-4 h-32 animate-pulse" />
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
        <div className="surface-panel rounded-xl p-6 flex flex-col items-center justify-center text-center">
          <div className="w-12 h-12 rounded-2xl surface-raised flex items-center justify-center mb-3">
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
              onActivate={() => activateHost(host.id).then(() => toastSuccess("Active", host.name)).catch((err) => toastError("Error", err instanceof Error ? err.message : "Activation failed"))}
              onRemove={() => removeHost(host.id).then(() => toastInfo("Removed", host.name)).catch((err) => toastError("Error", err instanceof Error ? err.message : "Remove failed"))}
              onTelemetryChange={(port, token) => updateTelemetryPort(host.id, port, token).then(() => toastSuccess("Telemetry updated", `${host.name} uses authenticated port ${port}`)).catch((err) => {
                toastError("Error", err instanceof Error ? err.message : "Update failed");
                throw err;
              })}
            />
          ))}
        </div>
      )}
    </div>
  );
}
