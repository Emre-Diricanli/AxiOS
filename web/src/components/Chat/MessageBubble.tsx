import { useState } from "react";
import Markdown from "react-markdown";

interface MessageBubbleProps {
  role: "user" | "assistant" | "error";
  content: string;
  thinking?: string;
  model?: string;
  provider?: string;
}

// Collapsed-by-default reasoning block: dim, out of the way, one click to
// inspect what the model was thinking.
function ThinkingBlock({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-2 -mx-1">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 px-1 py-0.5 text-[10px] text-muted-foreground/70 hover:text-muted-foreground transition-colors"
      >
        <svg
          width="9"
          height="9"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          className={`transition-transform ${open ? "rotate-90" : ""}`}
        >
          <path d="M9 18l6-6-6-6" />
        </svg>
        <span className="italic">Thinking</span>
        <span className="w-1 h-1 rounded-full bg-current opacity-40 animate-pulse" />
      </button>
      {open && (
        <p className="mt-1 px-2.5 py-2 rounded-lg bg-secondary/40 border border-border/50 text-[11px] leading-relaxed text-muted-foreground/80 italic whitespace-pre-wrap break-words">
          {text}
        </p>
      )}
    </div>
  );
}

export function MessageBubble({ role, content, thinking, model, provider }: MessageBubbleProps) {
  if (role === "error") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[90%] rounded-lg px-3 py-2 bg-destructive/10 text-red-300 border border-destructive/20 text-xs">
          {content}
        </div>
      </div>
    );
  }

  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[90%] rounded-2xl px-3.5 py-2.5 text-[13px] leading-relaxed ${
          isUser
            ? "bg-primary text-primary-foreground rounded-br-sm"
            : "glass rounded-bl-sm"
        }`}
      >
        {!isUser && thinking && <ThinkingBlock text={thinking} />}
        {isUser ? (
          <p className="whitespace-pre-wrap break-words">{content}</p>
        ) : (
          <div className="prose-axios">
            <Markdown>{content}</Markdown>
          </div>
        )}
        {(model || provider) && !isUser && (
          <p className="text-[10px] text-muted-foreground mt-1.5 font-mono">
            {[provider, model].filter(Boolean).join(" / ")}
          </p>
        )}
      </div>
    </div>
  );
}
