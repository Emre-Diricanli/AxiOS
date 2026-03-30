import { useState, useEffect, useCallback, useRef } from "react";
import type { SystemStats } from "@/types/system";

interface UseSystemStatsReturn {
  stats: SystemStats | null;
  loading: boolean;
  error: string | null;
}

export function useSystemStats(intervalMs = 5000): UseSystemStatsReturn {
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const initialFetchDone = useRef(false);

  const fetchStats = useCallback(async (isInitial: boolean) => {
    if (isInitial) {
      setLoading(true);
    }
    try {
      const res = await fetch("/api/system/stats");
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const data: SystemStats = await res.json();
      setStats(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch stats");
    } finally {
      if (isInitial) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      fetchStats(true);
    }

    const id = setInterval(() => {
      fetchStats(false);
    }, intervalMs);

    return () => clearInterval(id);
  }, [fetchStats, intervalMs]);

  return { stats, loading, error };
}
