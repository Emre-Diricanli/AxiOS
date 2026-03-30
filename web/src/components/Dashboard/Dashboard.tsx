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

function GaugeRing({ percent, size = 80, stroke = 6 }: { percent: number; size?: number; stroke?: number }) {
  const r = (size - stroke) / 2;
  const circ = 2 * Math.PI * r;
  const offset = circ - (percent / 100) * circ;
  const color = usageColor(percent);

  return (
    <svg width={size} height={size} className="-rotate-90">
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="rgba(255,255,255,0.06)" strokeWidth={stroke} />
      <circle
        cx={size / 2} cy={size / 2} r={r} fill="none"
        stroke={color} strokeWidth={stroke} strokeLinecap="round"
        strokeDasharray={circ} strokeDashoffset={offset}
        style={{ transition: "stroke-dashoffset 0.8s ease, stroke 0.3s ease", filter: `drop-shadow(0 0 6px ${color}66)` }}
      />
    </svg>
  );
}

function StatCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="glass rounded-xl p-5 flex flex-col gap-3">
      <h3 className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{title}</h3>
      {children}
    </div>
  );
}

function QuickAction({ icon, label, sub }: { icon: React.ReactNode; label: string; sub: string }) {
  return (
    <button className="glass rounded-xl p-4 flex items-center gap-3 hover:bg-accent transition-colors text-left group">
      <div className="w-10 h-10 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary group-hover:glow-sm transition-all">
        {icon}
      </div>
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="text-[11px] text-muted-foreground">{sub}</p>
      </div>
    </button>
  );
}

