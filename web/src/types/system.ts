export interface SystemStats {
  hostname: string;
  os: string;
  arch: string;
  kernel: string;
  uptime: string;
  cpu: {
    model: string;
    cores: number;
    threads: number;
    usage_percent: number;
  };
  memory: {
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
    usage_percent: number;
  };
  disk: Array<{
    mount: string;
    device: string;
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
    usage_percent: number;
  }>;
  gpu: Array<{
    index: number;
    name: string;
    utilization_percent: number;
    memory_total_bytes: number;
    memory_used_bytes: number;
    memory_usage_percent: number;
    temperature_c: number;
  }>;
  network: {
    hostname: string;
    interfaces: Array<{
      name: string;
      ip: string;
      status: string;
    }>;
  };
}

export interface RunningModelStats {
  name: string;
  size_bytes: number;
  vram_bytes: number;
}
