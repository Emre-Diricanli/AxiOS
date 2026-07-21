export interface InstalledModel {
  name: string;
  size: number;
  size_human: string;
  modified: string;
  family: string;
  parameters: string;
  quantization: string;
}

export interface MarketplaceModel {
  name: string;
  description: string;
  tags: string[];
  category: string; // "general" | "code" | "vision" | "embedding"
  parameters: string;
  recommended: boolean;
  source?: "ollama" | "huggingface";
  pull_name?: string;
  author?: string;
  downloads?: number;
  likes?: number;
  last_modified?: string;
  license?: string;
  url?: string;
}

export interface PullProgress {
  status: string;
  digest?: string;
  total?: number;
  completed?: number;
  percent: number;
}
