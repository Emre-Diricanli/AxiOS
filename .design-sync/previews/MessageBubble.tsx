import { MessageBubble } from "axios-web";

export const Conversation = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 12, width: 520, padding: 8 }}>
    <MessageBubble role="user" content="What's using my memory?" />
    <MessageBubble
      role="assistant"
      content={
        "Chrome is the top consumer at **6.2 GB**, followed by `ollama serve` at 3.1 GB. Total pressure is moderate — 21 GB of 32 GB in use."
      }
      model="grok-4.5"
      provider="SuperGrok"
    />
  </div>
);

export const WithThinking = () => (
  <div style={{ width: 520, padding: 8 }}>
    <MessageBubble
      role="assistant"
      thinking="The user asked about memory pressure. I should check the process list and summarize the top consumers in plain terms rather than dumping raw output."
      content="Chrome is your top memory consumer at 6.2 GB across 42 renderer processes."
      model="claude-sonnet-4-6"
      provider="Anthropic"
    />
  </div>
);

export const ErrorState = () => (
  <div style={{ width: 520, padding: 8 }}>
    <MessageBubble role="error" content="AI request failed: rate limited by provider (429) — retrying in 8s" />
  </div>
);
