export interface ChatMessage {
  type: "user" | "assistant" | "error" | "status" | "tool_use" | "tool_result";
  content: string;
  sessionId: string;
  model?: string;
  toolName?: string;
  toolId?: string;
}

export interface StatusInfo {
  backend: "cloud" | "local";
  routing: string;
}
