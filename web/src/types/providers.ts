export interface CloudProvider {
  id: string;
  name: string;
  base_url: string;
  has_key: boolean;
  models: string[];
  active: boolean;
  compatible: string; // "anthropic" | "openai"
}
