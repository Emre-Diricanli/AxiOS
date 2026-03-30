import { useSystemStats } from "@/hooks/useSystemStats";
import type { SystemStats } from "@/types/system";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const value = bytes / Math.pow(k, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function usageColor(pct: number): string {
  if (pct < 50) return "#22c55e"; // green-500
  if (pct < 80) return "#eab308"; // yellow-500
  return "#ef4444"; // red-500
}

function usageGlow(pct: number): string {
  const color = usageColor(pct);
  return `0 0 8px ${color}66, 0 0 20px ${color}33`;
}

function usageGradient(pct: number): string {
  if (pct < 50) return "linear-gradient(90deg, #22c55e, #4ade80)";
  if (pct < 80) return "linear-gradient(90deg, #22c55e, #eab308)";
  return "linear-gradient(90deg, #eab308, #ef4444)";
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(100, Math.max(0, percent));
  return (
    <div
      style={{
        width: "100%",
        height: 10,
        borderRadius: 6,
        backgroundColor: "#262626",
        overflow: "hidden",
        position: "relative",
      }}
    >
      <div
        style={{
          width: `${clamped}%`,
          height: "100%",
          borderRadius: 6,
          background: usageGradient(clamped),
          boxShadow: usageGlow(clamped),
          transition: "width 0.6s ease",
        }}
      />
    </div>
  );
}

function Card({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div
      style={{
        backgroundColor: "#171717",
        border: "1px solid #262626",
        borderRadius: 12,
        padding: 24,
      }}
    >
      <h3
        style={{
          fontSize: 13,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
          color: "#a3a3a3",
          marginTop: 0,
          marginBottom: 16,
        }}
      >
        {title}
      </h3>
      {children}
    </div>
  );
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div
      style={{
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        padding: "6px 0",
      }}
    >
      <span style={{ color: "#a3a3a3", fontSize: 13 }}>{label}</span>
      <span style={{ color: "#e5e5e5", fontSize: 13, fontFamily: "monospace" }}>
        {value}
      </span>
    </div>
  );
}

function SkeletonCard() {
  return (
    <div
      style={{
        backgroundColor: "#171717",
        border: "1px solid #262626",
        borderRadius: 12,
        padding: 24,
      }}
    >
      <div
        style={{
          width: 100,
          height: 14,
          borderRadius: 4,
          backgroundColor: "#262626",
          marginBottom: 16,
        }}
      />
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        {[1, 2, 3].map((i) => (
          <div
            key={i}
            style={{
              width: `${60 + i * 10}%`,
              height: 12,
              borderRadius: 4,
              backgroundColor: "#262626",
              animation: "pulse 1.5s ease-in-out infinite",
            }}
          />
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Section renderers
// ---------------------------------------------------------------------------

function SystemInfoCard({ stats }: { stats: SystemStats }) {
  return (
    <Card title="System Info">
      <StatRow label="Hostname" value={stats.hostname} />
      <StatRow label="OS" value={`${stats.os} / ${stats.arch}`} />
      <StatRow label="Kernel" value={stats.kernel} />
      <StatRow label="Uptime" value={stats.uptime} />
    </Card>
  );
}

function CPUCard({ stats }: { stats: SystemStats }) {
  const { cpu } = stats;
  return (
    <Card title="CPU">
      <StatRow label="Model" value={cpu.model} />
      <StatRow label="Cores / Threads" value={`${cpu.cores} / ${cpu.threads}`} />
      <div style={{ marginTop: 12, marginBottom: 6 }}>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            marginBottom: 6,
          }}
        >
          <span style={{ color: "#a3a3a3", fontSize: 13 }}>Usage</span>
          <span
            style={{
              color: usageColor(cpu.usage_percent),
              fontSize: 13,
              fontFamily: "monospace",
              fontWeight: 600,
            }}
          >
            {cpu.usage_percent.toFixed(1)}%
          </span>
        </div>
        <ProgressBar percent={cpu.usage_percent} />
      </div>
    </Card>
  );
}

function MemoryCard({ stats }: { stats: SystemStats }) {
  const { memory } = stats;
  return (
    <Card title="Memory">
      <StatRow label="Total" value={formatBytes(memory.total_bytes)} />
      <StatRow label="Used" value={formatBytes(memory.used_bytes)} />
      <StatRow label="Available" value={formatBytes(memory.available_bytes)} />
      <div style={{ marginTop: 12, marginBottom: 6 }}>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            marginBottom: 6,
          }}
        >
          <span style={{ color: "#a3a3a3", fontSize: 13 }}>Usage</span>
          <span
            style={{
              color: usageColor(memory.usage_percent),
              fontSize: 13,
              fontFamily: "monospace",
              fontWeight: 600,
            }}
          >
            {memory.usage_percent.toFixed(1)}%
          </span>
        </div>
        <ProgressBar percent={memory.usage_percent} />
      </div>
    </Card>
  );
}

function DiskCards({ stats }: { stats: SystemStats }) {
  if (!stats.disk || stats.disk.length === 0) {
    return (
      <Card title="Disk">
        <span style={{ color: "#737373", fontSize: 13 }}>
          No disk information available
        </span>
      </Card>
    );
  }

  return (
    <>
      {stats.disk.map((d) => (
        <Card key={d.mount} title={`Disk ${d.mount}`}>
          <StatRow label="Device" value={d.device} />
          <StatRow label="Total" value={formatBytes(d.total_bytes)} />
          <StatRow label="Used" value={formatBytes(d.used_bytes)} />
          <StatRow label="Available" value={formatBytes(d.available_bytes)} />
          <div style={{ marginTop: 12, marginBottom: 6 }}>
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                marginBottom: 6,
              }}
            >
              <span style={{ color: "#a3a3a3", fontSize: 13 }}>Usage</span>
              <span
                style={{
                  color: usageColor(d.usage_percent),
                  fontSize: 13,
                  fontFamily: "monospace",
                  fontWeight: 600,
                }}
              >
                {d.usage_percent.toFixed(1)}%
              </span>
            </div>
            <ProgressBar percent={d.usage_percent} />
          </div>
        </Card>
      ))}
    </>
  );
}

