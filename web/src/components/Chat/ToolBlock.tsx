interface ToolBlockProps {
  type: "tool_use" | "tool_result";
  toolName: string;
  content: string;
}

export function ToolBlock({ type, toolName, content }: ToolBlockProps) {
  // Clean up the server__tool format for display
  const displayName = toolName.replace("__", " > ");

  if (type === "tool_use") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[80%] rounded-lg border border-neutral-700 bg-neutral-900 text-sm font-mono">
          <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-700 text-neutral-400">
            <span className="text-yellow-500">&#9654;</span>
            <span>{displayName}</span>
          </div>
          <pre className="px-3 py-2 text-neutral-300 overflow-x-auto whitespace-pre-wrap break-all">
            {formatInput(content)}
          </pre>
        </div>
      </div>
    );
  }

  return (
    <div className="flex justify-start">
      <div className="max-w-[80%] rounded-lg border border-neutral-700 bg-neutral-900 text-sm font-mono">
        <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-700 text-neutral-400">
          <span className="text-green-500">&#10003;</span>
          <span>{displayName}</span>
        </div>
        <pre className="px-3 py-2 text-neutral-300 overflow-x-auto whitespace-pre-wrap break-all max-h-64 overflow-y-auto">
          {content.length > 2000 ? content.slice(0, 2000) + "\n...(truncated)" : content}
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
