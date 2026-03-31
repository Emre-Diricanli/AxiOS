import { useState } from "react";

interface ToolBlockProps {
  type: "tool_use" | "tool_result";
  toolName: string;
  content: string;
}

export function ToolBlock({ type, toolName, content }: ToolBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const displayName = toolName.replace("__", " > ");

  return (
    <div className="flex justify-start">
      <button
        onClick={() => setExpanded(!expanded)}
        className="max-w-[95%] rounded-lg border border-border bg-card/50 text-xs font-mono overflow-hidden text-left transition-colors hover:bg-card/80"
      >
        <div className="flex items-center gap-2 px-3 py-1.5">
          {type === "tool_use" ? (
            <div className="w-3.5 h-3.5 rounded-full bg-amber-500/20 flex items-center justify-center shrink-0">
              <span className="text-amber-400 text-[7px]">&#9654;</span>
            </div>
          ) : (
            <div className="w-3.5 h-3.5 rounded-full bg-emerald-500/20 flex items-center justify-center shrink-0">
              <span className="text-emerald-400 text-[7px]">&#10003;</span>
            </div>
          )}
          <span className="text-[11px] text-muted-foreground flex-1">{displayName}</span>
          <svg
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            className={`text-muted-foreground transition-transform ${expanded ? "rotate-180" : ""}`}
          >
            <path d="M6 9l6 6 6-6" />
          </svg>
        </div>
        {expanded && (
          <pre className="px-3 py-2 border-t border-border text-foreground/60 overflow-x-auto whitespace-pre-wrap break-all text-[11px] max-h-48 overflow-y-auto scrollbar-none">
            {type === "tool_use"
              ? formatInput(content)
              : content.length > 1500
                ? content.slice(0, 1500) + "\n...(truncated)"
                : content}
          </pre>
        )}
      </button>
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