function NetworkCard({ stats }: { stats: SystemStats }) {
  const ifaces = stats.network.interfaces ?? [];
  return (
    <Card title="Network">
      {ifaces.length === 0 ? (
        <span style={{ color: "#737373", fontSize: 13 }}>
          No interfaces found
        </span>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {ifaces.map((iface) => (
            <div
              key={iface.name}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "8px 12px",
                backgroundColor: "#0a0a0a",
                borderRadius: 8,
                border: "1px solid #1a1a1a",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <div
                  style={{
                    width: 8,
                    height: 8,
                    borderRadius: "50%",
                    backgroundColor:
                      iface.status === "up" ? "#22c55e" : "#ef4444",
                    boxShadow:
                      iface.status === "up"
                        ? "0 0 6px #22c55e88"
                        : "0 0 6px #ef444488",
                  }}
                />
                <span
                  style={{
                    color: "#e5e5e5",
                    fontSize: 13,
                    fontFamily: "monospace",
                  }}
                >
                  {iface.name}
                </span>
              </div>
              <span
                style={{
                  color: "#a3a3a3",
                  fontSize: 13,
                  fontFamily: "monospace",
                }}
              >
                {iface.ip || "no address"}
              </span>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Main dashboard
// ---------------------------------------------------------------------------

export function SystemDashboard() {
  const { stats, loading, error } = useSystemStats(5000);

  return (
    <div
      style={{
        backgroundColor: "#0a0a0a",
        minHeight: "100%",
        padding: 24,
        color: "#e5e5e5",
        fontFamily:
          '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
      }}
    >
      <style>
        {`@keyframes pulse {
          0%, 100% { opacity: 0.4; }
          50% { opacity: 0.8; }
        }`}
      </style>

      <h2
        style={{
          fontSize: 20,
          fontWeight: 600,
          marginTop: 0,
          marginBottom: 24,
          color: "#f5f5f5",
        }}
      >
        System Dashboard
      </h2>

      {error && (
        <div
          style={{
            padding: "12px 16px",
            marginBottom: 16,
            backgroundColor: "#1c0a0a",
            border: "1px solid #7f1d1d",
            borderRadius: 8,
            color: "#fca5a5",
            fontSize: 13,
          }}
        >
          Failed to load system stats: {error}
        </div>
      )}

      {loading && !stats ? (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))",
            gap: 16,
          }}
        >
          {[1, 2, 3, 4].map((i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      ) : stats ? (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))",
            gap: 16,
          }}
        >
          <SystemInfoCard stats={stats} />
          <CPUCard stats={stats} />
          <MemoryCard stats={stats} />
          <NetworkCard stats={stats} />
          <DiskCards stats={stats} />
        </div>
      ) : null}
    </div>
  );
}
