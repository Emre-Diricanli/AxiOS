interface MessageBubbleProps {
  role: "user" | "assistant" | "error";
  content: string;
  model?: string;
}

export function MessageBubble({ role, content, model }: MessageBubbleProps) {
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
            ? "bg-primary text-primary-foreground rounded-br-md shadow-[0_2px_12px_rgba(99,102,241,0.25)]"
            : "glass rounded-bl-md"
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
