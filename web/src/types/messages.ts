export interface ChatMessage {
  type: "user" | "assistant" | "error" | "status";
  content: string;
  sessionId: string;
  model?: string;
}

export interface StatusInfo {
  backend: "cloud" | "local";
  routing: string;
}
