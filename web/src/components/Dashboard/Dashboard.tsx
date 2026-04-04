import { useSystemStats } from "@/hooks/useSystemStats";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function usageColor(pct: number): string {
  if (pct < 50) return "#22c55e";
  if (pct < 80) return "#f59e0b";
  return "#ef4444";
}

function GaugeRing({ percent, size = 72, stroke = 6 }: { percent: number; size?: number; stroke?: number }) {
  const r = (size - stroke) / 2;
  const circ = 2 * Math.PI * r;
  const offset = circ - (percent / 100) * circ;
  const color = usageColor(percent);

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="rgba(255,255,255,0.04)" strokeWidth={stroke} />
        <circle
          cx={size / 2} cy={size / 2} r={r} fill="none"
          stroke={color} strokeWidth={stroke} strokeLinecap="round"
          strokeDasharray={circ} strokeDashoffset={offset}
          style={{ transition: "stroke-dashoffset 0.8s ease, stroke 0.3s ease", filter: `drop-shadow(0 0 8px ${color}44)` }}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-base font-bold font-mono" style={{ color }}>{percent.toFixed(0)}</span>
        <span className="text-[8px] text-muted-foreground -mt-0.5">%</span>
      </div>
    </div>
  );
}

export function Dashboard() {
  const { stats, loading } = useSystemStats(3000);

  if (loading && !stats) {
    return (
      <div className="p-8 space-y-6">
        <div className="h-8 w-48 rounded-lg bg-secondary animate-pulse" />
        <div className="grid grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="glass rounded-2xl h-44 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  const primaryIface = stats?.network.interfaces.find((i) => i.ip && i.status === "up");

  return (
    <div className="p-5 space-y-5 overflow-y-auto h-full scrollbar-none">
      {/* Header */}
      <div className="flex items-end justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight animate-fade-up">
            Axi<span className="text-primary text-glow">OS</span>
          </h2>
          {stats && (
            <p className="text-sm text-muted-foreground mt-1 animate-fade-up delay-100">
              {stats.hostname} &middot; {stats.uptime}
            </p>
          )}
        </div>
        {stats && primaryIface && (
          <div className="flex items-center gap-2 glass-subtle rounded-full px-4 py-2 animate-fade-up delay-200">
            <div className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
            <span className="text-xs font-mono text-foreground/80">{primaryIface.ip}</span>
            <span className="text-[10px] text-muted-foreground">{primaryIface.name}</span>
          </div>
        )}
      </div>

      {/* Gauges */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          {/* CPU */}
          <div className="glass rounded-2xl p-4 animate-fade-up delay-200">
            <div className="flex items-center gap-3">
              <GaugeRing percent={stats.cpu.usage_percent} />
              <div className="flex-1 min-w-0 space-y-2">
                <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">CPU</h3>
                <p className="text-sm font-medium text-foreground truncate">{stats.cpu.model}</p>
                <p className="text-xs text-muted-foreground">{stats.cpu.cores} cores &middot; {stats.cpu.threads} threads</p>
              </div>
            </div>
          </div>

          {/* Memory */}
          <div className="glass rounded-2xl p-4 animate-fade-up delay-300">
            <div className="flex items-center gap-3">
              <GaugeRing percent={stats.memory.usage_percent} />
              <div className="flex-1 min-w-0 space-y-2">
                <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Memory</h3>
                <p className="text-sm font-medium text-foreground">{formatBytes(stats.memory.used_bytes)} <span className="text-muted-foreground font-normal">/ {formatBytes(stats.memory.total_bytes)}</span></p>
                <p className="text-xs text-muted-foreground">{formatBytes(stats.memory.available_bytes)} available</p>
              </div>
            </div>
          </div>

          {/* Disk */}
          {stats.disk.length > 0 && (
            <div className="glass rounded-2xl p-4 animate-fade-up delay-400">
              <div className="flex items-center gap-5">
                <GaugeRing percent={stats.disk[0].usage_percent} />
                <div className="flex-1 min-w-0 space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Storage</h3>
                  <p className="text-sm font-medium text-foreground">{formatBytes(stats.disk[0].used_bytes)} <span className="text-muted-foreground font-normal">/ {formatBytes(stats.disk[0].total_bytes)}</span></p>
                  <p className="text-xs text-muted-foreground">{formatBytes(stats.disk[0].available_bytes)} free &middot; {stats.disk[0].mount}</p>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* System info + Network row */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {/* System info */}
          <div className="glass rounded-2xl p-4 space-y-3 animate-fade-up delay-500">
            <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">System</h3>
            <div className="grid grid-cols-2 gap-y-3 gap-x-6">
              {[
                ["Hostname", stats.hostname],
                ["OS", `${stats.os} / ${stats.arch}`],
                ["Kernel", stats.kernel],
                ["Uptime", stats.uptime],
              ].map(([label, value]) => (
                <div key={label}>
                  <p className="text-[10px] text-muted-foreground uppercase tracking-wider">{label}</p>
                  <p className="text-sm font-mono text-foreground truncate">{value}</p>
                </div>
              ))}
            </div>
          </div>

          {/* Network */}
          <div className="glass rounded-2xl p-4 space-y-3 animate-fade-up delay-600">
            <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Network</h3>
            {stats.network.interfaces.length === 0 ? (
              <p className="text-xs text-muted-foreground">No active connections</p>
            ) : (
              <div className="space-y-2">
                {stats.network.interfaces.map((iface) => (
                  <div key={iface.name} className="flex items-center justify-between py-2 border-b border-border last:border-0">
                    <div className="flex items-center gap-2.5">
                      <div className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
                      <span className="text-sm font-mono text-foreground">{iface.name}</span>
                    </div>
                    <span className="text-sm font-mono text-muted-foreground">{iface.ip}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Additional storage */}
      {stats && stats.disk.length > 1 && (
        <div className="glass rounded-2xl p-4 space-y-4 animate-fade-up delay-700">
          <h3 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">All Storage</h3>
          {stats.disk.map((d) => (
            <div key={d.mount} className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <span className="font-mono text-foreground">{d.mount}</span>
                <span className="text-muted-foreground">{formatBytes(d.available_bytes)} free of {formatBytes(d.total_bytes)}</span>
              </div>
              <div className="h-1.5 rounded-full bg-secondary overflow-hidden">
                <div
                  className="h-full rounded-full transition-all duration-700"
                  style={{
                    width: `${d.usage_percent}%`,
                    backgroundColor: usageColor(d.usage_percent),
                    boxShadow: `0 0 8px ${usageColor(d.usage_percent)}33`,
                  }}
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
