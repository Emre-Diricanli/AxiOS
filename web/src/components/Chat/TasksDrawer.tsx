import { useEffect, useState, useCallback } from "react";

interface CodeTask {
  id: string;
  prompt: string;
  directory: string;
  status: "queued" | "running" | "done" | "failed" | "aborted";
  created_at: string;
  cost_usd?: number;
  last_error?: string;
}

const STATUS_STYLES: Record<CodeTask["status"], string> = {
  queued: "bg-yellow-400",
  running: "bg-blue-400 animate-pulse",
  done: "bg-emerald-400",
  failed: "bg-red-400",
  aborted: "bg-gray-400",
};

function taskAge(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

// Drawer listing delegated opencode tasks with status, cost, and abort.
export function TasksDrawer() {
  const [tasks, setTasks] = useState<CodeTask[]>([]);
  const [unavailable, setUnavailable] = useState(false);

  const fetchTasks = useCallback(async () => {
    try {
      const res = await fetch("/api/code/tasks");
      if (!res.ok) {
        setUnavailable(true);
        return;
      }
      const data = await res.json();
      setTasks(data.tasks ?? []);
      setUnavailable(false);
    } catch {
      setUnavailable(true);
    }
  }, []);

  useEffect(() => {
    fetchTasks();
    const id = setInterval(fetchTasks, 3000);
    return () => clearInterval(id);
  }, [fetchTasks]);

  const abortTask = async (id: string) => {
    try {
      await fetch(`/api/code/tasks/${encodeURIComponent(id)}`, { method: "DELETE" });
      await fetchTasks();
    } catch {
      // ignore
    }
  };

  return (
    <div className="border-b border-border bg-background/50 max-h-52 overflow-y-auto scrollbar-none">
      {unavailable ? (
        <div className="px-4 py-3 text-xs text-muted-foreground text-center">opencode integration unavailable</div>
      ) : tasks.length === 0 ? (
        <div className="px-4 py-3 text-xs text-muted-foreground text-center">
          No delegated tasks yet — ask the chat to delegate a coding task
        </div>
      ) : (
        <div className="py-1">
          {tasks.map((t) => (
            <div key={t.id} className="flex items-center gap-2.5 px-4 py-2 group hover:bg-secondary/50 transition-colors">
              <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_STYLES[t.status] ?? "bg-gray-500"}`} title={t.status} />
              <div className="min-w-0 flex-1">
                <p className="text-xs text-foreground/80 truncate">{t.prompt}</p>
                <p className="text-[10px] text-muted-foreground mt-0.5 font-mono truncate">
                  {t.status}
                  {typeof t.cost_usd === "number" && t.cost_usd > 0 && <> &middot; ${t.cost_usd.toFixed(4)}</>}
                  {" "}&middot; {taskAge(t.created_at)}
                  {t.last_error && <> &middot; {t.last_error}</>}
                </p>
              </div>
              {(t.status === "running" || t.status === "queued") && (
                <button
                  onClick={() => abortTask(t.id)}
                  title="Abort task"
                  className="w-5 h-5 rounded flex items-center justify-center opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-400 hover:bg-red-400/10 transition-all shrink-0"
                >
                  <svg width="9" height="9" viewBox="0 0 24 24" fill="currentColor">
                    <rect x="6" y="6" width="12" height="12" rx="2" />
                  </svg>
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
