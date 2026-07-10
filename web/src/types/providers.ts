export interface CloudProvider {
  id: string;
  name: string;
  base_url: string;
  has_key: boolean;
  models: string[];
  active: boolean;
  compatible: string; // "anthropic" | "openai"
}

/* xAI SuperGrok subscription OAuth (device-code flow).
   Mirrors XAIOAuthStatus in internal/axiosd/xai_oauth.go. */

export type XAIOAuthState = "idle" | "pending" | "connected" | "error";

export interface XAIOAuthStatus {
  state: XAIOAuthState;
  user_code?: string;
  verification_uri?: string;
  expires_at?: string;
  error?: string;
  // True when stored credentials exist, regardless of any in-flight flow.
  connected: boolean;
}
