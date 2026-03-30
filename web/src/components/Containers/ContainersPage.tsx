import { useState, useCallback } from "react";
import { useDocker } from "@/hooks/useDocker";
import type { Container, ContainerStats, RunContainerRequest } from "@/types/docker";

/* ── Status helpers ─────────────────────────────────────────────────── */

function stateColor(state: string): string {
  switch (state) {
    case "running":
      return "#22c55e";
    case "paused":
      return "#f59e0b";
    default:
      return "#ef4444";
  }
}

function stateGlow(state: string): string {
  switch (state) {
    case "running":
      return "0 0 8px rgba(34,197,94,0.5)";
    case "paused":
      return "0 0 8px rgba(245,158,11,0.5)";
    default:
      return "0 0 8px rgba(239,68,68,0.5)";
  }
}

/* ── Reusable small components ──────────────────────────────────────── */

function ModalOverlay({
  children,
  onClose,
}: {
  children: React.ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-background/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative glass rounded-2xl w-full max-w-lg max-h-[80vh] overflow-y-auto scrollbar-none mx-4 p-6 shadow-2xl">
        {children}
      </div>
    </div>
  );
}

function ModalHeader({
  title,
  onClose,
}: {
  title: string;
  onClose: () => void;
}) {
  return (
    <div className="flex items-center justify-between mb-5">
      <h2 className="text-base font-semibold">{title}</h2>
      <button
        onClick={onClose}
        className="w-8 h-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M18 6L6 18M6 6l12 12" />
        </svg>
      </button>
    </div>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label className="block text-xs font-medium text-muted-foreground mb-1.5">
      {children}
    </label>
  );
}

function TextInput({
  value,
  onChange,
  placeholder,
  mono,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  mono?: boolean;
}) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className={`w-full rounded-lg bg-secondary border border-border px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-primary/40 focus:ring-1 focus:ring-primary/20 transition-colors ${
        mono ? "font-mono" : ""
      }`}
    />
  );
}

function DynamicListField({
  label,
  placeholder,
  values,
  onChange,
}: {
  label: string;
  placeholder: string;
  values: string[];
  onChange: (v: string[]) => void;
}) {
  return (
    <div>
      <FieldLabel>{label}</FieldLabel>
      <div className="space-y-2">
        {values.map((val, i) => (
          <div key={i} className="flex gap-2">
            <input
              type="text"
              value={val}
              onChange={(e) => {
                const next = [...values];
                next[i] = e.target.value;
                onChange(next);
              }}
              placeholder={placeholder}
              className="flex-1 rounded-lg bg-secondary border border-border px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-primary/40 focus:ring-1 focus:ring-primary/20 transition-colors"
            />
            <button
              onClick={() => onChange(values.filter((_, j) => j !== i))}
              className="w-8 h-9 rounded-lg flex items-center justify-center text-destructive hover:bg-destructive/10 transition-colors shrink-0"
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              >
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        ))}
        <button
          onClick={() => onChange([...values, ""])}
          className="text-xs text-primary hover:text-primary/80 transition-colors"
        >
          + Add
        </button>
      </div>
    </div>
  );
}

/* ── Run Container Modal ────────────────────────────────────────────── */

function RunContainerModal({
  onClose,
  onRun,
}: {
  onClose: () => void;
  onRun: (req: RunContainerRequest) => Promise<void>;
}) {
  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  const [ports, setPorts] = useState<string[]>([]);
  const [env, setEnv] = useState<string[]>([]);
  const [volumes, setVolumes] = useState<string[]>([]);
  const [restart, setRestart] = useState("no");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handleRun = async () => {
    if (!image.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      await onRun({
        image: image.trim(),
        name: name.trim() || undefined,
        ports: ports.filter((p) => p.trim()),
        env: env.filter((e) => e.trim()),
        volumes: volumes.filter((v) => v.trim()),
        restart,
      });
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to run container");
    } finally {
      setBusy(false);
    }
  };

  return (
    <ModalOverlay onClose={onClose}>
      <ModalHeader title="Run Container" onClose={onClose} />

      <div className="space-y-4">
        <div>
          <FieldLabel>Image *</FieldLabel>
          <TextInput
            value={image}
            onChange={setImage}
            placeholder="nginx:latest"
            mono
          />
        </div>

        <div>
          <FieldLabel>Container Name</FieldLabel>
          <TextInput
            value={name}
            onChange={setName}
            placeholder="my-container"
            mono
          />
        </div>

        <DynamicListField
          label="Port Mappings"
          placeholder="8080:80"
          values={ports}
          onChange={setPorts}
        />

        <DynamicListField
          label="Environment Variables"
          placeholder="KEY=value"
          values={env}
          onChange={setEnv}
        />

        <DynamicListField
          label="Volumes"
          placeholder="/host/path:/container/path"
          values={volumes}
          onChange={setVolumes}
        />

        <div>
          <FieldLabel>Restart Policy</FieldLabel>
          <select
            value={restart}
            onChange={(e) => setRestart(e.target.value)}
            className="w-full rounded-lg bg-secondary border border-border px-3 py-2 text-sm text-foreground focus:outline-none focus:border-primary/40 focus:ring-1 focus:ring-primary/20 transition-colors"
          >
            <option value="no">No</option>
            <option value="always">Always</option>
            <option value="unless-stopped">Unless Stopped</option>
            <option value="on-failure">On Failure</option>
          </select>
        </div>

        {err && (
          <p className="text-xs text-destructive">{err}</p>
        )}

        <button
          onClick={handleRun}
          disabled={busy || !image.trim()}
          className="w-full rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {busy ? "Pulling & Running..." : "Pull & Run"}
        </button>
      </div>
    </ModalOverlay>
  );
}

