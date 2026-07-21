import { useCallback, useEffect, useState } from "react";
import type { OllamaHost } from "@/types/hosts";

export interface InferenceMetrics {
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
  tokens_per_second: number;
  completed_at: string;
}

export interface RuntimeStatus {
  backend: "cloud" | "local" | "supergrok" | string;
  routing: "auto" | "cloud_only" | "local_only" | "cost_aware" | string;
  provider?: string;
  model?: string;
  localModel?: string;
  inference?: InferenceMetrics;
}

interface UseRuntimeStatusReturn {
  daemonOnline: boolean;
  status: RuntimeStatus | null;
  activeHost: OllamaHost | null;
  loading: boolean;
}

export function useRuntimeStatus(intervalMs = 5000): UseRuntimeStatusReturn {
  const [daemonOnline, setDaemonOnline] = useState(false);
  const [status, setStatus] = useState<RuntimeStatus | null>(null);
  const [activeHost, setActiveHost] = useState<OllamaHost | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const [healthResponse, statusResponse, hostsResponse] = await Promise.all([
        fetch("/api/health"),
        fetch("/api/status"),
        fetch("/api/hosts"),
      ]);
      if (!healthResponse.ok || !statusResponse.ok) {
        throw new Error("daemon unavailable");
      }
      const [runtimeData, hostsData] = await Promise.all([
        statusResponse.json() as Promise<RuntimeStatus>,
        hostsResponse.ok ? hostsResponse.json() : Promise.resolve({ hosts: [] }),
      ]);
      const hosts = (hostsData.hosts ?? hostsData ?? []) as OllamaHost[];
      setStatus(runtimeData);
      setActiveHost(hosts.find((host) => host.active) ?? null);
      setDaemonOnline(true);
    } catch {
      setDaemonOnline(false);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, intervalMs);
    return () => clearInterval(timer);
  }, [intervalMs, refresh]);

  return { daemonOnline, status, activeHost, loading };
}
