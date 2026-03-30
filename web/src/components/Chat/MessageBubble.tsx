interface MessageBubbleProps {
  role: "user" | "assistant" | "error";
  content: string;
  model?: string;
}

export function MessageBubble({ role, content, model }: MessageBubbleProps) {
  if (role === "error") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[80%] rounded-lg px-4 py-2 bg-red-950 text-red-200 border border-red-800">
          {content}
        </div>
      </div>
    );
  }

  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[80%] rounded-lg px-4 py-2 ${
          isUser
            ? "bg-blue-600 text-white"
            : "bg-neutral-800 text-neutral-100"
        }`}
      >
        <p className="whitespace-pre-wrap">{content}</p>
        {model && !isUser && (
          <p className="text-xs text-neutral-500 mt-1">{model}</p>
        )}
      </div>
    </div>
  );
}
