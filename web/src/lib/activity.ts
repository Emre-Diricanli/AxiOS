import { useEffect, useState } from "react";

export type ActivityKind = "tool" | "approval" | "files" | "task";

export interface AIActivity {
  id: string;
  kind: ActivityKind;
  title: string;
  detail?: string;
  status?: "pending" | "success" | "denied" | "failed";
  timestamp: string;
}

const STORAGE_KEY = "axios-ai-activity";

export function getActivity(): AIActivity[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]") as AIActivity[];
  } catch {
    return [];
  }
}

export function recordActivity(activity: Omit<AIActivity, "id" | "timestamp">): void {
  const next: AIActivity = {
    ...activity,
    detail: activity.detail?.slice(0, 800),
    id: crypto.randomUUID(),
    timestamp: new Date().toISOString(),
  };
  const activities = [next, ...getActivity()].slice(0, 100);
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(activities));
  } catch {
    return;
  }
  window.dispatchEvent(new CustomEvent<AIActivity>("axios-activity", { detail: next }));
}

export function useAIActivity(): AIActivity[] {
  const [activities, setActivities] = useState<AIActivity[]>(getActivity);
  useEffect(() => {
    const update = () => setActivities(getActivity());
    window.addEventListener("axios-activity", update);
    window.addEventListener("storage", update);
    return () => {
      window.removeEventListener("axios-activity", update);
      window.removeEventListener("storage", update);
    };
  }, []);
  return activities;
}
