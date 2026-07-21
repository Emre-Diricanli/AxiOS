export interface OllamaHost {
  id: string;
  name: string;
  host: string;
  port: number;
  telemetry_port: number;
  has_telemetry_token: boolean;
  status: "online" | "offline" | "checking";
  models: string[];
  active: boolean;
  gpu_info: string;
}