export function Dashboard() {
  const { stats, loading } = useSystemStats(3000);

  if (loading && !stats) {
    return (
      <div className="p-6 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="glass rounded-xl p-5 h-40 animate-pulse" />
        ))}
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 overflow-y-auto h-full scrollbar-none">
      {/* Welcome */}
      <div>
        <h2 className="text-2xl font-bold tracking-tight">
          Welcome to Axi<span className="text-primary text-glow">OS</span>
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          {stats ? `${stats.hostname} — ${stats.os}/${stats.arch} — Kernel ${stats.kernel}` : "Loading system info..."}
        </p>
      </div>

      {/* Metrics row */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
          {/* CPU */}
          <StatCard title="CPU">
            <div className="flex items-center gap-4">
              <div className="relative">
                <GaugeRing percent={stats.cpu.usage_percent} />
                <div className="absolute inset-0 flex items-center justify-center">
                  <span className="text-lg font-bold font-mono" style={{ color: usageColor(stats.cpu.usage_percent) }}>
                    {stats.cpu.usage_percent.toFixed(0)}%
                  </span>
                </div>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-xs text-foreground truncate">{stats.cpu.model}</p>
                <p className="text-[11px] text-muted-foreground mt-1">{stats.cpu.cores}C / {stats.cpu.threads}T</p>
              </div>
            </div>
          </StatCard>

          {/* Memory */}
          <StatCard title="Memory">
            <div className="flex items-center gap-4">
              <div className="relative">
                <GaugeRing percent={stats.memory.usage_percent} />
                <div className="absolute inset-0 flex items-center justify-center">
                  <span className="text-lg font-bold font-mono" style={{ color: usageColor(stats.memory.usage_percent) }}>
                    {stats.memory.usage_percent.toFixed(0)}%
                  </span>
                </div>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-xs text-foreground">{formatBytes(stats.memory.used_bytes)} used</p>
                <p className="text-[11px] text-muted-foreground mt-1">of {formatBytes(stats.memory.total_bytes)}</p>
              </div>
            </div>
          </StatCard>

          {/* Disk */}
          {stats.disk.length > 0 && (
            <StatCard title={`Disk ${stats.disk[0].mount}`}>
              <div className="flex items-center gap-4">
                <div className="relative">
                  <GaugeRing percent={stats.disk[0].usage_percent} />
                  <div className="absolute inset-0 flex items-center justify-center">
                    <span className="text-lg font-bold font-mono" style={{ color: usageColor(stats.disk[0].usage_percent) }}>
                      {stats.disk[0].usage_percent.toFixed(0)}%
                    </span>
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-xs text-foreground">{formatBytes(stats.disk[0].used_bytes)} used</p>
                  <p className="text-[11px] text-muted-foreground mt-1">of {formatBytes(stats.disk[0].total_bytes)}</p>
                </div>
              </div>
            </StatCard>
          )}

          {/* Uptime / Network */}
          <StatCard title="System">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Uptime</span>
                <span className="text-xs font-mono text-foreground">{stats.uptime}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Hostname</span>
                <span className="text-xs font-mono text-foreground">{stats.hostname}</span>
              </div>
              {stats.network.interfaces.length > 0 && (
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">IP</span>
                  <span className="text-xs font-mono text-foreground">{stats.network.interfaces[0].ip || "N/A"}</span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">Kernel</span>
                <span className="text-xs font-mono text-foreground truncate max-w-[120px]">{stats.kernel}</span>
              </div>
            </div>
          </StatCard>
        </div>
      )}

      {/* Network interfaces */}
      {stats && stats.network.interfaces.length > 0 && (
        <StatCard title="Network Interfaces">
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-2">
            {stats.network.interfaces.map((iface) => (
              <div key={iface.name} className="flex items-center gap-3 rounded-lg bg-secondary p-3">
                <div
                  className="w-2 h-2 rounded-full shrink-0"
                  style={{
                    backgroundColor: iface.status === "up" ? "#22c55e" : "#ef4444",
                    boxShadow: `0 0 8px ${iface.status === "up" ? "#22c55e66" : "#ef444466"}`,
                  }}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-mono text-foreground">{iface.name}</p>
                  <p className="text-[10px] font-mono text-muted-foreground truncate">{iface.ip || "no address"}</p>
                </div>
              </div>
            ))}
          </div>
        </StatCard>
      )}

      {/* Additional disks */}
      {stats && stats.disk.length > 1 && (
        <StatCard title="Storage">
          <div className="space-y-3">
            {stats.disk.map((d) => (
              <div key={d.mount} className="space-y-1.5">
                <div className="flex items-center justify-between text-xs">
                  <span className="font-mono text-foreground">{d.mount}</span>
                  <span className="text-muted-foreground">{formatBytes(d.available_bytes)} free</span>
                </div>
                <div className="h-2 rounded-full bg-secondary overflow-hidden">
                  <div
                    className="h-full rounded-full transition-all duration-700"
                    style={{
                      width: `${d.usage_percent}%`,
                      backgroundColor: usageColor(d.usage_percent),
                      boxShadow: `0 0 8px ${usageColor(d.usage_percent)}44`,
                    }}
                  />
                </div>
                <div className="flex justify-between text-[10px] text-muted-foreground font-mono">
                  <span>{d.device}</span>
                  <span>{d.usage_percent.toFixed(1)}% — {formatBytes(d.used_bytes)} / {formatBytes(d.total_bytes)}</span>
                </div>
              </div>
            ))}
          </div>
        </StatCard>
      )}

      {/* Quick actions */}
      <div>
        <h3 className="text-sm font-semibold mb-3 text-muted-foreground uppercase tracking-wider">Quick Actions</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3">
          <QuickAction
            icon={<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M4 17l6-5-6-5M12 19h8" /></svg>}
            label="Open Terminal"
            sub="System shell access"
          />
          <QuickAction
            icon={<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>}
            label="Browse Files"
            sub="Filesystem explorer"
          />
          <QuickAction
            icon={<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="2" y="2" width="20" height="8" rx="2" /><rect x="2" y="14" width="20" height="8" rx="2" /><circle cx="6" cy="6" r="1" /><circle cx="6" cy="18" r="1" /></svg>}
            label="Containers"
            sub="Docker management"
          />
          <QuickAction
            icon={<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" /></svg>}
            label="Ask Claude"
            sub="AI assistant"
          />
        </div>
      </div>
    </div>
  );
}
