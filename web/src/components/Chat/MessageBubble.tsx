interface MessageBubbleProps {
  role: "user" | "assistant" | "error";
  content: string;
  model?: string;
}

export function MessageBubble({ role, content, model }: MessageBubbleProps) {
  if (role === "error") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[90%] rounded-lg px-3 py-2 bg-destructive/10 text-destructive border border-destructive/20 text-xs">
          {content}
        </div>
      </div>
    );
  }

  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[90%] rounded-xl px-3.5 py-2.5 text-[13px] leading-relaxed ${
          isUser
            ? "bg-primary text-primary-foreground rounded-br-sm"
            : "bg-secondary text-secondary-foreground border rounded-bl-sm"
        }`}
      >
        <p className="whitespace-pre-wrap break-words">{content}</p>
        {model && !isUser && (
          <p className="text-[10px] text-muted-foreground mt-1.5 font-mono">{model}</p>
        )}
      </div>
    </div>
  );
}
