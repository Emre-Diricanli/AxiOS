import type {
  ObsidianEntry,
  ObsidianNote,
  ObsidianSearchHit,
  ObsidianVaultStatus,
} from "@/types/obsidian";

export const OBSIDIAN_SETTINGS_EVENT = "axios-open-obsidian-settings";

export class ObsidianApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
    this.name = "ObsidianApiError";
  }
}

export function openObsidianSettings(): void {
  window.dispatchEvent(new Event(OBSIDIAN_SETTINGS_EVENT));
}

async function parseError(response: Response): Promise<string> {
  const body = (await response.json().catch(() => null)) as { error?: string } | null;
  return body?.error || `Request failed (${response.status})`;
}

async function obsidianRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const message = await parseError(response);
    if (response.status === 409) {
      openObsidianSettings();
    }
    throw new ObsidianApiError(message, response.status);
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export function getObsidianStatus(): Promise<ObsidianVaultStatus> {
  return obsidianRequest<ObsidianVaultStatus>("/api/obsidian/status");
}

export function setObsidianVault(path: string): Promise<ObsidianVaultStatus> {
  return obsidianRequest<ObsidianVaultStatus>("/api/obsidian/vault", {
    method: "PUT",
    body: JSON.stringify({ path }),
  });
}

export async function listObsidianNotes(folder = "", recursive = false): Promise<ObsidianEntry[]> {
  const params = new URLSearchParams();
  if (folder) params.set("folder", folder);
  if (recursive) params.set("recursive", "true");
  const suffix = params.size ? `?${params.toString()}` : "";
  const response = await obsidianRequest<{ entries: ObsidianEntry[] }>(`/api/obsidian/notes${suffix}`);
  return response.entries ?? [];
}

export function getObsidianNote(path: string): Promise<ObsidianNote> {
  return obsidianRequest<ObsidianNote>(`/api/obsidian/note?${new URLSearchParams({ path })}`);
}

export function saveObsidianNote(path: string, content: string): Promise<{ ok: true }> {
  return obsidianRequest<{ ok: true }>("/api/obsidian/note", {
    method: "PUT",
    body: JSON.stringify({ path, content }),
  });
}

export function deleteObsidianNote(path: string): Promise<void> {
  return obsidianRequest<void>(`/api/obsidian/note?${new URLSearchParams({ path })}`, {
    method: "DELETE",
  });
}

export async function searchObsidianNotes(
  query: string,
  tag: string,
  limit = 20
): Promise<ObsidianSearchHit[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (query.trim()) params.set("q", query.trim());
  if (tag.trim()) params.set("tag", tag.trim());
  const response = await obsidianRequest<{ hits: ObsidianSearchHit[] }>(
    `/api/obsidian/search?${params}`
  );
  return response.hits ?? [];
}

export function formatObsidianBytes(bytes = 0): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}
