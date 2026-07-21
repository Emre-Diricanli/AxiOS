import { useState, useEffect, useCallback, useRef } from "react";
import type { OllamaHost } from "@/types/hosts";

interface UseHostsReturn {
  hosts: OllamaHost[];
  loading: boolean;
  error: string | null;
  addHost: (name: string, host: string, port: number, telemetryPort?: number, telemetryToken?: string) => Promise<void>;
  removeHost: (id: string) => Promise<void>;
  activateHost: (id: string) => Promise<void>;
  updateTelemetryPort: (id: string, port: number, token?: string) => Promise<void>;
  checkHealth: () => Promise<void>;
  refreshHosts: () => Promise<void>;
}

export function useHosts(): UseHostsReturn {
  const [hosts, setHosts] = useState<OllamaHost[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const initialFetchDone = useRef(false);

  const fetchHosts = useCallback(async () => {
    try {
      const res = await fetch("/api/hosts");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setHosts(data.hosts ?? data ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch hosts");
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      setLoading(true);
      fetchHosts().finally(() => setLoading(false));
    }
  }, [fetchHosts]);

  // Auto-refresh every 15 seconds
  useEffect(() => {
    const interval = setInterval(() => {
      fetchHosts();
    }, 15000);
    return () => clearInterval(interval);
  }, [fetchHosts]);

  const refreshHosts = useCallback(async () => {
    await fetchHosts();
  }, [fetchHosts]);

  const addHost = useCallback(
    async (name: string, host: string, port: number, telemetryPort = 3000, telemetryToken = "") => {
      try {
        const res = await fetch("/api/hosts", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, host, port, telemetry_port: telemetryPort, telemetry_token: telemetryToken }),
        });
        if (!res.ok) throw new Error(`Failed to add host: HTTP ${res.status}`);
        await fetchHosts();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to add host";
        setError(msg);
        throw err;
      }
    },
    [fetchHosts]
  );

  const removeHost = useCallback(
    async (id: string) => {
      try {
        const res = await fetch(`/api/hosts?id=${encodeURIComponent(id)}`, {
          method: "DELETE",
        });
        if (!res.ok) throw new Error(`Failed to remove host: HTTP ${res.status}`);
        await fetchHosts();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to remove host";
        setError(msg);
        throw err;
      }
    },
    [fetchHosts]
  );

  const activateHost = useCallback(
    async (id: string) => {
      try {
        const res = await fetch(`/api/hosts/activate?id=${encodeURIComponent(id)}`, {
          method: "POST",
        });
        if (!res.ok) throw new Error(`Failed to activate host: HTTP ${res.status}`);
        await fetchHosts();
        window.dispatchEvent(new CustomEvent("axios-host-changed", { detail: { id } }));
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to activate host";
        setError(msg);
        throw err;
      }
    },
    [fetchHosts]
  );

  const checkHealth = useCallback(async () => {
    try {
      const res = await fetch("/api/hosts/health", { method: "POST" });
      if (!res.ok) throw new Error(`Health check failed: HTTP ${res.status}`);
      await fetchHosts();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Health check failed";
      setError(msg);
    }
  }, [fetchHosts]);

  const updateTelemetryPort = useCallback(async (id: string, port: number, token?: string) => {
    const payload: { id: string; telemetry_port: number; telemetry_token?: string } = { id, telemetry_port: port };
    if (token !== undefined) payload.telemetry_token = token;
    const res = await fetch("/api/hosts", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(`Failed to update telemetry port: HTTP ${res.status}`);
    await fetchHosts();
    window.dispatchEvent(new CustomEvent("axios-host-changed", { detail: { id } }));
  }, [fetchHosts]);

  return {
    hosts,
    loading,
    error,
    addHost,
    removeHost,
    activateHost,
    updateTelemetryPort,
    checkHealth,
    refreshHosts,
  };
}
