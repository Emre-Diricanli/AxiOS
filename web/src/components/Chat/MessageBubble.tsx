interface MessageBubbleProps {
  role: "user" | "assistant" | "error";
  content: string;
  model?: string;
}

export function MessageBubble({ role, content, model }: MessageBubbleProps) {
  if (role === "error") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[90%] rounded-lg px-3 py-2 bg-red-500/10 text-red-300 border border-red-500/20 text-xs">
          {content}
        </div>
      </div>
    );
  }

  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[90%] rounded-lg px-3 py-2 text-[13px] leading-relaxed ${
          isUser
            ? "bg-blue-600/90 text-white"
            : "bg-white/[0.05] text-neutral-200 border border-white/[0.06]"
        }`}
      >
        <p className="whitespace-pre-wrap break-words">{content}</p>
        {model && !isUser && (
          <p className="text-[10px] text-neutral-600 mt-1.5 font-mono">{model}</p>
        )}
      </div>
    </div>
  );
}
