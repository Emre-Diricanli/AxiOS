export interface ChatMessage {
  type:
    | "user"
    | "assistant"
    | "error"
    | "status"
    | "tool_use"
    | "tool_result"
    | "approval_request"
    | "approval_response";
  content?: string;
  sessionId?: string;
  model?: string;
  provider?: string;
  toolName?: string;
  toolId?: string;

  // Approval flow fields. approval_request (daemon -> UI) carries id, tool,
  // and params; approval_response (UI -> daemon) carries id and approve.
  id?: string;
  tool?: string;
  params?: unknown;
  approve?: boolean;
}

export type ApprovalStatus = "pending" | "approved" | "denied" | "expired";

export interface StatusInfo {
  backend: "cloud" | "local";
  routing: string;
}

export interface FileEntry {
  name: string;
  type: "file" | "dir";
  size: number;
  permissions?: string;
  mod_time?: string;
}
