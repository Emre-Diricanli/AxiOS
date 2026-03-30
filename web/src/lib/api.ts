import type { StatusInfo } from "@/types/messages";

export async function getStatus(): Promise<StatusInfo> {
  const res = await fetch("/api/status");
  return res.json();
}

export async function getHealth(): Promise<{ status: string }> {
  const res = await fetch("/api/health");
  return res.json();
}
