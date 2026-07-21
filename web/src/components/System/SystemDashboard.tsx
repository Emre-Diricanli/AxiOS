import { useMemo } from "react";
import { useMetricsHistory } from "@/hooks/useMetricsHistory";
import type { MetricSample } from "@/hooks/useMetricsHistory";
import type { SystemStats } from "@/types/system";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { AreaChart } from "./charts/AreaChart";

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function usageClasses(percent: number): { text: string; indicator: string } {
  if (percent >= 80) return { text: "text-red-400", indicator: "bg-red-400" };
  if (percent >= 50) return { text: "text-amber-400", indicator: "bg-amber-400" };
  return { text: "text-emerald-400", indicator: "bg-emerald-400" };
}

// Hex equivalents of the usageClasses scale, for the SVG charts.
function usageHex(percent: number): string {
  if (percent >= 80) return "#f87171";
  if (percent >= 50) return "#fbbf24";
  return "#34d399";
}

type Series = Array<{ t: number; value: number }>;

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono text-xs text-foreground/85 text-right truncate">{value}</span>
    </div>
  );
}

function UsageBlock({ label = "Usage", percent }: { label?: string; percent: number }) {
  const colors = usageClasses(percent);
  return (
    <div className="mt-3 pt-3 border-t border-border">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className={`text-xs font-mono font-semibold ${colors.text}`}>{percent.toFixed(1)}%</span>
      </div>
      <Progress value={percent} indicatorClassName={colors.indicator} />
    </div>
  );
}

