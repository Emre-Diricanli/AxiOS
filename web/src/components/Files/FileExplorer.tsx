import { useState, useCallback, useMemo, useRef, useEffect } from "react";
import { useFileSystem } from "@/hooks/useFileSystem";
import { Breadcrumb } from "./Breadcrumb";
import { FileIcon } from "./FileIcon";
import { FilePreview } from "./FilePreview";
import { FileEditor } from "./FileEditor";
import { ImageViewer } from "./ImageViewer";
import { ContextMenu } from "./ContextMenu";
import type { FileEntry } from "@/types/messages";
import { toastSuccess, toastError, toastInfo } from "@/hooks/useToast";

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "svg", "webp", "ico", "bmp"]);
function isImageFile(name: string): boolean {
  if (!name.includes(".")) return false;
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return IMAGE_EXTENSIONS.has(ext);
}

type ViewMode = "grid" | "list";

const EDITABLE_EXTENSIONS = new Set([
  "txt", "md", "js", "ts", "jsx", "tsx", "py", "go", "rs", "java", "c", "cpp",
  "h", "cs", "rb", "php", "swift", "kt", "sh", "bash", "zsh", "json", "yaml",
  "yml", "toml", "xml", "html", "css", "scss", "sql", "dockerfile", "makefile",
  "gitignore", "env", "cfg", "conf", "ini", "log", "svg", "vue", "svelte", "r",
  "lua", "csv", "mod", "sum", "lock", "mdx",
]);

function isEditableFile(name: string): boolean {
  if (!name.includes(".")) {
    return EDITABLE_EXTENSIONS.has(name.toLowerCase());
  }
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return EDITABLE_EXTENSIONS.has(ext);
}

/* ---------- Helpers ---------- */

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return "--";
  const d = new Date(dateStr);
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function getFileKind(entry: FileEntry): string {
  if (entry.type === "dir") return "Folder";
  if (!entry.name.includes(".")) return "File";
  const ext = entry.name.split(".").pop()?.toLowerCase() ?? "";
  const map: Record<string, string> = {
    ts: "TypeScript", tsx: "TypeScript", js: "JavaScript", jsx: "JavaScript",
    py: "Python", go: "Go", rs: "Rust", java: "Java", c: "C Source",
    cpp: "C++ Source", h: "Header", cs: "C#", rb: "Ruby", php: "PHP",
    swift: "Swift", kt: "Kotlin", sh: "Shell Script",
    md: "Markdown", txt: "Text", pdf: "PDF Document",
    json: "JSON", yaml: "YAML", yml: "YAML", toml: "TOML", xml: "XML",
    png: "PNG Image", jpg: "JPEG Image", jpeg: "JPEG Image", gif: "GIF Image",
    svg: "SVG Image", webp: "WebP Image",
    mp4: "MP4 Video", mov: "QuickTime Movie", mkv: "MKV Video",
    mp3: "MP3 Audio", wav: "WAV Audio", flac: "FLAC Audio",
    zip: "ZIP Archive", tar: "TAR Archive", gz: "GZip Archive",
    css: "CSS", scss: "SCSS", html: "HTML",
  };
  return map[ext] ?? `${ext.toUpperCase()} File`;
}

/* ---------- Upload helper ---------- */

async function uploadFiles(files: FileList | File[], destPath: string): Promise<string[]> {
  const formData = new FormData();
  for (const file of Array.from(files)) {
    formData.append("files", file);
  }
  const resp = await fetch(`/api/fs/upload?path=${encodeURIComponent(destPath)}`, {
    method: "POST",
    body: formData,
  });
  const data = await resp.json();
  if (!resp.ok) throw new Error(data.error || "Upload failed");
  return data.files as string[];
}

/* ---------- Bulk delete helper ---------- */

async function bulkDeleteFiles(paths: string[]): Promise<void> {
  const resp = await fetch("/api/fs/bulk-delete", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ paths }),
  });
  const data = await resp.json();
  if (!resp.ok) throw new Error(data.error || "Bulk delete failed");
}

/* ---------- Download helper ---------- */

function downloadFile(filePath: string, fileName: string) {
  const a = document.createElement("a");
  a.href = `/api/fs/raw?path=${encodeURIComponent(filePath)}`;
  a.download = fileName;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

/* ---------- Path helpers ---------- */

function entryPath(currentPath: string, name: string): string {
  return currentPath === "/" ? `/${name}` : `${currentPath}/${name}`;
}

function splitNameExt(name: string): [string, string] {
  const dotIdx = name.lastIndexOf(".");
  if (dotIdx <= 0) return [name, ""];
  return [name.slice(0, dotIdx), name.slice(dotIdx)];
}

/* ---------- InlineRenameInput ---------- */

function InlineRenameInput({
  initialName,
  isDir,
  onConfirm,
  onCancel,
}: {
  initialName: string;
  isDir: boolean;
  onConfirm: (newName: string) => void;
  onCancel: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(initialName);

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.focus();
    if (!isDir && initialName.includes(".")) {
      const [baseName] = splitNameExt(initialName);
      el.setSelectionRange(0, baseName.length);
    } else {
      el.select();
    }
  }, [initialName, isDir]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === "Enter") {
      const trimmed = value.trim();
      if (trimmed && trimmed !== initialName) {
        onConfirm(trimmed);
      } else {
        onCancel();
      }
    } else if (e.key === "Escape") {
      onCancel();
    }
  };

  return (
    <input
      ref={inputRef}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onKeyDown={handleKeyDown}
      onBlur={onCancel}
      className="bg-white/10 border border-primary/50 rounded px-1.5 py-0.5 text-xs text-foreground outline-none w-full max-w-[200px]"
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
    />
  );
}

