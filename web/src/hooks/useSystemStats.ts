import { useState, useEffect, useCallback } from "react";
import type { OllamaHost } from "@/types/hosts";
import type { RunningModelStats, SystemStats } from "@/types/system";

export interface HostTelemetry {
  host: OllamaHost;
  source: "local" | "agent" | "ollama";
  system?: SystemStats;
  ollama_version?: string;
  running_models: RunningModelStats[];
  latency_ms: number;
  message?: string;
}

interface UseSystemStatsReturn {
  stats: SystemStats | null;
  loading: boolean;
  error: string | null;
  telemetry: HostTelemetry | null;
  isRemote: boolean;
}

export function useSystemStats(intervalMs = 5000, hostId?: string): UseSystemStatsReturn {
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [telemetry, setTelemetry] = useState<HostTelemetry | null>(null);

  const fetchStats = useCallback(async (isInitial: boolean) => {
    if (isInitial) {
      setLoading(true);
    }
    try {
      const query = hostId ? `?id=${encodeURIComponent(hostId)}` : "";
      const res = await fetch(`/api/hosts/stats${query}`);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const data: HostTelemetry = await res.json();
      setTelemetry(data);
      setStats(data.system ?? null);
      setError(data.system ? null : data.message ?? "Hardware telemetry unavailable");
    } catch (err) {
      setStats(null);
      setTelemetry(null);
      setError(err instanceof Error ? err.message : "Failed to fetch stats");
    } finally {
      if (isInitial) {
        setLoading(false);
      }
    }
  }, [hostId]);

  useEffect(() => {
    fetchStats(true);

    const id = setInterval(() => {
      fetchStats(false);
    }, intervalMs);

    return () => clearInterval(id);
  }, [fetchStats, intervalMs]);

  useEffect(() => {
    const refreshForHost = (event: Event) => {
      const changedHost = (event as CustomEvent<{ id?: string }>).detail?.id;
      if (!hostId || !changedHost || changedHost === hostId) fetchStats(true);
    };
    window.addEventListener("axios-host-changed", refreshForHost);
    return () => window.removeEventListener("axios-host-changed", refreshForHost);
  }, [fetchStats]);

  return { stats, loading, error, telemetry, isRemote: Boolean(telemetry && telemetry.source !== "local") };
}
