import { useEffect, useRef, useState } from "react";
import { useSystemStats } from "@/hooks/useSystemStats";
import type { HostTelemetry } from "@/hooks/useSystemStats";
import type { SystemStats } from "@/types/system";

// One telemetry sample in the rolling history buffer.
export interface MetricSample {
  t: number; // epoch ms
  cpu: number;
  mem: number;
  gpus: Array<{ util: number; vramPercent: number; temp: number }>;
  disk: Array<{ mount: string; percent: number }>;
}

// CAPACITY samples at a 1s poll ≈ 5 minutes of history (charts show the
// trailing 60s; the extra buffer leaves room for future scrubbing).
const CAPACITY = 300;

interface UseMetricsHistoryReturn {
  stats: SystemStats | null;
  telemetry: HostTelemetry | null;
  isRemote: boolean;
  loading: boolean;
  error: string | null;
  history: MetricSample[];
}

// useMetricsHistory polls the active host's telemetry and accumulates a
// rolling history buffer for time-series charts. History tracks whichever
// host is active; it resets when the host identity changes or telemetry
// becomes unavailable. Polling only runs while the consuming page is mounted.
export function useMetricsHistory(intervalMs = 1000): UseMetricsHistoryReturn {
  const { stats, telemetry, isRemote, loading, error } = useSystemStats(intervalMs);

  const bufferRef = useRef<MetricSample[]>([]);
  const hostKeyRef = useRef<string | null>(null);
  const [, setTick] = useState(0);

  useEffect(() => {
    const hostKey = telemetry ? `${telemetry.source}:${telemetry.host.id ?? telemetry.host.host}:${telemetry.host.port}` : null;

    // Host switched (or telemetry lost): start a fresh buffer so charts never
    // blend samples from two different machines.
    if (hostKey !== hostKeyRef.current) {
      hostKeyRef.current = hostKey;
      bufferRef.current = [];
    }

    if (!stats) {
      setTick((tick) => tick + 1);
      return;
    }

    const sample: MetricSample = {
      t: Date.now(),
      cpu: stats.cpu.usage_percent,
      mem: stats.memory.usage_percent,
      gpus: (stats.gpu ?? []).map((gpu) => ({
        util: gpu.utilization_percent,
        vramPercent: gpu.memory_usage_percent,
        temp: gpu.temperature_c,
      })),
      disk: (stats.disk ?? []).map((disk) => ({ mount: disk.mount, percent: disk.usage_percent })),
    };

    const buffer = bufferRef.current;
    buffer.push(sample);
    if (buffer.length > CAPACITY) {
      buffer.splice(0, buffer.length - CAPACITY);
    }
    setTick((tick) => tick + 1);
  }, [stats, telemetry]);

  // Return a snapshot: the ring buffer is mutated in place, so consumers
  // memoizing on `history` need a fresh reference per poll tick.
  return { stats, telemetry, isRemote, loading, error, history: bufferRef.current.slice() };
}
