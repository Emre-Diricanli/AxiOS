import { useEffect, useState } from "react";
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

      {/* Content preview */}
      <div className="flex-1 overflow-y-auto scrollbar-none min-h-0">
        {file.type === "file" && isImageFile(file.name) && (
          <div className="p-4 flex flex-col items-center gap-2">
            <div className="w-full aspect-square rounded-lg bg-white/[0.03] border border-border flex items-center justify-center">
              <div className="text-center">
                <FileIcon name={file.name} isDir={false} size="lg" />
                <p className="text-xs text-muted-foreground mt-2">Image Preview</p>
              </div>
            </div>
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