/* ---------- InlineCreateInput ---------- */

function InlineCreateInput({
  placeholder,
  onConfirm,
  onCancel,
}: {
  placeholder: string;
  onConfirm: (name: string) => void;
  onCancel: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState("");

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === "Enter") {
      const trimmed = value.trim();
      if (trimmed) onConfirm(trimmed);
      else onCancel();
    } else if (e.key === "Escape") {
      onCancel();
    }
  };

  return (
    <div className="flex items-center gap-2.5 px-4 py-1.5">
      <FileIcon name={placeholder.includes("folder") ? "__dir__" : "untitled"} isDir={placeholder.includes("folder")} size="sm" />
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={onCancel}
        placeholder={placeholder}
        className="bg-white/10 border border-primary/50 rounded px-1.5 py-0.5 text-xs text-foreground placeholder:text-muted-foreground/40 outline-none flex-1 max-w-[300px]"
        onClick={(e) => e.stopPropagation()}
      />
    </div>
  );
}

/* ---------- DeleteConfirmDialog ---------- */

function DeleteConfirmDialog({
  name,
  onConfirm,
  onCancel,
}: {
  name: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
      if (e.key === "Enter") onConfirm();
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onConfirm, onCancel]);

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="glass rounded-xl p-5 shadow-2xl max-w-sm w-full mx-4">
        <h3 className="text-sm font-medium text-foreground mb-2">Delete &quot;{name}&quot;?</h3>
        <p className="text-xs text-muted-foreground mb-4">
          This action cannot be undone. The item will be permanently removed.
        </p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-xs rounded-lg bg-white/[0.06] hover:bg-white/[0.1] text-foreground transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-xs rounded-lg bg-red-500/20 hover:bg-red-500/30 text-red-400 transition-colors"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

/* ---------- Sidebar Items ---------- */

interface SidebarItem {
  id: string;
  label: string;
  path: string;
  icon: "home" | "desktop" | "documents" | "downloads" | "root" | "drive";
}

const FAVORITES: SidebarItem[] = [
  { id: "home", label: "Home", path: "~", icon: "home" },
  { id: "desktop", label: "Desktop", path: "~/Desktop", icon: "desktop" },
  { id: "documents", label: "Documents", path: "~/Documents", icon: "documents" },
  { id: "downloads", label: "Downloads", path: "~/Downloads", icon: "downloads" },
  { id: "root", label: "Root", path: "/", icon: "root" },
];

const LOCATIONS: SidebarItem[] = [
  { id: "disk", label: "Macintosh HD", path: "/", icon: "drive" },
];

function SidebarIcon({ type, active }: { type: SidebarItem["icon"]; active: boolean }) {
  const color = active ? "text-primary" : "text-muted-foreground";
  const size = 15;

  switch (type) {
    case "home":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M2 8.5l6-6 6 6" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
          <path d="M3.5 7.5v5.5a1 1 0 001 1h7a1 1 0 001-1V7.5" stroke="currentColor" strokeWidth="1.3" />
          <path d="M6 14V10.5a1 1 0 011-1h2a1 1 0 011 1V14" stroke="currentColor" strokeWidth="1.3" />
        </svg>
      );
    case "desktop":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <rect x="1.5" y="2.5" width="13" height="9" rx="1" stroke="currentColor" strokeWidth="1.3" />
          <path d="M5.5 14h5M8 11.5v2.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "documents":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M4 1.5h5.5L13 5v9a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 013 14V3a1.5 1.5 0 011-1.5z" stroke="currentColor" strokeWidth="1.3" />
          <path d="M9 1.5V5.5h4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
          <path d="M5.5 8.5h5M5.5 11h3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "downloads":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M8 2v8M5 7.5L8 10.5 11 7.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M2.5 11v2a1.5 1.5 0 001.5 1.5h8a1.5 1.5 0 001.5-1.5v-2" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "root":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M4 7L8 3l4 4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M4 12L8 8l4 4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      );
    case "drive":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <rect x="1.5" y="5" width="13" height="6" rx="1.5" stroke="currentColor" strokeWidth="1.3" />
          <circle cx="11.5" cy="8" r="1" fill="currentColor" />
          <path d="M3.5 8h4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
  }
}

/* ---------- Sort indicator ---------- */

function SortIndicator({ active, dir }: { active: boolean; dir: "asc" | "desc" }) {
  if (!active) return <span className="ml-1 opacity-0 text-[8px]">^</span>;
  return (
    <svg
      width="10"
      height="10"
      viewBox="0 0 10 10"
      fill="currentColor"
      className={`ml-1 text-primary inline-block transition-transform ${dir === "desc" ? "rotate-180" : ""}`}
    >
      <path d="M2 7L5 3l3 4H2z" />
    </svg>
  );
}

/* ---------- FileExplorer ---------- */

