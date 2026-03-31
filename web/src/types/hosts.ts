export interface OllamaHost {
  id: string;
  name: string;
  host: string;
  port: number;
  status: "online" | "offline" | "checking";
  models: string[];
  active: boolean;
  gpu_info: string;
}
