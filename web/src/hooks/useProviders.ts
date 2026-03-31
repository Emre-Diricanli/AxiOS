import { useState, useEffect, useCallback, useRef } from "react";
import type { CloudProvider } from "@/types/providers";

interface UseProvidersReturn {
  providers: CloudProvider[];
  loading: boolean;
  error: string | null;
  setAPIKey: (providerId: string, apiKey: string) => Promise<void>;
  removeAPIKey: (providerId: string) => Promise<void>;
  activateProvider: (providerId: string, model: string) => Promise<void>;
  refreshProviders: () => Promise<void>;
}

export function useProviders(): UseProvidersReturn {
  const [providers, setProviders] = useState<CloudProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const initialFetchDone = useRef(false);

  const fetchProviders = useCallback(async () => {
    try {
      const res = await fetch("/api/providers");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setProviders(data.providers ?? data ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch providers");
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      setLoading(true);
      fetchProviders().finally(() => setLoading(false));
    }
  }, [fetchProviders]);

  const refreshProviders = useCallback(async () => {
    await fetchProviders();
  }, [fetchProviders]);

  const setAPIKey = useCallback(
    async (providerId: string, apiKey: string) => {
      try {
        const res = await fetch("/api/providers/key", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ provider: providerId, api_key: apiKey }),
        });
        if (!res.ok) throw new Error(`Failed to set API key: HTTP ${res.status}`);
        await fetchProviders();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to set API key";
        setError(msg);
        throw err;
      }
    },
    [fetchProviders]
  );

  const removeAPIKey = useCallback(
    async (providerId: string) => {
      try {
        const res = await fetch(
          `/api/providers/key?provider=${encodeURIComponent(providerId)}`,
          { method: "DELETE" }
        );
        if (!res.ok) throw new Error(`Failed to remove API key: HTTP ${res.status}`);
        await fetchProviders();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to remove API key";
        setError(msg);
        throw err;
      }
    },
    [fetchProviders]
  );

  const activateProvider = useCallback(
    async (providerId: string, model: string) => {
      try {
        const res = await fetch("/api/providers/activate", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ provider: providerId, model }),
        });
        if (!res.ok) throw new Error(`Failed to activate provider: HTTP ${res.status}`);
        await fetchProviders();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to activate provider";
        setError(msg);
        throw err;
      }
    },
    [fetchProviders]
  );

  return {
    providers,
    loading,
    error,
    setAPIKey,
    removeAPIKey,
    activateProvider,
    refreshProviders,
  };
}