function SystemCard({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function SystemInfoCard({ stats }: { stats: SystemStats }) {
  return (
    <SystemCard title="System" description="Host identity and runtime">
      <StatRow label="Hostname" value={stats.hostname} />
      <StatRow label="Operating system" value={`${stats.os} / ${stats.arch}`} />
      <StatRow label="Kernel" value={stats.kernel} />
      <StatRow label="Uptime" value={stats.uptime} />
    </SystemCard>
  );
}

function CPUCard({ stats, series }: { stats: SystemStats; series: Series }) {
  return (
    <SystemCard title="Processor" description={stats.cpu.model}>
      <StatRow label="Physical cores" value={String(stats.cpu.cores)} />
      <StatRow label="Logical threads" value={String(stats.cpu.threads)} />
      <UsageBlock percent={stats.cpu.usage_percent} />
      <div className="mt-3">
        <AreaChart samples={series} color={usageHex(stats.cpu.usage_percent)} height={110} />
      </div>
    </SystemCard>
  );
}

function MemoryCard({ stats, series }: { stats: SystemStats; series: Series }) {
  return (
    <SystemCard title="Memory" description={`${formatBytes(stats.memory.available_bytes)} available`}>
      <StatRow label="Total" value={formatBytes(stats.memory.total_bytes)} />
      <StatRow label="Used" value={formatBytes(stats.memory.used_bytes)} />
      <StatRow label="Available" value={formatBytes(stats.memory.available_bytes)} />
      <UsageBlock percent={stats.memory.usage_percent} />
      <div className="mt-3">
        <AreaChart samples={series} color={usageHex(stats.memory.usage_percent)} height={110} />
      </div>
    </SystemCard>
  );
}

function GPUCards({ stats, history }: { stats: SystemStats; history: MetricSample[] }) {
  if (!stats.gpu?.length) {
    return (
      <SystemCard title="GPU" description="Hardware acceleration">
        <p className="text-sm text-muted-foreground">No NVIDIA GPU telemetry is available.</p>
      </SystemCard>
    );
  }
  return (
    <>
      {stats.gpu.map((gpu, i) => {
        const series = history.map((s) => ({ t: s.t, value: s.gpus[i]?.util ?? 0 }));
        return (
          <SystemCard key={gpu.index} title={`GPU ${gpu.index}`} description={gpu.name}>
            <StatRow label="Temperature" value={`${gpu.temperature_c.toFixed(0)}°C`} />
            <StatRow label="VRAM used" value={`${formatBytes(gpu.memory_used_bytes)} / ${formatBytes(gpu.memory_total_bytes)}`} />
            <UsageBlock label="Compute" percent={gpu.utilization_percent} />
            <UsageBlock label="VRAM" percent={gpu.memory_usage_percent} />
            <div className="mt-3">
              <AreaChart samples={series} color={usageHex(gpu.utilization_percent)} height={90} />
            </div>
          </SystemCard>
        );
      })}
    </>
  );
}

function DiskCards({ stats }: { stats: SystemStats }) {
  if (!stats.disk.length) {
    return (
      <SystemCard title="Storage">
        <p className="text-sm text-muted-foreground">No disk information is available.</p>
      </SystemCard>
    );
  }
  return (
    <>
      {stats.disk.map((disk) => (
        <SystemCard key={disk.mount} title={`Storage · ${disk.mount}`} description={disk.device}>
          <StatRow label="Total" value={formatBytes(disk.total_bytes)} />
          <StatRow label="Used" value={formatBytes(disk.used_bytes)} />
          <StatRow label="Available" value={formatBytes(disk.available_bytes)} />
          <UsageBlock percent={disk.usage_percent} />
        </SystemCard>
      ))}
    </>
  );
}

function NetworkCard({ stats }: { stats: SystemStats }) {
  return (
    <SystemCard title="Network" description="Available interfaces">
      {stats.network.interfaces.length === 0 ? (
        <p className="text-sm text-muted-foreground">No interfaces found.</p>
      ) : (
        <div className="space-y-2">
          {stats.network.interfaces.map((networkInterface) => (
            <div key={networkInterface.name} className="surface-raised rounded-lg flex items-center justify-between gap-3 px-3 py-2.5">
              <div className="flex items-center gap-2 min-w-0">
                <span className={`w-2 h-2 rounded-full ${networkInterface.status === "up" ? "bg-emerald-400" : "bg-red-400"}`} />
                <span className="text-sm font-medium truncate">{networkInterface.name}</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <span className="text-xs font-mono text-muted-foreground">{networkInterface.ip || "no address"}</span>
                <Badge variant={networkInterface.status === "up" ? "secondary" : "destructive"}>{networkInterface.status}</Badge>
              </div>
            </div>
          ))}
        </div>
      )}
    </SystemCard>
  );
}

function SkeletonCard() {
  return (
    <Card className="h-48 animate-pulse">
      <CardHeader>
        <div className="h-4 w-28 rounded bg-secondary" />
        <div className="h-3 w-40 rounded bg-secondary" />
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="h-3 w-full rounded bg-secondary" />
        <div className="h-3 w-4/5 rounded bg-secondary" />
        <div className="h-3 w-3/5 rounded bg-secondary" />
      </CardContent>
    </Card>
  );
}

export function SystemDashboard() {
  const { stats, telemetry, isRemote, loading, error, history } = useMetricsHistory(1000);

  const cpuSeries = useMemo(() => history.map((s) => ({ t: s.t, value: s.cpu })), [history]);
  const memSeries = useMemo(() => history.map((s) => ({ t: s.t, value: s.mem })), [history]);

  return (
    <div className="min-h-full bg-workspace p-5 md:p-6">
      <div className="mb-5 animate-fade-up">
        <p className="text-xs uppercase tracking-[0.16em] text-primary mb-1">Telemetry</p>
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold tracking-tight">{telemetry?.host.name ?? "System Details"}</h2>
          {isRemote && <Badge variant="default">Remote</Badge>}
        </div>
        <p className="text-sm text-muted-foreground mt-1">
          {telemetry ? `${telemetry.host.host}:${telemetry.host.port} · ${telemetry.latency_ms} ms · ${telemetry.source} telemetry` : "Live hardware, storage, and network information."}
        </p>
      </div>
      {error && (
        <div className="mb-4 rounded-xl border border-red-400/25 bg-red-400/[0.08] px-4 py-3 text-sm text-red-300">
          {error}
          {telemetry?.running_models.map((model) => (
            <span key={model.name} className="block mt-1 text-xs text-muted-foreground">
              Running {model.name} · {formatBytes(model.vram_bytes)} VRAM allocated
            </span>
          ))}
        </div>
      )}
      {loading && !stats ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3 gap-4">
          {[1, 2, 3, 4, 5, 6].map((item) => <SkeletonCard key={item} />)}
        </div>
      ) : stats ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3 gap-4">
          <div className="animate-fade-up delay-100"><SystemInfoCard stats={stats} /></div>
          <div className="animate-fade-up delay-200"><CPUCard stats={stats} series={cpuSeries} /></div>
          <div className="animate-fade-up delay-300"><MemoryCard stats={stats} series={memSeries} /></div>
          <div className="animate-fade-up delay-400"><NetworkCard stats={stats} /></div>
          <GPUCards stats={stats} history={history} />
          <DiskCards stats={stats} />
        </div>
      ) : null}
    </div>
  );
}