/* ── Compose Modal ──────────────────────────────────────────────────── */

function ComposeModal({
  onClose,
  onUp,
  onDown,
}: {
  onClose: () => void;
  onUp: (yaml: string, project: string) => Promise<void>;
  onDown: (project: string) => Promise<void>;
}) {
  const [project, setProject] = useState("");
  const [yaml, setYaml] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handleAction = async (action: "up" | "down") => {
    if (!project.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      if (action === "up") {
        await onUp(yaml, project.trim());
      } else {
        await onDown(project.trim());
      }
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : `Compose ${action} failed`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <ModalOverlay onClose={onClose}>
      <ModalHeader title="Deploy Stack" onClose={onClose} />

      <div className="space-y-4">
        <div>
          <FieldLabel>Project Name *</FieldLabel>
          <TextInput
            value={project}
            onChange={setProject}
            placeholder="my-stack"
            mono
          />
        </div>

        <div>
          <FieldLabel>Compose YAML</FieldLabel>
          <textarea
            value={yaml}
            onChange={(e) => setYaml(e.target.value)}
            placeholder={"version: '3'\nservices:\n  web:\n    image: nginx"}
            rows={12}
            className="w-full rounded-lg bg-secondary border border-border px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-primary/40 focus:ring-1 focus:ring-primary/20 transition-colors resize-none scrollbar-none"
          />
        </div>

        {err && (
          <p className="text-xs text-destructive">{err}</p>
        )}

        <div className="flex gap-3">
          <button
            onClick={() => handleAction("up")}
            disabled={busy || !project.trim()}
            className="flex-1 rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {busy ? "Deploying..." : "Deploy"}
          </button>
          <button
            onClick={() => handleAction("down")}
            disabled={busy || !project.trim()}
            className="flex-1 rounded-lg bg-destructive/10 text-destructive py-2.5 text-sm font-medium hover:bg-destructive/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            Tear Down
          </button>
        </div>
      </div>
    </ModalOverlay>
  );
}

/* ── Logs Modal ─────────────────────────────────────────────────────── */

function LogsModal({
  container,
  onClose,
  getLogs,
}: {
  container: Container;
  onClose: () => void;
  getLogs: (id: string, tail: number) => Promise<string>;
}) {
  const [logs, setLogs] = useState("");
  const [tail, setTail] = useState(100);
  const [loadingLogs, setLoadingLogs] = useState(false);

  const fetchLogs = useCallback(
    async (t: number) => {
      setLoadingLogs(true);
      try {
        const text = await getLogs(container.id, t);
        setLogs(text);
      } catch (e) {
        setLogs(
          e instanceof Error ? `Error: ${e.message}` : "Failed to fetch logs"
        );
      } finally {
        setLoadingLogs(false);
      }
    },
    [container.id, getLogs]
  );

  // fetch on mount
  useState(() => {
    fetchLogs(tail);
  });

  const handleTailChange = (t: number) => {
    setTail(t);
    fetchLogs(t);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-background/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative glass rounded-2xl w-full max-w-3xl max-h-[85vh] flex flex-col mx-4 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="shrink-0 px-6 py-4 flex items-center justify-between border-b border-border">
          <div>
            <h2 className="text-base font-semibold">Logs</h2>
            <p className="text-xs text-muted-foreground font-mono mt-0.5">
              {container.name}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <select
              value={tail}
              onChange={(e) => handleTailChange(Number(e.target.value))}
              className="rounded-lg bg-secondary border border-border px-2 py-1 text-xs text-foreground focus:outline-none"
            >
              <option value={50}>50 lines</option>
              <option value={100}>100 lines</option>
              <option value={500}>500 lines</option>
              <option value={0}>All</option>
            </select>
            <button
              onClick={() => fetchLogs(tail)}
              disabled={loadingLogs}
              className="w-8 h-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
              title="Refresh"
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className={loadingLogs ? "animate-spin" : ""}
              >
                <path d="M21 2v6h-6M3 12a9 9 0 0115.36-6.36L21 8M3 22v-6h6M21 12a9 9 0 01-15.36 6.36L3 16" />
              </svg>
            </button>
            <button
              onClick={onClose}
              className="w-8 h-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              >
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Log output */}
        <div className="flex-1 overflow-y-auto scrollbar-none p-4 bg-[#0a0a10]">
          <pre className="text-xs font-mono text-foreground/80 whitespace-pre-wrap break-all leading-relaxed">
            {loadingLogs ? "Loading..." : logs || "No logs available."}
          </pre>
        </div>
      </div>
    </div>
  );
}

