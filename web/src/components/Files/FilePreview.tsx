import { useEffect, useState } from "react";
import { FileIcon } from "./FileIcon";
import type { FileEntry } from "@/types/messages";

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return "-";
  const d = new Date(dateStr);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const TEXT_EXTENSIONS = new Set([
  "txt", "md", "json", "yaml", "yml", "toml", "xml", "csv", "ini",
  "conf", "cfg", "env", "log",
  "ts", "tsx", "js", "jsx", "py", "go", "rs", "rb", "java", "c", "cpp",
  "h", "cs", "php", "swift", "kt", "scala", "sh", "bash", "zsh",
  "css", "scss", "html", "vue", "svelte", "sql", "r", "lua",
  "gitignore", "dockerignore", "Makefile", "Dockerfile",
  "mod", "sum", "lock",
]);

function isTextFile(name: string): boolean {
  if (!name.includes(".")) {
    // Files without extensions could be text (Makefile, Dockerfile, etc)
    return TEXT_EXTENSIONS.has(name);
  }
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return TEXT_EXTENSIONS.has(ext);
}

interface FilePreviewProps {
  file: FileEntry;
  currentPath: string;
  onClose: () => void;
}

export function FilePreview({ file, currentPath, onClose }: FilePreviewProps) {
  const [content, setContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const filePath =
    currentPath === "/" ? `/${file.name}` : `${currentPath}/${file.name}`;

  useEffect(() => {
    if (file.type === "dir" || !isTextFile(file.name)) return;
    // Only try to load text preview for reasonably-sized files (< 512 KB)
    if (file.size > 512 * 1024) return;

    setLoading(true);
    setError(null);
    fetch(`/api/fs/read?path=${encodeURIComponent(filePath)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((data: { content: string }) => {
        setContent(data.content);
      })
      .catch((err: Error) => {
        setError(err.message);
      })
      .finally(() => setLoading(false));
  }, [filePath, file.type, file.name, file.size]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="bg-neutral-900 border border-neutral-700 rounded-lg shadow-2xl w-full max-w-2xl max-h-[80vh] flex flex-col mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-neutral-800">
          <div className="flex items-center gap-3 min-w-0">
            <FileIcon name={file.name} isDir={file.type === "dir"} className="text-xl" />
            <h3 className="text-neutral-100 font-medium truncate">{file.name}</h3>
          </div>
          <button
            onClick={onClose}
            className="text-neutral-400 hover:text-neutral-100 p-1 rounded hover:bg-neutral-800 transition-colors"
          >
            <span className="text-lg leading-none">{"\u2715"}</span>
          </button>
        </div>

        {/* Info */}
        <div className="px-5 py-4 border-b border-neutral-800 grid grid-cols-2 gap-3 text-sm">
          <div>
            <span className="text-neutral-500">Path</span>
            <p className="text-neutral-200 font-mono text-xs mt-0.5 truncate">{filePath}</p>
          </div>
          <div>
            <span className="text-neutral-500">Size</span>
            <p className="text-neutral-200 font-mono mt-0.5">{formatSize(file.size)}</p>
          </div>
          {file.permissions && (
            <div>
              <span className="text-neutral-500">Permissions</span>
              <p className="text-neutral-200 font-mono mt-0.5">{file.permissions}</p>
            </div>
          )}
          {file.mod_time && (
            <div>
              <span className="text-neutral-500">Modified</span>
              <p className="text-neutral-200 mt-0.5">{formatDate(file.mod_time)}</p>
            </div>
          )}
        </div>

        {/* Content preview */}
        {file.type === "file" && isTextFile(file.name) && (
          <div className="flex-1 overflow-auto min-h-0">
            {loading && (
              <div className="p-5 text-neutral-500 text-sm">Loading preview...</div>
            )}
            {error && (
              <div className="p-5 text-red-400 text-sm">Failed to load preview: {error}</div>
            )}
            {content !== null && (
              <pre className="p-5 text-xs font-mono text-neutral-300 whitespace-pre-wrap break-words leading-relaxed">
                {content.length > 10000 ? content.slice(0, 10000) + "\n\n... (truncated)" : content}
              </pre>
            )}
          </div>
        )}

        {file.type === "file" && !isTextFile(file.name) && (
          <div className="p-5 text-neutral-500 text-sm italic">
            Binary file -- no preview available
          </div>
        )}

        {file.size > 512 * 1024 && isTextFile(file.name) && (
          <div className="p-5 text-neutral-500 text-sm italic">
            File too large for preview ({formatSize(file.size)})
          </div>
        )}
      </div>
    </div>
  );
}