export function FileExplorer() {
  const {
    currentPath,
    entries,
    loading,
    error,
    navigateTo,
    refresh,
    sortKey,
    sortDir,
    setSort,
    showDotfiles,
    setShowDotfiles,
  } = useFileSystem("/");

  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const [searchQuery, setSearchQuery] = useState("");
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [showPreview, setShowPreview] = useState(false);
  const [editingFile, setEditingFile] = useState<{ path: string; name: string } | null>(null);
  const [viewingImageIndex, setViewingImageIndex] = useState<number | null>(null);

  // Multi-select state
  const [selectedFiles, setSelectedFiles] = useState<Set<string>>(new Set());
  const lastClickedRef = useRef<string | null>(null);

  // Context menu state
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; entry: FileEntry | null } | null>(null);
  const [clipboard, setClipboard] = useState<string | null>(null);
  const [renamingEntry, setRenamingEntry] = useState<string | null>(null);
  const [creating, setCreating] = useState<"file" | "folder" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<FileEntry | null>(null);

  // Drag & drop state
  const [isDragOver, setIsDragOver] = useState(false);
  const dragCounterRef = useRef(0);

  // Upload state
  const [uploading, setUploading] = useState(false);
  const [uploadFileCount, setUploadFileCount] = useState(0);

  // Navigation history
  const [history, setHistory] = useState<string[]>(["/"]);
  const [historyIndex, setHistoryIndex] = useState(0);
  const suppressHistoryRef = useRef(false);

  // Hidden file input ref for manual file pick
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleNavigate = useCallback(
    (path: string) => {
      const normalized = path === "" ? "/" : path;
      navigateTo(normalized);
      setSelectedFiles(new Set());
      setSearchQuery("");

      if (suppressHistoryRef.current) {
        suppressHistoryRef.current = false;
        return;
      }
      setHistory((prev) => {
        const sliced = prev.slice(0, historyIndex + 1);
        return [...sliced, normalized];
      });
      setHistoryIndex((prev) => prev + 1);
    },
    [navigateTo, historyIndex],
  );

  const canGoBack = historyIndex > 0;
  const canGoForward = historyIndex < history.length - 1;

  const goBack = useCallback(() => {
    if (!canGoBack) return;
    const newIdx = historyIndex - 1;
    setHistoryIndex(newIdx);
    suppressHistoryRef.current = true;
    navigateTo(history[newIdx]);
    setSelectedFiles(new Set());
    setSearchQuery("");
  }, [canGoBack, historyIndex, history, navigateTo]);

  const goForward = useCallback(() => {
    if (!canGoForward) return;
    const newIdx = historyIndex + 1;
    setHistoryIndex(newIdx);
    suppressHistoryRef.current = true;
    navigateTo(history[newIdx]);
    setSelectedFiles(new Set());
    setSearchQuery("");
  }, [canGoForward, historyIndex, history, navigateTo]);

  // Sidebar navigation (resolve ~ paths)
  const handleSidebarNav = useCallback(
    (path: string) => {
      // Pass ~ paths directly — the backend expands them to the actual home dir
      handleNavigate(path);
    },
    [handleNavigate],
  );

  // Filter entries by search
  const filteredEntries = useMemo(() => {
    if (!searchQuery.trim()) return entries;
    const q = searchQuery.toLowerCase();
    return entries.filter((e) => e.name.toLowerCase().includes(q));
  }, [entries, searchQuery]);

  // Multi-select handler
  const handleSelect = useCallback(
    (entry: FileEntry, e: React.MouseEvent) => {
      const name = entry.name;
      const isMeta = e.metaKey || e.ctrlKey;
      const isShift = e.shiftKey;

      if (isShift && lastClickedRef.current) {
        // Range select
        const names = filteredEntries.map((en) => en.name);
        const startIdx = names.indexOf(lastClickedRef.current);
        const endIdx = names.indexOf(name);
        if (startIdx !== -1 && endIdx !== -1) {
          const lo = Math.min(startIdx, endIdx);
          const hi = Math.max(startIdx, endIdx);
          const rangeNames = names.slice(lo, hi + 1);
          setSelectedFiles((prev) => {
            const next = new Set(prev);
            for (const n of rangeNames) next.add(n);
            return next;
          });
        }
      } else if (isMeta) {
        // Toggle individual
        setSelectedFiles((prev) => {
          const next = new Set(prev);
          if (next.has(name)) {
            next.delete(name);
          } else {
            next.add(name);
          }
          return next;
        });
        lastClickedRef.current = name;
      } else {
        // Single select
        setSelectedFiles(new Set([name]));
        lastClickedRef.current = name;
        setShowPreview(true);
      }
    },
    [filteredEntries],
  );

  // Get the currently selected FileEntry (single selection for preview)
  const selectedFile = useMemo(() => {
    if (selectedFiles.size !== 1) return null;
    const name = Array.from(selectedFiles)[0];
    return filteredEntries.find((e) => e.name === name) ?? null;
  }, [selectedFiles, filteredEntries]);

  // Build image list from current directory for the viewer
  const imageFiles = useMemo(() => {
    return filteredEntries
      .filter((e) => e.type === "file" && isImageFile(e.name))
      .map((e) => ({
        name: e.name,
        path: currentPath === "/" ? `/${e.name}` : `${currentPath}/${e.name}`,
      }));
  }, [filteredEntries, currentPath]);

  const handleOpen = useCallback(
    (entry: FileEntry) => {
      if (entry.type === "dir") {
        const next =
          currentPath === "/" ? `/${entry.name}` : `${currentPath}/${entry.name}`;
        handleNavigate(next);
      } else if (isImageFile(entry.name)) {
        // Open image in fullscreen viewer
        const idx = imageFiles.findIndex((img) => img.name === entry.name);
        setViewingImageIndex(idx >= 0 ? idx : 0);
      } else if (isEditableFile(entry.name)) {
        const fullPath =
          currentPath === "/" ? `/${entry.name}` : `${currentPath}/${entry.name}`;
        setEditingFile({ path: fullPath, name: entry.name });
        setShowPreview(false);
        setSelectedFiles(new Set());
      } else {
        setSelectedFiles(new Set([entry.name]));
        setShowPreview(true);
      }
    },
    [currentPath, handleNavigate, imageFiles],
  );

  // --- Drag & Drop handlers ---
  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current++;
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragOver(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current--;
    if (dragCounterRef.current === 0) {
      setIsDragOver(false);
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDrop = useCallback(
    async (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      dragCounterRef.current = 0;
      setIsDragOver(false);

      const files = e.dataTransfer.files;
      if (!files || files.length === 0) return;

      setUploading(true);
      setUploadFileCount(files.length);
      try {
        await uploadFiles(files, currentPath);
        toastSuccess("Uploaded", `${files.length} file${files.length !== 1 ? "s" : ""} uploaded`);
        refresh();
      } catch (err) {
        toastError("Upload failed", err instanceof Error ? err.message : "Unknown error");
        console.error("Upload failed:", err);
      } finally {
        setUploading(false);
        setUploadFileCount(0);
      }
    },
    [currentPath, refresh],
  );

  // --- File input change handler (for manual pick) ---
  const handleFileInputChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0) return;

      setUploading(true);
      setUploadFileCount(files.length);
      try {
        await uploadFiles(files, currentPath);
        toastSuccess("Uploaded", `${files.length} file${files.length !== 1 ? "s" : ""} uploaded`);
        refresh();
      } catch (err) {
        toastError("Upload failed", err instanceof Error ? err.message : "Unknown error");
        console.error("Upload failed:", err);
      } finally {
        setUploading(false);
        setUploadFileCount(0);
        // Reset so the same file can be re-selected
        if (fileInputRef.current) fileInputRef.current.value = "";
      }
    },
    [currentPath, refresh],
  );

  // --- Bulk delete handler ---
  const handleBulkDelete = useCallback(async () => {
    if (selectedFiles.size === 0) return;
    const paths = Array.from(selectedFiles).map((name) =>
      currentPath === "/" ? `/${name}` : `${currentPath}/${name}`,
    );
    try {
      await bulkDeleteFiles(paths);
      toastSuccess("Deleted", `${paths.length} item${paths.length !== 1 ? "s" : ""} removed`);
      setSelectedFiles(new Set());
      setShowPreview(false);
      refresh();
    } catch (err) {
      toastError("Delete failed", err instanceof Error ? err.message : "Unknown error");
      console.error("Bulk delete failed:", err);
    }
  }, [selectedFiles, currentPath, refresh]);

  // --- Download handler ---
  const handleDownload = useCallback(() => {
    if (selectedFiles.size !== 1) return;
    const name = Array.from(selectedFiles)[0];
    const entry = filteredEntries.find((e) => e.name === name);
    if (!entry || entry.type === "dir") return;
    const fullPath = currentPath === "/" ? `/${name}` : `${currentPath}/${name}`;
    downloadFile(fullPath, name);
  }, [selectedFiles, filteredEntries, currentPath]);

  /* ---------- Context menu handlers ---------- */

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, entry: FileEntry | null) => {
      e.preventDefault();
      e.stopPropagation();
      setContextMenu({ x: e.clientX, y: e.clientY, entry });
    },
    [],
  );

  const handleRenameConfirm = useCallback(
    async (oldName: string, newName: string) => {
      const oldPath = entryPath(currentPath, oldName);
      try {
        const res = await fetch("/api/fs/rename", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path: oldPath, new_name: newName }),
        });
        if (!res.ok) {
          const data = await res.json();
          toastError("Rename failed", data.error);
          console.error("Rename failed:", data.error);
        } else {
          toastSuccess("Renamed", `${oldName} \u2192 ${newName}`);
        }
      } catch (err) {
        toastError("Rename failed", err instanceof Error ? err.message : "Unknown error");
        console.error("Rename failed:", err);
      }
      setRenamingEntry(null);
      refresh();
    },
    [currentPath, refresh],
  );

  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteTarget) return;
    const deleteName = deleteTarget.name;
    const p = entryPath(currentPath, deleteName);
    try {
      const res = await fetch("/api/fs/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: p }),
      });
      if (!res.ok) {
        const data = await res.json();
        toastError("Delete failed", data.error);
        console.error("Delete failed:", data.error);
      } else {
        toastSuccess("Deleted", `${deleteName} removed`);
      }
    } catch (err) {
      toastError("Delete failed", err instanceof Error ? err.message : "Unknown error");
      console.error("Delete failed:", err);
    }
    setDeleteTarget(null);
    setSelectedFiles(new Set());
    refresh();
  }, [currentPath, deleteTarget, refresh]);

  const handleCreateConfirm = useCallback(
    async (name: string, type: "file" | "folder") => {
      const p = entryPath(currentPath, name);
      try {
        if (type === "folder") {
          const res = await fetch("/api/fs/mkdir", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: p }),
          });
          if (!res.ok) {
            const data = await res.json();
            toastError("Create failed", data.error);
            console.error("Mkdir failed:", data.error);
          } else {
            toastSuccess("Created", `${name} created`);
          }
        } else {
          const res = await fetch("/api/fs/write", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: p, content: "" }),
          });
          if (!res.ok) {
            const data = await res.json();
            toastError("Create failed", data.error);
            console.error("Create file failed:", data.error);
          } else {
            toastSuccess("Created", `${name} created`);
          }
        }
      } catch (err) {
        toastError("Create failed", err instanceof Error ? err.message : "Unknown error");
        console.error("Create failed:", err);
      }
      setCreating(null);
      refresh();
    },
    [currentPath, refresh],
  );

  const handlePaste = useCallback(async () => {
    if (!clipboard) return;
    const fileName = clipboard.split("/").pop() ?? "";
    const destination = entryPath(currentPath, fileName);
    try {
      const res = await fetch("/api/fs/copy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source: clipboard, destination }),
      });
      if (!res.ok) {
        const data = await res.json();
        console.error("Paste failed:", data.error);
      }
    } catch (err) {
      console.error("Paste failed:", err);
    }
    refresh();
  }, [clipboard, currentPath, refresh]);

  const handleContextAction = useCallback(
    (action: string, entry?: FileEntry) => {
      switch (action) {
        case "open":
          if (entry) handleOpen(entry);
          break;
        case "open-editor":
          if (entry) {
            setEditingFile({ path: entryPath(currentPath, entry.name), name: entry.name });
            setShowPreview(false);
            setSelectedFiles(new Set());
          }
          break;
        case "rename":
          if (entry) setRenamingEntry(entry.name);
          break;
        case "copy":
          if (entry) {
            const copyPath = entryPath(currentPath, entry.name);
            setClipboard(copyPath);
            toastInfo("Copied", `${copyPath} copied to clipboard`);
          }
          break;
        case "move":
          if (entry) {
            const src = entryPath(currentPath, entry.name);
            const dest = window.prompt("Move to (full destination path):", src);
            if (dest && dest !== src) {
              fetch("/api/fs/move", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ source: src, destination: dest }),
              })
                .then((res) => {
                  if (!res.ok) return res.json().then((d) => console.error("Move failed:", d.error));
                })
                .catch((err) => console.error("Move failed:", err))
                .finally(() => refresh());
            }
          }
          break;
        case "download":
          if (entry) {
            const rawUrl = `/api/fs/raw?path=${encodeURIComponent(entryPath(currentPath, entry.name))}`;
            window.open(rawUrl, "_blank");
          }
          break;
        case "delete":
          if (entry) setDeleteTarget(entry);
          break;
        case "new-folder":
          setCreating("folder");
          break;
        case "new-file":
          setCreating("file");
          break;
        case "paste":
          handlePaste();
          break;
        case "refresh":
          refresh();
          break;
      }
    },
    [currentPath, handleOpen, handlePaste, refresh],
  );

  const renderFileName = (entry: FileEntry) => {
    if (renamingEntry === entry.name) {
      return (
        <InlineRenameInput
          initialName={entry.name}
          isDir={entry.type === "dir"}
          onConfirm={(newName) => handleRenameConfirm(entry.name, newName)}
          onCancel={() => setRenamingEntry(null)}
        />
      );
    }
    return entry.name;
  };

  // Stats
  const dirCount = filteredEntries.filter((e) => e.type === "dir").length;
  const fileCount = filteredEntries.filter((e) => e.type === "file").length;
  const selectedCount = selectedFiles.size;

  // Can download: exactly one file (not directory) selected
  const canDownload = useMemo(() => {
    if (selectedFiles.size !== 1) return false;
    const name = Array.from(selectedFiles)[0];
    const entry = filteredEntries.find((e) => e.name === name);
    return entry?.type === "file";
  }, [selectedFiles, filteredEntries]);

  // Check if a sidebar item matches current path
  const isSidebarActive = (item: SidebarItem) => {
    if (item.path === "/") return currentPath === "/";
    if (item.path === "~") return currentPath.endsWith(currentPath.split("/").pop() ?? "") && currentPath.includes("/Users/") || currentPath.includes("/home/") || currentPath.includes("/root");
    // For ~/Desktop etc, check if current path ends with the subfolder
    const sub = item.path.replace("~/", "");
    return currentPath.endsWith("/" + sub) && (currentPath.includes("/Users/") || currentPath.includes("/home/") || currentPath.includes("/root"));
  };

  return (
    <div className="flex h-full overflow-hidden">
      {/* Hidden file input for manual upload */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="hidden"
        onChange={handleFileInputChange}
      />

      {/* ===== Sidebar ===== */}
      {sidebarOpen && (
        <div className="w-[200px] shrink-0 glass-subtle border-r border-border flex flex-col overflow-y-auto scrollbar-none">
          {/* Favorites */}
          <div className="px-3 pt-3 pb-1">
            <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-widest mb-1">
              Favorites
            </p>
          </div>
          <div className="px-1.5 space-y-0.5">
            {FAVORITES.map((item) => {
              const active = isSidebarActive(item);
              return (
                <button
                  key={item.id}
                  onClick={() => handleSidebarNav(item.path)}
                  className={`w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-xs transition-all duration-150 ${
                    active
                      ? "bg-accent text-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-white/[0.04]"
                  }`}
                >
                  <SidebarIcon type={item.icon} active={active} />
                  <span className="truncate">{item.label}</span>
                </button>
              );
            })}
          </div>

          {/* Locations */}
          <div className="px-3 pt-4 pb-1">
            <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-widest mb-1">
              Locations
            </p>
          </div>
          <div className="px-1.5 space-y-0.5">
            {LOCATIONS.map((item) => {
              const active = isSidebarActive(item);
              return (
                <button
                  key={item.id}
                  onClick={() => handleSidebarNav(item.path)}
                  className={`w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-xs transition-all duration-150 ${
                    active
                      ? "bg-accent text-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-white/[0.04]"
                  }`}
                >
                  <SidebarIcon type={item.icon} active={active} />
                  <span className="truncate">{item.label}</span>
                </button>
              );
            })}
          </div>

          <div className="flex-1" />
        </div>
      )}

      {/* ===== Main area ===== */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Editor mode — replaces toolbar + content when editing a file */}
        {editingFile && (
          <FileEditor
            filePath={editingFile.path}
            fileName={editingFile.name}
            onClose={() => setEditingFile(null)}
          />
        )}

        {/* Normal file browser (hidden when editor is open) */}
        {!editingFile && <>
        {/* Toolbar */}
        <div className="flex items-center gap-1.5 px-2 h-10 border-b border-border shrink-0">
          {/* Sidebar toggle */}
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
            title={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
              <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" />
              <path d="M6 2.5v11" />
            </svg>
          </button>

          {/* Separator */}
          <div className="w-px h-4 bg-border" />

          {/* Back / Forward */}
          <button
            onClick={goBack}
            disabled={!canGoBack}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] disabled:opacity-20 disabled:hover:bg-transparent transition-colors"
            title="Back"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M10 3L5 8l5 5" />
            </svg>
          </button>
          <button
            onClick={goForward}
            disabled={!canGoForward}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] disabled:opacity-20 disabled:hover:bg-transparent transition-colors"
            title="Forward"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M6 3l5 5-5 5" />
            </svg>
          </button>

          {/* Breadcrumb */}
          <div className="flex-1 min-w-0 mx-1">
            <Breadcrumb path={currentPath} onNavigate={handleNavigate} />
          </div>

          {/* Upload button */}
          <button
            onClick={() => fileInputRef.current?.click()}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
            title="Upload files"
          >
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M8 10V2M5 4.5L8 1.5l3 3" strokeLinecap="round" strokeLinejoin="round" />
              <path d="M2.5 11v2a1.5 1.5 0 001.5 1.5h8a1.5 1.5 0 001.5-1.5v-2" strokeLinecap="round" />
            </svg>
          </button>

          {/* Download button (single file selected) */}
          {canDownload && (
            <button
              onClick={handleDownload}
              className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
              title="Download file"
            >
              <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M8 2v8M5 7.5L8 10.5 11 7.5" strokeLinecap="round" strokeLinejoin="round" />
                <path d="M2.5 11v2a1.5 1.5 0 001.5 1.5h8a1.5 1.5 0 001.5-1.5v-2" strokeLinecap="round" />
              </svg>
            </button>
          )}

          {/* Bulk delete button */}
          {selectedCount > 1 && (
            <>
              <div className="w-px h-4 bg-border" />
              <button
                onClick={handleBulkDelete}
                className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-destructive/10 text-destructive hover:bg-destructive/20 transition-colors text-xs"
                title={`Delete ${selectedCount} items`}
              >
                <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <path d="M2 4h12M5 4V2.5a.5.5 0 01.5-.5h5a.5.5 0 01.5.5V4M6.5 7v4.5M9.5 7v4.5" strokeLinecap="round" />
                  <path d="M3.5 4l.5 9.5a1 1 0 001 .5h6a1 1 0 001-.5l.5-9.5" />
                </svg>
                <span>Delete</span>
                <span className="bg-destructive/20 text-destructive text-[10px] font-semibold px-1.5 py-0.5 rounded-full">
                  {selectedCount}
                </span>
              </button>
            </>
          )}

          {/* Search */}
          <div className="relative shrink-0">
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              className="absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground/50"
            >
              <circle cx="7" cy="7" r="4.5" />
              <path d="M10.5 10.5L14 14" />
            </svg>
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search"
              className="w-[120px] focus:w-[160px] h-7 pl-7 pr-2 text-xs rounded-md bg-white/[0.04] border border-border text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:border-primary/40 transition-all duration-200"
            />
          </div>

          {/* Dotfiles toggle */}
          <button
            onClick={() => setShowDotfiles(!showDotfiles)}
            title={showDotfiles ? "Hide dotfiles" : "Show dotfiles"}
            className={`px-1.5 py-1 rounded-md text-[10px] font-mono font-bold transition-colors shrink-0 ${
              showDotfiles
                ? "bg-primary/15 text-primary border-glow"
                : "text-muted-foreground hover:text-foreground hover:bg-white/[0.06]"
            }`}
          >
            .*
          </button>

          {/* View mode */}
          <div className="flex items-center border border-border rounded-md overflow-hidden shrink-0">
            <button
              onClick={() => setViewMode("grid")}
              className={`p-1.5 transition-colors ${
                viewMode === "grid"
                  ? "bg-white/[0.08] text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              title="Grid view"
            >
              <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
                <rect x="0" y="0" width="6" height="6" rx="1.5" />
                <rect x="8" y="0" width="6" height="6" rx="1.5" />
                <rect x="0" y="8" width="6" height="6" rx="1.5" />
                <rect x="8" y="8" width="6" height="6" rx="1.5" />
              </svg>
            </button>
            <button
              onClick={() => setViewMode("list")}
              className={`p-1.5 transition-colors ${
                viewMode === "list"
                  ? "bg-white/[0.08] text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              title="List view"
            >
              <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
                <rect x="0" y="1" width="14" height="2" rx="0.5" />
                <rect x="0" y="6" width="14" height="2" rx="0.5" />
                <rect x="0" y="11" width="14" height="2" rx="0.5" />
              </svg>
            </button>
          </div>

          {/* Refresh */}
          <button
            onClick={refresh}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
            title="Refresh"
          >
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M13 8a5 5 0 1 1-1-3M13 2v3h-3" />
            </svg>
          </button>
        </div>

        {/* Content + Preview wrapper */}
        <div className="flex-1 flex min-h-0">
          {/* Content area with drag & drop */}
          <div
            className="flex-1 overflow-y-auto scrollbar-none min-h-0 min-w-0 relative"
            onDragEnter={handleDragEnter}
            onDragLeave={handleDragLeave}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            onContextMenu={(e) => {
              const target = e.target as HTMLElement;
              if (!target.closest("[data-file-entry]")) {
                handleContextMenu(e, null);
              }
            }}
          >
            {/* Drag overlay */}
            {isDragOver && (
              <div className="absolute inset-0 z-50 flex items-center justify-center bg-primary/10 backdrop-blur-[2px] border-2 border-dashed border-primary rounded-lg m-2 pointer-events-none"
                style={{ boxShadow: "0 0 20px hsl(var(--primary) / 0.15) inset, 0 0 40px hsl(var(--primary) / 0.08)" }}
              >
                <div className="flex flex-col items-center gap-3 text-primary">
                  <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 15V3M8 7l4-4 4 4" />
                    <path d="M20 21H4a1 1 0 01-1-1v-4" />
                    <path d="M21 16v4a1 1 0 01-1 1" />
                  </svg>
                  <span className="text-sm font-medium">Drop files here to upload</span>
                  <span className="text-xs text-primary/60">Files will be uploaded to {currentPath}</span>
                </div>
              </div>
            )}

            {/* Loading */}
            {loading && (
              <div className="flex items-center justify-center h-32">
                <div className="w-5 h-5 border-2 border-border border-t-primary rounded-full animate-spin" />
              </div>
            )}

            {/* Error */}
            {error && (
              <div className="m-4 p-4 rounded-xl bg-destructive/5 border border-destructive/20">
                <p className="text-destructive text-xs">{error}</p>
                <button
                  onClick={refresh}
                  className="mt-2 text-[10px] text-destructive/80 hover:text-destructive underline"
                >
                  Retry
                </button>
              </div>
            )}

            {/* Inline create row */}
            {creating && (
              <InlineCreateInput
                placeholder={creating === "folder" ? "New folder name..." : "New file name..."}
                onConfirm={(name) => handleCreateConfirm(name, creating)}
                onCancel={() => setCreating(null)}
              />
            )}

            {/* Empty */}
            {!loading && !error && filteredEntries.length === 0 && (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground/40">
                <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1" className="mb-2">
                  <path d="M2 4l2-2h4l1 1h5a1 1 0 011 1v8a1 1 0 01-1 1H2a1 1 0 01-1-1V4z" />
                </svg>
                <span className="text-xs">
                  {searchQuery ? "No matches found" : "Empty directory"}
                </span>
              </div>
            )}

            {/* Grid view */}
            {!loading && !error && filteredEntries.length > 0 && viewMode === "grid" && (
              <div className="p-2 grid grid-cols-[repeat(auto-fill,minmax(88px,1fr))] gap-0.5">
                {filteredEntries.map((entry) => {
                  const isSelected = selectedFiles.has(entry.name);
                  return (
                    <button
                      key={entry.name}
                      data-file-entry
                      onClick={(e) => handleSelect(entry, e)}
                      onDoubleClick={() => handleOpen(entry)}
                      onContextMenu={(e) => handleContextMenu(e, entry)}
                      className={`flex flex-col items-center gap-1 p-2 rounded-lg transition-all duration-150 group text-center ${
                        isSelected
                          ? "border-glow bg-accent/50"
                          : "border border-transparent hover:bg-secondary hover:scale-[1.02]"
                      }`}
                    >
                      <div className="transition-transform duration-150 group-hover:scale-105">
                        <FileIcon
                          name={entry.name}
                          isDir={entry.type === "dir"}
                          size="lg"
                        />
                      </div>
                      <span className="text-[11px] text-foreground/80 truncate w-full leading-tight mt-0.5">
                        {renderFileName(entry)}
                      </span>
                      {entry.type === "file" && renamingEntry !== entry.name && (
                        <span className="text-[9px] font-mono text-muted-foreground/40">
                          {formatSize(entry.size)}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}

            {/* List view */}
            {!loading && !error && filteredEntries.length > 0 && viewMode === "list" && (
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-[10px] text-muted-foreground border-b border-border sticky top-0 bg-background/80 backdrop-blur-sm z-10">
                    <th className="py-2 px-4 font-medium">
                      <button
                        onClick={() => setSort("name")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center"
                      >
                        Name
                        <SortIndicator active={sortKey === "name"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium">
                      <button
                        onClick={() => setSort("mod_time")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center"
                      >
                        Date Modified
                        <SortIndicator active={sortKey === "mod_time"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium text-right">
                      <button
                        onClick={() => setSort("size")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center justify-end"
                      >
                        Size
                        <SortIndicator active={sortKey === "size"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium uppercase tracking-wider">Kind</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredEntries.map((entry, idx) => {
                    const isSelected = selectedFiles.has(entry.name);
                    const isEven = idx % 2 === 0;
                    return (
                      <tr
                        key={entry.name}
                        data-file-entry
                        onClick={(e) => handleSelect(entry, e)}
                        onDoubleClick={() => handleOpen(entry)}
                        onContextMenu={(e) => handleContextMenu(e, entry)}
                        className={`cursor-pointer transition-colors duration-100 ${
                          isSelected
                            ? "bg-accent border-l-2 border-l-primary"
                            : isEven
                              ? "hover:bg-white/[0.03] border-l-2 border-l-transparent"
                              : "bg-white/[0.01] hover:bg-white/[0.04] border-l-2 border-l-transparent"
                        }`}
                      >
                        <td className="py-1.5 px-4">
                          <div className="flex items-center gap-2.5">
                            <FileIcon name={entry.name} isDir={entry.type === "dir"} size="sm" />
                            <span className={`truncate ${isSelected ? "text-foreground font-medium" : "text-foreground/80"}`}>
                              {renderFileName(entry)}
                            </span>
                          </div>
                        </td>
                        <td className="py-1.5 px-4 text-muted-foreground">{formatDate(entry.mod_time)}</td>
                        <td className="py-1.5 px-4 text-right font-mono text-muted-foreground">
                          {entry.type === "dir" ? "--" : formatSize(entry.size)}
                        </td>
                        <td className="py-1.5 px-4 text-muted-foreground">{getFileKind(entry)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </div>

          {/* Preview panel */}
          {showPreview && selectedFile && (
            <FilePreview
              file={selectedFile}
              currentPath={currentPath}
              onClose={() => {
                setShowPreview(false);
                setSelectedFiles(new Set());
              }}
            />
          )}
        </div>

        {/* Upload indicator toast */}
        {uploading && (
          <div className="absolute bottom-12 left-1/2 -translate-x-1/2 z-50 flex items-center gap-2.5 px-4 py-2.5 rounded-lg bg-background/95 border border-border shadow-lg backdrop-blur-sm">
            <div className="w-4 h-4 border-2 border-border border-t-primary rounded-full animate-spin" />
            <span className="text-xs text-foreground">
              Uploading {uploadFileCount} file{uploadFileCount !== 1 ? "s" : ""}...
            </span>
          </div>
        )}

        {/* Status bar */}
        <div className="flex items-center justify-between px-3 py-1.5 border-t border-border text-[10px] text-muted-foreground shrink-0">
          <span>
            {filteredEntries.length} item{filteredEntries.length !== 1 ? "s" : ""}
            {dirCount > 0 && ` (${dirCount} folder${dirCount !== 1 ? "s" : ""})`}
            {fileCount > 0 && ` (${fileCount} file${fileCount !== 1 ? "s" : ""})`}
            {selectedCount > 0 && `, ${selectedCount} selected`}
          </span>
          <div className="flex items-center gap-3">
            {clipboard && (
              <>
                <span className="text-primary/60 font-mono truncate max-w-[120px]" title={clipboard}>
                  Copied: {clipboard.split("/").pop()}
                </span>
                <span className="text-muted-foreground/30">|</span>
              </>
            )}
            <span className="font-mono text-muted-foreground/50">{currentPath}</span>
            <span className="text-muted-foreground/30">|</span>
            <span className="text-muted-foreground/40">Local Storage</span>
          </div>
        </div>
        </>}
      </div>

      {/* Context menu overlay */}
      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          entry={contextMenu.entry}
          currentPath={currentPath}
          onClose={() => setContextMenu(null)}
          onAction={handleContextAction}
          hasClipboard={!!clipboard}
        />
      )}

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteConfirmDialog
          name={deleteTarget.name}
          onConfirm={handleDeleteConfirm}
          onCancel={() => setDeleteTarget(null)}
        />
      )}

      {/* Image viewer overlay */}
      {viewingImageIndex !== null && imageFiles.length > 0 && (
        <ImageViewer
          images={imageFiles}
          currentIndex={viewingImageIndex}
          onClose={() => setViewingImageIndex(null)}
          onNavigate={setViewingImageIndex}
        />
      )}
    </div>
  );
}
