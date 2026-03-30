import { useState, useEffect, useCallback, useRef } from "react";
import type {
  Container,
  ContainerStats,
  DockerImage,
  RunContainerRequest,
} from "@/types/docker";

interface UseDockerReturn {
  containers: Container[];
  stats: ContainerStats[];
  images: DockerImage[];
  loading: boolean;
  error: string | null;
  startContainer: (id: string) => Promise<void>;
  stopContainer: (id: string) => Promise<void>;
  restartContainer: (id: string) => Promise<void>;
  removeContainer: (id: string, force: boolean) => Promise<void>;
  runContainer: (req: RunContainerRequest) => Promise<void>;
  pullImage: (image: string) => Promise<void>;
  getLogs: (id: string, tail: number) => Promise<string>;
  composeUp: (yaml: string, project: string) => Promise<void>;
  composeDown: (project: string) => Promise<void>;
  fetchImages: () => Promise<void>;
}

export function useDocker(intervalMs = 5000): UseDockerReturn {
  const [containers, setContainers] = useState<Container[]>([]);
  const [stats, setStats] = useState<ContainerStats[]>([]);
  const [images, setImages] = useState<DockerImage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const initialFetchDone = useRef(false);

  const fetchContainers = useCallback(async (isInitial: boolean) => {
    if (isInitial) setLoading(true);
    try {
      const res = await fetch("/api/docker/containers");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setContainers(data.containers ?? data ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch containers");
    } finally {
      if (isInitial) setLoading(false);
    }
  }, []);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch("/api/docker/stats");
      if (!res.ok) return;
      const data = await res.json();
      setStats(data.stats ?? data ?? []);
    } catch {
      // stats are best-effort, don't set error
    }
  }, []);

  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      fetchContainers(true);
      fetchStats();
    }

    const id = setInterval(() => {
      fetchContainers(false);
      fetchStats();
    }, intervalMs);

    return () => clearInterval(id);
  }, [fetchContainers, fetchStats, intervalMs]);

  const containerAction = useCallback(
    async (id: string, action: string) => {
      const res = await fetch(
        `/api/docker/containers/action?id=${encodeURIComponent(id)}&action=${action}`,
        { method: "POST" }
      );
      if (!res.ok) throw new Error(`Action ${action} failed: HTTP ${res.status}`);
      await fetchContainers(false);
    },
    [fetchContainers]
  );

  const startContainer = useCallback(
    (id: string) => containerAction(id, "start"),
    [containerAction]
  );

  const stopContainer = useCallback(
    (id: string) => containerAction(id, "stop"),
    [containerAction]
  );

  const restartContainer = useCallback(
    (id: string) => containerAction(id, "restart"),
    [containerAction]
  );

  const removeContainer = useCallback(
    async (id: string, force: boolean) => {
      const res = await fetch(
        `/api/docker/containers?id=${encodeURIComponent(id)}&force=${force}`,
        { method: "DELETE" }
      );
      if (!res.ok) throw new Error(`Remove failed: HTTP ${res.status}`);
      await fetchContainers(false);
    },
    [fetchContainers]
  );

  const runContainer = useCallback(
    async (req: RunContainerRequest) => {
      const res = await fetch("/api/docker/containers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      });
      if (!res.ok) throw new Error(`Run container failed: HTTP ${res.status}`);
      await fetchContainers(false);
    },
    [fetchContainers]
  );

  const pullImage = useCallback(async (image: string) => {
    const res = await fetch("/api/docker/images/pull", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ image }),
    });
    if (!res.ok) throw new Error(`Pull image failed: HTTP ${res.status}`);
  }, []);

  const getLogs = useCallback(async (id: string, tail: number): Promise<string> => {
    const res = await fetch(
      `/api/docker/containers/logs?id=${encodeURIComponent(id)}&tail=${tail}`
    );
    if (!res.ok) throw new Error(`Fetch logs failed: HTTP ${res.status}`);
    return res.text();
  }, []);

  const composeUp = useCallback(
    async (yaml: string, project: string) => {
      const res = await fetch("/api/docker/compose", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ yaml, project, action: "up" }),
      });
      if (!res.ok) throw new Error(`Compose up failed: HTTP ${res.status}`);
      await fetchContainers(false);
    },
    [fetchContainers]
  );

  const composeDown = useCallback(
    async (_project: string) => {
      const res = await fetch("/api/docker/compose", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ yaml: "", project: _project, action: "down" }),
      });
      if (!res.ok) throw new Error(`Compose down failed: HTTP ${res.status}`);
      await fetchContainers(false);
    },
    [fetchContainers]
  );

  const fetchImages = useCallback(async () => {
    const res = await fetch("/api/docker/images");
    if (!res.ok) throw new Error(`Fetch images failed: HTTP ${res.status}`);
    const data = await res.json();
    setImages(data.images ?? data ?? []);
  }, []);

  return {
    containers,
    stats,
    images,
    loading,
    error,
    startContainer,
    stopContainer,
    restartContainer,
    removeContainer,
    runContainer,
    pullImage,
    getLogs,
    composeUp,
    composeDown,
    fetchImages,
  };
}
