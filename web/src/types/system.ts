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
  network: {
    hostname: string;
    interfaces: Array<{
      name: string;
      ip: string;
      status: string;
    }>;
  };
}
