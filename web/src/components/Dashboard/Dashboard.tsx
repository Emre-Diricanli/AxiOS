import { useCallback, useEffect, useMemo, useState } from "react";
import { useDocker } from "@/hooks/useDocker";
import { useRuntimeStatus } from "@/hooks/useRuntimeStatus";
import { useSystemStats } from "@/hooks/useSystemStats";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useAIActivity } from "@/lib/activity";

interface SessionActivity {
  id: string;
  title: string;
  message_count: number;
  updated_at: string;
}

interface CodeTaskActivity {
  id: string;
  prompt: string;
  status: "queued" | "running" | "done" | "failed" | "aborted";
  created_at: string;
}

interface ActivityItem {
  id: string;
  title: string;
  meta: string;
  timestamp: string;
  tone: "good" | "warning" | "neutral";
}

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function relativeTime(value: string): string {
  const elapsed = Date.now() - new Date(value).getTime();
  const minutes = Math.max(0, Math.floor(elapsed / 60000));
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function usageTone(percent: number): string {
  if (percent >= 85) return "bg-red-400";
  if (percent >= 70) return "bg-amber-400";
  return "bg-emerald-400";
}

function MetricBar({ percent }: { percent: number }) {
  return <Progress value={percent} indicatorClassName={usageTone(percent)} />;
}

function SectionCard({
  title,
  eyebrow,
  action,
  children,
  className = "",
}: {
  title: string;
  eyebrow?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={`border-t border-border py-4 ${className}`}>
      <div className="flex items-start justify-between gap-3 mb-4">
        <div>
          {eyebrow && <p className="text-[10px] uppercase tracking-[0.12em] text-muted-foreground mb-1">{eyebrow}</p>}
          <h3 className="text-sm font-medium">{title}</h3>
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

function QuickAction({ label, description, onClick }: { label: string; description: string; onClick: () => void }) {
  return (
    <Button
      type="button"
      variant="ghost"
      onClick={onClick}
      className="group h-auto w-full items-stretch flex-col text-left rounded-md border-0 border-b border-border bg-transparent px-0 py-3 hover:bg-transparent"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-foreground/85 group-hover:text-primary transition-colors">{label}</span>
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="text-muted-foreground group-hover:text-primary group-hover:translate-x-0.5 transition-all" aria-hidden="true">
          <path d="m9 18 6-6-6-6" />
        </svg>
      </div>
      <p className="text-xs font-normal whitespace-normal text-muted-foreground mt-1 leading-relaxed">{description}</p>
    </Button>
  );
}

export function Dashboard() {
  const aiActivity = useAIActivity();
  const { stats, telemetry, isRemote, loading: statsLoading, error: statsError } = useSystemStats(3000);
  const { containers, loading: dockerLoading, error: dockerError } = useDocker(6000);
  const { daemonOnline, status, activeHost, loading: runtimeLoading } = useRuntimeStatus(4000);
  const [sessions, setSessions] = useState<SessionActivity[]>([]);
  const [tasks, setTasks] = useState<CodeTaskActivity[]>([]);

  const fetchActivity = useCallback(async () => {
    const [sessionsResponse, tasksResponse] = await Promise.allSettled([
      fetch("/api/chat/sessions"),
      fetch("/api/code/tasks"),
    ]);
    if (sessionsResponse.status === "fulfilled" && sessionsResponse.value.ok) {
      const data = await sessionsResponse.value.json();
      setSessions(data.sessions ?? []);
    }
    if (tasksResponse.status === "fulfilled" && tasksResponse.value.ok) {
      const data = await tasksResponse.value.json();
      setTasks(data.tasks ?? []);
    }
  }, []);

  useEffect(() => {
    fetchActivity();
    const timer = setInterval(fetchActivity, 10000);
    return () => clearInterval(timer);
  }, [fetchActivity]);

  const activity = useMemo<ActivityItem[]>(() => {
    const liveItems = aiActivity.map((item) => ({
      id: item.id,
      title: item.title,
      meta: `${item.kind}${item.detail ? ` · ${item.detail}` : ""}`,
      timestamp: item.timestamp,
      tone: item.status === "success" ? "good" as const : item.status === "failed" || item.status === "denied" ? "warning" as const : "neutral" as const,
    }));
    const sessionItems = sessions.map((session) => ({
      id: `session-${session.id}`,
      title: session.title || "AxiOS conversation",
      meta: `${session.message_count} messages`,
      timestamp: session.updated_at,
      tone: "neutral" as const,
    }));
    const taskItems = tasks.map((task) => ({
      id: `task-${task.id}`,
      title: task.prompt,
      meta: `Coding task · ${task.status}`,
      timestamp: task.created_at,
      tone: task.status === "done" ? "good" as const : task.status === "failed" ? "warning" as const : "neutral" as const,
    }));
    return [...liveItems, ...sessionItems, ...taskItems]
      .sort((left, right) => new Date(right.timestamp).getTime() - new Date(left.timestamp).getTime())
      .slice(0, 5);
  }, [aiActivity, sessions, tasks]);

  const storageWarnings = (stats?.disk ?? []).filter((disk) => disk.usage_percent >= 80);
  const runningContainers = containers.filter((container) => container.state.toLowerCase() === "running").length;
  const unhealthyContainers = containers.filter((container) => ["dead", "exited"].includes(container.state.toLowerCase())).length;
  const gpus = stats?.gpu ?? [];
  const isPrivate = status?.backend === "local" || status?.routing === "local_only";
  const openChat = (prompt: string) => window.dispatchEvent(new CustomEvent("axios-open-chat", { detail: { prompt } }));
  const navigate = (tab: string) => window.dispatchEvent(new CustomEvent("axios-navigate", { detail: tab }));

  if ((statsLoading || runtimeLoading) && !stats && !status) {
    return (
      <div className="p-6 space-y-4 h-full overflow-hidden">
        <div className="h-8 w-64 rounded bg-secondary animate-pulse" />
        <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
          {[1, 2, 3, 4, 5, 6].map((item) => <div key={item} className="h-36 rounded bg-secondary animate-pulse" />)}
        </div>
      </div>
    );
  }

  return (
    <div className="px-6 py-5 max-[640px]:px-4 space-y-2 overflow-y-auto h-full scrollbar-none">
      <header className="flex flex-col md:flex-row md:items-end justify-between gap-3 pb-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-semibold tracking-tight">{telemetry?.host.name ?? stats?.hostname ?? "AxiOS"}</h2>
            {isRemote && <span className="rounded border border-primary/25 bg-primary/10 px-2 py-0.5 text-xs text-primary">Remote compute</span>}
          </div>
          <p className="text-sm text-muted-foreground mt-1">
            {telemetry ? `${telemetry.host.host}:${telemetry.host.port} · ${telemetry.latency_ms} ms` : daemonOnline ? "All control services connected" : "Daemon connection interrupted"}
            {stats?.uptime ? ` · Up ${stats.uptime}` : ""}
          </p>
        </div>
        <div className={`flex items-center gap-2 px-2.5 py-1 border rounded-md ${
          storageWarnings.length > 0 || unhealthyContainers > 0 || statsError
            ? "border-amber-400/25 bg-amber-400/10 text-amber-300"
            : "border-emerald-400/20 bg-emerald-400/[0.08] text-emerald-300"
        }`}>
          <span className={`w-1.5 h-1.5 rounded-full ${storageWarnings.length > 0 || unhealthyContainers > 0 ? "bg-amber-400" : "bg-emerald-400"}`} />
          <span className="text-xs font-medium">
            {storageWarnings.length > 0
              ? `${storageWarnings.length} storage warning${storageWarnings.length > 1 ? "s" : ""}`
              : unhealthyContainers > 0
                ? `${unhealthyContainers} container${unhealthyContainers > 1 ? "s" : ""} stopped`
                : statsError
                  ? "System metrics unavailable"
                  : "Systems nominal"}
          </span>
        </div>
      </header>

      {statsError && telemetry && !stats && (
        <div className="border-y border-amber-400/25 bg-amber-400/[0.05] px-3 py-3">
          <p className="text-sm text-amber-300">Remote hardware details unavailable</p>
          <p className="text-xs text-muted-foreground mt-1">{statsError}</p>
          {telemetry.running_models.map((model) => (
            <p key={model.name} className="text-xs text-muted-foreground mt-1">
              Running {model.name} · {formatBytes(model.vram_bytes)} VRAM allocated
            </p>
          ))}
        </div>
      )}

      <div className="grid grid-cols-1 xl:grid-cols-[1.35fr_1fr] gap-x-8">
        <SectionCard title={status?.model || status?.localModel || "No model selected"} eyebrow="AI Runtime">
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            {[
              ["Provider", status?.provider || (status?.backend === "local" ? "Ollama" : "Unknown")],
              ["Route", `${status?.backend ?? "unknown"} / ${(status?.routing ?? "unknown").replace(/_/g, " ")}`],
              ["Host", telemetry?.host.name || activeHost?.name || stats?.hostname || "Unknown"],
              ["Privacy", isPrivate ? "Local only" : "Cloud enabled"],
            ].map(([label, value]) => (
              <div key={label} className="border-l border-border pl-3 min-w-0 first:border-l-0 first:pl-0">
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">{label}</p>
                <p className="text-xs text-foreground/80 mt-1 truncate">{value}</p>
              </div>
            ))}
          </div>
          <div className="mt-4 flex items-center justify-between gap-4 border-t border-border pt-3">
            <div>
              <p className="text-[9px] uppercase tracking-wider text-muted-foreground">Latest inference</p>
              <p className="text-lg font-semibold text-primary mt-0.5">
                {status?.inference?.tokens_per_second
                  ? `${status.inference.tokens_per_second.toFixed(1)} tok/s`
                  : "Awaiting sample"}
              </p>
            </div>
            <div className="text-right text-[10px] text-muted-foreground font-mono">
              {status?.inference ? (
                <>
                  <p>{status.inference.output_tokens} output tokens</p>
                  <p>{(status.inference.duration_ms / 1000).toFixed(1)}s · {relativeTime(status.inference.completed_at)}</p>
                </>
              ) : <p>Run a model request to measure</p>}
            </div>
          </div>
        </SectionCard>

        <SectionCard title="Quick Actions" eyebrow="Operate">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
            <QuickAction label="Analyze health" description="Inspect the active compute host." onClick={() => openChat("Analyze the active AI compute host's health and prioritize any issues you find.")} />
            <QuickAction label="Inspect containers" description="Review Docker state and failures." onClick={() => openChat("Inspect my containers, check failures and resource usage, and summarize what needs attention.")} />
            <QuickAction label="Optimize model" description="Match models to available GPUs." onClick={() => openChat("Review my current model and GPU resources, then recommend the best performance configuration.")} />
            <QuickAction label="Open terminal" description="Switch to direct shell access." onClick={() => navigate("terminal")} />
          </div>
        </SectionCard>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 border-y border-border divide-y md:divide-y-0 md:divide-x divide-border">
        {[
          { label: "CPU", value: stats?.cpu.usage_percent ?? 0, detail: stats?.cpu.model ?? "Unavailable" },
          { label: "Memory", value: stats?.memory.usage_percent ?? 0, detail: stats ? `${formatBytes(stats.memory.available_bytes)} available` : "Unavailable" },
          { label: "Primary storage", value: stats?.disk[0]?.usage_percent ?? 0, detail: stats?.disk[0] ? `${formatBytes(stats.disk[0].available_bytes)} free` : "Unavailable" },
        ].map((metric) => (
          <div key={metric.label} className="px-4 py-4 first:pl-0 last:pr-0">
            <div className="flex items-end justify-between gap-3 mb-3">
              <div className="min-w-0">
                <p className="text-[9px] uppercase tracking-[0.15em] text-muted-foreground">{metric.label}</p>
                <p className="text-[10px] text-foreground/55 truncate mt-1">{metric.detail}</p>
              </div>
              <span className="text-xl font-semibold font-mono" style={{ color: metric.value >= 85 ? "#f87171" : metric.value >= 70 ? "#fbbf24" : "#e4e4ec" }}>
                {metric.value.toFixed(0)}%
              </span>
            </div>
            <MetricBar percent={metric.value} />
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-x-8">
        <SectionCard
          title="GPU Fleet"
          eyebrow="Acceleration"
          action={<button type="button" onClick={() => navigate("system")} className="text-[10px] text-muted-foreground hover:text-primary">View system</button>}
        >
          {gpus.length === 0 ? (
            <div className="border border-dashed border-border py-8 text-center">
              <p className="text-xs text-muted-foreground">{telemetry?.source === "ollama" ? "Connect the remote AxiOS telemetry port to identify GPU hardware" : "No NVIDIA GPU telemetry available"}</p>
              {telemetry?.running_models.map((model) => (
                <p key={model.name} className="text-xs text-foreground/70 mt-2">{model.name} · {formatBytes(model.vram_bytes)} VRAM</p>
              ))}
            </div>
          ) : (
            <div className="space-y-3">
              {gpus.map((gpu) => (
                <div key={gpu.index} className="border-b border-border py-3 last:border-b-0">
                  <div className="flex items-start justify-between gap-3 mb-3">
                    <div className="min-w-0">
                      <p className="text-xs font-medium truncate">GPU {gpu.index} · {gpu.name}</p>
                      <p className="text-[10px] text-muted-foreground mt-0.5">{gpu.temperature_c.toFixed(0)}°C · {gpu.utilization_percent.toFixed(0)}% compute</p>
                    </div>
                    <span className="text-[10px] font-mono text-foreground/70 shrink-0">
                      {formatBytes(gpu.memory_used_bytes)} / {formatBytes(gpu.memory_total_bytes)}
                    </span>
                  </div>
                  <MetricBar percent={gpu.memory_usage_percent} />
                </div>
              ))}
            </div>
          )}
        </SectionCard>

        <SectionCard
          title={isRemote ? "Local Docker Health" : "Docker Health"}
          eyebrow={isRemote ? "Local workloads" : "Workloads"}
          action={<button type="button" onClick={() => navigate("containers")} className="text-[10px] text-muted-foreground hover:text-primary">Manage</button>}
        >
          {dockerError ? (
            <div className="border border-amber-400/20 bg-amber-400/[0.06] p-3 text-xs text-amber-300">Docker unavailable · {dockerError}</div>
          ) : (
            <>
              <div className="grid grid-cols-3 gap-2 mb-4">
                {[
                  ["Total", containers.length],
                  ["Running", runningContainers],
                  ["Stopped", containers.length - runningContainers],
                ].map(([label, value]) => (
                  <div key={label} className="border-l border-border p-3 text-center first:border-l-0">
                    <p className="text-lg font-semibold font-mono">{dockerLoading ? "—" : value}</p>
                    <p className="text-[9px] uppercase tracking-wider text-muted-foreground mt-1">{label}</p>
                  </div>
                ))}
              </div>
              <div className="space-y-2">
                {containers.slice(0, 4).map((container) => (
                  <div key={container.id} className="flex items-center justify-between gap-3 py-1.5">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${container.state.toLowerCase() === "running" ? "bg-emerald-400" : "bg-amber-400"}`} />
                      <span className="text-xs text-foreground/75 truncate">{container.name}</span>
                    </div>
                    <span className="text-[10px] text-muted-foreground font-mono shrink-0">{container.state}</span>
                  </div>
                ))}
                {!dockerLoading && containers.length === 0 && <p className="text-xs text-muted-foreground text-center py-3">No containers deployed</p>}
              </div>
            </>
          )}
        </SectionCard>
      </div>

      <SectionCard title="Recent AI Activity" eyebrow="Timeline">
        {activity.length === 0 ? (
          <p className="text-xs text-muted-foreground text-center py-5">Your conversations and delegated coding tasks will appear here.</p>
        ) : (
          <div className="divide-y divide-border">
            {activity.map((item) => (
              <div key={item.id} className="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0">
                <span className={`w-2 h-2 rounded-full shrink-0 ${item.tone === "good" ? "bg-emerald-400" : item.tone === "warning" ? "bg-amber-400" : "bg-primary"}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-xs text-foreground/80 truncate">{item.title}</p>
                  <p className="text-[10px] text-muted-foreground mt-0.5 truncate">{item.meta}</p>
                </div>
                <span className="text-[10px] text-muted-foreground font-mono shrink-0">{relativeTime(item.timestamp)}</span>
              </div>
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  );
}
