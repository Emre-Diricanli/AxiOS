import { useState, useEffect, useCallback, useRef } from "react";
import type { XAIOAuthStatus } from "@/types/providers";

interface UseXAIOAuthReturn {
  status: XAIOAuthStatus | null;
  starting: boolean;
  start: () => Promise<void>;
}

// Drives the xAI SuperGrok device-code OAuth flow: fetches status once on
// mount, polls every 2s while a flow is pending, and exposes start() to
// kick off (or retry) a flow.
export function useXAIOAuth(): UseXAIOAuthReturn {
  const [status, setStatus] = useState<XAIOAuthStatus | null>(null);
  const [starting, setStarting] = useState(false);
  const initialFetchDone = useRef(false);

  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch("/api/providers/xai/oauth/status");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data: XAIOAuthStatus = await res.json();
      setStatus(data);
    } catch {
      // Keep the last known status; the pending poll will retry.
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      fetchStatus();
    }
  }, [fetchStatus]);

  // Poll every 2s while the device-code flow is pending
  useEffect(() => {
    if (status?.state !== "pending") return;
    const id = setInterval(fetchStatus, 2000);
    return () => clearInterval(id);
  }, [status?.state, fetchStatus]);

  const start = useCallback(async () => {
    setStarting(true);
    try {
      const res = await fetch("/api/providers/xai/oauth/start", {
        method: "POST",
      });
      // Both 200 (pending) and 502 (error) carry an XAIOAuthStatus body.
      const data: XAIOAuthStatus | null = await res.json().catch(() => null);
      if (data) {
        setStatus(data);
      } else {
        setStatus({
          state: "error",
          error: `Failed to start SuperGrok connect: HTTP ${res.status}`,
          connected: false,
        });
      }
    } catch (err) {
      setStatus({
        state: "error",
        error:
          err instanceof Error
            ? err.message
            : "Failed to start SuperGrok connect",
        connected: false,
      });
    } finally {
      setStarting(false);
    }
  }, []);

  return { status, starting, start };
}
