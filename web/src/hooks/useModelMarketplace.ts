import { useState, useEffect, useCallback, useMemo } from "react";
import { useModelDownloads } from "@/contexts/ModelDownloadContext";
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
  refreshAll: () => Promise<void>;
  isInstalled: (name: string) => boolean;
}

interface UseHuggingFaceModelsReturn {
  models: MarketplaceModel[];
  loading: boolean;
  error: string | null;
}

export function useHuggingFaceModels(query: string | null): UseHuggingFaceModelsReturn {
  const [models, setModels] = useState<MarketplaceModel[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (query === null) {
      setModels([]);
      setLoading(false);
      setError(null);
      return;
    }

    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      setLoading(true);
      setError(null);
      try {
        const params = new URLSearchParams({ limit: "24" });
        if (query.trim()) params.set("q", query.trim());
        const response = await fetch(`/api/models/search?${params}`, { signal: controller.signal });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        setModels(data.models ?? []);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : "Failed to search Hugging Face");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    }, query.trim() ? 350 : 0);

    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [query]);

  return { models, loading, error };
}

export function useModelMarketplace(hostId?: string, hostName?: string): UseModelMarketplaceReturn {
  const [installed, setInstalled] = useState<InstalledModel[]>([]);
  const [marketplace, setMarketplace] = useState<MarketplaceModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { downloads, completionVersion, startDownload } = useModelDownloads();
  const hostQuery = hostId ? `host_id=${encodeURIComponent(hostId)}` : "";
  const withHost = (path: string) => hostQuery ? `${path}${path.includes("?") ? "&" : "?"}${hostQuery}` : path;

  const fetchInstalled = useCallback(async () => {
    try {
      const res = await fetch(withHost("/api/models/installed"));
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setInstalled(data.models ?? data ?? []);
      setError(data.warning ?? null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch installed models");
    }
  }, [hostId]);

  const fetchMarketplace = useCallback(async () => {
    try {
      const res = await fetch(withHost("/api/models/marketplace"));
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setMarketplace(data.models ?? data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch marketplace");
    }
  }, [hostId]);

  useEffect(() => {
    setLoading(true);
    setInstalled([]);
    Promise.all([fetchInstalled(), fetchMarketplace()]).finally(() => setLoading(false));
  }, [fetchInstalled, fetchMarketplace]);

  useEffect(() => {
    if (completionVersion > 0) void fetchInstalled();
  }, [completionVersion, fetchInstalled]);

  const refreshInstalled = useCallback(async () => {
    await fetchInstalled();
  }, [fetchInstalled]);

  const refreshAll = useCallback(async () => {
    await Promise.all([fetchInstalled(), fetchMarketplace()]);
  }, [fetchInstalled, fetchMarketplace]);

  const pullModel = useCallback(
    (name: string) => {
      startDownload(name, hostId, hostName);
    },
    [hostId, hostName, startDownload]
  );

  const pulling = useMemo(() => new Map(
    downloads
      .filter((download) => download.hostId === hostId && (download.state === "starting" || download.state === "downloading"))
      .map((download) => [download.name, download.progress])
  ), [downloads, hostId]);

  const deleteModel = useCallback(
    async (name: string) => {
      const res = await fetch(
        withHost(`/api/models/delete?name=${encodeURIComponent(name)}`),
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
    refreshAll,
    isInstalled,
  };
}
