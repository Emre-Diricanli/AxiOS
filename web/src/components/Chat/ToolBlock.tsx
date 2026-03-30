interface ToolBlockProps {
  type: "tool_use" | "tool_result";
  toolName: string;
  content: string;
}

export function ToolBlock({ type, toolName, content }: ToolBlockProps) {
  const displayName = toolName.replace("__", " > ");

  return (
    <div className="flex justify-start">
      <div className="max-w-[95%] rounded-lg border bg-card text-xs font-mono overflow-hidden">
        <div className="flex items-center gap-1.5 px-3 py-1.5 border-b text-muted-foreground">
          {type === "tool_use" ? (
            <span className="text-amber-400 text-[10px]">&#9654;</span>
          ) : (
            <span className="text-emerald-400 text-[10px]">&#10003;</span>
          )}
          <span className="text-[10px]">{displayName}</span>
        </div>
        <pre className="px-3 py-2 text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all text-[11px] max-h-40 overflow-y-auto">
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
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return raw;
  }
}
