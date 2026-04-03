import { useEffect, useState, useCallback } from "react";
import Markdown from "react-markdown";
import { FileIcon } from "./FileIcon";
import type { FileEntry } from "@/types/messages";
import { getFileCategory, type FileCategory } from "./FileIcon";

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
    return TEXT_EXTENSIONS.has(name);
  }
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return TEXT_EXTENSIONS.has(ext);
}

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "svg", "webp", "ico", "bmp"]);

function isImageFile(name: string): boolean {
  if (!name.includes(".")) return false;
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return IMAGE_EXTENSIONS.has(ext);
}

const CATEGORY_LABELS: Record<FileCategory, string> = {
  code: "Source Code",
  image: "Image",
  video: "Video",
  audio: "Audio",
  document: "Document",
  config: "Configuration",
  archive: "Archive",
  executable: "Executable",
  default: "File",
};

interface FilePreviewProps {
  file: FileEntry;
  currentPath: string;
  onClose: () => void;
}

export function FilePreview({ file, currentPath, onClose }: FilePreviewProps) {
  const [content, setContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [visible, setVisible] = useState(false);
  const [aiResponse, setAiResponse] = useState<string | null>(null);
  const [aiLoading, setAiLoading] = useState(false);
  const [aiError, setAiError] = useState<string | null>(null);

  const filePath =
    currentPath === "/" ? `/${file.name}` : `${currentPath}/${file.name}`;

  const category = file.type === "dir" ? "default" : getFileCategory(file.name);
  const kind = file.type === "dir" ? "Folder" : CATEGORY_LABELS[category];

  // Slide-in animation
  useEffect(() => {
    requestAnimationFrame(() => setVisible(true));
  }, []);

  const handleClose = () => {
    setVisible(false);
    setTimeout(onClose, 200);
  };

  // Reset AI state when file changes
  useEffect(() => {
    setAiResponse(null);
    setAiError(null);
    setAiLoading(false);
  }, [filePath]);

  useEffect(() => {
    setContent(null);
    setError(null);
    if (file.type === "dir" || !isTextFile(file.name)) return;
    if (file.size > 512 * 1024) return;

    setLoading(true);
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

  const handleAskAI = useCallback(async () => {
    if (!content) return;

    setAiLoading(true);
    setAiError(null);
    setAiResponse(null);

    try {
      const res = await fetch("/api/ai/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          prompt: `Explain this file (${file.name}). What does it do, what are the key parts, and how is it structured?`,
          context: content.length > 15000 ? content.slice(0, 15000) + "\n\n... (truncated)" : content,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
        throw new Error(data.error || `HTTP ${res.status}`);
      }

      const data: { response: string } = await res.json();
      setAiResponse(data.response);
    } catch (err: any) {
      setAiError(err.message);
    } finally {
      setAiLoading(false);
    }
  }, [content, file.name]);

  return (
    <div
      className={`flex flex-col h-full border-l border-border glass-subtle transition-all duration-200 ease-out overflow-hidden ${
        visible ? "w-[300px] opacity-100" : "w-0 opacity-0"
      }`}
      style={{ minWidth: visible ? 300 : 0 }}
    >
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Preview</span>
        <button
          onClick={handleClose}
          className="text-muted-foreground hover:text-foreground p-1 rounded-md hover:bg-white/[0.06] transition-colors"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M4 4l8 8M12 4l-8 8" />
          </svg>
        </button>
      </div>

      {/* File icon + name */}
      <div className="flex flex-col items-center gap-3 px-4 py-6 border-b border-border shrink-0">
        <FileIcon name={file.name} isDir={file.type === "dir"} size="lg" />
        <div className="text-center min-w-0 w-full">
          <p className="text-sm font-medium text-foreground truncate">{file.name}</p>
          <p className="text-xs text-muted-foreground mt-0.5">{kind}</p>
        </div>
      </div>

      {/* Metadata */}
      <div className="px-4 py-3 border-b border-border shrink-0 space-y-2">
        <div className="flex justify-between text-xs">
          <span className="text-muted-foreground">Size</span>
          <span className="text-foreground/80 font-mono">{file.type === "dir" ? "--" : formatSize(file.size)}</span>
        </div>
        {file.mod_time && (
          <div className="flex justify-between text-xs">
            <span className="text-muted-foreground">Modified</span>
            <span className="text-foreground/80">{formatDate(file.mod_time)}</span>
          </div>
        )}
        {file.permissions && (
          <div className="flex justify-between text-xs">
            <span className="text-muted-foreground">Permissions</span>
            <span className="text-foreground/80 font-mono">{file.permissions}</span>
          </div>
        )}
        <div className="flex justify-between text-xs">
          <span className="text-muted-foreground">Path</span>
          <span className="text-foreground/80 font-mono truncate ml-4 max-w-[160px]" title={filePath}>
            {filePath}
          </span>
        </div>
      </div>

      {/* Ask AI button — shown for text files that have been loaded */}
      {file.type === "file" && isTextFile(file.name) && content !== null && (
        <div className="px-4 py-3 border-b border-border shrink-0">
          <button
            onClick={handleAskAI}
            disabled={aiLoading}
            className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-xs font-medium bg-gradient-to-r from-primary/20 to-purple-500/20 text-primary hover:from-primary/30 hover:to-purple-500/30 border border-primary/20 transition-all duration-150 disabled:opacity-50"
          >
            {aiLoading ? (
              <div className="w-3 h-3 border border-primary/30 border-t-primary rounded-full animate-spin" />
            ) : (
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <path d="M8 1v14M1 8h14M3 3l10 10M13 3L3 13" />
              </svg>
            )}
            {aiLoading ? "Analyzing..." : "Ask AI about this file"}
          </button>
        </div>
      )}

      {/* AI Response */}
      {(aiResponse || aiError) && (
        <div className="px-4 py-3 border-b border-border shrink-0 max-h-[300px] overflow-y-auto scrollbar-none">
          {aiError && (
            <div className="px-3 py-2 rounded-lg bg-destructive/10 border border-destructive/20 text-xs text-red-300">
              {aiError}
            </div>
          )}
          {aiResponse && (
            <div className="prose-axios">
              <Markdown>{aiResponse}</Markdown>
            </div>
          )}
        </div>
      )}

      {/* Content preview */}
      <div className="flex-1 overflow-y-auto scrollbar-none min-h-0">
        {file.type === "file" && isImageFile(file.name) && (
          <div className="p-4">
            <img
              src={`/api/fs/raw?path=${encodeURIComponent(filePath)}`}
              alt={file.name}
              className="w-full rounded-lg border border-border object-contain max-h-[400px] bg-black/20"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = "none";
              }}
            />
            <p className="text-[10px] text-muted-foreground text-center mt-2">{formatSize(file.size)}</p>
          </div>
        )}

        {file.type === "file" && isTextFile(file.name) && (
          <>
            {loading && (
              <div className="p-4 flex items-center gap-2">
                <div className="w-3 h-3 border border-muted-foreground/30 border-t-primary rounded-full animate-spin" />
                <span className="text-xs text-muted-foreground">Loading...</span>
              </div>
            )}
            {error && (
              <div className="p-4 text-xs text-red-400">Failed to load: {error}</div>
            )}
            {content !== null && (
              <pre className="p-4 text-[11px] font-mono text-foreground/70 whitespace-pre-wrap break-words leading-relaxed">
                {content.length > 10000
                  ? content.slice(0, 10000) + "\n\n... (truncated)"
                  : content}
              </pre>
            )}
            {file.size > 512 * 1024 && (
              <div className="p-4 text-xs text-muted-foreground italic">
                File too large for preview ({formatSize(file.size)})
              </div>
            )}
          </>
        )}

        {file.type === "file" && !isTextFile(file.name) && !isImageFile(file.name) && (
          <div className="p-4 text-xs text-muted-foreground italic">
            No preview available for this file type.
          </div>
        )}
      </div>
    </div>
  );
}
