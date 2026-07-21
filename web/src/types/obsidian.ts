export interface ObsidianVaultStatus {
  configured: boolean;
  vault_path?: string;
  name?: string;
  looks_like_vault?: boolean;
  notes?: number;
  folders?: number;
  size_bytes?: number;
  error?: string;
}

export interface ObsidianEntry {
  path: string;
  name: string;
  is_folder: boolean;
  size: number;
  modified: string;
}

export interface ObsidianNote {
  path: string;
  content: string;
  frontmatter: Record<string, unknown> | null;
  tags: string[];
  size: number;
  modified: string;
}

export interface ObsidianSearchHit {
  path: string;
  name: string;
  snippet: string;
  modified: string;
}
