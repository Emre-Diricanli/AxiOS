import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { PullProgress } from "@/types/models";

export type ModelDownloadState = "starting" | "downloading" | "complete" | "error";

export interface ModelDownload {
  key: string;
  name: string;
  hostId?: string;
  hostName: string;
  state: ModelDownloadState;
  progress: PullProgress;
  startedAt: number;
  finishedAt?: number;
}

interface ModelDownloadContextValue {
  downloads: ModelDownload[];
  completionVersion: number;
  startDownload: (name: string, hostId?: string, hostName?: string) => void;
  dismissDownload: (key: string) => void;
  clearFinished: () => void;
}

const ModelDownloadContext = createContext<ModelDownloadContextValue | null>(null);

function downloadKey(name: string, hostId?: string) {
  return `${hostId ?? "active"}::${name}`;
}

function withHost(path: string, hostId?: string) {
  if (!hostId) return path;
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}host_id=${encodeURIComponent(hostId)}`;
}

export function ModelDownloadProvider({ children }: { children: ReactNode }) {
  const [downloads, setDownloads] = useState<Map<string, ModelDownload>>(new Map());
  const [completionVersion, setCompletionVersion] = useState(0);
  const activeDownloads = useRef(new Set<string>());

  const updateDownload = useCallback((key: string, update: (download: ModelDownload) => ModelDownload) => {
    setDownloads((previous) => {
      const current = previous.get(key);
      if (!current) return previous;
      const next = new Map(previous);
      next.set(key, update(current));
      return next;
    });
  }, []);

  const startDownload = useCallback((name: string, hostId?: string, hostName?: string) => {
    const key = downloadKey(name, hostId);
    if (activeDownloads.current.has(key)) return;

    activeDownloads.current.add(key);
    setDownloads((previous) => {
      const next = new Map(previous);
      next.set(key, {
        key,
        name,
        hostId,
        hostName: hostName ?? "Active host",
        state: "starting",
        progress: { status: "Starting download", percent: 0 },
        startedAt: Date.now(),
      });
      return next;
    });

    void (async () => {
      try {
        const response = await fetch(withHost("/api/models/pull", hostId), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name }),
        });
        if (!response.ok || !response.body) {
          throw new Error(`Pull failed: HTTP ${response.status}`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        while (true) {
          const { value, done } = await reader.read();
          buffer += decoder.decode(value, { stream: !done });
          const events = buffer.split("\n\n");
          buffer = events.pop() ?? "";
          for (const event of events) {
            const line = event.split("\n").find((entry) => entry.startsWith("data: "));
            if (!line) continue;
            const progress = JSON.parse(line.slice(6)) as PullProgress;
            if (progress.percent < 0) throw new Error(progress.status);
            updateDownload(key, (download) => ({
              ...download,
              state: "downloading",
              progress,
            }));
          }
          if (done) break;
        }

        updateDownload(key, (download) => ({
          ...download,
          state: "complete",
          progress: { ...download.progress, status: "Download complete", percent: 100 },
          finishedAt: Date.now(),
        }));
        setCompletionVersion((version) => version + 1);
      } catch (error) {
        const message = error instanceof Error ? error.message : "Model download failed";
        updateDownload(key, (download) => ({
          ...download,
          state: "error",
          progress: { ...download.progress, status: message, percent: -1 },
          finishedAt: Date.now(),
        }));
      } finally {
        activeDownloads.current.delete(key);
      }
    })();
  }, [updateDownload]);

  const dismissDownload = useCallback((key: string) => {
    if (activeDownloads.current.has(key)) return;
    setDownloads((previous) => {
      const next = new Map(previous);
      next.delete(key);
      return next;
    });
  }, []);

  const clearFinished = useCallback(() => {
    setDownloads((previous) => new Map(
      Array.from(previous.entries()).filter(([key]) => activeDownloads.current.has(key))
    ));
  }, []);

  const value = useMemo<ModelDownloadContextValue>(() => ({
    downloads: Array.from(downloads.values()),
    completionVersion,
    startDownload,
    dismissDownload,
    clearFinished,
  }), [clearFinished, completionVersion, dismissDownload, downloads, startDownload]);

  return (
    <ModelDownloadContext.Provider value={value}>
      {children}
    </ModelDownloadContext.Provider>
  );
}

export function useModelDownloads() {
  const context = useContext(ModelDownloadContext);
  if (!context) throw new Error("useModelDownloads must be used within ModelDownloadProvider");
  return context;
}