/* ── Container Row ──────────────────────────────────────────────────── */

function ContainerRow({
  container,
  stat,
  onStart,
  onStop,
  onRestart,
  onRemove,
  onLogs,
}: {
  container: Container;
  stat?: ContainerStats;
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onRemove: () => void;
  onLogs: () => void;
}) {
  const isRunning = container.state === "running";
  const isPaused = container.state === "paused";

  return (
    <div
      className={`flex items-center gap-4 px-4 py-3 hover:bg-accent/50 transition-colors group ${
        isRunning ? "border-l-2 border-l-emerald-500/40" : "border-l-2 border-l-transparent"
      }`}
    >
      {/* Status dot */}
      <div
        className="w-2.5 h-2.5 rounded-full shrink-0"
        style={{
          backgroundColor: stateColor(container.state),
          boxShadow: stateGlow(container.state),
        }}
      />

      {/* Name & image */}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground truncate">
          {container.name}
        </p>
        <p className="text-[11px] font-mono text-muted-foreground truncate">
          {container.image}
        </p>
      </div>

      {/* Status */}
      <div className="hidden md:block w-28 shrink-0">
        <p className="text-xs text-muted-foreground truncate">
          {container.status}
        </p>
      </div>

      {/* Ports */}
      <div className="hidden lg:block w-36 shrink-0">
        <p className="text-xs font-mono text-muted-foreground truncate">
          {container.ports || "-"}
        </p>
      </div>

      {/* CPU / MEM stats */}
      <div className="hidden xl:block w-32 shrink-0">
        {stat ? (
          <div className="space-y-0.5">
            <p className="text-[10px] font-mono text-muted-foreground">
              CPU {stat.cpu_perc}
            </p>
            <p className="text-[10px] font-mono text-muted-foreground">
              MEM {stat.mem_usage}
            </p>
          </div>
        ) : (
          <p className="text-[10px] font-mono text-muted-foreground">-</p>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1 shrink-0 opacity-60 group-hover:opacity-100 transition-opacity">
        {isRunning || isPaused ? (
          <ActionBtn
            title="Stop"
            onClick={onStop}
            icon={
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="currentColor"
                stroke="none"
              >
                <rect x="6" y="6" width="12" height="12" rx="1" />
              </svg>
            }
          />
        ) : (
          <ActionBtn
            title="Start"
            onClick={onStart}
            className="text-emerald-400 hover:bg-emerald-400/10"
            icon={
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="currentColor"
                stroke="none"
              >
                <polygon points="6,4 20,12 6,20" />
              </svg>
            }
          />
        )}

        <ActionBtn
          title="Restart"
          onClick={onRestart}
          icon={
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M21 2v6h-6M3 12a9 9 0 0115.36-6.36L21 8M3 22v-6h6M21 12a9 9 0 01-15.36 6.36L3 16" />
            </svg>
          }
        />

        <ActionBtn
          title="Logs"
          onClick={onLogs}
          icon={
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
              <polyline points="14 2 14 8 20 8" />
              <line x1="16" y1="13" x2="8" y2="13" />
              <line x1="16" y1="17" x2="8" y2="17" />
            </svg>
          }
        />

        <ActionBtn
          title="Remove"
          onClick={onRemove}
          className="text-destructive hover:bg-destructive/10"
          icon={
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polyline points="3 6 5 6 21 6" />
              <path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
              <path d="M10 11v6M14 11v6M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2" />
            </svg>
          }
        />
      </div>
    </div>
  );
}

function ActionBtn({
  title,
  onClick,
  icon,
  className,
}: {
  title: string;
  onClick: () => void;
  icon: React.ReactNode;
  className?: string;
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      className={`w-8 h-8 rounded-lg flex items-center justify-center transition-colors ${
        className ?? "text-muted-foreground hover:text-foreground hover:bg-secondary"
      }`}
    >
      {icon}
    </button>
  );
}

/* ── Main Page ──────────────────────────────────────────────────────── */

export function ContainersPage() {
  const docker = useDocker(5000);
  const [showRun, setShowRun] = useState(false);
  const [showCompose, setShowCompose] = useState(false);
  const [logsContainer, setLogsContainer] = useState<Container | null>(null);

  const getStatForContainer = (id: string): ContainerStats | undefined =>
    docker.stats.find((s) => s.id === id || s.name === id);

  const runningCount = docker.containers.filter(
    (c) => c.state === "running"
  ).length;

  if (docker.loading && docker.containers.length === 0) {
    return (
      <div className="p-6 space-y-4">
        {[1, 2, 3].map((i) => (
          <div key={i} className="glass rounded-xl p-5 h-16 animate-pulse" />
        ))}
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto scrollbar-none flex flex-col">
      {/* Header */}
      <div className="shrink-0 px-6 py-5 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Containers</h2>
          <span className="rounded-full bg-primary/15 text-primary text-[11px] font-mono font-medium px-2.5 py-0.5">
            {runningCount}/{docker.containers.length}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowCompose(true)}
            className="rounded-lg bg-secondary text-foreground px-3.5 py-2 text-xs font-medium hover:bg-secondary/80 transition-colors flex items-center gap-2"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
              <polyline points="14 2 14 8 20 8" />
            </svg>
            Deploy Stack
          </button>
          <button
            onClick={() => setShowRun(true)}
            className="rounded-lg bg-primary text-primary-foreground px-3.5 py-2 text-xs font-medium hover:bg-primary/90 transition-colors flex items-center gap-2"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Run Container
          </button>
        </div>
      </div>

      {/* Error banner */}
      {docker.error && (
        <div className="mx-6 mb-4 rounded-lg bg-destructive/10 border border-destructive/20 px-4 py-3">
          <p className="text-xs text-destructive">{docker.error}</p>
        </div>
      )}

      {/* Table header */}
      <div className="shrink-0 px-6">
        <div className="flex items-center gap-4 px-4 py-2 text-[10px] uppercase tracking-wider font-semibold text-muted-foreground">
          <div className="w-2.5 shrink-0" />
          <div className="flex-1 min-w-0">Container</div>
          <div className="hidden md:block w-28 shrink-0">Status</div>
          <div className="hidden lg:block w-36 shrink-0">Ports</div>
          <div className="hidden xl:block w-32 shrink-0">Resources</div>
          <div className="w-[144px] shrink-0 text-right">Actions</div>
        </div>
      </div>

      {/* Container list */}
      <div className="flex-1 min-h-0 overflow-y-auto scrollbar-none px-6 pb-6">
        {docker.containers.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="w-16 h-16 mx-auto mb-4 rounded-2xl glass flex items-center justify-center glow-primary">
              <svg
                width="28"
                height="28"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className="text-primary"
              >
                <rect x="2" y="2" width="20" height="8" rx="2" />
                <rect x="2" y="14" width="20" height="8" rx="2" />
                <circle cx="6" cy="6" r="1" />
                <circle cx="6" cy="18" r="1" />
              </svg>
            </div>
            <h3 className="text-sm font-medium text-foreground mb-1">
              No containers found
            </h3>
            <p className="text-xs text-muted-foreground mb-4">
              Run a container or deploy a stack to get started
            </p>
            <button
              onClick={() => setShowRun(true)}
              className="rounded-lg bg-primary text-primary-foreground px-4 py-2 text-xs font-medium hover:bg-primary/90 transition-colors"
            >
              Run Container
            </button>
          </div>
        ) : (
          <div className="glass-subtle rounded-xl overflow-hidden divide-y divide-border">
            {docker.containers.map((c) => (
              <ContainerRow
                key={c.id}
                container={c}
                stat={getStatForContainer(c.id)}
                onStart={() => docker.startContainer(c.id)}
                onStop={() => docker.stopContainer(c.id)}
                onRestart={() => docker.restartContainer(c.id)}
                onRemove={() => docker.removeContainer(c.id, true)}
                onLogs={() => setLogsContainer(c)}
              />
            ))}
          </div>
        )}
      </div>

      {/* Modals */}
      {showRun && (
        <RunContainerModal
          onClose={() => setShowRun(false)}
          onRun={docker.runContainer}
        />
      )}
      {showCompose && (
        <ComposeModal
          onClose={() => setShowCompose(false)}
          onUp={docker.composeUp}
          onDown={docker.composeDown}
        />
      )}
      {logsContainer && (
        <LogsModal
          container={logsContainer}
          onClose={() => setLogsContainer(null)}
          getLogs={docker.getLogs}
        />
      )}
    </div>
  );
}
