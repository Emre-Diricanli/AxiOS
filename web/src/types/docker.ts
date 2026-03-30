export interface Container {
  id: string;
  name: string;
  image: string;
  status: string;
  state: string; // "running" | "exited" | "paused" | "created" | "dead"
  ports: string;
  created: string;
}

export interface ContainerStats {
  id: string;
  name: string;
  cpu_percent: string;
  mem_usage: string;
  mem_percent: string;
  net_io: string;
  block_io: string;
}

export interface DockerImage {
  id: string;
  repository: string;
  tag: string;
  size: string;
  created: string;
}

export interface RunContainerRequest {
  image: string;
  name?: string;
  ports?: string[];
  env?: string[];
  volumes?: string[];
  restart?: string;
}

export interface ComposeRequest {
  yaml: string;
  project: string;
  action: "up" | "down";
}
