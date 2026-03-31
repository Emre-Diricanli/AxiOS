import { useState, useEffect, useCallback, useRef } from "react";
import type { InstalledModel, MarketplaceModel, PullProgress } from "@/types/models";

interface UseModelMarketplaceReturn {
  installed: InstalledModel[];
  marketplace: MarketplaceModel[];
  pulling: Map<string, PullProgress>;
  loading: boolean;
  error: string | null;
  pullModel: (name: string) => void;
  deleteModel: (name: string) => Promise<void>;
  refreshInstalled: () => Promise<void>;
  isInstalled: (name: string) => boolean;
}

export function useModelMarketplace(): UseModelMarketplaceReturn {
  const [installed, setInstalled] = useState<InstalledModel[]>([]);
  const [marketplace, setMarketplace] = useState<MarketplaceModel[]>([]);
  const [pulling, setPulling] = useState<Map<string, PullProgress>>(new Map());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const initialFetchDone = useRef(false);

  const fetchInstalled = useCallback(async () => {
    try {
      const res = await fetch("/api/models/installed");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setInstalled(data.models ?? data ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch installed models");
    }
  }, []);

  const fetchMarketplace = useCallback(async () => {
    try {
      const res = await fetch("/api/models/marketplace");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setMarketplace(data.models ?? data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch marketplace");
    }
  }, []);

  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      setLoading(true);
      Promise.all([fetchInstalled(), fetchMarketplace()]).finally(() => {
        setLoading(false);
      });
    }
  }, [fetchInstalled, fetchMarketplace]);

  const refreshInstalled = useCallback(async () => {
    await fetchInstalled();
  }, [fetchInstalled]);

  const pullModel = useCallback(
    (name: string) => {
      setPulling((prev) => {
        const next = new Map(prev);
        next.set(name, { status: "starting", percent: 0 });
        return next;
      });

      const eventSource = new EventSource(
        `/api/models/pull?name=${encodeURIComponent(name)}`
      );

      // Also fire a POST to initiate the pull (the SSE connection reads progress)
      fetch("/api/models/pull", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      }).catch(() => {
        // The EventSource approach or POST approach — try both patterns
      });

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as PullProgress;
          setPulling((prev) => {
            const next = new Map(prev);
            next.set(name, data);
            return next;
          });

          if (data.status === "success" || data.status === "done" || data.percent >= 100) {
            eventSource.close();
            setPulling((prev) => {
              const next = new Map(prev);
              next.delete(name);
              return next;
            });
            fetchInstalled();
          }
        } catch {
          // ignore parse errors
        }
      };

      eventSource.onerror = () => {
        eventSource.close();
        // Check if pull was actually successful by refreshing
        setPulling((prev) => {
          const next = new Map(prev);
          next.delete(name);
          return next;
        });
        fetchInstalled();
      };
    },
    [fetchInstalled]
  );

  const deleteModel = useCallback(
    async (name: string) => {
      const res = await fetch(
        `/api/models/delete?name=${encodeURIComponent(name)}`,
        { method: "DELETE" }
      );
      if (!res.ok) throw new Error(`Delete failed: HTTP ${res.status}`);
      await fetchInstalled();
    },
    [fetchInstalled]
  );

  const isInstalled = useCallback(
    (name: string) => {
      const prefix = name.split(":")[0].toLowerCase();
      return installed.some((m) => m.name.toLowerCase().startsWith(prefix));
    },
    [installed]
  );

  return {
    installed,
    marketplace,
    pulling,
    loading,
    error,
    pullModel,
    deleteModel,
    refreshInstalled,
    isInstalled,
  };
}
