interface ToolBlockProps {
  type: "tool_use" | "tool_result";
  toolName: string;
  content: string;
}

export function ToolBlock({ type, toolName, content }: ToolBlockProps) {
  const displayName = toolName.replace("__", " > ");

  return (
    <div className="flex justify-start">
      <div className="max-w-[95%] rounded-xl glass text-xs font-mono overflow-hidden">
        <div className="flex items-center gap-2 px-3 py-2 border-b border-border">
          {type === "tool_use" ? (
            <div className="w-4 h-4 rounded-full bg-amber-500/20 flex items-center justify-center">
              <span className="text-amber-400 text-[8px]">&#9654;</span>
            </div>
          ) : (
            <div className="w-4 h-4 rounded-full bg-emerald-500/20 flex items-center justify-center">
              <span className="text-emerald-400 text-[8px]">&#10003;</span>
            </div>
          )}
          <span className="text-[11px] text-muted-foreground">{displayName}</span>
        </div>
        <pre className="px-3 py-2 text-foreground/60 overflow-x-auto whitespace-pre-wrap break-all text-[11px] max-h-36 overflow-y-auto scrollbar-none">
          {type === "tool_use"
            ? formatInput(content)
            : content.length > 1500
              ? content.slice(0, 1500) + "\n...(truncated)"
              : content}
        </pre>
      </div>
    </div>
  );
}

function formatInput(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
